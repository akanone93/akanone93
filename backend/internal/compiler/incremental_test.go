// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	"bytes"
	"reflect"
	"slices"
	"testing"
	"testing/fstest"

	"github.com/nightCode42/plux3/backend/internal/schema"
)

// sameBundles fails the test unless two results hold byte-identical
// bundles and files (CMP-002).
func sameBundles(t *testing.T, got, want *Result) {
	t.Helper()
	if want.App == nil || got.App == nil {
		t.Fatalf("missing app bundle: got %v, want %v", got.App != nil, want.App != nil)
	}
	if !bytes.Equal(got.App.Data, want.App.Data) {
		t.Errorf("app bundle differs: %d bytes, want %d", len(got.App.Data), len(want.App.Data))
	}
	if len(got.Plugins) != len(want.Plugins) {
		t.Fatalf("%d plugin bundles, want %d", len(got.Plugins), len(want.Plugins))
	}
	for i := range want.Plugins {
		if got.Plugins[i].Key != want.Plugins[i].Key || !bytes.Equal(got.Plugins[i].Data, want.Plugins[i].Data) {
			t.Errorf("plugin bundle %s differs: %d bytes, want %d", want.Plugins[i].Key, len(got.Plugins[i].Data), len(want.Plugins[i].Data))
		}
		if !slices.Equal(got.Plugins[i].Features, want.Plugins[i].Features) {
			t.Errorf("plugin %s features %v, want %v", want.Plugins[i].Key, got.Plugins[i].Features, want.Plugins[i].Features)
		}
	}
	if !reflect.DeepEqual(got.Files, want.Files) {
		t.Errorf("files differ: %d, want %d", len(got.Files), len(want.Files))
	}
	if !reflect.DeepEqual(got.Graph.Edges, want.Graph.Edges) {
		t.Errorf("reference graphs differ: %d edges, want %d", len(got.Graph.Edges), len(want.Graph.Edges))
	}
}

// loadProject loads a project the way Compile does.
func loadProject(t testing.TB, m fstest.MapFS) *schema.Project {
	t.Helper()
	l, err := NewLoader(DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	p, diags := l.Load(m)
	if len(diags) > 0 {
		t.Fatalf("diagnostics:\n%s", list(diags))
	}
	return p
}

// TestCompileMemoMatchesCompile checks that capturing a Memo does not change
// the bundles (CMP-051, CMP-002).
func TestCompileMemoMatchesCompile(t *testing.T) {
	t.Parallel()
	for name, project := range map[string]func(testing.TB) fstest.MapFS{
		"fixture": fixture,
		"many":    func(t testing.TB) fstest.MapFS { return manyPages(t, 12) },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			m := project(t)
			want := compileFS(m)
			got, memo := CompileMemo(m, DefaultOptions())
			if memo == nil {
				t.Fatalf("no memo; diagnostics:\n%s", list(got.Diagnostics))
			}
			sameBundles(t, got, want)
		})
	}
}

// TestRecompileMatchesCompile checks that recompiling an edited page gives
// the bytes of a full compilation (CMP-051, CMP-002).
func TestRecompileMatchesCompile(t *testing.T) {
	t.Parallel()
	m := fixture(t)
	before, memo := CompileMemo(m, DefaultOptions())
	if memo == nil {
		t.Fatal("no memo")
	}
	edit(t, m, calculatorPage, func(doc map[string]any) {
		at(t, doc, sliderNode)["props"].(map[string]any)["max"] = 72
	})
	res, next, ok := Recompile(memo, loadProject(t, m), DefaultOptions(), []string{calculatorPage})
	if !ok {
		t.Fatal("Recompile declined")
	}
	if bytes.Equal(res.Plugins[0].Data, before.Plugins[0].Data) {
		t.Error("the edit did not change the bundle")
	}
	sameBundles(t, res, compileFS(m))
	// A second edit builds on the first one's Memo.
	edit(t, m, calculatorPage, func(doc map[string]any) { doc["description"] = "edited twice" })
	res, _, ok = Recompile(next, loadProject(t, m), DefaultOptions(), []string{calculatorPage})
	if !ok {
		t.Fatal("second Recompile declined")
	}
	sameBundles(t, res, compileFS(m))
}
