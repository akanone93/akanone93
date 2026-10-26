// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package pxl

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// serviceEnv is the use site of the tests: a loan page, a list of loans and
// an enum.
func serviceEnv(t *testing.T) *Service {
	t.Helper()
	env, err := NewEnv(EnvSpec{
		Types: map[string]TypeSpec{
			"Loan": {Fields: map[string]string{"amount": "decimal", "months": "int", "status": "string?"}},
			"Tier": {Enum: []string{"gold", "silver"}},
		},
		Roots: map[string]string{"page": "Loan", "items": "list<Loan>", "tier": "Tier", "name": "string?"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return NewService(env, DefaultOptions())
}

// TestServiceConformance runs every conformance vector against the language
// service: the diagnostics with their ranges, or the result type.
//
// Verifies: PXL-007, PXL-008.
func TestServiceConformance(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(vectorDir, "*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no vectors in %s: %v", vectorDir, err)
	}
	for _, path := range files {
		t.Run(filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path) //nolint:gosec // G304: test vectors under the repository root.
			if err != nil {
				t.Fatal(err)
			}
			var f vectorFile
			if err := decodeJSON(data, &f); err != nil {
				t.Fatal(err)
			}
			env, err := NewEnv(f.Env)
			if err != nil {
				t.Fatal(err)
			}
			opts := DefaultOptions()
			opts.Limits = f.limits()
			svc := NewService(env, opts)
			for _, c := range f.Cases {
				got := svc.Check(c.Expr)
				var errs []vectorDiag
				for _, d := range got.Diagnostics {
					if d.Severity == "error" {
						errs = append(errs, vectorDiag{Code: d.Code, Range: [2]int{d.Start, d.End}})
					}
				}
				if len(c.Diagnostics) > 0 {
					if !slices.Equal(errs, c.Diagnostics) {
						t.Errorf("%s: %q: diagnostics %v, want %v", c.Name, c.Expr, errs, c.Diagnostics)
					}
					continue
				}
				if len(errs) > 0 || got.Type != c.Type {
					t.Errorf("%s: %q: type %q with %v, want %q", c.Name, c.Expr, got.Type, errs, c.Type)
				}
			}
		})
	}
}

func TestServiceHover(t *testing.T) {
	svc := serviceEnv(t)
	cases := []struct {
		name, src string
		offset    int
		typ       string
		start     int
		end       int
		sigs      bool
	}{
		{name: "root", src: "page.months + 1", offset: 2, typ: "Loan", start: 0, end: 4},
		{name: "field", src: "page.months + 1", offset: 6, typ: "int", start: 0, end: 11},
		{name: "whole", src: "page.months + 1", offset: 14, typ: "int", start: 14, end: 15},
		{name: "function name", src: "len(name ?? \"\")", offset: 1, typ: "int", start: 0, end: 3, sigs: true},
		{name: "macro variable", src: "items.map(x, x.months)", offset: 13, typ: "Loan", start: 13, end: 14},
		{name: "after an error", src: "page.nope + len(name ?? \"a\")", offset: 18, typ: "string?", start: 16, end: 20},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h, ok := svc.Hover(c.src, c.offset)
			if !ok {
				t.Fatalf("no hover at %d", c.offset)
			}
			if h.Type != c.typ || h.Start != c.start || h.End != c.end || (len(h.Signatures) > 0) != c.sigs {
				t.Errorf("hover = %+v, want type %s [%d,%d) signatures %v", h, c.typ, c.start, c.end, c.sigs)
			}
		})
	}
	if _, ok := svc.Hover("page.", 2); ok {
		t.Error("hover on a syntax error: want none")
	}
	if _, ok := svc.Hover("page", 99); ok {
		t.Error("hover past the end: want none")
	}
}

func labels(l CompletionList) []string {
	out := make([]string, len(l.Items))
	for i, it := range l.Items {
		out[i] = it.Label
	}
	return out
}

func TestServiceComplete(t *testing.T) {
	svc := serviceEnv(t)
	t.Run("fields after a dot", func(t *testing.T) {
		l := svc.Complete("page.", 5)
		if got := labels(l); !slices.Equal(got, []string{"amount", "months", "status"}) || l.From != 5 || l.To != 5 {
			t.Errorf("got %v from %d to %d", got, l.From, l.To)
		}
	})
	t.Run("fields filter by prefix and replace the word", func(t *testing.T) {
		l := svc.Complete("1 + page.mo + 2", 11)
		if got := labels(l); !slices.Equal(got, []string{"months"}) || l.From != 9 || l.To != 11 {
			t.Errorf("got %v from %d to %d", got, l.From, l.To)
		}
	})
	t.Run("macros after a list", func(t *testing.T) {
		if got := labels(svc.Complete("items.", 6)); !slices.Equal(got, []string{"map", "filter", "any", "all", "sortBy"}) {
			t.Errorf("got %v", got)
		}
	})
	t.Run("namespace functions", func(t *testing.T) {
		got := labels(svc.Complete("format.nu", 9))
		if !slices.Contains(got, "number") {
			t.Errorf("got %v", got)
		}
	})
	t.Run("roots, functions and keywords", func(t *testing.T) {
		got := labels(svc.Complete("n", 1))
		for _, want := range []string{"name", "now", "null"} {
			if !slices.Contains(got, want) {
				t.Errorf("%q missing from %v", want, got)
			}
		}
		if slices.Contains(got, "page") {
			t.Errorf("page does not start with n: %v", got)
		}
	})
	t.Run("macro variables", func(t *testing.T) {
		l := svc.Complete("items.map(row, ro)", 17)
		if len(l.Items) == 0 || l.Items[0].Label != "row" || l.Items[0].Kind != "variable" || l.Items[0].Detail != "Loan" {
			t.Errorf("got %+v", l.Items)
		}
	})
	t.Run("an expression left open at the cursor", func(t *testing.T) {
		for _, src := range []string{"items.map(row, ro", `items.map(row, "a(" + ro`, "[items.map(row, ro"} {
			l := svc.Complete(src, len([]rune(src)))
			if len(l.Items) == 0 || l.Items[0].Label != "row" {
				t.Errorf("%q: got %v", src, labels(l))
			}
		}
	})
	t.Run("members of a macro variable", func(t *testing.T) {
		if got := labels(svc.Complete("items.map(row, row.)", 19)); !slices.Equal(got, []string{"amount", "months", "status"}) {
			t.Errorf("got %v", got)
		}
	})
	t.Run("nothing inside a string or a number", func(t *testing.T) {
		if got := svc.Complete(`"pa`, 3); len(got.Items) != 0 {
			t.Errorf("string: %v", labels(got))
		}
		if got := svc.Complete("12a", 3); len(got.Items) != 0 {
			t.Errorf("number: %v", labels(got))
		}
	})
	t.Run("offsets count code points", func(t *testing.T) {
		l := svc.Complete(`"é😀" + pa`, 9)
		if got := labels(l); !slices.Contains(got, "page") || l.From != 7 || l.To != 9 {
			t.Errorf("got %v from %d to %d", got, l.From, l.To)
		}
	})
	t.Run("an offset outside the text", func(t *testing.T) {
		if got := svc.Complete("page", 40); len(got.Items) != 0 {
			t.Errorf("got %v", labels(got))
		}
	})
}

func TestServiceSignatureHelp(t *testing.T) {
	svc := serviceEnv(t)
	t.Run("first argument", func(t *testing.T) {
		h, ok := svc.SignatureHelp("replace(", 8)
		if !ok || h.ActiveParameter != 0 || len(h.Signatures) == 0 || len(h.Signatures[0].Parameters) != 3 {
			t.Errorf("got %+v, %v", h, ok)
		}
	})
	t.Run("later argument after a nested bracket and a string", func(t *testing.T) {
		h, ok := svc.SignatureHelp(`replace(page.name ?? "a,b", ["x", "y"], `, 40)
		if !ok || h.ActiveParameter != 2 {
			t.Errorf("got %+v, %v", h, ok)
		}
	})
	t.Run("inside a list literal the call is still the function", func(t *testing.T) {
		h, ok := svc.SignatureHelp(`contains("a", [1, `, 18)
		if !ok || h.ActiveParameter != 1 && h.ActiveParameter != 0 {
			t.Errorf("got %+v, %v", h, ok)
		}
	})
	t.Run("a closed call is no longer active", func(t *testing.T) {
		if _, ok := svc.SignatureHelp("len(name ?? \"\") + ", 18); ok {
			t.Error("want none after the closing parenthesis")
		}
	})
	t.Run("macro", func(t *testing.T) {
		h, ok := svc.SignatureHelp("items.filter(x, ", 16)
		if !ok || h.ActiveParameter != 1 || h.Signatures[0].Label != macroSignatures["filter"] {
			t.Errorf("got %+v, %v", h, ok)
		}
	})
	t.Run("namespace function", func(t *testing.T) {
		h, ok := svc.SignatureHelp("format.number(", 14)
		if !ok || len(h.Signatures) < 1 || h.Signatures[0].Label[:13] != "format.number" {
			t.Errorf("got %+v, %v", h, ok)
		}
	})
	t.Run("unknown function and plain parentheses", func(t *testing.T) {
		if _, ok := svc.SignatureHelp("nope(", 5); ok {
			t.Error("unknown function")
		}
		if _, ok := svc.SignatureHelp("(1 + ", 5); ok {
			t.Error("grouping parentheses")
		}
		if _, ok := svc.SignatureHelp("len(", 99); ok {
			t.Error("offset outside the text")
		}
	})
}

func TestServiceCheckReportsRanges(t *testing.T) {
	svc := serviceEnv(t)
	a := svc.Check("page.nope")
	if len(a.Diagnostics) != 1 || a.Type != "" || a.Diagnostics[0].Severity != "error" || a.Diagnostics[0].End <= a.Diagnostics[0].Start {
		t.Errorf("got %+v", a)
	}
	if a := svc.Check("page.months + 1"); a.Type != "int" || len(a.Diagnostics) != 0 {
		t.Errorf("got %+v", a)
	}
}
