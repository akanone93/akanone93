// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package document_test

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

// Verifies: STU-034, SCH-006, SCH-025.
// Creating, duplicating, deleting and restoring a plugin keeps the app document's plugins list equal to the live
// plugins, so the app never fails the layout check (PLX-1020) because of a change made in Studio. A duplicate's pages
// get routes of their own and its links stay inside the copy, so it clashes with nothing (PLX-1103).
func TestPluginLifecycleKeepsTheAppValid(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.docs.Import(ctx, f.owner, f.app, "", "s", project(t, "loan-calculator")); err != nil {
		t.Fatal(err)
	}
	listed := func() []string {
		t.Helper()
		app, err := f.docs.GetDocument(ctx, f.owner, f.app, "", "app.json")
		if err != nil {
			t.Fatal(err)
		}
		var doc struct {
			Plugins []string `json:"plugins"`
		}
		if err := json.Unmarshal(app.Content, &doc); err != nil {
			t.Fatal(err)
		}
		return doc.Plugins
	}
	layout := func(when string) {
		t.Helper()
		diags, err := f.docs.ValidateDraft(ctx, f.owner, f.app, "")
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range diags {
			if d.Code == plxerr.InvalidProjectLayout || d.Code == plxerr.DuplicateRouteName {
				t.Errorf("%s: %s %s %s", when, d.Code, d.File, d.Message)
			}
		}
	}
	plugins, err := f.docs.ListPlugins(ctx, f.owner, f.app, storage.Cursor{}, 10)
	if err != nil || len(plugins) != 1 {
		t.Fatalf("plugins: %v %v", plugins, err)
	}
	source := plugins[0]

	created, err := f.docs.CreateStarterPlugin(ctx, f.owner, f.app, "cards", "Cards")
	if err != nil {
		t.Fatal(err)
	}
	if got := listed(); !slices.Equal(got, []string{"loans", "cards"}) {
		t.Errorf("after create: %v", got)
	}
	starter, err := f.docs.ListDocuments(ctx, f.owner, f.app, created.ID, "", 200, true)
	if err != nil || len(starter) != 2 {
		t.Fatalf("a new plugin brings its manifest and first screen: %d %v", len(starter), err)
	}
	for _, d := range starter {
		if d.Path == "plugins/cards/pages/home.page.json" && !strings.Contains(string(d.Content), `"route":"cards"`) {
			t.Errorf("the first screen's route is the plugin key: %s", d.Content)
		}
	}
	layout("after create")

	copied, err := f.docs.DuplicatePlugin(ctx, f.owner, source.ID, "loans-two", "Loans two", "dup")
	if err != nil {
		t.Fatal(err)
	}
	if got := listed(); !slices.Equal(got, []string{"loans", "loans-two", "cards"}) {
		t.Errorf("after duplicate (next to its source): %v", got)
	}
	routes := map[string]string{}
	var navigates []string
	docs, err := f.docs.ListDocuments(ctx, f.owner, f.app, copied.ID, "", 200, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range docs {
		var doc map[string]any
		if err := json.Unmarshal(d.Content, &doc); err != nil {
			t.Fatal(err)
		}
		if doc["kind"] == "page" {
			r, _ := doc["route"].(string)
			routes[doc["key"].(string)] = r
		}
		navigates = append(navigates, routesOpened(doc)...)
	}
	if routes["calculator"] != "loan-calculator-copy" || routes["result"] != "result-copy" {
		t.Errorf("copied routes: %v", routes)
	}
	if !slices.Equal(navigates, []string{"result-copy"}) {
		t.Errorf("the copy's links must open the copy's pages: %v", navigates)
	}
	layout("after duplicate")

	again, err := f.docs.DuplicatePlugin(ctx, f.owner, source.ID, "loans-three", "Loans three", "dup-2")
	if err != nil {
		t.Fatal(err)
	}
	docs, _ = f.docs.ListDocuments(ctx, f.owner, f.app, again.ID, "", 200, true)
	for _, d := range docs {
		var doc map[string]any
		_ = json.Unmarshal(d.Content, &doc)
		if doc["kind"] == "page" && doc["key"] == "result" && doc["route"] != "result-copy-2" {
			t.Errorf("a second copy takes the next free route: %v", doc["route"])
		}
	}
	layout("after a second duplicate")

	item, err := f.docs.DeletePlugin(ctx, f.owner, copied.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := listed(); !slices.Equal(got, []string{"loans", "loans-three", "cards"}) {
		t.Errorf("after delete: %v", got)
	}
	layout("after delete")
	if err := f.tenancy.RestoreFromTrash(ctx, f.owner, item.ID); err != nil {
		t.Fatal(err)
	}
	if got := listed(); !slices.Equal(got, []string{"loans", "loans-three", "cards", "loans-two"}) {
		t.Errorf("after restore: %v", got)
	}
	layout("after restore")
	if _, err := f.docs.DeletePlugin(ctx, f.owner, created.ID); err != nil {
		t.Fatal(err)
	}
	if got := listed(); slices.Contains(got, "cards") {
		t.Errorf("a deleted plugin is still listed: %v", got)
	}
}

// routesOpened lists the routes a document's navigate, openDialog and openBottomSheet actions open.
func routesOpened(v any) []string {
	var out []string
	switch t := v.(type) {
	case map[string]any:
		if a, _ := t["action"].(string); a == "navigate" || a == "openDialog" || a == "openBottomSheet" {
			if in, ok := t["input"].(map[string]any); ok {
				if r, ok := in["route"].(string); ok {
					out = append(out, r)
				}
			}
		}
		for _, k := range slices.Sorted(func(yield func(string) bool) {
			for k := range t {
				if !yield(k) {
					return
				}
			}
		}) {
			out = append(out, routesOpened(t[k])...)
		}
	case []any:
		for _, c := range t {
			out = append(out, routesOpened(c)...)
		}
	}
	return out
}
