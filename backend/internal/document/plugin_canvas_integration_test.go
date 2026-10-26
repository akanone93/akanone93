// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package document_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
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

// Verifies: STU-034, LIM-001.
// The owning team of a plugin must belong to the caller's organisation and
// the icon must be an asset of the plugin's app; a reference that does not
// resolve fails exactly like a missing one.
func TestPluginMetadataReferences(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	plugin := f.plugin(t)
	admin := person(f.owner.UserID, true)

	mine, err := f.tenancy.CreateTeam(ctx, f.owner, "core", "Core")
	if err != nil {
		t.Fatal(err)
	}
	other, err := f.tenancy.CreateOrganization(ctx, admin, "other", "Other")
	if err != nil {
		t.Fatal(err)
	}
	otherOwner, err := f.auth.Resolve(ctx, admin, other.ID)
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := f.tenancy.CreateTeam(ctx, otherOwner, "spies", "Spies")
	if err != nil {
		t.Fatal(err)
	}

	if got, err := f.docs.UpdatePluginMetadata(ctx, f.owner, plugin, "", tenancy.Metadata{OwningTeamID: &mine.ID}); err != nil || got.OwningTeamID != mine.ID {
		t.Fatalf("own team: %+v %v", got, err)
	}
	missing := "0190b3a0-0000-7000-8000-0000000000ff"
	_, errForeign := f.docs.UpdatePluginMetadata(ctx, f.owner, plugin, "", tenancy.Metadata{OwningTeamID: &foreign.ID})
	_, errMissing := f.docs.UpdatePluginMetadata(ctx, f.owner, plugin, "", tenancy.Metadata{OwningTeamID: &missing})
	if code(errForeign) != plxerr.ResourceNotFound || code(errMissing) != plxerr.ResourceNotFound || errForeign.Error() != errMissing.Error() {
		t.Errorf("another organisation's team must read as missing: %v | %v", errForeign, errMissing)
	}

	const icon = "0190b3a0-0000-7000-8000-0000000000a1"
	err = f.db.InTx(ctx, storage.Tenant{OrganizationID: f.org, UserID: f.owner.UserID}, func(ctx context.Context, tx pgx.Tx) error {
		_, err := dbgen.New(tx).InsertAsset(ctx, dbgen.InsertAssetParams{
			ID: storage.MustUUID("0190b3a0-0000-7000-8000-000000000001"), AssetID: storage.MustUUID(icon),
			OrganizationID: storage.MustUUID(f.org), AppID: storage.MustUUID(f.app), File: "images/icon.png",
			MediaType: "image/png", Sha256: make([]byte, 32), Size: 1, Processing: "ready",
			UploadedByKind: "user", UploadedByID: f.owner.UserID, UploadedBy: "owner",
		})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := f.docs.UpdatePluginMetadata(ctx, f.owner, plugin, "", tenancy.Metadata{IconAssetID: &[]string{icon}[0]}); err != nil || got.IconAssetID != icon {
		t.Fatalf("own asset as icon: %+v %v", got, err)
	}
	if _, err := f.docs.UpdatePluginMetadata(ctx, f.owner, plugin, "", tenancy.Metadata{IconAssetID: &missing}); code(err) != plxerr.ResourceNotFound {
		t.Errorf("an unknown asset as icon: %v", err)
	}
}
