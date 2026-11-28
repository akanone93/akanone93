// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package release_test

import (
	"context"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/rollout"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

// sequencesIn lists the sequences of an environment's releases, newest first.
func (w *rolloutWorld) sequencesIn(t *testing.T, envID string) []int64 {
	t.Helper()
	rs, err := w.rel.ListReleases(context.Background(), w.viewer, w.app, envID, 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]int64, 0, len(rs))
	for _, r := range rs {
		out = append(out, r.Sequence)
	}
	return out
}

// Verifies: REL-004, REL-050.
// An environment's releases are those made in it and those that reached it:
// promoted to one of its channels, even after the channel moved on, or rolled
// out there (build log row 322). Releases made in development and never
// promoted are not listed for production; a channel's current release is
// listed even without a record of its promotion, as for a channel pointed
// before the record existed.
func TestEnvironmentListsTheReleasesThatReachedIt_REL_004(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := newRolloutWorld(t)
	first, second := w.first.Sequence, w.second.Sequence
	dev, staging := w.envs["development"].ID, w.envs["staging"].ID

	if got := w.sequencesIn(t, w.prod.ID); !slices.Equal(got, []int64{first}) {
		t.Fatalf("production after release %d was promoted: %v", first, got)
	}
	if got := w.sequencesIn(t, staging); len(got) != 0 {
		t.Fatalf("staging, which nothing reached: %v", got)
	}
	w.create(t, w.input(rollout.Stage{BasisPoints: 1000}))
	if got := w.sequencesIn(t, w.prod.ID); !slices.Equal(got, []int64{second, first}) {
		t.Fatalf("production with release %d rolling out: %v", second, got)
	}
	if got := w.sequencesIn(t, dev); !slices.Equal(got, []int64{second, first}) {
		t.Fatalf("development, where both were made: %v", got)
	}

	// Promoted to staging one after the other, both stay listed.
	for _, seq := range []int64{first, second} {
		if _, err := w.rel.PromoteRelease(ctx, w.owner, w.app, seq, staging, ""); err != nil {
			t.Fatal(err)
		}
	}
	if got := w.sequencesIn(t, staging); !slices.Equal(got, []int64{second, first}) {
		t.Fatalf("staging after both were promoted: %v", got)
	}

	// Without the records, as before they existed, staging still lists what
	// its channel points at now.
	if err := w.db.InTx(ctx, storage.Tenant{OrganizationID: w.org}, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "DELETE FROM channel_promotions WHERE environment_id = $1", storage.MustUUID(staging))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got := w.sequencesIn(t, staging); !slices.Equal(got, []int64{second}) {
		t.Fatalf("staging without the records: %v", got)
	}
}
