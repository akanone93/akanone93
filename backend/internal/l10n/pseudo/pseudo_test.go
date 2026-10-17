// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package pseudo

import (
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/nightCode42/plux3/backend/internal/l10n/mf"
)

// Verifies: I18N-002, I18N-010.
func TestAccentedGolden(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ in, want string }{
		{"Hello", "[Ĥéłłó~~]"},
		{"Save changes", "[Šáṽé çĥáñĝéš~~~~~]"},
		{"Hi {name}!", "[Ĥí {name}!~~]"},
		{"{n, plural, one {# item} other {# items}}", "[{n, plural, one {# íţéḿ} other {# íţéḿš}}~~~~]"},
		{"It''s {who}", "[Íţ''š {who}~~]"},
		{"", "[]"},
	} {
		got, err := Message(Accented, c.in)
		if err != nil {
			t.Fatalf("%q: %v", c.in, err)
		}
		if got != c.want {
			t.Errorf("Message(en-XA, %q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// Verifies: I18N-002, I18N-010.
func TestRightToLeftGolden(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ in, want string }{
		{"Hello", "‮Hello‬"},
		{"Hi {name}!", "‮Hi {name}!‬"},
		{"{n, plural, one {# item} other {# items}}", "‮{n, plural, one {# item} other {# items}}‬"},
	} {
		got, err := Message(RTL, c.in)
		if err != nil {
			t.Fatalf("%q: %v", c.in, err)
		}
		if got != c.want {
			t.Errorf("Message(ar-XB, %q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// Verifies: I18N-002.
func TestSyntaxIsUntouchedAndLongerBy35Percent(t *testing.T) {
	t.Parallel()
	const in = "You have {count, plural, one {# new message} other {# new messages}} from {sender}, {when, date, short}."
	got, err := Message(Accented, in)
	if err != nil {
		t.Fatal(err)
	}
	src, err := mf.Parse(in)
	if err != nil {
		t.Fatal(err)
	}
	out, err := mf.Parse(got)
	if err != nil {
		t.Fatalf("the pseudo message %q does not parse: %v", got, err)
	}
	if !reflect.DeepEqual(args(src.Nodes), args(out.Nodes)) {
		t.Errorf("arguments changed: %v -> %v", args(src.Nodes), args(out.Nodes))
	}
	if n, m := utf8.RuneCountInString(in), utf8.RuneCountInString(got); m*100 < n*110 || m*100 > n*140 {
		t.Errorf("length %d -> %d is not longer by the padding of the literal text", n, m)
	}
	if !strings.HasPrefix(got, "[") || !strings.HasSuffix(got, "]") {
		t.Errorf("not bracketed: %q", got)
	}
}

// args lists the names of the arguments of nodes, with the selectors of
// their cases.
func args(nodes []mf.Node) []string {
	var out []string
	for i := range nodes {
		n := &nodes[i]
		if n.Name != "" {
			out = append(out, n.Name)
		}
		for _, c := range n.Cases {
			out = append(out, c.Selector)
			out = append(out, args(c.Body)...)
		}
	}
	return out
}

// Verifies: I18N-002.
func TestMessagesAreDeterministicAndSkipInvalidOnes(t *testing.T) {
	t.Parallel()
	in := map[string]string{"a": "One", "b": "{broken", "c": "Two {x}"}
	a, b := Messages(Accented, in), Messages(Accented, in)
	if !reflect.DeepEqual(a, b) {
		t.Error("not deterministic")
	}
	if _, ok := a["b"]; ok || len(a) != 2 {
		t.Errorf("messages = %v, want the invalid one left out", a)
	}
}

// Verifies: I18N-002.
func TestUnknownPseudoLocale(t *testing.T) {
	t.Parallel()
	if _, err := Message("en-XZ", "x"); err == nil {
		t.Error("an unknown tag must fail")
	}
	if !IsTag(Accented) || !IsTag(RTL) || IsTag("en") {
		t.Error("IsTag")
	}
}
