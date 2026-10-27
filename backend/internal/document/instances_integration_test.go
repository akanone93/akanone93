// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package document_test

import (
	"context"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// Verifies: STU-084, SCH-030.
// ComponentInstances finds the nodes of an app's drafts that instantiate a
// component, with the props they override, by JSON pointer, and nothing for
// another component or for a reader without access.
func TestComponentInstances_STU_084(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	plugin := f.plugin(t)
	const component = "01a0c450-6c00-7030-8000-000000000001"
	page := []byte(`{"schemaVersion":"1.0.0","kind":"page","id":"01a0c450-6c00-7012-8000-000000022cce","key":"home","pageKind":"screen","title":"Home",` +
		`"root":{"id":"01a0c450-6c00-7026-8000-00000004977a","type":"Column","children":[` +
		`{"id":"01a0c450-6c00-7026-8000-000000049781","component":{"id":"` + component + `","version":1},"props":{"title":"A","tone":"warm"}},` +
		`{"id":"01a0c450-6c00-7026-8000-000000049782","component":{"id":"01a0c450-6c00-7030-8000-000000000002","version":1},"props":{"title":"B"}}]}}`)
	if _, _, err := f.docs.AcquireLock(ctx, f.owner, f.app, plugin, "tab", false); err != nil {
		t.Fatal(err)
	}
	if _, err := f.docs.PutDocument(ctx, f.owner, f.app, plugin, "tab", "plugins/loans/pages/home.page.json", page, 0); err != nil {
		t.Fatalf("PutDocument: %v", err)
	}
	got, err := f.docs.ComponentInstances(ctx, f.owner, f.app, component)
	if err != nil || len(got) != 1 {
		t.Fatalf("ComponentInstances = %+v, %v; want one", got, err)
	}
	in := got[0]
	if in.File != "plugins/loans/pages/home.page.json" || in.Path != "/root/children/0" || len(in.Props) != 2 || in.Props[0] != "title" || in.Props[1] != "tone" {
		t.Errorf("instance = %+v", in)
	}
	if none, err := f.docs.ComponentInstances(ctx, f.owner, f.app, "01a0c450-6c00-7030-8000-0000000000ff"); err != nil || len(none) != 0 {
		t.Errorf("an unused component = %+v, %v", none, err)
	}
	if _, err := f.docs.ComponentInstances(ctx, f.viewer, f.app, component); err != nil {
		t.Errorf("a viewer, who may read the app, = %v", err)
	}
	if _, err := f.docs.ComponentInstances(ctx, f.owner, "01a0c450-6c00-7030-8000-0000000000aa", component); code(err) != plxerr.ResourceNotFound {
		t.Errorf("an unknown app = %v, want ResourceNotFound", err)
	}
}
