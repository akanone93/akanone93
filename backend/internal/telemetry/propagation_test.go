// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package telemetry

import (
	"context"
	"testing"
	"time"
)

// changes is a ControlChanges with one current version.
type changes struct {
	version int64
	at      time.Time
}

func (c changes) ChangedAt(_ context.Context, _ Source, version int64) (time.Time, bool, error) {
	return c.at, version == c.version, nil
}

// propagated collects what a Service records.
type propagated struct {
	apps      []string
	durations []time.Duration
}

func (p *propagated) ControlPropagated(app string, d time.Duration) {
	p.apps = append(p.apps, app)
	p.durations = append(p.durations, d)
}

func appliedEvent(version string, at time.Time) queued {
	return queued{Name: "control_applied", Time: at, Fields: []byte(`{"version":` + version + `,"outcome":"applied"}`)}
}

func propagationService(t *testing.T, c changes, now time.Time) (*Service, *propagated) {
	t.Helper()
	rec := &propagated{}
	return &Service{o: Options{Changes: c, Propagation: rec}, now: func() time.Time { return now }}, rec
}

// Verifies: NFR-024, OBS-005.
// The duration runs from the change to the matching control_applied event;
// an older version, other events and a malformed version record nothing.
func TestMeasurePropagation_NFR_024(t *testing.T) {
	t.Parallel()
	change := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	s, rec := propagationService(t, changes{version: 7, at: change}, change.Add(time.Minute))
	s.measurePropagation(t.Context(), message{Source: Source{AppID: "app-1"}, Events: []queued{
		appliedEvent("7", change.Add(12*time.Second)),
		appliedEvent("6", change.Add(13*time.Second)),
		appliedEvent(`"seven"`, change.Add(14*time.Second)),
		{Name: "screen_view", Time: change.Add(15 * time.Second), Fields: []byte(`{"version":7}`)},
		{Name: "control_applied", Time: change.Add(16 * time.Second), Fields: []byte(`{}`)},
	}})
	if len(rec.durations) != 1 || rec.durations[0] != 12*time.Second || rec.apps[0] != "app-1" {
		t.Fatalf("recorded %v for %v, want one 12s sample for app-1", rec.durations, rec.apps)
	}
}

// Verifies: NFR-024.
// A device clock outside the window from the change to now is not trusted:
// the arrival time stands in for it.
func TestMeasurePropagationDistrustsDeviceClock_NFR_024(t *testing.T) {
	t.Parallel()
	change := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	now := change.Add(30 * time.Second)
	s, rec := propagationService(t, changes{version: 3, at: change}, now)
	s.measurePropagation(t.Context(), message{Events: []queued{
		appliedEvent("3", change.Add(-time.Hour)),
		appliedEvent("3", now.Add(time.Hour)),
	}})
	if len(rec.durations) != 2 || rec.durations[0] != 30*time.Second || rec.durations[1] != 30*time.Second {
		t.Fatalf("recorded %v, want two 30s samples", rec.durations)
	}
}

// Verifies: NFR-024.
// Without the collaborators the measurement is off and costs nothing.
func TestMeasurePropagationOffWithoutCollaborators_NFR_024(t *testing.T) {
	t.Parallel()
	s := &Service{now: time.Now}
	s.measurePropagation(t.Context(), message{Events: []queued{appliedEvent("1", time.Now())}})
}
