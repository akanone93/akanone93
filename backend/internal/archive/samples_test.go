// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package archive_test

import (
	"bytes"
	"flag"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/archive"
	"github.com/nightCode42/plux3/backend/internal/document"
	"github.com/nightCode42/plux3/backend/internal/schema"
)

var update = flag.Bool("update", false, "rewrite the sample archives from their source projects")

// sources are the reference apps' projects in the SCH-006 layout, with the
// description each sample carries (STU-015, F17).
var sources = []struct{ id, name, description string }{
	{"plux_bank", "Plux Bank", "A banking app with transfers, a secure release policy and three languages."},
	{"plux_express", "Plux Express", "A delivery app with live tracking, functions and four languages."},
	{"starter", "Starter", "The smallest app that runs: one plugin, one page, two languages."},
}

// project reads a source project in the layout the archive carries.
func project(t testing.TB, id string) []document.File {
	t.Helper()
	root := filepath.Join("..", "..", "..", "schema", "testdata", "documents", id)
	var files []document.File
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if _, ok := schema.PlaceOf(rel); !ok && !strings.HasPrefix(rel, "assets/") {
			return nil
		}
		data, err := os.ReadFile(p) //nolint:gosec // a fixture of this repository
		if err != nil {
			return err
		}
		files = append(files, document.File{Path: rel, Content: data})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// TestSamplesAreCurrent_STU_015 rebuilds every embedded sample from its source
// project and compares bytes. Run with -update to rewrite them.
func TestSamplesAreCurrent_STU_015(t *testing.T) {
	for _, s := range sources {
		data, err := archive.Pack(archive.AppInfo{Key: strings.ReplaceAll(s.id, "_", "-"), Name: s.name, Description: s.description}, project(t, s.id))
		if err != nil {
			t.Fatal(err)
		}
		file := filepath.Join("samples", s.id+".plux")
		if *update {
			if err := os.WriteFile(file, data, 0o644); err != nil { //nolint:gosec // a committed sample
				t.Fatal(err)
			}
			continue
		}
		have, err := os.ReadFile(file) //nolint:gosec // a committed sample
		if err != nil || !bytes.Equal(have, data) {
			t.Errorf("%s is stale: run go test ./internal/archive -run Samples -update", file)
		}
	}
	samples, err := archive.Embedded()
	if err != nil || len(samples) != len(sources) {
		t.Fatalf("embedded samples: %d %v", len(samples), err)
	}
}
