// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package release_test

import (
	"context"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/rollout"
)

// A person advances an active rollout to its next stage at once and
// reschedules it; both are audited, refused for a viewer and invisible to
// another organisation.
//
// Verifies: REL-052, REL-053, STU-101.
func TestRolloutAdvanceAndReschedule_REL_053(t *testing.T) {
	t.Parallel()
	w := newRolloutWorld(t)
	ctx := context.Background()
	r := w.create(t, w.input(rollout.Stage{BasisPoints: 1000, DurationSeconds: 86400}, rollout.Stage{BasisPoints: 5000, DurationSeconds: 86400}, rollout.Stage{BasisPoints: 10000}))

	if _, err := w.svc.AdvanceNow(ctx, w.viewer, r.ID); err == nil {
		t.Error("a viewer advanced a rollout")
	}
	moved, err := w.svc.AdvanceNow(ctx, w.owner, r.ID)
	if err != nil || moved.StageIndex != 1 || moved.BasisPoints != 5000 {
		t.Fatalf("AdvanceNow = %+v, %v", moved, err)
	}
	if last, err := w.svc.AdvanceNow(ctx, w.owner, r.ID); err != nil || last.StageIndex != 2 || last.BasisPoints != 10000 {
		t.Fatalf("AdvanceNow to the last stage = %+v, %v", last, err)
	}
	_, err = w.svc.AdvanceNow(ctx, w.owner, r.ID)
	wantCode(t, err, plxerr.OutOfRange)
	if n := w.auditCount(t, audit.RolloutAdvanced); n != 2 {
		t.Errorf("%d advances audited", n)
	}

	end := w.now.Add(48 * time.Hour)
	if got, err := w.svc.Reschedule(ctx, w.owner, r.ID, rollout.Window{End: end}); err != nil || got.EndAt.Sub(end).Abs() > time.Millisecond {
		t.Fatalf("Reschedule = %+v, %v", got, err)
	}
	if got, err := w.svc.Reschedule(ctx, w.owner, r.ID, rollout.Window{End: end}); err != nil || got.EndAt.Sub(end).Abs() > time.Millisecond {
		t.Errorf("a repeated Reschedule = %+v, %v", got, err)
	}
	_, err = w.svc.Reschedule(ctx, w.owner, r.ID, rollout.Window{Start: w.now.Add(time.Hour)})
	wantCode(t, err, plxerr.OutOfRange)
	_, err = w.svc.Reschedule(ctx, w.owner, r.ID, rollout.Window{End: w.now.Add(-time.Hour)})
	wantCode(t, err, plxerr.OutOfRange)
	if got, err := w.svc.Reschedule(ctx, w.owner, r.ID, rollout.Window{ClearEnd: true}); err != nil || !got.EndAt.IsZero() {
		t.Errorf("clearing the end = %+v, %v", got, err)
	}
	if n := w.auditCount(t, audit.RolloutRescheduled); n != 3 {
		t.Errorf("%d reschedules audited", n)
	}

	admin := auth.Identity{Kind: auth.KindUser, ID: w.owner.UserID, UserID: w.owner.UserID, Display: "Admin", SecondFactor: true, InstallationAdmin: true}.WithStepUp(0)
	other, err := w.tenancy.CreateOrganization(ctx, admin, "other", "Other")
	if err != nil {
		t.Fatal(err)
	}
	stranger, err := w.auth.Resolve(ctx, admin, other.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.svc.AdvanceNow(ctx, stranger, r.ID); err == nil {
		t.Error("another organisation advanced a rollout")
	}
	if _, err := w.svc.Reschedule(ctx, stranger, r.ID, rollout.Window{ClearEnd: true}); err == nil {
		t.Error("another organisation rescheduled a rollout")
	}
}

// A rollout that has not started may move its start, but not into the window
// of another open rollout of the channel (PLX-3153).
//
// Verifies: REL-052.
func TestRolloutRescheduleStartAndOverlap_REL_052(t *testing.T) {
	t.Parallel()
	w := newRolloutWorld(t)
	ctx := context.Background()
	in := w.input(rollout.Stage{BasisPoints: 10000})
	in.StartAt = w.now.Add(time.Hour)
	r := w.create(t, in)
	start := w.now.Add(2 * time.Hour)
	got, err := w.svc.Reschedule(ctx, w.owner, r.ID, rollout.Window{Start: start})
	if err != nil || got.StartAt.Sub(start).Abs() > time.Millisecond || got.State != rollout.StateScheduled {
		t.Fatalf("Reschedule = %+v, %v", got, err)
	}
	_, err = w.svc.AdvanceNow(ctx, w.owner, r.ID)
	wantCode(t, err, plxerr.RevisionConflict)
	_, err = w.svc.Reschedule(ctx, w.owner, r.ID, rollout.Window{Start: start, End: start})
	wantCode(t, err, plxerr.OutOfRange)
}
