// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package airgap

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// Archive locations of the bundle's parts.
const (
	docsPrefix = "docs/operations/"
)

// requiredDocs are the documents every bundle carries: the egress list the
// installation is checked against and the install guide itself.
var requiredDocs = []string{"egress.md", "air-gapped.md"}

// versionPattern keeps a version usable in a file name.
var versionPattern = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z.+_-]{0,63}$`)

// Options says what goes into a bundle.
type Options struct {
	// Version is the Plux version the bundle installs; it names the file.
	Version string
	// ImageArchives are OCI image layouts as tar archives, one or more
	// images each.
	ImageArchives []string
	// ChartDir is the Helm chart directory, for example deploy/helm/plux.
	ChartDir string
	// DocsDir holds the operations documents (docs/operations).
	DocsDir string
	// Key signs the manifest.
	Key ed25519.PrivateKey
	// TempDir is where the bundle is staged; empty is the system default.
	// It needs room for the images.
	TempDir string
}

// Name is the conventional file name of the bundle for a version.
func Name(version string) string { return "plux-airgap-" + version + ".tar" }

// Build writes the bundle to out and returns its manifest. The input is
// staged in a temporary directory that is removed before Build returns.
func Build(opts Options, out io.Writer) (Manifest, error) {
	if !versionPattern.MatchString(opts.Version) {
		return Manifest{}, fmt.Errorf("airgap.Build: %q is not a usable version", opts.Version)
	}
	if len(opts.ImageArchives) == 0 {
		return Manifest{}, errors.New("airgap.Build: at least one image archive is needed")
	}
	stage, err := os.MkdirTemp(opts.TempDir, "plux-airgap-")
	if err != nil {
		return Manifest{}, fmt.Errorf("airgap.Build: %w", err)
	}
	defer func() { _ = os.RemoveAll(stage) }()
	images, err := stageImages(stage, opts.ImageArchives)
	if err != nil {
		return Manifest{}, err
	}
	if err := stageChart(stage, opts.ChartDir); err != nil {
		return Manifest{}, err
	}
	if err := stageDocs(stage, opts.DocsDir); err != nil {
		return Manifest{}, err
	}
	files, err := hashTree(stage)
	if err != nil {
		return Manifest{}, err
	}
	m := Manifest{Format: FormatVersion, Version: opts.Version, Files: files, Images: images}
	signed, err := signManifest(m, opts.Key)
	if err != nil {
		return Manifest{}, err
	}
	if err := writeArchive(out, stage, signed, files); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

// stageImages merges the image archives into stage/images.
func stageImages(stage string, archives []string) ([]Image, error) {
	b, err := newLayoutBuilder(filepath.Join(stage, "images"))
	if err != nil {
		return nil, err
	}
	for _, a := range archives {
		if err := b.addArchive(a); err != nil {
			return nil, err
		}
	}
	return b.finish()
}

// stageChart writes the chart directory as a deterministic .tgz.
func stageChart(stage, chartDir string) error {
	if _, err := os.Stat(filepath.Join(chartDir, "Chart.yaml")); err != nil {
		return fmt.Errorf("airgap.stageChart: %s is not a Helm chart: %w", chartDir, err)
	}
	dir := filepath.Join(stage, "chart")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("airgap.stageChart: %w", err)
	}
	name := filepath.Base(filepath.Clean(chartDir))
	f, err := os.Create(filepath.Join(dir, name+".tgz")) //nolint:gosec // G304: below the private staging directory.
	if err != nil {
		return fmt.Errorf("airgap.stageChart: %w", err)
	}
	werr := writeChartTgz(f, chartDir, name)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		return fmt.Errorf("airgap.stageChart: %w", werr)
	}
	return nil
}

// writeChartTgz packs the chart as name/... in sorted order with no time or
// owner, so the same chart always gives the same bytes.
func writeChartTgz(w io.Writer, chartDir, name string) error {
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	root := os.DirFS(chartDir)
	err := fs.WalkDir(root, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("%s is not a regular file", p)
		}
		raw, err := fs.ReadFile(root, p)
		if err != nil {
			return err //nolint:wrapcheck // wrapped by the caller
		}
		return writeEntry(tw, path.Join(name, p), int64(len(raw)), bytes.NewReader(raw))
	})
	if err != nil {
		return err //nolint:wrapcheck // wrapped by the caller
	}
	if err := tw.Close(); err != nil {
		return fmt.Errorf("close the chart archive: %w", err)
	}
	return gz.Close() //nolint:wrapcheck // wrapped by the caller
}

// stageDocs copies the operations documents into stage/docs/operations and
// refuses a set without the egress list or the install guide.
func stageDocs(stage, docsDir string) error {
	dst := filepath.Join(stage, filepath.FromSlash(docsPrefix))
	if err := os.MkdirAll(dst, 0o700); err != nil {
		return fmt.Errorf("airgap.stageDocs: %w", err)
	}
	matches, err := filepath.Glob(filepath.Join(docsDir, "*.md"))
	if err != nil {
		return fmt.Errorf("airgap.stageDocs: %w", err)
	}
	root, err := os.OpenRoot(dst)
	if err != nil {
		return fmt.Errorf("airgap.stageDocs: %w", err)
	}
	defer func() { _ = root.Close() }()
	for _, m := range matches {
		raw, err := os.ReadFile(m) //nolint:gosec // G304: the operator names the docs directory.
		if err != nil {
			return fmt.Errorf("airgap.stageDocs: %w", err)
		}
		if err := root.WriteFile(filepath.Base(m), raw, 0o600); err != nil {
			return fmt.Errorf("airgap.stageDocs: %w", err)
		}
	}
	for _, need := range requiredDocs {
		if _, err := os.Stat(filepath.Join(dst, need)); err != nil {
			return fmt.Errorf("airgap.stageDocs: %s is required in %s", need, docsDir)
		}
	}
	return nil
}

// hashTree lists every file below stage, sorted by archive path, with its
// size and SHA-256.
func hashTree(stage string) ([]File, error) {
	var files []File
	err := filepath.WalkDir(stage, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(stage, p)
		if err != nil {
			return err //nolint:wrapcheck // wrapped by the caller
		}
		f, err := os.Open(p) //nolint:gosec // G304: below the private staging directory.
		if err != nil {
			return err //nolint:wrapcheck // wrapped by the caller
		}
		defer func() { _ = f.Close() }()
		h := sha256.New()
		n, err := io.Copy(h, f)
		if err != nil {
			return err //nolint:wrapcheck // wrapped by the caller
		}
		files = append(files, File{Path: filepath.ToSlash(rel), Size: n, SHA256: hex.EncodeToString(h.Sum(nil))})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("airgap.hashTree: %w", err)
	}
	slices.SortFunc(files, func(a, b File) int { return strings.Compare(a.Path, b.Path) })
	return files, nil
}

// writeArchive writes manifest.json and then every file in manifest order.
func writeArchive(out io.Writer, stage string, signed []byte, files []File) error {
	tw := tar.NewWriter(out)
	if err := writeEntry(tw, ManifestPath, int64(len(signed)), bytes.NewReader(signed)); err != nil {
		return fmt.Errorf("airgap.writeArchive: %w", err)
	}
	for _, f := range files {
		if err := copyEntry(tw, filepath.Join(stage, filepath.FromSlash(f.Path)), f); err != nil {
			return fmt.Errorf("airgap.writeArchive: %w", err)
		}
	}
	if err := tw.Close(); err != nil {
		return fmt.Errorf("airgap.writeArchive: %w", err)
	}
	return nil
}

// copyEntry writes one staged file to the archive.
func copyEntry(tw *tar.Writer, src string, f File) error {
	in, err := os.Open(src) //nolint:gosec // G304: below the private staging directory.
	if err != nil {
		return err //nolint:wrapcheck // wrapped by the caller
	}
	defer func() { _ = in.Close() }()
	return writeEntry(tw, f.Path, f.Size, in)
}

// writeEntry writes a regular file entry with no time, owner or group.
func writeEntry(tw *tar.Writer, name string, size int64, r io.Reader) error {
	if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: name, Size: size, Mode: 0o644, Format: tar.FormatPAX}); err != nil {
		return fmt.Errorf("write header %s: %w", name, err)
	}
	if _, err := io.Copy(tw, r); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	return nil
}
