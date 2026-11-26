// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	"strings"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/pxl"
	"github.com/nightCode42/plux3/backend/internal/schema/registry"
)

// TestGenericEventPayloads checks that a generic widget's type arguments
// are substituted into its event payload types.
// Verifies: ACT-001.
func TestGenericEventPayloads(t *testing.T) {
	t.Parallel()
	env, err := pxl.NewEnv(pxl.EnvSpec{})
	if err != nil {
		t.Fatal(err)
	}
	str, err := env.ParseType("string")
	if err != nil {
		t.Fatal(err)
	}
	w := &registry.Widget{Type: "DragTarget", TypeParameters: []string{"T"}, Events: []registry.Event{
		{Name: "onAccept", Payload: "T"}, {Name: "onList", Payload: "list<T>"}, {Name: "onLeave"},
	}}
	n := &node{widget: w, typeArgs: map[string]*pxl.Type{"T": str}}
	tc := &typer{}
	for event, want := range map[string]string{"onAccept": "string", "onList": "list<string>", "onLeave": ""} {
		if got := tc.payload(n, event); got != want {
			t.Errorf("payload of %s = %q, want %q", event, got, want)
		}
	}
	n.typeArgs = nil
	if got := tc.payload(n, "onAccept"); got != "T" {
		t.Errorf("unbound payload = %q, want T", got)
	}
}

// TestDragTargetOnAcceptPayload checks the generated descriptor: the
// payload of DragTarget.onAccept is its type parameter, so a DragTarget
// bound to a type sees that type as the payload (ACT-001).
// Verifies: ACT-001.
func TestDragTargetOnAcceptPayload(t *testing.T) {
	t.Parallel()
	w, ok := registry.LookupWidget("DragTarget")
	if !ok {
		t.Fatal("DragTarget is not in the widget registry")
	}
	env, err := pxl.NewEnv(pxl.EnvSpec{})
	if err != nil {
		t.Fatal(err)
	}
	someType, err := env.ParseType("string")
	if err != nil {
		t.Fatal(err)
	}
	tc := &typer{}
	n := &node{widget: &w, typeArgs: map[string]*pxl.Type{"T": someType}}
	if got := tc.payload(n, "onAccept"); got != "string" {
		t.Errorf("payload of DragTarget<string>.onAccept = %q, want string", got)
	}
	n.typeArgs = nil
	if got := tc.payload(n, "onAccept"); got != "T" {
		t.Errorf("unbound payload of DragTarget.onAccept = %q, want T", got)
	}
}

// Verifies: ACT-001, WGT-001.
// A DragTarget's dataType names the type of what it takes without
// filtering anything: it types the onAccept payload, an accepts of another
// type and an unknown type are errors, and the prop is not written to the
// bundle, whose node carries the type argument instead (build log row 149).
func TestDragTargetDataTypeBindsThePayloadType(t *testing.T) {
	t.Parallel()
	compile := func(props, when string) *Result {
		target := `[{"id": "%ID%", "type": "DragTarget", "props": ` + props + `,
			"slots": {"child": {"id": "%ID%", "type": "Text", "props": {"data": "Drop here"}}},
			"events": {"onAccept": {"steps": [{"id": "c", "action": "condition", "input": {"when": {"$expr": "` + when + `"}}}]}}}]`
		m := a11yProject(t, "standard", "#000000", "#FFFFFF", target)
		edit(t, m, "app.json", func(doc map[string]any) { pinned(t, doc, "0.6.0") })
		return compileFS(m)
	}
	res := compile(`{"dataType": "int"}`, "event > 1")
	if res.Diagnostics.HasErrors() {
		t.Fatalf("diagnostics:\n%s", list(res.Diagnostics))
	}
	nodes, strs := pageNodes(t, readAll(t, res)[1], calculatorID)
	n := nodes["01a0c450-6c00-7fff-8000-000000000001"]
	if n == nil || n.PropsLength() != 0 || n.TypeArgumentsLength() != 1 || strs[n.TypeArguments(0)] != "int" {
		t.Fatalf("the DragTarget node should carry the type argument int and no prop")
	}
	for name, c := range map[string]struct {
		props, when, ptr string
		code             plxerr.Code
	}{
		"a payload of another type":  {`{"dataType": "string"}`, "event > 1", "", plxerr.PXLTypeMismatch},
		"an accepts of another type": {`{"dataType": "int", "accepts": {"$expr": "'x'"}}`, "event > 1", "/props/accepts", plxerr.PropTypeMismatch},
		"an unknown type":            {`{"dataType": "Nope"}`, "true", "/props/dataType", plxerr.UnknownType},
	} {
		found := false
		for _, d := range compile(c.props, c.when).Diagnostics {
			found = found || d.Code == c.code && strings.HasSuffix(d.Path, c.ptr)
		}
		if !found {
			t.Errorf("%s: no %v ending at %q", name, c.code, c.ptr)
		}
	}
}
