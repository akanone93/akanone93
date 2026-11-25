// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package release_test

import (
	"context"
	"testing"
)

// Verifies: GEN-001, SYN-007.
// A channel's release is read for the project download with its bundles
// and their signatures; a channel without a release, or that does not
// exist, has sequence 0 (build log row 294).
func TestChannelRelease(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	prod := f.envs["production"]
	if cr, err := f.rel.ChannelRelease(ctx, f.org, prod.ID, "production"); err != nil || cr.Sequence != 0 || len(cr.Versions) != 0 {
		t.Fatalf("before a release: %+v %v", cr, err)
	}
	f.publish(t, "", false)
	f.publish(t, f.loans, false)
	first, _, err := f.rel.CreateRelease(ctx, f.owner, f.app, f.envs["development"].ID, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.rel.PromoteRelease(ctx, f.owner, f.app, first.Sequence, prod.ID, ""); err != nil {
		t.Fatal(err)
	}
	cr, err := f.rel.ChannelRelease(ctx, f.org, prod.ID, "production")
	if err != nil || cr.Sequence != first.Sequence || cr.Confidential || len(cr.Versions) != 2 {
		t.Fatalf("after a promotion: %+v %v", cr, err)
	}
	for _, v := range cr.Versions {
		if len(v.BundleSHA256) != 64 || len(v.Signature) == 0 || v.KeyID == "" {
			t.Errorf("a version without its hash or signature: %+v", v)
		}
	}
	if cr, err := f.rel.ChannelRelease(ctx, f.org, prod.ID, "beta"); err != nil || cr.Sequence != 0 {
		t.Errorf("a channel that does not exist: %+v %v", cr, err)
	}
}
