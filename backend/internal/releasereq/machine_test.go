// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package releasereq

import (
	"errors"
	"slices"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// Verifies: STU-100.
// The steps run in the order of F11, the last step has no successor, and an
// end state is not a step.
func TestStatesFollowTheFlow_STU_100(t *testing.T) {
	t.Parallel()
	state, order := StateSelect, []State{StateSelect}
	for {
		next, ok := state.Next()
		if !ok {
			break
		}
		order = append(order, next)
		state = next
	}
	want := []State{StateSelect, StateBuild, StateReports, StateDiffs, StateChangelog, StateTarget, StateApproval, StateRollout, StateRelease}
	if !slices.Equal(order, want) {
		t.Fatalf("steps = %v, want %v", order, want)
	}
	for _, end := range []State{StateReleased, StateRejected, StateExpired, StateCancelled} {
		if end.Open() || !end.Valid() {
			t.Errorf("%s: Open = %v, Valid = %v", end, end.Open(), end.Valid())
		}
		if _, ok := end.Next(); ok {
			t.Errorf("%s has a next step", end)
		}
	}
	if State("bogus").Valid() {
		t.Error("an unknown state is valid")
	}
	if !StateReports.Passed(StateBuild) || StateBuild.Passed(StateReports) || StateBuild.Passed(StateBuild) {
		t.Error("Passed does not order the steps")
	}
}

// Verifies: GOV-022.
// The content hash changes with any bundle and not with the order the builds
// came in, so an approval is bound to exactly the content it was given for.
func TestContentHashIsBoundToTheBundles_GOV_022(t *testing.T) {
	t.Parallel()
	a := Build{PluginKey: "a", BundleSHA256: "aa"}
	b := Build{PluginKey: "b", BundleSHA256: "bb"}
	if contentHash([]Build{a, b}) != contentHash([]Build{b, a}) {
		t.Error("the content hash depends on the order of the builds")
	}
	if contentHash([]Build{a, b}) == contentHash([]Build{a, {PluginKey: "b", BundleSHA256: "cc"}}) {
		t.Error("a changed bundle left the content hash unchanged")
	}
	r := Request{State: StateRollout, ApprovedHash: "h", ApprovalRequestID: "x", ContentHash: "h"}
	r.backToBuild()
	if r.State != StateBuild || r.ApprovedHash != "" || r.ApprovalRequestID != "" || r.ContentHash != "" {
		t.Errorf("backToBuild left %+v", r)
	}
}

// Verifies: SRV-051, STU-100.
// The reports step is blocked by an error and needs every warning
// acknowledged.
func TestReportsStep_SRV_051(t *testing.T) {
	t.Parallel()
	blocked := Request{State: StateReports, Builds: []Build{{PluginKey: "a", Errors: 1}}}
	if err := blocked.acknowledge(Step{}); err != nil || blocked.Hold != HoldBlocked || blocked.State != StateReports {
		t.Errorf("errors: %v, %+v", err, blocked)
	}
	warned := Request{State: StateReports, Builds: []Build{{PluginKey: "a", Warnings: []string{"w1", "w2"}}}}
	if err := warned.acknowledge(Step{AcknowledgedWarnings: []string{"w1"}}); !errors.Is(err, plxerr.New(plxerr.WarningsNotAcknowledged, "")) {
		t.Errorf("an unacknowledged warning: %v", err)
	}
	if err := warned.acknowledge(Step{AcknowledgedWarnings: []string{"w2", "w1"}}); err != nil || warned.State != StateDiffs || !slices.Equal(warned.Acknowledged, []string{"w1", "w2"}) {
		t.Errorf("all acknowledged: %v, %+v", err, warned)
	}
}
