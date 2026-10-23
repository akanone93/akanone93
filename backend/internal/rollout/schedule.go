// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package rollout

import (
	"time"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// Stage is one step of a rollout's schedule: the share of devices offered the
// candidate, and how long the share holds before the next stage begins
// (REL-052).
type Stage struct {
	// BasisPoints is the share, in 1/10000 of the buckets.
	BasisPoints int `json:"basisPoints"`
	// DurationSeconds is how long the stage lasts. It is positive on every
	// stage but the last. On the last stage it is how long a full rollout
	// soaks before the candidate is promoted, and zero on a stage below
	// 100%, which holds until the rollout ends.
	DurationSeconds int64 `json:"durationSeconds"`
}

// Duration is the stage's length.
func (s Stage) Duration() time.Duration { return time.Duration(s.DurationSeconds) * time.Second }

// ValidateStages checks a schedule: at least one stage, shares from 1 to 10000
// basis points that never decrease, positive durations before the last stage,
// and no duration on a last stage that does not reach every device.
func ValidateStages(stages []Stage) error {
	if len(stages) == 0 {
		return plxerr.New(plxerr.MissingProperty, "a rollout needs at least one stage")
	}
	prev := 0
	for i, s := range stages {
		last := i == len(stages)-1
		switch {
		case s.BasisPoints < 1 || s.BasisPoints > Buckets:
			return plxerr.New(plxerr.OutOfRange, "stage %d has a share of %d basis points; use 1 to %d", i+1, s.BasisPoints, Buckets)
		case s.BasisPoints < prev:
			return plxerr.New(plxerr.OutOfRange, "stage %d lowers the share from %d to %d; a rollout only grows", i+1, prev, s.BasisPoints)
		case s.DurationSeconds < 0 || !last && s.DurationSeconds == 0:
			return plxerr.New(plxerr.OutOfRange, "stage %d needs a positive duration", i+1)
		case last && s.DurationSeconds > 0 && s.BasisPoints < Buckets:
			return plxerr.New(plxerr.OutOfRange, "the last stage holds until the rollout ends unless it reaches %d basis points; give it no duration", Buckets)
		}
		prev = s.BasisPoints
	}
	return nil
}

// Step is where a schedule stands at a moment.
type Step struct {
	// Index is the stage in force and Began when it began, computed from the
	// schedule rather than from the clock so that every scheduler agrees.
	Index int
	Began time.Time
	// Complete is set when the last stage reaches every device and has
	// soaked: the candidate is to be promoted.
	Complete bool
}

// Progress moves a schedule from the stage it is in, which began at began, to
// the stage in force at now. It skips every stage whose time has passed, so a
// scheduler that was down for hours catches up in one step.
func Progress(stages []Stage, index int, began, now time.Time) Step {
	for {
		s := stages[index]
		due := began.Add(s.Duration())
		if now.Before(due) {
			return Step{Index: index, Began: began}
		}
		if index == len(stages)-1 {
			return Step{Index: index, Began: began, Complete: s.BasisPoints == Buckets}
		}
		began, index = due, index+1
	}
}
