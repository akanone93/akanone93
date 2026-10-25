// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package document_test

import (
	"context"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/tenancy"
)

// TestPluginMetadataDuplicateAndSummaries covers plan P11 §4.3 #13 and #14
// (STU-034): metadata, archiving, duplication and the bounded summary.
func TestPluginMetadataDuplicateAndSummaries(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	plugin := f.plugin(t)

	desc, tags, archived := "Loan products", []string{"finance", "core"}, true
	got, err := f.docs.UpdatePluginMetadata(ctx, f.owner, plugin, "", tenancy.Metadata{Description: &desc, SetTags: true, Tags: tags, Archived: &archived})
	if err != nil || got.Name != "Loans" || got.Description != desc || len(got.Tags) != 2 || !got.Archived {
		t.Fatalf("UpdatePluginMetadata: %+v %v", got, err)
	}
	archived = false
	if got, err = f.docs.UpdatePluginMetadata(ctx, f.owner, plugin, "Lending", tenancy.Metadata{Archived: &archived}); err != nil || got.Archived || got.Name != "Lending" || got.Description != desc {
		t.Fatalf("restore keeps the other metadata: %+v %v", got, err)
	}
	if _, err := f.docs.UpdatePluginMetadata(ctx, f.developer, plugin, "", tenancy.Metadata{Description: &desc}); code(err) != plxerr.PermissionDenied {
		t.Errorf("a developer changed metadata: %v", err)
	}
	long := string(make([]byte, 3000))
	if _, err := f.docs.UpdatePluginMetadata(ctx, f.owner, plugin, "", tenancy.Metadata{Description: &long}); code(err) != plxerr.InvalidFormat {
		t.Errorf("an over-long description: %v", err)
	}

	copied, err := f.docs.DuplicatePlugin(ctx, f.owner, plugin, "loans-copy", "Loans copy", "dup-1")
	if err != nil || copied.Key != "loans-copy" || copied.DraftRevision == 0 {
		t.Fatalf("DuplicatePlugin: %+v %v", copied, err)
	}
	if _, err := f.docs.DuplicatePlugin(ctx, f.owner, plugin, "loans-copy", "Again", "dup-2"); code(err) != plxerr.ResourceExists {
		t.Errorf("a duplicate key: %v", err)
	}

	sums, err := f.docs.ListPluginSummaries(ctx, f.viewer, f.app)
	if err != nil || len(sums) != 2 {
		t.Fatalf("ListPluginSummaries: %d %v", len(sums), err)
	}
	if sums[0].Plugin.Key != "loans" || sums[1].Plugin.Key != "loans-copy" {
		t.Errorf("summaries are in key order: %s, %s", sums[0].Plugin.Key, sums[1].Plugin.Key)
	}
	if !sums[0].UnpublishedChanges || sums[0].Versions != 0 || sums[0].LockHolder != "" || sums[0].Killed {
		t.Errorf("an unpublished plugin: %+v", sums[0])
	}
	if _, _, err := f.docs.AcquireLock(ctx, f.owner, f.app, plugin, "tab-1", false); err != nil {
		t.Fatal(err)
	}
	if sums, err = f.docs.ListPluginSummaries(ctx, f.viewer, f.app); err != nil || sums[0].LockHolder == "" {
		t.Errorf("the lock holder is shown: %+v %v", sums, err)
	}
}
