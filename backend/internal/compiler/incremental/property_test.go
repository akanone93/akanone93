// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package incremental

import (
	"fmt"
	"math/rand/v2"
	"testing"
	"testing/fstest"

	"github.com/nightCode42/plux3/backend/internal/bundle"
	"github.com/nightCode42/plux3/backend/internal/compiler"
)

// editor applies edits to a project and compiles it both ways.
type editor struct {
	t      testing.TB
	m      fstest.MapFS
	c      *Cache
	copies int
	// padded is the number of Text nodes a copy has beyond the calculator
	// page's own twelve.
	padded int
	fresh  int
	// original holds the content of a page that an edit broke.
	original map[string]string
	// sections models the sections a device holds, by applying Changed and
	// Removed to it.
	sections map[sectionID][32]byte
	steps    int
	incr     int
	// reasons counts the full compilations by their reason.
	reasons map[string]int
}

// breakOp is the index of the operation that breaks a page or mends it.
const breakOp = 10

// sectionID identifies a section of a bundle.
type sectionID struct {
	bundle string
	kind   bundle.SectionKind
	id     bundle.ID
}

// newEditor compiles the project once.
func newEditor(t testing.TB, copies, nodes int) *editor {
	t.Helper()
	m := manyPages(t, copies, nodes)
	// The widgets that raise locale data need a newer runtime than the
	// fixture's.
	edit(t, m, "app.json", func(doc map[string]any) { doc["minRuntimeVersion"] = "0.6.0" })
	e := &editor{t: t, m: m, copies: copies, padded: nodes - 12, original: map[string]string{}, sections: map[sectionID][32]byte{}, reasons: map[string]int{}}
	e.c = NewCache(opts())
	e.check(incr(t, e.c, e.m))
	return e
}

// freshID returns an ID no document uses.
func (e *editor) freshID() string {
	e.fresh++
	return fmt.Sprintf("01d0%04x-6c00-7000-8000-%012x", e.fresh>>16, e.fresh)
}

// page returns the file of the n-th page: the copies, then the two
// original pages.
func (e *editor) page(n int) string {
	switch n %= e.copies + 2; {
	case n < e.copies:
		return pageFile(n)
	case n == e.copies:
		return calculatorPage
	default:
		return resultPage
	}
}

// padNode returns the n-th padded Text node of a copy, or nil.
func (e *editor) padNode(doc map[string]any, n int) map[string]any {
	children, _ := at(e.t, doc, column)["children"].([]any)
	if e.padded == 0 {
		return nil
	}
	i := 12 + n%e.padded
	if i >= len(children) {
		return nil
	}
	node, _ := children[i].(map[string]any)
	if node["type"] != "Text" {
		return nil
	}
	return node
}

// baseState is the number of state entries a page has before any edit adds
// one: three on the calculator pages, none on the result page.
func baseState(file string) int {
	if file == resultPage {
		return 0
	}
	return 3
}

// editOps are the edits a step can make.
var editOps = []func(e *editor, file string, a, b int){
	func(e *editor, file string, a, b int) { // the slider
		if file != resultPage {
			edit(e.t, e.m, file, func(doc map[string]any) { at(e.t, doc, sliderNode)["props"].(map[string]any)["max"] = 40 + b%60 })
		}
	},
	func(e *editor, file string, a, b int) { // the description
		edit(e.t, e.m, file, func(doc map[string]any) { doc["description"] = fmt.Sprintf("edit %d", b) })
	},
	func(e *editor, file string, a, b int) { // the expression of a padded node
		edit(e.t, e.m, file, func(doc map[string]any) {
			if file == resultPage {
				return
			}
			if n := e.padNode(doc, a); n != nil {
				n["props"] = map[string]any{"data": map[string]any{"$expr": fmt.Sprintf(`"Month " + string(page.months + %d)`, b)}}
			}
		})
	},
	func(e *editor, file string, a, b int) { // a new node
		edit(e.t, e.m, file, func(doc map[string]any) {
			if file == resultPage {
				return
			}
			col := at(e.t, doc, column)
			col["children"] = append(col["children"].([]any), textNode(e.freshID(), fmt.Sprintf(`"Added " + string(page.months * %d)`, b)))
		})
	},
	func(e *editor, file string, a, b int) { // drop the last node
		edit(e.t, e.m, file, func(doc map[string]any) {
			if file == resultPage {
				return
			}
			col := at(e.t, doc, column)
			if children := col["children"].([]any); len(children) > 12 {
				col["children"] = children[:len(children)-1]
			}
		})
	},
	func(e *editor, file string, a, b int) { // a new state entry
		edit(e.t, e.m, file, func(doc map[string]any) {
			st, _ := doc["state"].([]any)
			doc["state"] = append(st, map[string]any{"id": e.freshID(), "name": fmt.Sprintf("extra%d", e.fresh), "type": "string", "default": ""})
		})
	},
	func(e *editor, file string, a, b int) { // drop the last state entry
		edit(e.t, e.m, file, func(doc map[string]any) {
			if st, _ := doc["state"].([]any); len(st) > baseState(file) {
				doc["state"] = st[:len(st)-1]
			}
		})
	},
	func(e *editor, file string, a, b int) { // the persistence of the last state entry
		edit(e.t, e.m, file, func(doc map[string]any) {
			if st, _ := doc["state"].([]any); len(st) > baseState(file) {
				st[len(st)-1].(map[string]any)["persistence"] = []string{"memory", "session"}[b%2]
			}
		})
	},
	func(e *editor, file string, a, b int) { // the lifecycle
		edit(e.t, e.m, file, func(doc map[string]any) {
			if file != resultPage {
				doc["lifecycle"].(map[string]any)["onEnter"].(map[string]any)["steps"].([]any)[0].(map[string]any)["input"] = map[string]any{"name": fmt.Sprintf("opened_%d", b)}
			}
		})
	},
	func(e *editor, file string, a, b int) { // the layout of the file only
		e.m[file] = &fstest.MapFile{Data: append(e.m[file].Data, '\n')}
	},
	func(e *editor, file string, a, b int) { // break the page, or mend it
		for broken, orig := range e.original {
			e.m[broken] = &fstest.MapFile{Data: []byte(orig)}
			delete(e.original, broken)
			return
		}
		e.original[file] = string(e.m[file].Data)
		edit(e.t, e.m, file, func(doc map[string]any) { doc["root"].(map[string]any)["type"] = "NoSuchWidget" })
	},
	func(e *editor, file string, a, b int) { // the plugin, a document outside the pages
		edit(e.t, e.m, pluginFile, func(doc map[string]any) { doc["description"] = fmt.Sprintf("plugin %d", b) })
	},
	func(e *editor, file string, a, b int) { // an action graph of the page
		g := graphFiles(a % e.copies)[b%2]
		edit(e.t, e.m, g, func(doc map[string]any) { doc["description"] = fmt.Sprintf("graph %d", b) })
	},
	func(e *editor, file string, a, b int) { // the page the others navigate to
		edit(e.t, e.m, resultPage, func(doc map[string]any) { doc["description"] = fmt.Sprintf("result %d", b) })
	},
	func(e *editor, file string, a, b int) { // an icon, then another, then none
		if file == resultPage {
			return
		}
		edit(e.t, e.m, file, func(doc map[string]any) {
			deco := at(e.t, doc, column+"/children/0/props/decoration")
			switch b % 3 {
			case 0:
				delete(deco, "suffixIcon")
			case 1:
				deco["suffixIcon"] = map[string]any{"name": "home"}
			default:
				deco["suffixIcon"] = map[string]any{"name": "search"}
			}
		})
	},
	func(e *editor, file string, a, b int) { // a currency
		e.labelled(file, "AmountInput", map[string]any{"currency": []string{"EUR", "USD", "CHF"}[b%3]})
	},
	func(e *editor, file string, a, b int) { // a widget that lists territories
		e.labelled(file, "CountryPicker", nil)
	},
}

// labelled adds a node with an accessible name to the column of a page.
func (e *editor) labelled(file, typ string, props map[string]any) {
	if file == resultPage {
		return
	}
	edit(e.t, e.m, file, func(doc map[string]any) {
		col := at(e.t, doc, column)
		node := map[string]any{"id": e.freshID(), "type": typ, "semantics": map[string]any{"label": map[string]any{"$t": "01a0c450-6c00-7003-8000-000000005ccd"}}}
		if props != nil {
			node["props"] = props
		}
		col["children"] = append(col["children"].([]any), node)
	})
}

// step makes one edit and compares the compilations.
func (e *editor) step(op, a, b int) {
	e.t.Helper()
	e.steps++
	file := e.page(a)
	if len(e.original) > 0 {
		op = breakOp // a broken page is mended before anything else
	}
	editOps[op%len(editOps)](e, file, a, b)
	res := incr(e.t, e.c, e.m)
	if res.Full {
		e.reasons[res.FullReason]++
	}
	e.check(res)
}

// check compares an incremental result with the full compilation of the
// project (CMP-002) and its Changed with the sections it replaced.
func (e *editor) check(res *Result) {
	e.t.Helper()
	mustMatch(e.t, res, full(e.m))
	if !res.Full {
		e.incr++
	}
	if res.App == nil {
		return
	}
	for _, r := range res.Removed {
		delete(e.sections, sectionID{r.Bundle, r.Kind, r.ID})
	}
	for _, s := range res.Changed {
		parsed, err := bundle.ReadStructure(bundleOf(res, s.Bundle).Data)
		if err != nil {
			e.t.Fatal(err)
		}
		got, ok := parsed.Section(s.Kind, s.ID)
		if !ok || string(got.Data) != string(s.Data) {
			e.t.Fatalf("changed section %v of %s is not the bundle's", s.Kind, s.Bundle)
		}
		e.sections[sectionID{s.Bundle, s.Kind, s.ID}] = got.Hash
	}
	now := map[sectionID][32]byte{}
	for _, b := range append([]*compiler.Bundle{res.App}, res.Plugins...) {
		parsed, err := bundle.ReadStructure(b.Data)
		if err != nil {
			e.t.Fatal(err)
		}
		for _, s := range parsed.Sections {
			now[sectionID{b.Key, s.Kind, s.ID}] = s.Hash
		}
	}
	if len(now) != len(e.sections) {
		e.t.Fatalf("%d sections after applying Changed and Removed, the bundles have %d", len(e.sections), len(now))
	}
	for k, h := range now {
		if e.sections[k] != h {
			e.t.Fatalf("section %v of %s differs from the one the device holds", k.kind, k.bundle)
		}
	}
}

// bundleOf returns the bundle of a result by key.
func bundleOf(res *Result, key string) *compiler.Bundle {
	if res.App.Key == key {
		return res.App
	}
	for _, b := range res.Plugins {
		if b.Key == key {
			return b
		}
	}
	return nil
}

// TestEditSequencesMatchTheFullCompiler compiles generated edit sequences
// incrementally and in full and compares the bytes (CMP-051, CMP-002).
func TestEditSequencesMatchTheFullCompiler(t *testing.T) {
	t.Parallel()
	steps := 30
	if testing.Short() {
		steps = 10
	}
	for seed := uint64(1); seed <= 3; seed++ {
		t.Run(fmt.Sprintf("seed=%d", seed), func(t *testing.T) {
			t.Parallel()
			rng := rand.New(rand.NewPCG(seed, 99)) //nolint:gosec // G404: a seeded generator makes the edit sequence reproducible.
			e := newEditor(t, 4, 18)
			for range steps {
				e.step(rng.IntN(len(editOps)), rng.IntN(1<<10), rng.IntN(1<<10))
			}
			t.Logf("%d of %d steps were incremental; the others were full: %v", e.incr, e.steps, e.reasons)
			if e.incr < steps/3 {
				t.Errorf("only %d of %d steps were incremental", e.incr, e.steps)
			}
		})
	}
}

// FuzzEditSequences is the property test over arbitrary edit sequences:
// every three bytes are an operation and its two arguments.
func FuzzEditSequences(f *testing.F) {
	f.Add([]byte{0, 1, 2, 2, 3, 4, 5, 6, 7})
	f.Add([]byte{10, 1, 1, 10, 1, 1, 0, 1, 40})
	f.Add([]byte{14, 0, 3, 5, 2, 2, 6, 2, 2, 11, 0, 0})
	f.Add([]byte{15, 1, 1, 15, 1, 0, 15, 1, 2, 15, 3, 0})
	f.Add([]byte{15, 0, 1, 16, 0, 0, 4, 0, 0, 4, 0, 0, 15, 0, 2, 4, 0, 0})
	f.Fuzz(func(t *testing.T, data []byte) {
		e := newEditor(t, 3, 14)
		for i := 0; i+2 < len(data) && i < 3*10; i += 3 {
			e.step(int(data[i]), int(data[i+1]), int(data[i+2]))
		}
	})
}
