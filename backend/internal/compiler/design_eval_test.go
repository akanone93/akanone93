// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	"encoding/json"
	"strings"
	"testing"
)

// designAnswer evaluates a request and decodes its results.
func designAnswer(t *testing.T, d *DesignEvaluator, v *Validator, req map[string]any) []map[string]any {
	t.Helper()
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Results []map[string]any `json:"results"`
		Error   string           `json:"error"`
	}
	if err := json.Unmarshal(d.Evaluate(v, data), &out); err != nil || out.Error != "" {
		t.Fatalf("answer: %v %s", err, out.Error)
	}
	return out.Results
}

// Verifies: STU-005 (ADR-0112).
// The canvas evaluates a page's expressions as its check compiled them, with
// design-time values read leniently, the locale's formats and messages, and
// a list item's values over its page's.
func TestDesignEvaluate(t *testing.T) {
	t.Parallel()
	v := newValidator(t, fixture(t))
	data := pageEdit(t, func(doc map[string]any) {
		props := at(t, doc, sliderNode)["props"].(map[string]any)
		props["label"] = map[string]any{"$expr": "format.number(page.months * 1000)"}
		props["semanticLabel"] = map[string]any{"$expr": "t('km', {'n': string(page.months)})"}
	})
	d := NewDesignEvaluator()
	page := map[string]any{"file": calculatorPage, "data": json.RawMessage(data)}
	items := []map[string]any{
		{"page": 0, "values": 0, "src": "string(page.amount)"},
		{"page": 0, "values": 0, "src": "double(page.months)"},
		{"page": 0, "values": 1, "src": "double(page.months)"},
		{"page": 0, "values": 0, "src": "format.number(page.months * 1000)"},
		{"page": 0, "values": 0, "src": "t('km', {'n': string(page.months)})"},
		// Not on the page, and naming no page.
		{"page": 0, "values": 0, "src": "page.months + 1"},
		{"page": 1, "values": 0, "src": "1"},
	}
	got := designAnswer(t, d, v, map[string]any{
		"locale":   "de",
		"messages": map[string]string{"km": "{n} km"},
		"pages":    []any{page},
		// The page state lacks months in the first set; the second extends it.
		"values": []any{
			map[string]any{"roots": map[string]any{"page": map[string]any{"amount": "1250.50", "months": nil}}},
			map[string]any{"base": 0, "roots": map[string]any{"page": map[string]any{"amount": "1", "months": 7}}},
		},
		"items": items,
	})
	want := []any{"1250.50", 0.0, 7.0, "0", "0 km"}
	for i, w := range want {
		if got[i]["value"] != w {
			t.Errorf("%s: got %v, want %v", items[i]["src"], got[i], w)
		}
	}
	for _, i := range []int{5, 6} {
		if got[i]["error"] == nil {
			t.Errorf("%s: got %v, want an error", items[i]["src"], got[i])
		}
	}
	// A later batch keeps the messages and the page's programs; the locale formats.
	again := designAnswer(t, d, v, map[string]any{
		"locale": "de",
		"pages":  []any{page},
		"values": []any{map[string]any{"roots": map[string]any{"page": map[string]any{"months": 12}}}},
		"items": []map[string]any{
			{"page": 0, "values": 0, "src": "format.number(page.months * 1000)"},
			{"page": 0, "values": 0, "src": "t('km', {'n': string(page.months)})"},
		},
	})
	if again[0]["value"] != "12.000" || again[1]["value"] != "12 km" {
		t.Errorf("got %v", again)
	}
	if len(d.pages) != 1 {
		t.Errorf("the evaluator holds %d pages, want 1", len(d.pages))
	}
}

// Verifies: STU-005 (ADR-0112).
// A malformed request or a page that does not check answers errors, never a
// panic.
func TestDesignEvaluateFailsSafely(t *testing.T) {
	t.Parallel()
	v := newValidator(t, fixture(t))
	d := NewDesignEvaluator()
	if got := string(d.Evaluate(v, []byte(`{`))); !strings.HasPrefix(got, `{"error":`) {
		t.Errorf("got %s", got)
	}
	got := designAnswer(t, d, v, map[string]any{
		"pages":  []any{map[string]any{"file": calculatorPage, "data": json.RawMessage(`{}`)}},
		"values": []any{map[string]any{"roots": map[string]any{}}},
		"items":  []map[string]any{{"page": 0, "values": 0, "src": "1"}, {"page": 0, "values": 3, "src": "1"}},
	})
	for i := range got {
		if got[i]["error"] == nil {
			t.Errorf("item %d: got %v, want an error", i, got[i])
		}
	}
}
