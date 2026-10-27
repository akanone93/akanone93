// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Package releasereq holds release requests, the server-side state of
// Studio's publish wizard (STU-100, ADR-0097): the nine steps of the publish
// flow as a state machine, the build, report and approval results kept with
// the request, and the queued visual-diff render job. The wizard only
// displays what is stored here, so closing the tab loses nothing.
//
// An approval is bound to the content hash of the builds (GOV-022). The
// approval engine evaluates the policies (ADR-0076); a request never
// duplicates them.
package releasereq

// State is the step a request is at, or how it ended.
type State string

// The steps of the publish flow, in order (F11), and the end states.
const (
	StateSelect    State = "select"
	StateBuild     State = "build"
	StateReports   State = "reports"
	StateDiffs     State = "diffs"
	StateChangelog State = "changelog"
	StateTarget    State = "target"
	StateApproval  State = "approval"
	StateRollout   State = "rollout"
	StateRelease   State = "release"

	StateReleased  State = "released"
	StateRejected  State = "rejected"
	StateExpired   State = "expired"
	StateCancelled State = "cancelled"
)

// Hold says why an open request waits at its step.
type Hold string

// The holds. A failed or blocked request returns to the step that failed
// once the cause is fixed (F11).
const (
	HoldNone     Hold = ""
	HoldFailed   Hold = "failed"
	HoldBlocked  Hold = "blocked"
	HoldAwaiting Hold = "awaiting_approval"
)

var steps = []State{
	StateSelect, StateBuild, StateReports, StateDiffs, StateChangelog,
	StateTarget, StateApproval, StateRollout, StateRelease,
}

// position is the index of a step, or -1 for an end state.
func (s State) position() int {
	for i, step := range steps {
		if step == s {
			return i
		}
	}
	return -1
}

// Open reports whether the request is still at a step.
func (s State) Open() bool { return s.position() >= 0 }

// Valid reports whether s is a state of the machine.
func (s State) Valid() bool {
	switch s {
	case StateReleased, StateRejected, StateExpired, StateCancelled:
		return true
	}
	return s.Open()
}

// Next is the step after s; it is false at the last step and for an end state.
func (s State) Next() (State, bool) {
	i := s.position()
	if i < 0 || i+1 >= len(steps) {
		return "", false
	}
	return steps[i+1], true
}

// Passed reports whether the request has moved beyond step: it is at a later
// step. A repeated call that completes a step already passed is a replay.
func (s State) Passed(step State) bool {
	return s.position() > step.position() && step.position() >= 0
}
