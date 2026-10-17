// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package literals

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/schema/registry"
)

// decode reads a node document.
func decode(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// paths lists the paths of literals as slash-joined strings.
func paths(ls []Literal) []string {
	out := make([]string, len(ls))
	for i, l := range ls {
		out[i] = strings.Join(l.Path, "/") + "=" + l.Text
	}
	return out
}

// Verifies: I18N-007.
func TestNodeFindsLiteralsInPropsFieldsAndLists(t *testing.T) {
	t.Parallel()
	n := decode(t, `{"type": "TextFormField", "props": {
		"decoration": {"labelText": "Name", "helperText": {"$t": "x"}, "hintText": "e.g. Ada", "prefixText": "$"},
		"initialValue": "Ada", "obscuringCharacter": "*"}}`)
	got := paths(Node(n))
	want := []string{"props/decoration/labelText=Name", "props/decoration/hintText=e.g. Ada"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("literals = %v, want %v", got, want)
	}
	n = decode(t, `{"type": "PopupMenuButton", "props": {
		"items": [{"value": "a", "label": "Open"}, {"value": "b", "label": {"$expr": "x"}}, {"value": "c", "label": "Close"}],
		"tooltip": "More"}}`)
	got = paths(Node(n))
	want = []string{"props/items/0/label=Open", "props/items/2/label=Close", "props/tooltip=More"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("literals = %v, want %v", got, want)
	}
}

// Verifies: I18N-007.
func TestNodeFindsSemanticsAndSkipsNonText(t *testing.T) {
	t.Parallel()
	n := decode(t, `{"type": "Text", "props": {"data": "123", "locale": "en"}, "semantics": {"label": "Heading", "hint": {"$t": "k"}}}`)
	got := Node(n)
	if len(got) != 1 || got[0].Prop != "semantics.label" || got[0].Text != "Heading" {
		t.Errorf("literals = %+v, want only the semantics label", got)
	}
	if got := Node(decode(t, `{"type": "NoSuchWidget", "props": {"data": "Hello"}}`)); len(got) != 0 {
		t.Errorf("an unknown widget has literals: %+v", got)
	}
	if got := Node(decode(t, `{"component": {"id": "x"}, "props": {"label": "Hello"}}`)); len(got) != 0 {
		t.Errorf("a component instance has literals: %+v", got)
	}
}

// Verifies: I18N-007.
func TestTranslatable(t *testing.T) {
	t.Parallel()
	for s, want := range map[string]bool{"Hello": true, "Größe": true, "ሰላም": true, "": false, "-": false, "$12.50": false, "  ": false} {
		if Translatable(s) != want {
			t.Errorf("Translatable(%q) = %t", s, !want)
		}
	}
}

// Every prop and field marked translatable is a string, a list of them or a
// value type holding some; a mark anywhere else would never be read.
//
// Verifies: I18N-007.
func TestMarksSitOnTextProps(t *testing.T) {
	t.Parallel()
	for _, w := range registry.Widgets() {
		for _, p := range w.Props {
			if p.Translatable && strings.TrimSuffix(p.Type, "?") != "string" {
				t.Errorf("%s.%s is marked translatable but is a %s", w.Type, p.Name, p.Type)
			}
		}
	}
	for _, v := range registry.ValueTypes() {
		for _, f := range v.Fields {
			if f.Translatable && strings.TrimSuffix(f.Type, "?") != "string" {
				t.Errorf("%s.%s is marked translatable but is a %s", v.Name, f.Name, f.Type)
			}
		}
	}
}
