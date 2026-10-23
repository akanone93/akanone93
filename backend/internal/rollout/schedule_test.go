// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package rollout

import (
	"errors"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

var t0 = time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)

// campaign is 1% for an hour, 10% for a day, 50% for a day, then everyone.
func campaign() []Stage {
	return []Stage{{100, 3600}, {1000, 86400}, {5000, 86400}, {10000, 0}}
}

// Verifies: REL-052.
func TestValidateStages_REL_052(t *testing.T) {
	good := [][]Stage{
		campaign(),
		{{10000, 0}},
		{{500, 60}, {10000, 3600}},
		{{2500, 0}},
	}
	for _, s := range good {
		if err := ValidateStages(s); err != nil {
			t.Errorf("ValidateStages(%v) = %v", s, err)
		}
	}
	bad := map[string][]Stage{
		"none":                             nil,
		"zero share":                       {{0, 60}, {10000, 0}},
		"share beyond 100%":                {{10001, 0}},
		"share falls":                      {{5000, 60}, {1000, 0}},
		"no duration before last":          {{100, 0}, {10000, 0}},
		"negative duration":                {{100, -1}, {10000, 0}},
		"duration on a partial last stage": {{100, 60}, {5000, 60}},
	}
	for name, s := range bad {
		err := ValidateStages(s)
		var pe *plxerr.Error
		if !errors.As(err, &pe) || (pe.Code != plxerr.OutOfRange && pe.Code != plxerr.MissingProperty) {
			t.Errorf("%s: ValidateStages = %v", name, err)
		}
	}
}

// Verifies: REL-052.
func TestProgress_REL_052(t *testing.T) {
	stages := campaign()
	h := time.Hour
	tests := []struct {
		name     string
		index    int
		now      time.Duration
		want     int
		began    time.Duration
		complete bool
	}{
		{"before the first stage ends", 0, 30 * time.Minute, 0, 0, false},
		{"exactly at the first stage's end", 0, h, 1, h, false},
		{"inside the second stage", 0, 5 * h, 1, h, false},
		{"into the third", 0, 25 * h, 2, 25 * h, false},
		{"a long outage skips to the last stage and completes", 0, 100 * h, 3, 49 * h, true},
		{"from the second stage", 1, 30 * h, 2, 25 * h, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			began := t0
			if tc.index == 1 {
				began = t0.Add(h)
			}
			got := Progress(stages, tc.index, began, t0.Add(tc.now))
			if got.Index != tc.want || !got.Began.Equal(t0.Add(tc.began)) || got.Complete != tc.complete {
				t.Errorf("Progress = {%d %s %v}, want {%d %s %v}", got.Index, got.Began.Sub(t0), got.Complete, tc.want, tc.began, tc.complete)
			}
		})
	}
}

// Progress depends only on the schedule and the moment, not on how often it
// is asked, so two schedulers agree on the stage and its start.
//
// Verifies: REL-052.
func TestProgress_REL_052_Deterministic(t *testing.T) {
	stages := campaign()
	now := t0.Add(30 * time.Hour)
	a := Progress(stages, 0, t0, now)
	b := Progress(stages, a.Index, a.Began, now)
	if a != b {
		t.Errorf("Progress is not stable: %+v then %+v", a, b)
	}
}

// A last stage below 100% holds for ever; one at 100% with a soak time
// completes when the soak has passed.
//
// Verifies: REL-052.
func TestProgress_REL_052_LastStage(t *testing.T) {
	hold := []Stage{{100, 60}, {2500, 0}}
	if got := Progress(hold, 0, t0, t0.Add(1000*time.Hour)); got.Index != 1 || got.Complete {
		t.Errorf("a partial last stage = %+v, want held at stage 1", got)
	}
	soak := []Stage{{100, 60}, {10000, 3600}}
	if got := Progress(soak, 1, t0, t0.Add(30*time.Minute)); got.Complete {
		t.Errorf("the soak ended early: %+v", got)
	}
	if got := Progress(soak, 1, t0, t0.Add(time.Hour)); !got.Complete {
		t.Errorf("the soak did not end: %+v", got)
	}
}
