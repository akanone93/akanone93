// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

// validDeclaration is a function document with one test case.
const validDeclaration = `{
  "schemaVersion": "1.0.0",
  "kind": "function",
  "id": "0194f2a0-7b1c-7d3e-8a5f-3c4d5e6f7a8b",
  "name": "bank.transferFee",
  "placement": "server",
  "capabilities": {"claims": true},
  "tests": [{"name": "echo", "input": {"a": 1}, "output": {"a": 1}}]
}`

// testLoader returns the document loader the builder role uses.
func testLoader(t *testing.T) *schema.Loader {
	t.Helper()
	v, err := schema.NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	return schema.NewLoader(v, schema.DefaultMigrator(), limits.Defaults())
}

// crate returns source files with the given function.json.
func crate(declaration string) fstest.MapFS {
	files := fstest.MapFS{"Cargo.toml": {Data: []byte("[package]\nname = \"fee\"\n")}}
	if declaration != "" {
		files[DeclarationFile] = &fstest.MapFile{Data: []byte(declaration)}
	}
	return files
}

// Verifies: FN-008, FN-016.
func TestReadDeclarationAcceptsAMatchingDocument_FN_008(t *testing.T) {
	d, diags := ReadDeclaration(testLoader(t), crate(validDeclaration), "bank.transferFee", PlacementServer)
	if d == nil || diags.HasErrors() {
		t.Fatalf("declaration = %v, diagnostics = %v", d, diags)
	}
	if d.Placement() != PlacementServer || d.Function.Name != "bank.transferFee" || len(d.Function.Tests) != 1 {
		t.Errorf("declaration = %+v", d.Function)
	}
	if !bytes.HasPrefix(d.Document, []byte(`{"capabilities":`)) {
		t.Errorf("the stored document is not canonical JSON: %s", d.Document)
	}
	if src := d.Source(crate(validDeclaration)); src.Name != "bank.transferFee" || src.Placement != PlacementServer {
		t.Errorf("source = %+v", src)
	}
	// An empty placement accepts the document's own.
	if d, _ := ReadDeclaration(testLoader(t), crate(validDeclaration), "bank.transferFee", ""); d == nil {
		t.Error("an unspecified placement must accept the document's")
	}
}

// Verifies: FN-008, FN-016.
func TestReadDeclarationRefusesMismatchesWithPLX7205_FN_008(t *testing.T) {
	cases := map[string]struct {
		name      string
		placement Placement
		path      string
	}{
		"name":      {"other.name", PlacementServer, "/name"},
		"placement": {"bank.transferFee", PlacementDevice, "/placement"},
	}
	for label, c := range cases {
		d, diags := ReadDeclaration(testLoader(t), crate(validDeclaration), c.name, c.placement)
		if d != nil || len(diags) == 0 {
			t.Fatalf("%s: declaration = %v", label, d)
		}
		last := diags[len(diags)-1]
		if last.Code != plxerr.FunctionDeclarationMismatch || last.Path != c.path || last.File != DeclarationFile {
			t.Errorf("%s: diagnostic = %+v", label, last)
		}
	}
}

// Verifies: FN-008.
func TestReadDeclarationReportsInvalidDocuments_FN_008(t *testing.T) {
	for label, doc := range map[string]string{
		"missing":      "",
		"not JSON":     "{",
		"wrong kind":   strings.Replace(validDeclaration, `"function"`, `"page"`, 1),
		"bad name":     strings.Replace(validDeclaration, "bank.transferFee", "Bank", 1),
		"bad property": strings.Replace(validDeclaration, `"placement"`, `"surprise": 1, "placement"`, 1),
	} {
		d, diags := ReadDeclaration(testLoader(t), crate(doc), "bank.transferFee", "")
		if d != nil || !diags.HasErrors() {
			t.Errorf("%s: declaration = %v, diagnostics = %v", label, d, diags)
		}
	}
}

// entry is one tar entry of a test archive.
type entry struct {
	header tar.Header
	body   string
}

// tarball packs the entries, in order, as a gzip-compressed tar archive.
func tarball(t *testing.T, entries ...entry) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	for _, e := range entries {
		h := e.header
		if h.Typeflag == 0 {
			h.Typeflag = tar.TypeReg
		}
		if h.Typeflag == tar.TypeReg {
			h.Size = int64(len(e.body))
		}
		if err := tw.WriteHeader(&h); err != nil {
			t.Fatal(err)
		}
		if h.Typeflag == tar.TypeReg {
			if _, err := tw.Write([]byte(e.body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// file is a regular tar entry.
func file(name, body string) entry {
	return entry{header: tar.Header{Name: name, Mode: 0o644}, body: body}
}

// dir is a directory tar entry.
func dir(name string) entry {
	return entry{header: tar.Header{Name: name, Typeflag: tar.TypeDir, Mode: 0o755}}
}

// fsReadFile reads a file of an extracted archive as a string.
func fsReadFile(files fs.FS, name string) (string, error) {
	b, err := fs.ReadFile(files, name)
	return string(b), err
}

// Verifies: FN-016.
func TestExtractArchiveReadsACleanCrate_FN_016(t *testing.T) {
	archive := tarball(t, dir("./"), file("./Cargo.toml", "[package]"), dir("src/"), file("src/lib.rs", "fn main() {}"))
	files, err := ExtractArchive(bytes.NewReader(archive), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"Cargo.toml": "[package]", "src/lib.rs": "fn main() {}"} {
		got, err := fsReadFile(files, name)
		if err != nil || got != want {
			t.Errorf("%s = %q, %v", name, got, err)
		}
	}
}

// Verifies: FN-003.
func TestExtractArchiveKeepsAnEmptyVendorDirectory_FN_003(t *testing.T) {
	files, err := ExtractArchive(bytes.NewReader(tarball(t, file("Cargo.toml", "x"), dir("vendor/"))), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if entries, err := fs.ReadDir(files, "vendor"); err != nil || len(entries) != 0 {
		t.Errorf("vendor = %v, %v; an empty vendor/ means nothing is vendored", entries, err)
	}
}

// Verifies: FN-016.
func TestExtractArchiveRefusesUnsafeAndOversizedArchives_FN_016(t *testing.T) {
	tooMany := make([]entry, 0, 40)
	for i := range 40 {
		tooMany = append(tooMany, file("f"+string(rune('a'+i%26))+string(rune('a'+i/26)), ""))
	}
	cases := map[string]struct {
		archive []byte
		code    plxerr.Code
	}{
		"parent path":   {tarball(t, file("../evil", "x")), plxerr.FunctionBuildFailed},
		"absolute path": {tarball(t, file("/etc/passwd", "x")), plxerr.FunctionBuildFailed},
		"symlink":       {tarball(t, entry{header: tar.Header{Name: "link", Typeflag: tar.TypeSymlink, Linkname: "/etc", Mode: 0o777}}), plxerr.FunctionBuildFailed},
		"duplicate":     {tarball(t, file("a", "1"), file("a", "2")), plxerr.FunctionBuildFailed},
		"empty":         {tarball(t), plxerr.FunctionBuildFailed},
		"not gzip":      {[]byte("plain text"), plxerr.FunctionBuildFailed},
		"large file":    {tarball(t, file("big", strings.Repeat("x", 5000))), plxerr.FunctionBuildLimitExceeded},
		"many entries":  {tarball(t, tooMany...), plxerr.FunctionBuildLimitExceeded},
	}
	for label, c := range cases {
		_, err := ExtractArchive(bytes.NewReader(c.archive), 4096)
		if code, ok := plxerr.CodeOf(err); !ok || code != c.code {
			t.Errorf("%s: error = %v; want %s", label, err, c.code)
		}
	}
}
