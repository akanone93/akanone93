// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package document

import (
	"encoding/json"
	"strings"
	"testing"
)

// Verifies: STU-034.
// Duplicating rewrites every identifier inside the documents, at any depth,
// deterministically, and every reference to one follows it.
func TestDuplicateRewritesEveryID(t *testing.T) {
	t.Parallel()
	const page, node, action = "01a0c450-6c00-7012-8000-000000022cce", "n-1", "act_9"
	docs := []Document{
		{Path: "plugins/a/pages/p.json", Content: []byte(`{"id":"` + page + `","root":{"id":"` + node +
			`","children":[{"id":"c2","on":{"tap":"` + action + `"}}]},"actions":[{"id":"` + action +
			`","target":"#/nodes/` + node + `"}]}`)},
		{Path: "plugins/a/plugin.json", Content: []byte(`{"id":"pl-1","entryPage":"` + page + `","pages":["` + page + `"]}`)},
	}
	renamed := collectIDs(docs, "copy")
	again := collectIDs(docs, "copy")
	for k, v := range renamed {
		if again[k] != v {
			t.Errorf("not deterministic for %s", k)
		}
	}
	if len(renamed) != 5 {
		t.Fatalf("ids found: %v", renamed)
	}
	if renamed[page] == page || len(renamed[page]) != len(page) {
		t.Errorf("a UUID stays a UUID: %s", renamed[page])
	}
	var out []string
	for _, d := range docs {
		b, err := rewriteIDs(d.Content, renamed)
		if err != nil {
			t.Fatal(err)
		}
		if !json.Valid(b) {
			t.Fatalf("invalid JSON: %s", b)
		}
		out = append(out, string(b))
	}
	all := strings.Join(out, "")
	for old := range renamed {
		if strings.Contains(all, old) {
			t.Errorf("source id %s survived: %s", old, all)
		}
	}
	for _, want := range []string{renamed[action], "#/nodes/" + renamed[node], renamed[page]} {
		if strings.Count(all, want) < 1 {
			t.Errorf("reference %s lost", want)
		}
	}
	if strings.Count(all, renamed[page]) != 3 {
		t.Errorf("page id and its two references: %s", all)
	}
}

// A short identifier is replaced as a whole string only, never inside text.
func TestRewriteIDsLeavesTextWithShortIDs(t *testing.T) {
	renamed := map[string]string{"c2": "x9"}
	out, err := rewriteIDs([]byte(`{"id":"c2","text":"step c2 of 3"}`), renamed)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != `{"id":"x9","text":"step c2 of 3"}` {
		t.Errorf("got %s", out)
	}
}
