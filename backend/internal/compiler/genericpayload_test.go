// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	"testing"

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
