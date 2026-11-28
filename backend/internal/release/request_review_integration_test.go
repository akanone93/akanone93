// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package release_test

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/approval"
	"github.com/nightCode42/plux3/backend/internal/releasereq"
	"github.com/nightCode42/plux3/backend/internal/security/settings"
)

// Verifies: GOV-023.
// A release request's own approval can be reviewed (build log row 311): the
// request sent to production waits for the standard profile's approver, and
// the review bundle shows what its builds produced — each part's version and
// bundle hash, one changelog line each — and its release notes. The review
// used to be refused (PLX-8033) because the release reviewer read every
// subject as a promotion.
func TestReleaseRequestApprovalCanBeReviewed_GOV_023(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	g := newGuarded(t, settings.Standard)
	k := g.requestsWith(t, 1, g.engine)
	r := toDiffsStep(t, g.fixture, k, "production", 2)
	var err error
	for _, step := range []releasereq.Step{
		{Kind: releasereq.StateDiffs},
		{Kind: releasereq.StateChangelog, Notes: "Fix the total"},
		{Kind: releasereq.StateTarget, EnvironmentID: g.envs["production"].ID, Channel: "production"},
	} {
		if r, err = k.svc.Advance(ctx, g.owner, g.app, r.ID, step); err != nil {
			t.Fatalf("%s: %v", step.Kind, err)
		}
	}
	if r, err = k.svc.RequestApproval(ctx, g.owner, g.app, r.ID); err != nil || r.Hold != releasereq.HoldAwaiting || r.ApprovalRequestID == "" {
		t.Fatalf("approval in production = %+v, %v; want it awaited", r, err)
	}

	review, err := g.engine.Review(ctx, g.viewer, g.app, r.ApprovalRequestID)
	if err != nil {
		t.Fatalf("Review of the request's approval: %v", err)
	}
	var parts []struct {
		PluginKey    string `json:"pluginKey"`
		Version      int64  `json:"version"`
		BundleSHA256 string `json:"bundleSha256"`
	}
	if err := json.Unmarshal(review.Evidence.Diff, &parts); err != nil {
		t.Fatalf("the diff %s: %v", review.Evidence.Diff, err)
	}
	if len(parts) != 1 || parts[0].PluginKey != "loans" || parts[0].Version != 2 || parts[0].BundleSHA256 != "aa" {
		t.Errorf("parts = %+v, want loans version 2 with its bundle hash", parts)
	}
	if !slices.Equal(review.Evidence.Changelog, []string{"loans version 2"}) || review.Evidence.Notes != "Fix the total" {
		t.Errorf("changelog %q, notes %q", review.Evidence.Changelog, review.Evidence.Notes)
	}
	if review.Request.Subject != "releaseRequest:"+r.ID || review.Request.Status != approval.StatusOpen {
		t.Errorf("request = %+v", review.Request)
	}
}
