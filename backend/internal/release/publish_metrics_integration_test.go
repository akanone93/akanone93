// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package release_test

import (
	"sync"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/release"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

// countingMetrics records the publish outcomes it is told about.
type countingMetrics struct {
	mu      sync.Mutex
	results map[string]int // guarded by mu
}

func (m *countingMetrics) PublishFinished(result string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.results[result]++
}

// TestPublishCountsFinishedJobs_OBS_005 checks that every committed publish
// outcome reaches plux_publish_jobs_total, which the publish job success
// SLO reads.
// Verifies: OBS-005.
func TestPublishCountsFinishedJobs_OBS_005(t *testing.T) {
	t.Parallel()
	m := &countingMetrics{results: map[string]int{}}
	f := newFixtureWith(t, "loan-calculator", limits.Set{}, func(o *release.Options) { o.Metrics = m })
	if job := f.publish(t, "", false); job.State != release.StateSucceeded {
		t.Fatalf("the app publish: %+v", job)
	}
	if job := f.publish(t, f.loans, false); job.State != release.StateSucceeded {
		t.Fatalf("the plugin publish: %+v", job)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.results[release.StateSucceeded] != 2 || len(m.results) != 1 {
		t.Errorf("recorded %v, want two succeeded jobs only", m.results)
	}
}
