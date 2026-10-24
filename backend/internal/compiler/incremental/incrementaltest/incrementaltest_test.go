// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package incrementaltest

import (
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

var root = filepath.Join("..", "..", "..", "..", "..")

// pageIDs reads the plugin's page list.
func pageIDs(t *testing.T, data []byte) []any {
	t.Helper()
	var plugin map[string]any
	if err := json.Unmarshal(data, &plugin); err != nil {
		t.Fatal(err)
	}
	pages, _ := plugin["pages"].([]any)
	return pages
}

// Verifies: CMP-051. The copies the incremental tests compile are separate
// pages: each has its own file, ID, route and padding, and the plugin lists
// them all, so an edit of one copy cannot touch another.
func TestManyPagesCopiesAreIndependent_CMP_051(t *testing.T) {
	base := Fixture(t, root)
	if _, ok := base[CalculatorPage]; !ok {
		t.Fatalf("the fixture lacks %s", CalculatorPage)
	}
	before := len(pageIDs(t, base[PluginFile].Data))
	var baseDoc map[string]any
	if err := json.Unmarshal(base[CalculatorPage].Data, &baseDoc); err != nil {
		t.Fatal(err)
	}
	baseChildren := len(At(t, baseDoc, column)["children"].([]any))

	m := ManyPages(t, root, 3, 20)
	if got := len(pageIDs(t, m[PluginFile].Data)); got != before+3 {
		t.Fatalf("plugin lists %d pages, want %d", got, before+3)
	}
	seen := map[string]bool{}
	for i := range 3 {
		file := PageFile(i)
		f, ok := m[file]
		if !ok {
			t.Fatalf("copy %d: no file %s", i, file)
		}
		var doc map[string]any
		if err := json.Unmarshal(f.Data, &doc); err != nil {
			t.Fatal(err)
		}
		id, _ := doc["id"].(string)
		if id == "" || id == CalculatorID || seen[id] {
			t.Errorf("copy %d: id %q is not fresh", i, id)
		}
		seen[id] = true
		if !strings.Contains(string(f.Data), `"route": "copy`) {
			t.Errorf("copy %d: route not rewritten", i)
		}
		col := At(t, doc, column)
		// pad numbers its Text nodes from 12, so nodes=20 adds eight.
		if n := len(col["children"].([]any)); n != baseChildren+8 {
			t.Errorf("copy %d: %d children, want %d after padding", i, n, baseChildren+8)
		}
	}
}

// Verifies: CMP-051. Edit rewrites a document through its callback and At
// walks object keys and list indices.
func TestEditAndAt_CMP_051(t *testing.T) {
	m := Fixture(t, root)
	Edit(t, m, CalculatorPage, func(doc map[string]any) {
		col := At(t, doc, column)
		col["children"] = append(col["children"].([]any), TextNode("01c1ffff-6c00-7000-8000-000000000001", `"x"`))
	})
	var doc map[string]any
	if err := json.Unmarshal(m[CalculatorPage].Data, &doc); err != nil {
		t.Fatal(err)
	}
	col := At(t, doc, column)
	children := col["children"].([]any)
	last := At(t, children, strconv.Itoa(len(children)-1))
	if last["type"] != "Text" {
		t.Errorf("last child type %v, want Text", last["type"])
	}
	expr := At(t, At(t, last, "props"), "data")["$expr"]
	if expr != `"x"` {
		t.Errorf("expression %v, want the edited one", expr)
	}
}

// fatalRecorder is a testing.TB whose Fatalf records instead of stopping.
type fatalRecorder struct {
	testing.TB
	msg string
}

func (*fatalRecorder) Helper() {}
func (r *fatalRecorder) Fatalf(format string, args ...any) {
	r.msg = format
	panic(r)
}

// Verifies: CMP-051. A bad path fails the test instead of returning a
// nil object: an index past the list, and a path that is not an object.
func TestAtRejectsBadPaths_CMP_051(t *testing.T) {
	doc := map[string]any{"list": []any{map[string]any{}}, "n": 1.0}
	for _, path := range []string{"list/5", "list/x", "n"} {
		rec := &fatalRecorder{TB: t}
		func() {
			defer func() {
				if recover() != rec {
					t.Errorf("At(%q) did not fail", path)
				}
			}()
			At(rec, doc, path)
		}()
	}
}
