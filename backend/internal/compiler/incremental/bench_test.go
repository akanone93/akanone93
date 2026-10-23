// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package incremental

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"testing/fstest"
	"time"
)

// latencyBudget is the time one edit of a page may take to compile at the
// 95th percentile (CMP-051, DEV-010); the whole edit-to-device budget is
// 500 ms.
const latencyBudget = 100 * time.Millisecond

// fiftyPages is a 50-page plugin of 100-node pages: the conformance project
// with 48 copies of its calculator page.
func fiftyPages(t testing.TB) fstest.MapFS { return manyPages(t, 48, 100) }

// editPage changes the description of the n-th copy, so that its document
// differs from every earlier version.
func editPage(t testing.TB, m fstest.MapFS, n, version int) {
	t.Helper()
	edit(t, m, pageFile(n%48), func(doc map[string]any) { doc["description"] = fmt.Sprintf("version %d", version) })
}

// BenchmarkEditOnePageOf50 compiles one edited page of a 50-page plugin
// incrementally, which TestEditLatency checks against its budget.
func BenchmarkEditOnePageOf50(b *testing.B) {
	m := fiftyPages(b)
	c := NewCache(opts())
	incr(b, c, m)
	b.ResetTimer()
	for i := range b.N {
		b.StopTimer()
		editPage(b, m, i, i)
		b.StartTimer()
		if res := incr(b, c, m); res.Full {
			b.Fatalf("a full compilation: %s", res.FullReason)
		}
	}
}

// BenchmarkFullCompile50Pages is the full compilation the edit avoids, for
// comparison.
func BenchmarkFullCompile50Pages(b *testing.B) {
	m := fiftyPages(b)
	for b.Loop() {
		if res := full(m); res.App == nil {
			b.Fatal("does not compile")
		}
	}
}

// percentile returns the p-th percentile of durations (nearest rank).
func percentile(ds []time.Duration, p int) time.Duration {
	s := slices.Clone(ds)
	slices.Sort(s)
	return s[max((len(s)*p+99)/100-1, 0)]
}

// TestEditLatency checks that compiling one edited page of a 50-page plugin
// takes at most 100 ms at the 95th percentile (CMP-051, DEV-010). Timings
// depend on the machine and are meaningless under the race detector, so the
// test runs in neither -short nor -race.
func TestEditLatency(t *testing.T) {
	if testing.Short() || raceEnabled {
		t.Skip("timing budgets run without -short and without the race detector")
	}
	m := fiftyPages(t)
	c := NewCache(opts())
	incr(t, c, m)
	var times []time.Duration
	for i := range 51 {
		editPage(t, m, i*7, i)
		start := time.Now()
		res, err := Compile(context.Background(), c, Input{FS: m})
		elapsed := time.Since(start)
		if err != nil {
			t.Fatal(err)
		}
		if res.Full {
			t.Fatalf("iteration %d was a full compilation: %s", i, res.FullReason)
		}
		if i > 0 { // the first edit warms the caches
			times = append(times, elapsed)
		}
	}
	p50, p95 := percentile(times, 50), percentile(times, 95)
	t.Logf("one page of 50: median %v, p95 %v over %d edits", p50, p95, len(times))
	if p95 > latencyBudget {
		t.Errorf("p95 %v is above the budget of %v", p95, latencyBudget)
	}
}
