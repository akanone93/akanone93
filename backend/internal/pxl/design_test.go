// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package pxl

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// Verifies: STU-005 (ADR-0112: design-time values are read leniently).
func TestDesignFromJSON(t *testing.T) {
	env, err := NewEnv(EnvSpec{
		Types: map[string]TypeSpec{
			"Tone":  {Enum: []string{"calm", "loud"}},
			"Item":  {Fields: map[string]string{"n": "int", "tone": "Tone", "note": "string?", "at": "date"}},
			"State": {Fields: map[string]string{"items": "list<Item>", "total": "money", "tags": "map<string,string>"}},
		},
		Roots: map[string]string{"page": "State"},
	})
	if err != nil {
		t.Fatal(err)
	}
	typ, _ := env.Root("page")
	dec := json.NewDecoder(strings.NewReader(`{"items":[{"n":3.0,"tone":"shout","extra":1},"x"],"tags":{"a":1}}`))
	dec.UseNumber()
	var raw any
	if err := dec.Decode(&raw); err != nil {
		t.Fatal(err)
	}
	got := DesignFromJSON(typ, raw).(Map)
	items := got["items"].(List)
	first := items[0].(Map)
	// A whole double fits an int; an unknown member, a wrong shape and absent fields read as empty values.
	if first["n"] != int64(3) || first["tone"] != "calm" || first["note"] != nil || first["at"] != Date(0) {
		t.Errorf("first item %v", first)
	}
	if !reflect.DeepEqual(items[1], EmptyValue(typ.named.Fields["items"].elem)) {
		t.Errorf("second item %v", items[1])
	}
	if got["tags"].(Map)["a"] != "" {
		t.Errorf("tags %v", got["tags"])
	}
	if m := got["total"].(Money); m.Currency != designMoneyCurrency || m.Amount.String() != "0" {
		t.Errorf("total %v", m)
	}
	if DesignFromJSON(typ, "not an object") == nil {
		t.Error("a value of the wrong shape is the type's empty value, not null")
	}
}
