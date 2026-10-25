// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"fmt"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

const (
	canvasPlugin = "01a0c450-6c00-7012-8000-000000022cce"
	canvasFrame  = "01a0c450-6c00-7026-8000-00000004977a"
)

func canvasDoc(extra string) []byte {
	return []byte(`{"schemaVersion":"1.0.0","kind":"canvas","id":"01a0c450-6c00-7033-8000-0000000a0001"` + extra + `}`)
}

func TestCanvasAccepts_STU_032(t *testing.T) {
	t.Parallel()
	l := newLoader(t)
	for name, doc := range map[string][]byte{
		"empty": canvasDoc(""),
		"app": canvasDoc(`,"positions":[{"id":"` + canvasPlugin + `","x":-120.5,"y":64}],` +
			`"frames":[{"id":"` + canvasFrame + `","label":"Onboarding","x":0,"y":0,"width":640,"height":480,"members":["` + canvasPlugin + `"]}]`),
	} {
		src, diags := l.ParseDocument("canvas.json", doc, KindCanvas)
		if src == nil || diags.HasErrors() {
			t.Errorf("%s: %s", name, list(diags))
		}
	}
}

func TestCanvasRejects_STU_032(t *testing.T) {
	t.Parallel()
	l := newLoader(t)
	pos := func(x string) string { return `,"positions":[{"id":"` + canvasPlugin + `","x":` + x + `,"y":0}]` }
	for name, doc := range map[string]string{
		"string coordinate": pos(`"left"`),
		"out of world":      pos(`2000000`),
		"missing y":         `,"positions":[{"id":"` + canvasPlugin + `","x":1}]`,
		"unknown field":     `,"zoom":2`,
		"zero-size frame":   `,"frames":[{"id":"` + canvasFrame + `","label":"A","x":0,"y":0,"width":0,"height":10}]`,
		"empty label":       `,"frames":[{"id":"` + canvasFrame + `","label":"","x":0,"y":0,"width":1,"height":1}]`,
		"duplicate member":  `,"frames":[{"id":"` + canvasFrame + `","label":"A","x":0,"y":0,"width":1,"height":1,"members":["` + canvasPlugin + `","` + canvasPlugin + `"]}]`,
	} {
		_, diags := l.ParseDocument("canvas.json", canvasDoc(doc), KindCanvas)
		if !diags.HasErrors() {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestCanvasLimits_STU_032(t *testing.T) {
	t.Parallel()
	l := newLoader(t)
	var b strings.Builder
	b.WriteString(`,"positions":[`)
	for i := 0; i <= 1000; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"id":"%s","x":%d,"y":0}`, canvasPlugin, i)
	}
	b.WriteString(`]`)
	_, diags := l.ParseDocument("canvas.json", canvasDoc(b.String()), KindCanvas)
	wantDiagnostic(t, diags, plxerr.LimitExceeded, "canvas.json", "/positions")
}

func TestCanvasLayout_STU_032(t *testing.T) {
	t.Parallel()
	for path, want := range map[string]Place{
		"canvas.json":               {Kind: KindCanvas},
		"plugins/loans/canvas.json": {Kind: KindCanvas, Plugin: "loans"},
	} {
		if got, ok := PlaceOf(path); !ok || got != want {
			t.Errorf("PlaceOf(%q) = %+v, %v", path, got, ok)
		}
	}
	if _, ok := PlaceOf("pages/canvas.json"); ok {
		t.Error("canvas.json outside the app and plugin roots was accepted")
	}
}

func TestCanvasLoadedNotAsError_STU_032(t *testing.T) {
	t.Parallel()
	fsys := exampleFS(t)
	fsys["canvas.json"] = &fstest.MapFile{Data: canvasDoc("")}
	p, diags := newLoader(t).Load(fsys)
	if diags.HasErrors() || p.Canvas == nil || p.Canvas.Doc == nil {
		t.Fatalf("canvas not loaded: %s", list(diags))
	}
}
