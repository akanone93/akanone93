// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package pxl

import (
	"reflect"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// contextualEnv declares an organisation list and the value types a map
// binds: a marker needs an id and a position, its title is nullable and its
// icon optional.
func contextualEnv(t *testing.T) *Env {
	t.Helper()
	env, err := NewEnv(EnvSpec{
		Types: map[string]TypeSpec{
			"Org":     {Fields: map[string]string{"slug": "string", "name": "string", "lat": "double", "lng": "double"}},
			"LatLng":  {Fields: map[string]string{"lat": "double", "lng": "double"}},
			"Marker":  {Fields: map[string]string{"id": "string", "position": "LatLng", "title": "string?", "icon": "string"}, Optional: []string{"icon"}},
			"Stage":   {Enum: []string{"done", "current", "pending"}},
			"Filters": {Fields: map[string]string{"open": "bool"}},
		},
		Roots: map[string]string{"orgs": "list<Org>", "sent": "bool", "maybe": "LatLng?"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return env
}

func expecting(t *testing.T, env *Env, typ string) Options {
	t.Helper()
	want, err := env.ParseType(typ)
	if err != nil {
		t.Fatal(err)
	}
	opts := DefaultOptions()
	opts.Expected = want
	return opts
}

// Verifies: PXL-001, PXL-002.
// Where a value of a known type is expected, literals take their type from
// the context: a map literal builds an object, items and branches follow
// the expected element, and a string names an enum member.
func TestContextualLiterals(t *testing.T) {
	t.Parallel()
	env := contextualEnv(t)
	for _, tc := range []struct {
		src, want string
	}{
		{`orgs.map(o, {"id": o.slug, "position": {"lat": o.lat, "lng": o.lng}, "title": o.name})`, "list<Marker>"},
		{`[{"id": "a", "position": {"lat": 1.0, "lng": 2.0}, "title": null}]`, "list<Marker>"},
		{`{"lat": 52.5, "lng": 13.4}`, "LatLng"},
		{`maybe ?? {"lat": 0.0, "lng": 0.0}`, "LatLng"},
		{`sent ? "current" : "done"`, "Stage"},
		{`sent ? {"lat": 1.0, "lng": 1.0} : {"lat": 2.0, "lng": 2.0}`, "LatLng"},
		{`orgs.map(o, {"id": o.slug, "position": {"lat": o.lat, "lng": o.lng}, "title": o.name}) + [{"id": "b", "position": {"lat": 0.0, "lng": 0.0}, "title": null}]`, "list<Marker>"},
		{`[{"lat": 1.0, "lng": 2.0}] + [] + [{"lat": 3.0, "lng": 4.0}]`, "list<LatLng>"},
	} {
		prog, typ, diags := Compile(tc.src, env, expecting(t, env, tc.want), plxerr.Location{})
		if prog == nil || diags.HasErrors() {
			t.Errorf("%s as %s: %v", tc.src, tc.want, diags)
			continue
		}
		if typ.String() != tc.want {
			t.Errorf("%s: type %s, want %s", tc.src, typ, tc.want)
		}
	}
	// Without an expected type the same literals keep their own types.
	if _, _, diags := Compile(`{"id": "a", "position": {"lat": 1.0, "lng": 2.0}}`, env, DefaultOptions(), plxerr.Location{}); !diags.HasErrors() {
		t.Error("a map of mixed values without a context is still refused")
	}
}

// Verifies: PXL-001.
// An object literal names fields the type has, gives each a value of its
// type and leaves out only nullable or optional fields; an enum string must
// name a member.
func TestContextualLiteralErrors(t *testing.T) {
	t.Parallel()
	env := contextualEnv(t)
	for _, tc := range []struct {
		src, want string
		code      plxerr.Code
	}{
		{`{"lat": 1.0, "lng": 2.0, "alt": 3.0}`, "LatLng", plxerr.PXLUnknownField},
		{`{"lat": 1.0}`, "LatLng", plxerr.PXLTypeMismatch},
		{`{"lat": "north", "lng": 2.0}`, "LatLng", plxerr.PXLTypeMismatch},
		{`{"lat": 1.0, "lat": 2.0, "lng": 0.0}`, "LatLng", plxerr.PXLInvalidLiteral},
		{`sent ? "current" : "later"`, "Stage", plxerr.PXLUnknownEnumMember},
		{`{"id": "a", "title": null}`, "Marker", plxerr.PXLTypeMismatch},
		{`[{"lat": 1.0, "lng": 2.0}] + [{"lat": "north", "lng": 2.0}]`, "list<LatLng>", plxerr.PXLTypeMismatch},
		{`sent + [{"lat": 1.0, "lng": 2.0}]`, "list<LatLng>", plxerr.PXLTypeMismatch},
	} {
		_, _, diags := Compile(tc.src, env, expecting(t, env, tc.want), plxerr.Location{})
		if !hasCode(diags, tc.code) {
			t.Errorf("%s as %s: want %s, got %v", tc.src, tc.want, tc.code, diags)
		}
	}
}

// Verifies: PXL-001.
// An object literal evaluates to a map keyed by field name, the form the
// runtimes decode value types from.
func TestContextualObjectsEvaluate(t *testing.T) {
	t.Parallel()
	env := contextualEnv(t)
	prog, _, diags := Compile(`orgs.map(o, {"id": o.slug, "position": {"lat": o.lat, "lng": o.lng}, "title": o.name})`, env, expecting(t, env, "list<Marker>"), plxerr.Location{})
	if prog == nil {
		t.Fatal(diags)
	}
	got, err := prog.Eval(map[string]Value{
		"orgs": List{Map{"slug": "bank", "name": "Nordvia", "lat": 52.5, "lng": 13.4}},
		"sent": true, "maybe": nil,
	}, DefaultOptions().Limits)
	if err != nil {
		t.Fatal(err)
	}
	want := List{Map{"id": "bank", "position": Map{"lat": 52.5, "lng": 13.4}, "title": "Nordvia"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

// Verifies: PXL-001.
// An append to a list of objects, `list + [literal]`, builds the literal as
// the list's element and evaluates to both lists joined in order.
func TestContextualConcatEvaluates(t *testing.T) {
	t.Parallel()
	env := contextualEnv(t)
	src := `[{"lat": 1.0, "lng": 2.0}] + [{"lat": 3.0, "lng": 4.0}]`
	prog, typ, diags := Compile(src, env, expecting(t, env, "list<LatLng>"), plxerr.Location{})
	if prog == nil || diags.HasErrors() {
		t.Fatal(diags)
	}
	if typ.String() != "list<LatLng>" {
		t.Errorf("type %s, want list<LatLng>", typ)
	}
	got, err := prog.Eval(map[string]Value{"orgs": List{}, "sent": false, "maybe": nil}, DefaultOptions().Limits)
	if err != nil {
		t.Fatal(err)
	}
	want := List{Map{"lat": 1.0, "lng": 2.0}, Map{"lat": 3.0, "lng": 4.0}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

func hasCode(diags plxerr.Diagnostics, code plxerr.Code) bool {
	for _, d := range diags {
		if d.Code == code {
			return true
		}
	}
	return false
}
