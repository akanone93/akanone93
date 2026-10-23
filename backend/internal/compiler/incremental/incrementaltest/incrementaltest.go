// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

// Package incrementaltest builds the projects the incremental compiler's
// tests and benchmarks compile: the conformance project with many copies of
// its calculator page. It is test support and is imported only by tests.
package incrementaltest

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

// fixtureRel is the conformance project (Appendix A), relative to the
// repository root.
const fixtureRel = "schema/testdata/documents/loan-calculator"

// Paths and IDs of the fixture.
const (
	// CalculatorPage is the page that is copied.
	CalculatorPage = "plugins/loans/pages/calculator.page.json"
	// PluginFile is the plugin that lists the pages.
	PluginFile = "plugins/loans/plugin.json"
	// CalculatorID is the ID of the calculator page.
	CalculatorID = "01a0c450-6c00-7012-8000-000000022cce"
	column       = "root/slots/body/slots/child"
)

// Patterns rewritten in page copies.
var (
	uuidPattern = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}`)
	keyPattern  = regexp.MustCompile(`"key": "([a-z-]+)"`)
)

// Fixture returns the conformance project in memory; dir is the path from
// the calling package's directory to the repository root.
func Fixture(t testing.TB, dir string) fstest.MapFS {
	t.Helper()
	m := fstest.MapFS{}
	root := os.DirFS(filepath.Join(dir, filepath.FromSlash(fixtureRel)))
	err := fs.WalkDir(root, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(root, p)
		m[p] = &fstest.MapFile{Data: data}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// PageFile is the document file of the n-th copy of the calculator page.
func PageFile(n int) string {
	return fmt.Sprintf("plugins/loans/pages/calculator-copy%d.page.json", n)
}

// ManyPages returns the conformance project with the given number of copies
// of the calculator page and its action graphs added to the loans plugin,
// each with its own IDs, keys and route, padded to the given number of nodes
// per page; root is the path from the calling package's directory to the
// repository root.
func ManyPages(t testing.TB, root string, copies, nodes int) fstest.MapFS {
	t.Helper()
	m := Fixture(t, root)
	docs := map[string]string{CalculatorPage: string(m[CalculatorPage].Data)}
	own := map[string]bool{}
	for file, f := range m {
		var doc map[string]any
		if err := json.Unmarshal(f.Data, &doc); err != nil {
			continue
		}
		if file == CalculatorPage || strings.HasSuffix(file, ".graph.json") && doc["page"] == CalculatorID {
			docs[file] = string(f.Data)
			collectUUIDs(doc, own)
		}
	}
	files := make([]string, 0, len(docs))
	for file := range docs {
		files = append(files, file)
	}
	slices.Sort(files)
	var pages []any
	for i := range copies {
		ids := map[string]string{}
		key := fmt.Sprintf("copy%d", i)
		for _, file := range files {
			copied := uuidPattern.ReplaceAllStringFunc(docs[file], func(id string) string {
				if !own[id] {
					return id
				}
				if _, ok := ids[id]; !ok {
					ids[id] = fmt.Sprintf("01c0%04x-6c00-7000-8000-%012x", i, len(ids))
				}
				return ids[id]
			})
			copied = keyPattern.ReplaceAllString(copied, `"key": "${1}-`+key+`"`)
			copied = strings.Replace(copied, `"route": "loan-calculator"`, `"route": "`+key+`"`, 1)
			name := strings.Replace(file, ".", "-"+key+".", 1)
			m[name] = &fstest.MapFile{Data: []byte(copied)}
			if file == CalculatorPage {
				Edit(t, m, name, func(doc map[string]any) { pad(t, doc, i, nodes) })
			}
		}
		pages = append(pages, ids[CalculatorID])
	}
	Edit(t, m, PluginFile, func(doc map[string]any) { doc["pages"] = append(doc["pages"].([]any), pages...) })
	return m
}

// Edit changes a JSON document of m.
func Edit(t testing.TB, m fstest.MapFS, file string, f func(doc map[string]any)) {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(m[file].Data, &doc); err != nil {
		t.Fatal(err)
	}
	f(doc)
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	m[file] = &fstest.MapFile{Data: data}
}

// pad adds Text nodes to the calculator page's column until the page has the
// given number of nodes.
func pad(t testing.TB, doc map[string]any, copy, nodes int) {
	t.Helper()
	col := At(t, doc, column)
	children := col["children"].([]any)
	for j := 12; j < nodes; j++ {
		children = append(children, TextNode(fmt.Sprintf("01c1%04x-6c00-7000-8000-%012x", copy, j), fmt.Sprintf(`"Month " + string(page.months + %d)`, j)))
	}
	col["children"] = children
}

// TextNode is a Text node showing an expression.
func TextNode(id, expr string) map[string]any {
	return map[string]any{"id": id, "type": "Text", "props": map[string]any{"data": map[string]any{"$expr": expr}}}
}

// At walks a path of object keys and list indices to an object.
func At(t testing.TB, doc any, path string) map[string]any {
	t.Helper()
	cur := doc
	for _, tok := range strings.Split(path, "/") {
		switch x := cur.(type) {
		case map[string]any:
			cur = x[tok]
		case []any:
			var i int
			if _, err := fmt.Sscanf(tok, "%d", &i); err != nil || i >= len(x) {
				t.Fatalf("path %s: bad index %s", path, tok)
			}
			cur = x[i]
		}
	}
	m, ok := cur.(map[string]any)
	if !ok {
		t.Fatalf("path %s is not an object", path)
	}
	return m
}

func collectUUIDs(v any, ids map[string]bool) {
	switch x := v.(type) {
	case map[string]any:
		for k, it := range x {
			if s, ok := it.(string); ok && k == "id" && uuidPattern.MatchString(s) {
				ids[s] = true
			}
			collectUUIDs(it, ids)
		}
	case []any:
		for _, it := range x {
			collectUUIDs(it, ids)
		}
	}
}
