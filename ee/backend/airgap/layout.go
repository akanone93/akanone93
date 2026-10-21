// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package airgap

import (
	"archive/tar"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// Paths of the OCI image layout inside a directory or the bundle's images/
// folder.
const (
	layoutMarker  = "oci-layout"
	layoutIndex   = "index.json"
	blobsDir      = "blobs/sha256"
	layoutVersion = `{"imageLayoutVersion":"1.0.0"}`
	indexMedia    = "application/vnd.oci.image.index.v1+json"
	refNameKey    = "org.opencontainers.image.ref.name"
	// containerdNameKey is where a containerd-backed `docker save` records
	// the name an image was saved under.
	containerdNameKey = "io.containerd.image.name"
	digestPrefix      = "sha256:"
	// maxMetaBytes bounds a JSON document read from an input that is not
	// yet trusted (index.json, a manifest, manifest.json). It protects this
	// offline tool's memory, not a product limit.
	maxMetaBytes = 16 << 20
	// maxGraphDepth bounds the nesting of image indexes.
	maxGraphDepth = 8
)

// descriptor is an OCI content descriptor, reduced to what the bundle uses.
type descriptor struct {
	MediaType   string            `json:"mediaType,omitempty"`
	Digest      string            `json:"digest"`
	Size        int64             `json:"size"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// ref is the name a descriptor carries in an index, or "".
func (d descriptor) ref() string {
	if n := d.Annotations[refNameKey]; n != "" {
		return n
	}
	return d.Annotations[containerdNameKey]
}

// graphDoc is the part of an image manifest or index the walk needs.
type graphDoc struct {
	Config    *descriptor  `json:"config"`
	Layers    []descriptor `json:"layers"`
	Manifests []descriptor `json:"manifests"`
}

// imageIndex is index.json.
type imageIndex struct {
	SchemaVersion int          `json:"schemaVersion"`
	MediaType     string       `json:"mediaType,omitempty"`
	Manifests     []descriptor `json:"manifests"`
}

// layoutBuilder merges OCI layouts read from tar archives into one layout
// staged in a directory. Blobs are stored once per digest.
type layoutBuilder struct {
	dir     string
	entries []descriptor
}

// newLayoutBuilder stages a layout below dir, which must exist.
func newLayoutBuilder(dir string) (*layoutBuilder, error) {
	if err := os.MkdirAll(filepath.Join(dir, filepath.FromSlash(blobsDir)), 0o700); err != nil {
		return nil, fmt.Errorf("airgap.newLayoutBuilder: %w", err)
	}
	return &layoutBuilder{dir: dir}, nil
}

// blobPath is where the staged blob with the given digest lives.
func (b *layoutBuilder) blobPath(digest string) string {
	return filepath.Join(b.dir, filepath.FromSlash(blobsDir), strings.TrimPrefix(digest, digestPrefix))
}

// addArchive merges the OCI layout in the tar archive at file: every blob is
// hashed while it is copied and must match its name, and every image the
// archive's index names must be complete.
func (b *layoutBuilder) addArchive(file string) error {
	f, err := os.Open(file) //nolint:gosec // G304: the operator names the image archives.
	if err != nil {
		return fmt.Errorf("airgap.addArchive: %w", err)
	}
	defer func() { _ = f.Close() }()
	index, err := b.readArchive(tar.NewReader(f))
	if err != nil {
		return fmt.Errorf("airgap.addArchive %s: %w", filepath.Base(file), err)
	}
	for _, d := range index.Manifests {
		if err := b.checkImage(d, 0, map[string]bool{}); err != nil {
			return fmt.Errorf("airgap.addArchive %s: %w", filepath.Base(file), err)
		}
		if err := b.addEntry(d); err != nil {
			return fmt.Errorf("airgap.addArchive %s: %w", filepath.Base(file), err)
		}
	}
	return nil
}

// readArchive stores the archive's blobs and returns its index.
func (b *layoutBuilder) readArchive(tr *tar.Reader) (imageIndex, error) {
	var rawIndex []byte
	var legacy bool
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return imageIndex{}, fmt.Errorf("read the archive: %w", err)
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		name := strings.TrimPrefix(path.Clean(h.Name), "./")
		switch {
		case name == layoutIndex:
			rawIndex, err = readBounded(tr)
		case strings.HasPrefix(name, blobsDir+"/"):
			err = b.storeBlob(strings.TrimPrefix(name, blobsDir+"/"), tr)
		case name == "manifest.json":
			legacy = true
		}
		if err != nil {
			return imageIndex{}, err
		}
	}
	if rawIndex == nil {
		if legacy {
			return imageIndex{}, errors.New("this is a legacy docker save archive; save an OCI layout instead (skopeo copy docker-daemon:<image> oci-archive:<file>:<ref>)")
		}
		return imageIndex{}, errors.New("the archive is not an OCI image layout (no index.json)")
	}
	var index imageIndex
	if err := json.Unmarshal(rawIndex, &index); err != nil {
		return imageIndex{}, fmt.Errorf("index.json: %w", err)
	}
	return index, nil
}

// readBounded reads a JSON document of at most maxMetaBytes.
func readBounded(r io.Reader) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(r, maxMetaBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	if len(raw) > maxMetaBytes {
		return nil, fmt.Errorf("a JSON document exceeds %d bytes", maxMetaBytes)
	}
	return raw, nil
}

// storeBlob copies a blob into the staging layout unless a blob with the
// same digest is there, and refuses content that does not hash to its name.
func (b *layoutBuilder) storeBlob(hexName string, r io.Reader) error {
	if raw, err := hex.DecodeString(hexName); err != nil || len(raw) != sha256.Size || hex.EncodeToString(raw) != hexName {
		return fmt.Errorf("blob %q is not named by a SHA-256", hexName)
	}
	dst := b.blobPath(digestPrefix + hexName)
	if _, err := os.Stat(dst); err == nil {
		return nil
	}
	tmp, err := os.CreateTemp(b.dir, "blob-*")
	if err != nil {
		return fmt.Errorf("stage blob: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	h := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(tmp, h), r)
	if closeErr := tmp.Close(); copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		return fmt.Errorf("stage blob %s: %w", hexName, copyErr)
	}
	if hex.EncodeToString(h.Sum(nil)) != hexName {
		return fmt.Errorf("blob %s does not match its digest", hexName)
	}
	if err := os.Rename(tmp.Name(), dst); err != nil {
		return fmt.Errorf("stage blob %s: %w", hexName, err)
	}
	return nil
}

// checkImage verifies that a descriptor's blob is staged with the stated size
// and, for manifests and indexes, that everything they reference is too.
func (b *layoutBuilder) checkImage(d descriptor, depth int, seen map[string]bool) error {
	if depth > maxGraphDepth {
		return errors.New("image indexes nest too deeply")
	}
	if err := b.checkBlob(d); err != nil {
		return err
	}
	if !isGraphMedia(d.MediaType) || seen[d.Digest] {
		return nil
	}
	seen[d.Digest] = true
	raw, err := b.readStagedJSON(d)
	if err != nil {
		return err
	}
	var doc graphDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return fmt.Errorf("%s: %w", d.Digest, err)
	}
	children := append([]descriptor{}, doc.Layers...)
	if doc.Config != nil {
		children = append(children, *doc.Config)
	}
	for _, c := range children {
		if err := b.checkBlob(c); err != nil {
			return err
		}
	}
	for _, m := range doc.Manifests {
		if err := b.checkImage(m, depth+1, seen); err != nil {
			return err
		}
	}
	return nil
}

// isGraphMedia reports whether a media type is an image manifest or index
// (OCI or Docker schema 2), which reference further blobs.
func isGraphMedia(mt string) bool {
	switch mt {
	case "application/vnd.oci.image.manifest.v1+json", indexMedia,
		"application/vnd.docker.distribution.manifest.v2+json",
		"application/vnd.docker.distribution.manifest.list.v2+json":
		return true
	}
	return false
}

// checkBlob verifies that the descriptor's blob is staged and has the stated
// size. Its content was hashed when it was stored.
func (b *layoutBuilder) checkBlob(d descriptor) error {
	if !strings.HasPrefix(d.Digest, digestPrefix) {
		return fmt.Errorf("unsupported digest %q", d.Digest)
	}
	st, err := os.Stat(b.blobPath(d.Digest))
	if err != nil {
		return fmt.Errorf("blob %s is referenced but missing", d.Digest)
	}
	if st.Size() != d.Size {
		return fmt.Errorf("blob %s has %d bytes, the descriptor says %d", d.Digest, st.Size(), d.Size)
	}
	return nil
}

// readStagedJSON reads a staged manifest or index.
func (b *layoutBuilder) readStagedJSON(d descriptor) ([]byte, error) {
	f, err := os.Open(b.blobPath(d.Digest))
	if err != nil {
		return nil, fmt.Errorf("blob %s: %w", d.Digest, err)
	}
	defer func() { _ = f.Close() }()
	return readBounded(f)
}

// addEntry adds an image to the merged index. The same name and digest twice
// is one image; the same name with two digests is a conflict.
func (b *layoutBuilder) addEntry(d descriptor) error {
	for _, e := range b.entries {
		if d.ref() != "" && e.ref() == d.ref() && e.Digest != d.Digest {
			return fmt.Errorf("the image %s appears with two different digests", d.ref())
		}
		if e.ref() == d.ref() && e.Digest == d.Digest {
			return nil
		}
	}
	b.entries = append(b.entries, d)
	return nil
}

// finish writes oci-layout and index.json, with the images in a fixed order,
// and returns the image list.
func (b *layoutBuilder) finish() ([]Image, error) {
	slices.SortFunc(b.entries, func(x, y descriptor) int {
		return strings.Compare(x.ref()+"\x00"+x.Digest, y.ref()+"\x00"+y.Digest)
	})
	raw, err := json.Marshal(imageIndex{SchemaVersion: 2, MediaType: indexMedia, Manifests: b.entries})
	if err != nil {
		return nil, fmt.Errorf("airgap.finish: %w", err)
	}
	if err := os.WriteFile(filepath.Join(b.dir, layoutIndex), raw, 0o600); err != nil {
		return nil, fmt.Errorf("airgap.finish: %w", err)
	}
	if err := os.WriteFile(filepath.Join(b.dir, layoutMarker), []byte(layoutVersion), 0o600); err != nil {
		return nil, fmt.Errorf("airgap.finish: %w", err)
	}
	return imagesOf(raw)
}

// imagesOf lists the images an index.json names.
func imagesOf(rawIndex []byte) ([]Image, error) {
	var index imageIndex
	if err := json.Unmarshal(rawIndex, &index); err != nil {
		return nil, fmt.Errorf("%w: index.json: %v", ErrMalformed, err)
	}
	out := make([]Image, 0, len(index.Manifests))
	for _, d := range index.Manifests {
		out = append(out, Image{Ref: d.ref(), Digest: d.Digest})
	}
	return out, nil
}
