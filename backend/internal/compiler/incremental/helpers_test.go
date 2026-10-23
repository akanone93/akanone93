// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package incremental

import (
	"context"
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

	"github.com/nightCode42/plux3/backend/internal/compiler"
	"github.com/nightCode42/plux3/backend/internal/icons"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// fixtureDir is the conformance project (Appendix A).
var fixtureDir = filepath.Join("..", "..", "..", "..", "schema", "testdata", "documents", "loan-calculator")

// Paths and IDs of the fixture.
const (
	calculatorPage = "plugins/loans/pages/calculator.page.json"
	resultPage     = "plugins/loans/pages/result.page.json"
	calculateGraph = "plugins/loans/actions/calculate.graph.json"
	pluginFile     = "plugins/loans/plugin.json"
	calculatorID   = "01a0c450-6c00-7012-8000-000000022cce"
	column         = "root/slots/body/slots/child"
	sliderNode     = column + "/children/2"
)

// Patterns rewritten in page copies.
var (
	uuidPattern = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}`)
	keyPattern  = regexp.MustCompile(`"key": "([a-z-]+)"`)
)

// fixture returns the conformance project in memory.
func fixture(t testing.TB) fstest.MapFS {
	t.Helper()
	m := fstest.MapFS{}
	root := os.DirFS(fixtureDir)
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

// pageFile is the document file of the n-th copy of the calculator page.
func pageFile(n int) string {
	return fmt.Sprintf("plugins/loans/pages/calculator-copy%d.page.json", n)
}

// graphFiles are the action graph files of the n-th copy.
func graphFiles(n int) []string {
	return []string{
		fmt.Sprintf("plugins/loans/actions/calculate-copy%d.graph.json", n),
		fmt.Sprintf("plugins/loans/actions/set-amount-copy%d.graph.json", n),
	}
}

// manyPages returns the conformance project with the given number of copies
// of the calculator page and its action graphs added to the loans plugin,
// each with its own IDs, keys and route, padded to the given number of nodes
// with Text nodes that bind their own expressions.
func manyPages(t testing.TB, copies, nodes int) fstest.MapFS {
	t.Helper()
	m := fixture(t)
	docs := map[string]string{calculatorPage: string(m[calculatorPage].Data)}
	own := map[string]bool{}
	for file, f := range m {
		var doc map[string]any
		if err := json.Unmarshal(f.Data, &doc); err != nil {
			continue
		}
		if file == calculatorPage || strings.HasSuffix(file, ".graph.json") && doc["page"] == calculatorID {
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
			if file == calculatorPage {
				edit(t, m, name, func(doc map[string]any) { pad(t, doc, i, nodes) })
			}
		}
		pages = append(pages, ids[calculatorID])
	}
	edit(t, m, pluginFile, func(doc map[string]any) { doc["pages"] = append(doc["pages"].([]any), pages...) })
	return m
}

// pad adds Text nodes to the calculator page's column until the page has the
// given number of nodes.
func pad(t testing.TB, doc map[string]any, copy, nodes int) {
	t.Helper()
	col := at(t, doc, column)
	children := col["children"].([]any)
	for j := 12; j < nodes; j++ {
		children = append(children, textNode(fmt.Sprintf("01c1%04x-6c00-7000-8000-%012x", copy, j), fmt.Sprintf(`"Month " + string(page.months + %d)`, j)))
	}
	col["children"] = children
}

// textNode is a Text node showing an expression.
func textNode(id, expr string) map[string]any {
	return map[string]any{"id": id, "type": "Text", "props": map[string]any{"data": map[string]any{"$expr": expr}}}
}

// collectUUIDs adds the values of every "id" key that holds a UUID.
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

// edit changes a JSON document of m.
func edit(t testing.TB, m fstest.MapFS, file string, f func(doc map[string]any)) {
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

// at walks a path of object keys and list indices to an object.
func at(t testing.TB, doc any, path string) map[string]any {
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

// pageID returns the ID of the page in a file.
func pageID(t testing.TB, m fstest.MapFS, file string) string {
	t.Helper()
	var doc struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(m[file].Data, &doc); err != nil {
		t.Fatal(err)
	}
	return doc.ID
}

// opts are the options every test compiles with: an icon font, so that the
// icons of the pages reach the bundles.
func opts() compiler.Options {
	o := compiler.DefaultOptions()
	o.IconFont = func(set icons.Set, names []string) ([]byte, error) {
		return fmt.Appendf(nil, "font %s %v", set, names), nil
	}
	return o
}

// incr compiles with the cache and fails the test on an error.
func incr(t testing.TB, c *Cache, m fstest.MapFS) *Result {
	t.Helper()
	res, err := Compile(context.Background(), c, Input{FS: m})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// full compiles with the full compiler.
func full(m fstest.MapFS) *compiler.Result { return compiler.Compile(m, opts()) }

// diagLines formats diagnostics, one per line.
func diagLines(ds plxerr.Diagnostics) []string {
	out := make([]string, len(ds))
	for i, d := range ds {
		out[i] = d.String()
	}
	return out
}

// mustMatch fails the test unless the incremental result has the
// diagnostics and the bytes of the full compilation (CMP-002).
func mustMatch(t testing.TB, got *Result, want *compiler.Result) {
	t.Helper()
	if g, w := diagLines(got.Diagnostics), diagLines(want.Diagnostics); !slices.Equal(g, w) {
		t.Fatalf("diagnostics differ\n got: %q\nwant: %q", g, w)
	}
	if (got.App == nil) != (want.App == nil) {
		t.Fatalf("app bundle: got %v, want %v", got.App != nil, want.App != nil)
	}
	if want.App == nil {
		return
	}
	if string(got.App.Data) != string(want.App.Data) {
		t.Errorf("app bundle differs: %d bytes, want %d", len(got.App.Data), len(want.App.Data))
	}
	if len(got.Plugins) != len(want.Plugins) {
		t.Fatalf("%d plugin bundles, want %d", len(got.Plugins), len(want.Plugins))
	}
	for i := range want.Plugins {
		if string(got.Plugins[i].Data) != string(want.Plugins[i].Data) {
			t.Errorf("plugin bundle %s differs: %d bytes, want %d", want.Plugins[i].Key, len(got.Plugins[i].Data), len(want.Plugins[i].Data))
		}
	}
	if len(got.Files) != len(want.Files) {
		t.Errorf("%d files, want %d", len(got.Files), len(want.Files))
	}
	for sum, data := range want.Files {
		if string(got.Files[sum]) != string(data) {
			t.Errorf("file %x differs", sum)
		}
	}
}
