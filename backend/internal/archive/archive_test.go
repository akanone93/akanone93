// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package archive_test

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/archive"
	"github.com/nightCode42/plux3/backend/internal/document"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

var big = archive.Limits{Bytes: 1 << 20, Entries: 100, ExpandedBytes: 1 << 20}

func code(err error) plxerr.Code {
	var e *plxerr.Error
	if errors.As(err, &e) {
		return e.Code
	}
	return 0
}

func files() []document.File {
	return []document.File{
		{Path: "plugins/b/plugin.json", Content: []byte(`{"b":1}`)},
		{Path: "app.json", Content: []byte(`{"a":1}`)},
	}
}

// build writes a raw archive with the given entries, in order.
func build(t *testing.T, entries ...[2]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		w, err := zw.Create(e[0])
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(e[1]))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func manifest(paths map[string]string) string {
	m := archive.Manifest{Format: archive.Format, Version: archive.Version}
	for p, c := range paths {
		sum := sha256.Sum256([]byte(c))
		m.Files = append(m.Files, archive.Entry{Path: p, Size: int64(len(c)), SHA256: hex.EncodeToString(sum[:])})
	}
	out, _ := json.Marshal(m)
	return string(out)
}

// TestPackIsDeterministic_STU_023 proves the same files give the same bytes
// whatever their order, and that the archive round-trips.
func TestPackIsDeterministic_STU_023(t *testing.T) {
	app := archive.AppInfo{Key: "shop", Name: "Shop", Tags: []string{"web"}}
	a, err := archive.Pack(app, files())
	if err != nil {
		t.Fatal(err)
	}
	reversed := files()
	reversed[0], reversed[1] = reversed[1], reversed[0]
	b, err := archive.Pack(app, reversed)
	if err != nil || !bytes.Equal(a, b) {
		t.Fatalf("the output depends on the order of the files: %v", err)
	}
	m, got, err := archive.Unpack(a, big)
	if err != nil || m.App.Key != "shop" || len(got) != 2 || got[0].Path != "app.json" {
		t.Fatalf("round trip: %+v %v %v", m, got, err)
	}
}

// TestUnpackRefusesUntrustedArchives_STU_024 walks the checks of ADR-0102.
func TestUnpackRefusesUntrustedArchives_STU_024(t *testing.T) {
	good := manifest(map[string]string{"app.json": "{}"})
	cases := []struct {
		name string
		data []byte
		lim  archive.Limits
		want plxerr.Code
	}{
		{"not a zip", []byte("PK nope"), big, plxerr.InvalidFormat},
		{"traversal", build(t, [2]string{archive.ManifestName, good}, [2]string{"project/../evil", "x"}), big, plxerr.InvalidProjectLayout},
		{"absolute", build(t, [2]string{archive.ManifestName, good}, [2]string{"/etc/passwd", "x"}), big, plxerr.InvalidProjectLayout},
		{"backslash", build(t, [2]string{archive.ManifestName, good}, [2]string{`project\app.json`, "x"}), big, plxerr.InvalidProjectLayout},
		{"outside project", build(t, [2]string{archive.ManifestName, good}, [2]string{"other/app.json", "{}"}), big, plxerr.InvalidProjectLayout},
		{"no manifest", build(t, [2]string{"project/app.json", "{}"}), big, plxerr.InvalidProjectLayout},
		{"hash mismatch", build(t, [2]string{archive.ManifestName, good}, [2]string{"project/app.json", "{ }"}), big, plxerr.InvalidProjectLayout},
		{"unlisted file", build(t, [2]string{archive.ManifestName, good}, [2]string{"project/app.json", "{}"}, [2]string{"project/extra.json", "{}"}), big, plxerr.InvalidProjectLayout},
		{"too many entries", build(t, [2]string{archive.ManifestName, good}, [2]string{"project/app.json", "{}"}), archive.Limits{Bytes: 1 << 20, Entries: 1, ExpandedBytes: 1 << 20}, plxerr.LimitExceeded},
		{"too many bytes", build(t, [2]string{archive.ManifestName, good}, [2]string{"project/app.json", "{}"}), archive.Limits{Bytes: 10, Entries: 10, ExpandedBytes: 1 << 20}, plxerr.LimitExceeded},
		{"expands too far", build(t, [2]string{archive.ManifestName, good}, [2]string{"project/app.json", "{}"}), archive.Limits{Bytes: 1 << 20, Entries: 10, ExpandedBytes: 4}, plxerr.LimitExceeded},
		{"newer version", build(t, [2]string{archive.ManifestName, `{"format":"plux-archive","version":2,"files":[]}`}), big, plxerr.InvalidFormat},
		{"wrong format", build(t, [2]string{archive.ManifestName, `{"format":"zip","version":1,"files":[]}`}), big, plxerr.InvalidFormat},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, _, err := archive.Unpack(c.data, c.lim); code(err) != c.want {
				t.Fatalf("got %v (%d), want code %d", err, code(err), c.want)
			}
		})
	}
	if _, _, err := archive.Unpack(build(t, [2]string{archive.ManifestName, good}, [2]string{"project/app.json", "{}"}), big); err != nil {
		t.Fatalf("the control archive is refused: %v", err)
	}
}

// TestUnpackCountsRealBytes_STU_024 proves the expanded-size limit holds
// when the header declares a small size and the data is large.
func TestUnpackCountsRealBytes_STU_024(t *testing.T) {
	payload := string(bytes.Repeat([]byte("a"), 4096))
	data := build(t, [2]string{archive.ManifestName, manifest(map[string]string{"app.json": payload})}, [2]string{"project/app.json", payload})
	if _, _, err := archive.Unpack(data, archive.Limits{Bytes: 1 << 20, Entries: 10, ExpandedBytes: 2048}); code(err) != plxerr.LimitExceeded {
		t.Fatalf("got %v", err)
	}
}

// TestPackRefusesBadNames_STU_023 keeps a bad name out of an archive.
func TestPackRefusesBadNames_STU_023(t *testing.T) {
	for _, name := range []string{"", "../x", "/x", "a//b", "a/./b", "a\\b", "c:x", "a\x00b", "a/"} {
		if _, err := archive.Pack(archive.AppInfo{}, []document.File{{Path: name, Content: []byte("x")}}); err == nil {
			t.Errorf("%q was packed", name)
		}
	}
	dup := []document.File{{Path: "a", Content: nil}, {Path: "a", Content: nil}}
	if _, err := archive.Pack(archive.AppInfo{}, dup); err == nil {
		t.Error("a duplicate path was packed")
	}
}
