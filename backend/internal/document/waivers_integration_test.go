// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package document_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/document"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

const waiverPage = "plugins/loans/pages/calculator.page.json"

// pageWithWaiver returns the stored calculator page with an Image node
// carrying the waiver, or without a waiver when it is empty.
func pageWithWaiver(ctx context.Context, t *testing.T, f *fixture, pluginID, waiver string) ([]byte, int64) {
	t.Helper()
	page, err := f.docs.GetDocument(ctx, f.owner, f.app, pluginID, waiverPage)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(page.Content, &doc); err != nil {
		t.Fatal(err)
	}
	column := doc["root"].(map[string]any)["slots"].(map[string]any)["body"].(map[string]any)["slots"].(map[string]any)["child"].(map[string]any)
	var kept []any
	for _, c := range column["children"].([]any) {
		if c.(map[string]any)["id"] != "01f0c450-6c00-7000-8000-0000000a11d1" {
			kept = append(kept, c)
		}
	}
	node := map[string]any{
		"id": "01f0c450-6c00-7000-8000-0000000a11d1", "type": "Image",
		"props": map[string]any{"source": map[string]any{"asset": map[string]any{"$asset": "01a0c450-6c00-700d-8000-000000019223"}}},
	}
	if waiver != "" {
		node["a11yWaivers"] = map[string]any{"imageAlternative": waiver}
	}
	column["children"] = append(kept, node)
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return out, page.Revision
}

// Verifies: A11Y-002.
// Under the maximum profile a save that adds or changes an a11yWaivers
// entry needs a11y.waive with a second factor (PLX-1411); resaving the
// same waiver, or removing it, needs nothing. Under the strict profile
// the permission is not asked for.
func TestWaiverChangeNeedsPermissionUnderMaximum_A11Y_002(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newFixture(t)
	if _, err := f.docs.Import(ctx, f.owner, f.app, "", "s", project(t, "loan-calculator")); err != nil {
		t.Fatal(err)
	}
	plugins, err := f.docs.ListPlugins(ctx, f.owner, f.app, storage.Cursor{}, 10)
	if err != nil || len(plugins) != 1 {
		t.Fatalf("plugins: %v %v", plugins, err)
	}
	loans := plugins[0].ID
	setProfile := func(profile string) {
		t.Helper()
		app, err := f.docs.GetDocument(ctx, f.owner, f.app, "", "app.json")
		if err != nil {
			t.Fatal(err)
		}
		var doc map[string]any
		if err := json.Unmarshal(app.Content, &doc); err != nil {
			t.Fatal(err)
		}
		doc["securityProfile"] = profile
		content, err := json.Marshal(doc)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.docs.PutDocument(ctx, f.owner, f.app, "", "s", "app.json", content, app.Revision); err != nil {
			t.Fatalf("set the profile %s: %v", profile, err)
		}
	}
	unverified := f.owner
	unverified.SecondFactor = false

	// Strict: anyone who may edit may waive.
	setProfile("strict")
	content, rev := pageWithWaiver(ctx, t, f, loans, "Decorative picture, one.")
	if _, err := f.docs.PutDocument(ctx, unverified, f.app, loans, "s", waiverPage, content, rev); err != nil {
		t.Fatalf("a waiver under strict: %v", err)
	}

	setProfile("maximum")
	// The same waiver again changes nothing.
	same, rev := pageWithWaiver(ctx, t, f, loans, "Decorative picture, one.")
	if _, err := f.docs.PutDocument(ctx, unverified, f.app, loans, "s", waiverPage, same, rev); err != nil {
		t.Fatalf("an unchanged waiver under maximum: %v", err)
	}
	// A changed one needs the permission and the second factor.
	changed, rev := pageWithWaiver(ctx, t, f, loans, "Decorative picture, two.")
	if _, err := f.docs.PutDocument(ctx, unverified, f.app, loans, "s", waiverPage, changed, rev); code(err) != plxerr.A11yWaiverPermissionRequired {
		t.Fatalf("a changed waiver without a second factor: %v", err)
	}
	developer := f.developer
	if _, err := f.docs.PutDocument(ctx, developer, f.app, loans, "dev", waiverPage, changed, rev); code(err) != plxerr.A11yWaiverPermissionRequired {
		t.Fatalf("a changed waiver by a developer: %v", err)
	}
	ops := []document.Op{{Op: "replace", Path: "/root/slots/body/slots/child/children/6/a11yWaivers/imageAlternative", Value: "Decorative picture, three."}}
	if _, err := f.docs.PatchDocument(ctx, unverified, f.app, loans, "s", waiverPage, ops, rev); code(err) != plxerr.A11yWaiverPermissionRequired {
		t.Fatalf("a patched waiver without a second factor: %v", err)
	}
	if _, err := f.docs.PutDocument(ctx, f.owner, f.app, loans, "s", waiverPage, changed, rev); err != nil {
		t.Fatalf("a changed waiver by the owner: %v", err)
	}
	// Removing a waiver needs nothing.
	removed, rev := pageWithWaiver(ctx, t, f, loans, "")
	if _, err := f.docs.PutDocument(ctx, unverified, f.app, loans, "s", waiverPage, removed, rev); err != nil {
		t.Fatalf("removing a waiver: %v", err)
	}
}
