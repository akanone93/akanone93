// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package airgap_test

import (
	"archive/tar"
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/nightCode42/plux3/backend/edition/licensefile"
	"github.com/nightCode42/plux3/ee/backend/airgap"
	"github.com/nightCode42/plux3/ee/backend/license"
)

// digestOf returns the OCI digest and hex name of content.
func digestOf(b []byte) (string, string) {
	sum := sha256.Sum256(b)
	h := hex.EncodeToString(sum[:])
	return "sha256:" + h, h
}

// ociImage writes an OCI layout tar with one image named ref whose layers
// are the given contents, and returns the tar's path.
func ociImage(t *testing.T, dir, file, ref string, layers ...string) string {
	t.Helper()
	files := map[string][]byte{}
	put := func(content []byte) (string, int) {
		d, h := digestOf(content)
		files["blobs/sha256/"+h] = content
		return d, len(content)
	}
	cfgD, cfgN := put([]byte(`{"architecture":"amd64","os":"linux","ref":"` + ref + `"}`))
	var layerDescs []map[string]any
	for _, l := range layers {
		d, n := put([]byte(l))
		layerDescs = append(layerDescs, map[string]any{"mediaType": "application/vnd.oci.image.layer.v1.tar", "digest": d, "size": n})
	}
	manifest, err := json.Marshal(map[string]any{
		"schemaVersion": 2, "mediaType": "application/vnd.oci.image.manifest.v1+json",
		"config": map[string]any{"mediaType": "application/vnd.oci.image.config.v1+json", "digest": cfgD, "size": cfgN},
		"layers": layerDescs,
	})
	if err != nil {
		t.Fatal(err)
	}
	mD, mN := put(manifest)
	index, err := json.Marshal(map[string]any{"schemaVersion": 2, "manifests": []map[string]any{{
		"mediaType": "application/vnd.oci.image.manifest.v1+json", "digest": mD, "size": mN,
		"annotations": map[string]string{"org.opencontainers.image.ref.name": ref},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	files["index.json"] = index
	files["oci-layout"] = []byte(`{"imageLayoutVersion":"1.0.0"}`)
	return writeTar(t, filepath.Join(dir, file), files)
}

// writeTar writes regular files in name order and returns the path.
func writeTar(t *testing.T, path string, files map[string][]byte) string {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, name := range sortedKeys(files) {
		if err := tw.WriteHeader(&tar.Header{Name: name, Size: int64(len(files[name])), Mode: 0o644, Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		_, _ = tw.Write(files[name])
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func sortedKeys(m map[string][]byte) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// fixture is a set of inputs for Build.
type fixture struct {
	opts airgap.Options
	pub  ed25519.PublicKey
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	dir := t.TempDir()
	chart := filepath.Join(dir, "plux")
	docs := filepath.Join(dir, "docs")
	for p, c := range map[string]string{
		filepath.Join(chart, "Chart.yaml"):          "name: plux\nversion: 0.1.0\n",
		filepath.Join(chart, "templates", "a.yaml"): "kind: ConfigMap\n",
		filepath.Join(docs, "egress.md"):            "# Egress list\n",
		filepath.Join(docs, "air-gapped.md"):        "# Air-gapped\n",
		filepath.Join(docs, "personal-data.md"):     "# Personal data\n",
		filepath.Join(docs, "not-a-document.txt"):   "ignored\n",
	} {
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(c), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	pub, priv, err := license.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	a := ociImage(t, dir, "a.tar", "registry.example/plux/server:0.0.1", "shared layer", "server layer")
	b := ociImage(t, dir, "b.tar", "registry.example/plux/valkey:8", "shared layer", "valkey layer")
	return fixture{pub: pub, opts: airgap.Options{
		Version: "0.0.1", ImageArchives: []string{a, b}, ChartDir: chart, DocsDir: docs, Key: priv, TempDir: dir,
	}}
}

func (f fixture) build(t *testing.T) []byte {
	t.Helper()
	var out bytes.Buffer
	if _, err := airgap.Build(f.opts, &out); err != nil {
		t.Fatalf("Build: %v", err)
	}
	return out.Bytes()
}

func (f fixture) keys() licensefile.KeySet { return licensefile.NewKeySet(f.pub) }

// readTar returns the entries of a tar in order.
func readTar(t *testing.T, raw []byte) (names []string, content map[string][]byte) {
	t.Helper()
	content = map[string][]byte{}
	tr := tar.NewReader(bytes.NewReader(raw))
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return names, content
		}
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(tr)
		names = append(names, h.Name)
		content[h.Name] = b
	}
}

// rewrite applies edit to the entries of a bundle and returns the new tar.
func rewrite(t *testing.T, raw []byte, edit func(names []string, content map[string][]byte) []string) []byte {
	t.Helper()
	names, content := readTar(t, raw)
	names = edit(names, content)
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, n := range names {
		if err := tw.WriteHeader(&tar.Header{Name: n, Size: int64(len(content[n])), Mode: 0o644, Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		_, _ = tw.Write(content[n])
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestBundleRoundTrip_DEP_040 builds a bundle, verifies it and checks that
// it holds the layout, the chart, the documents and the egress list.
//
// Verifies: DEP-040.
func TestBundleRoundTrip_DEP_040(t *testing.T) {
	f := newFixture(t)
	raw := f.build(t)

	m, err := airgap.Verify(bytes.NewReader(raw), f.keys())
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if m.Version != "0.0.1" || len(m.Images) != 2 {
		t.Fatalf("manifest = %+v", m)
	}
	if m.Images[0].Ref != "registry.example/plux/server:0.0.1" || m.Images[1].Ref != "registry.example/plux/valkey:8" {
		t.Errorf("images = %+v", m.Images)
	}
	names, _ := readTar(t, raw)
	if names[0] != airgap.ManifestPath {
		t.Errorf("first entry = %q, want the manifest", names[0])
	}
	for _, want := range []string{
		"images/oci-layout", "images/index.json", "chart/plux.tgz",
		"docs/operations/egress.md", "docs/operations/air-gapped.md", "docs/operations/personal-data.md",
	} {
		if !contains(names, want) {
			t.Errorf("the bundle has no %s", want)
		}
	}
	if contains(names, "docs/operations/not-a-document.txt") {
		t.Error("a non-markdown file was bundled")
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// TestBundleLayoutValid_DEP_040 checks the OCI layout: the marker, an index
// whose manifests exist as blobs, manifests whose config and layers exist,
// every blob named by its digest, and a shared layer stored once.
//
// Verifies: DEP-040.
func TestBundleLayoutValid_DEP_040(t *testing.T) {
	f := newFixture(t)
	_, content := readTar(t, f.build(t))

	if string(content["images/oci-layout"]) != `{"imageLayoutVersion":"1.0.0"}` {
		t.Errorf("oci-layout = %s", content["images/oci-layout"])
	}
	blobs := map[string][]byte{}
	for name, b := range content {
		if h, ok := strings.CutPrefix(name, "images/blobs/sha256/"); ok {
			if d, _ := digestOf(b); d != "sha256:"+h {
				t.Errorf("blob %s hashes to %s", h, d)
			}
			blobs["sha256:"+h] = b
		}
	}
	var index struct {
		Manifests []struct {
			Digest string `json:"digest"`
			Size   int    `json:"size"`
		} `json:"manifests"`
	}
	if err := json.Unmarshal(content["images/index.json"], &index); err != nil || len(index.Manifests) != 2 {
		t.Fatalf("index.json: %v %s", err, content["images/index.json"])
	}
	for _, d := range index.Manifests {
		b, ok := blobs[d.Digest]
		if !ok || len(b) != d.Size {
			t.Fatalf("index entry %s has no blob of size %d", d.Digest, d.Size)
		}
		var mf struct {
			Config struct{ Digest string }
			Layers []struct{ Digest string }
		}
		if err := json.Unmarshal(b, &mf); err != nil {
			t.Fatal(err)
		}
		for _, ref := range append([]struct{ Digest string }{mf.Config}, mf.Layers...) {
			if _, ok := blobs[ref.Digest]; !ok {
				t.Errorf("manifest %s references missing blob %s", d.Digest, ref.Digest)
			}
		}
	}
	// Two images of 4 blobs each (config, 2 layers, manifest) share one layer.
	if len(blobs) != 7 {
		t.Errorf("%d blobs, want 7 (the shared layer once)", len(blobs))
	}
}

// TestBuildIsDeterministic_DEP_040 builds twice and compares the bytes.
//
// Verifies: DEP-040, CMP-002.
func TestBuildIsDeterministic_DEP_040(t *testing.T) {
	f := newFixture(t)
	if !bytes.Equal(f.build(t), f.build(t)) {
		t.Error("two builds of the same input differ")
	}
}

// TestVerifyRefuses_DEP_040 mutates a good bundle in the ways an attacker or
// a bad transfer would and expects each to be refused with its own error.
//
// Verifies: DEP-040.
func TestVerifyRefuses_DEP_040(t *testing.T) {
	f := newFixture(t)
	good := f.build(t)
	otherPub, _, err := license.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}

	blobName := func(names []string) string {
		for _, n := range names {
			if strings.HasPrefix(n, "images/blobs/") {
				return n
			}
		}
		return ""
	}
	cases := []struct {
		name string
		keys licensefile.KeySet
		edit func(names []string, c map[string][]byte) []string
		want error
	}{
		{"changed byte", f.keys(), func(names []string, c map[string][]byte) []string {
			n := blobName(names)
			c[n] = append([]byte{c[n][0] ^ 1}, c[n][1:]...)
			return names
		}, airgap.ErrAltered},
		{"extra file", f.keys(), func(names []string, c map[string][]byte) []string {
			c["images/blobs/sha256/extra"] = []byte("x")
			return append(names, "images/blobs/sha256/extra")
		}, airgap.ErrExtra},
		{"missing file", f.keys(), func(names []string, c map[string][]byte) []string {
			n := "docs/operations/egress.md"
			delete(c, n)
			out := names[:0]
			for _, x := range names {
				if x != n {
					out = append(out, x)
				}
			}
			return out
		}, airgap.ErrMissing},
		{"wrong key", licensefile.NewKeySet(otherPub), nil, airgap.ErrSignature},
		{"no key", licensefile.KeySet{}, nil, airgap.ErrSignature},
		{"edited manifest", f.keys(), func(names []string, c map[string][]byte) []string {
			c[airgap.ManifestPath] = bytes.Replace(c[airgap.ManifestPath], []byte("0.0.1"), []byte("9.9.9"), 1)
			return names
		}, airgap.ErrSignature},
		{"no manifest", f.keys(), func(names []string, c map[string][]byte) []string {
			delete(c, airgap.ManifestPath)
			return names[1:]
		}, airgap.ErrNoManifest},
		{"repeated entry", f.keys(), func(names []string, c map[string][]byte) []string {
			return append(names, names[1])
		}, airgap.ErrMalformed},
		{"unclean path", f.keys(), func(names []string, c map[string][]byte) []string {
			c["../escape"] = []byte("x")
			return append(names, "../escape")
		}, airgap.ErrMalformed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := good
			if tc.edit != nil {
				raw = rewrite(t, good, tc.edit)
			}
			_, err := airgap.Verify(bytes.NewReader(raw), tc.keys)
			if !errors.Is(err, tc.want) {
				t.Fatalf("Verify = %v, want %v", err, tc.want)
			}
		})
	}
}

// TestVerifyRefusesNonRegularEntry_DEP_040 refuses a symbolic link even when
// every other file is intact.
//
// Verifies: DEP-040.
func TestVerifyRefusesNonRegularEntry_DEP_040(t *testing.T) {
	f := newFixture(t)
	good := f.build(t)
	names, content := readTar(t, good)
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, n := range names {
		_ = tw.WriteHeader(&tar.Header{Name: n, Size: int64(len(content[n])), Mode: 0o644, Typeflag: tar.TypeReg})
		_, _ = tw.Write(content[n])
	}
	_ = tw.WriteHeader(&tar.Header{Name: "link", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"})
	_ = tw.Close()
	withLink := buf.Bytes()
	if _, err := airgap.Verify(bytes.NewReader(withLink), f.keys()); !errors.Is(err, airgap.ErrMalformed) {
		t.Fatalf("Verify = %v, want ErrMalformed", err)
	}
}

// TestBuildRefuses_DEP_040 covers inputs that must not make a bundle.
//
// Verifies: DEP-040.
func TestBuildRefuses_DEP_040(t *testing.T) {
	f := newFixture(t)
	dir := t.TempDir()

	t.Run("blob that does not match its name", func(t *testing.T) {
		o := f.opts
		bad := writeTar(t, filepath.Join(dir, "bad.tar"), map[string][]byte{
			"index.json": []byte(`{"schemaVersion":2,"manifests":[]}`),
			"blobs/sha256/" + strings.Repeat("a", 64): []byte("not that"),
		})
		o.ImageArchives = []string{bad}
		if _, err := airgap.Build(o, io.Discard); err == nil || !strings.Contains(err.Error(), "does not match its digest") {
			t.Fatalf("Build = %v", err)
		}
	})
	t.Run("missing blob", func(t *testing.T) {
		o := f.opts
		d, _ := digestOf([]byte("m"))
		missing := writeTar(t, filepath.Join(dir, "missing.tar"), map[string][]byte{
			"index.json": []byte(`{"schemaVersion":2,"manifests":[{"digest":"` + d + `","size":1}]}`),
		})
		o.ImageArchives = []string{missing}
		if _, err := airgap.Build(o, io.Discard); err == nil || !strings.Contains(err.Error(), "missing") {
			t.Fatalf("Build = %v", err)
		}
	})
	t.Run("legacy docker save", func(t *testing.T) {
		o := f.opts
		o.ImageArchives = []string{writeTar(t, filepath.Join(dir, "legacy.tar"), map[string][]byte{"manifest.json": []byte("[]")})}
		if _, err := airgap.Build(o, io.Discard); err == nil || !strings.Contains(err.Error(), "legacy docker save") {
			t.Fatalf("Build = %v", err)
		}
	})
	t.Run("same name two digests", func(t *testing.T) {
		o := f.opts
		o.ImageArchives = []string{f.opts.ImageArchives[0], ociImage(t, dir, "c.tar", "registry.example/plux/server:0.0.1", "other")}
		if _, err := airgap.Build(o, io.Discard); err == nil || !strings.Contains(err.Error(), "two different digests") {
			t.Fatalf("Build = %v", err)
		}
	})
	t.Run("no install guide", func(t *testing.T) {
		o := f.opts
		o.DocsDir = t.TempDir()
		if _, err := airgap.Build(o, io.Discard); err == nil || !strings.Contains(err.Error(), "egress.md is required") {
			t.Fatalf("Build = %v", err)
		}
	})
	t.Run("bad version", func(t *testing.T) {
		o := f.opts
		o.Version = "../x"
		if _, err := airgap.Build(o, io.Discard); err == nil {
			t.Fatal("Build accepted a path as a version")
		}
	})
	t.Run("not a chart", func(t *testing.T) {
		o := f.opts
		o.ChartDir = t.TempDir()
		if _, err := airgap.Build(o, io.Discard); err == nil {
			t.Fatal("Build accepted a directory without Chart.yaml")
		}
	})
}
