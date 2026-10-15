// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package fnrunner

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/fnpb"
)

// The budgets of FN-023 and NFR-022, excluding the function's own work.
const (
	benchWarmBudgetP99  = time.Millisecond
	benchFirstBudgetP99 = 50 * time.Millisecond
)

// Samples per measurement. A p99 over 2,000 warm calls rests on the 20
// slowest; over 100 first calls, on the slowest one. The warm calls are
// measured in benchWarmRounds rounds and the gate reads the median round's
// p99, so one round disturbed by a neighbour on a shared CI runner (the
// reference runner, as for CMP-050) does not fail it, while a regression,
// which slows every round, does.
const (
	benchWarmUp       = 200
	benchWarmCalls    = 2000
	benchWarmRounds   = 5
	benchFirstSamples = 100
)

// benchEchoInput is the input of the probe's echo entry, the one a server
// function is dispatched to (index 0): the guest copies its input to its
// output, so the measurement is the runner's overhead and not the function's
// work.
const benchEchoInput = `{"loan":{"amount":1200,"months":12}}`

// percentiles are the quantiles of a measurement, in microseconds.
type percentiles struct {
	Samples int     `json:"samples"`
	P50us   float64 `json:"p50_us"`
	P99us   float64 `json:"p99_us"`
	MaxUs   float64 `json:"max_us"`
}

// summarise returns the nearest-rank p50 and p99 of d, which it sorts.
func summarise(d []time.Duration) percentiles {
	slices.Sort(d)
	at := func(q float64) float64 {
		rank := int(q*float64(len(d))+0.999999) - 1
		rank = max(0, min(rank, len(d)-1))
		return float64(d[rank].Nanoseconds()) / 1e3
	}
	return percentiles{Samples: len(d), P50us: at(0.50), P99us: at(0.99), MaxUs: at(1)}
}

// firstSamples measures the first invocation of benchFirstSamples fresh
// runners built from opts. prepare runs before the timer for the variants
// that compile at deploy.
func firstSamples(t *testing.T, opts Options, prepare *fnpb.PrepareRequest, call func(*Runner) time.Duration) []time.Duration {
	t.Helper()
	ctx := context.Background()
	out := make([]time.Duration, benchFirstSamples)
	for i := range out {
		o := opts
		o.Registerer = nil // each runner keeps its own private registry
		r, err := New(ctx, o)
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if prepare != nil {
			if _, err := r.Prepare(ctx, connect.NewRequest(prepare)); err != nil {
				t.Fatalf("Prepare: %v", err)
			}
		}
		out[i] = call(r)
		if err := r.Close(ctx); err != nil {
			t.Fatalf("Close: %v", err)
		}
	}
	return out
}

// Verifies: NFR-022, FN-023.
// A warm invocation costs at most 1 ms at p99 and the first invocation after
// deploy at most 50 ms at p99, excluding the function's own work. The module
// is the probe and the entry its echo, so the guest does no work of its own;
// the limits are the registry defaults.
//
// Warm is the runner's Invoke with the module compiled and cached and a fresh
// instance per call (FN-010) on pooled guest memory. First after deploy is
// what the design promises (plan p7 section 5.3): the deploy asked the runner
// to Prepare the module, which fetched, verified and compiled it, and the
// first call after that is timed. Two variants are reported beside it and not
// gated: the cold path, in which a runner with an empty cache fetches the
// artifact from the api role over the pinned mutual-TLS connection, verifies
// it (FN-009), compiles it and runs (a deploy whose Prepare failed, or a
// replica the Prepare did not reach), and the first call of a runner that
// starts with a populated compilation cache (a restart, or a second
// replica). The transport in front of the runner (ConnectRPC from the api
// role) is part of none of them. The test runs only with PLUX_BENCH_FN=1 and
// prints the result as JSON (make bench-fn).
func TestFunctionInvocationOverhead_NFR_022(t *testing.T) {
	if os.Getenv("PLUX_BENCH_FN") != "1" {
		t.Skip("set PLUX_BENCH_FN=1 to measure (make bench-fn)")
	}
	ctx := context.Background()
	var opts Options
	h := newHarness(t, harnessOptions{tweak: func(o *Options) { opts = *o }})
	prepare := &fnpb.PrepareRequest{ModuleSha256: h.probeDigest(), Grant: "bench-grant"}
	// Prepare once so the pinned connection to the api role is established
	// before the first sample: a runner process keeps it across deploys.
	if _, err := h.runner.Prepare(ctx, connect.NewRequest(prepare)); err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	msg := h.request(benchEchoInput)
	msg.Limits = &fnpb.InvocationLimits{ // the registry defaults of schema/limits.json
		TimeoutMs: 2000, MemoryBytes: 64 << 20, InputBytes: 1 << 20, OutputBytes: 1 << 20,
		HttpCalls: 10, CallDepth: 3, LogBytes: 4096,
	}
	invoke := func(r *Runner) time.Duration {
		start := time.Now()
		resp, err := r.invoke(ctx, msg, serverFunctionIndex)
		d := time.Since(start)
		if err != nil {
			t.Fatalf("invoke: %v", err)
		}
		if out, ok := resp.Msg.GetResult().(*fnpb.FunctionRunnerServiceInvokeResponse_Output); !ok || string(out.Output) != benchEchoInput {
			t.Fatalf("the echo entry returned %v", resp.Msg.GetResult())
		}
		return d
	}

	for range benchWarmUp {
		invoke(h.runner)
	}
	rounds := make([]percentiles, benchWarmRounds)
	for r := range rounds {
		warm := make([]time.Duration, benchWarmCalls)
		for i := range warm {
			warm[i] = invoke(h.runner)
		}
		rounds[r] = summarise(warm)
	}
	slices.SortFunc(rounds, func(a, b percentiles) int { return cmp.Compare(a.P99us, b.P99us) })

	cold := firstSamples(t, opts, nil, invoke)
	afterPrepare := firstSamples(t, opts, prepare, invoke)
	withCache := opts
	withCache.CacheDir = t.TempDir()
	firstSamples(t, withCache, nil, invoke) // populates the cache directory
	cached := firstSamples(t, withCache, nil, invoke)

	report := struct {
		Requirement       string        `json:"requirement"`
		Warm              percentiles   `json:"warm"`
		WarmRounds        []percentiles `json:"warmRounds"`
		FirstAfterDeploy  percentiles   `json:"firstAfterDeploy"`
		FirstCold         percentiles   `json:"firstCold"`
		FirstCompileCache percentiles   `json:"firstWithCompilationCache"`
	}{"NFR-022", rounds[len(rounds)/2], rounds, summarise(afterPrepare), summarise(cold), summarise(cached)}
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("%s\n", raw)

	if got := time.Duration(report.Warm.P99us * 1e3); got > benchWarmBudgetP99 {
		t.Errorf("warm p99 (median of %d rounds) = %v, budget %v (NFR-022)", benchWarmRounds, got, benchWarmBudgetP99)
	}
	if got := time.Duration(report.FirstAfterDeploy.P99us * 1e3); got > benchFirstBudgetP99 {
		t.Errorf("first-after-deploy p99 = %v, budget %v (NFR-022)", got, benchFirstBudgetP99)
	}
}
