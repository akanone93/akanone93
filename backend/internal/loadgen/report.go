// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package loadgen

import (
	"errors"
	"math"
	"sync"
	"time"

	"connectrpc.com/connect"
)

// Operation names, the keys of Report.Operations.
const (
	OpRegister  = "register"
	OpManifest  = "manifest"
	OpControl   = "control"
	OpTelemetry = "telemetry"
	OpFunction  = "function"
	OpToken     = "token"
)

// ReportSchema identifies the report's format.
const ReportSchema = "plux-load/1"

// Latency is a latency distribution in milliseconds.
type Latency struct {
	P50 float64 `json:"p50"`
	P95 float64 `json:"p95"`
	P99 float64 `json:"p99"`
	Max float64 `json:"max"`
}

// OpReport is what one operation did during the run.
type OpReport struct {
	Requests int64 `json:"requests"`
	Errors   int64 `json:"errors"`
	// RatePerSecond is the requests per second of the run's wall clock;
	// registration happens before the run and has no rate.
	RatePerSecond float64 `json:"ratePerSecond"`
	// RealRatePerSecond is the rate at the real pacing: RatePerSecond
	// divided by the run's pacing scale.
	RealRatePerSecond float64          `json:"realRatePerSecond"`
	LatencyMS         Latency          `json:"latencyMs"`
	ErrorCodes        map[string]int64 `json:"errorCodes,omitempty"`
}

// Report is the result of a run. It holds counts, rates and latencies, and
// nothing a device or a person sent.
type Report struct {
	Schema          string              `json:"schema"`
	Devices         int                 `json:"devices"`
	Registered      int                 `json:"registered"`
	DurationSeconds float64             `json:"durationSeconds"`
	PacingScale     float64             `json:"pacingScale"`
	Operations      map[string]OpReport `json:"operations"`
	// ControlApplied is how many control_applied events the devices sent.
	ControlApplied int64 `json:"controlApplied"`
}

// Errors is the total of failed requests of all operations.
func (r Report) Errors() int64 {
	var n int64
	for _, op := range r.Operations {
		n += op.Errors
	}
	return n
}

// histogramRatio is the growth of the histogram's buckets: a percentile is
// exact to within this fraction.
const histogramRatio = 1.04

// histogramFloor is the smallest latency told apart from zero.
const histogramFloor = 10 * time.Microsecond

// histogram counts latencies in exponentially growing buckets, so a run of
// any length needs a bounded amount of memory.
type histogram struct {
	counts []int64
	total  int64
	max    time.Duration
}

func bucketOf(d time.Duration) int {
	if d <= histogramFloor {
		return 0
	}
	return int(math.Ceil(math.Log(float64(d)/float64(histogramFloor)) / math.Log(histogramRatio)))
}

func (h *histogram) add(d time.Duration) {
	b := bucketOf(d)
	for len(h.counts) <= b {
		h.counts = append(h.counts, 0)
	}
	h.counts[b]++
	h.total++
	h.max = max(h.max, d)
}

// quantile returns the upper bound of the bucket holding the q-th
// quantile, 0 to 1, in milliseconds.
func (h *histogram) quantile(q float64) float64 {
	if h.total == 0 {
		return 0
	}
	rank := int64(math.Ceil(q * float64(h.total)))
	var seen int64
	for b, n := range h.counts {
		if seen += n; seen >= rank {
			upper := time.Duration(float64(histogramFloor) * math.Pow(histogramRatio, float64(b)))
			return toMS(min(upper, h.max))
		}
	}
	return toMS(h.max)
}

func toMS(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }

// op accumulates one operation.
type op struct {
	latency  histogram
	requests int64
	errors   int64
	codes    map[string]int64
}

// recorder collects what the devices did; it is safe for concurrent use.
type recorder struct {
	mu      sync.Mutex
	ops     map[string]*op
	applied int64
}

func newRecorder() *recorder { return &recorder{ops: map[string]*op{}} }

// observe records one request of an operation and its outcome.
func (r *recorder) observe(name string, took time.Duration, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	o := r.ops[name]
	if o == nil {
		o = &op{codes: map[string]int64{}}
		r.ops[name] = o
	}
	o.requests++
	o.latency.add(took)
	if err != nil {
		o.errors++
		o.codes[codeOf(err)]++
	}
}

// markApplied counts one control_applied event sent.
func (r *recorder) markApplied() {
	r.mu.Lock()
	r.applied++
	r.mu.Unlock()
}

// codeOf names a failure: the ConnectRPC code, or "network" when the
// request never got an answer.
func codeOf(err error) string {
	var ce *connect.Error
	if errors.As(err, &ce) {
		return ce.Code().String()
	}
	return "network"
}

// report builds the report of a run that lasted the given time.
func (r *recorder) report(c Config, registered int, took time.Duration) Report {
	r.mu.Lock()
	defer r.mu.Unlock()
	secs := took.Seconds()
	out := Report{
		Schema: ReportSchema, Devices: c.Devices, Registered: registered, DurationSeconds: secs,
		PacingScale: c.Scale, Operations: map[string]OpReport{}, ControlApplied: r.applied,
	}
	for name, o := range r.ops {
		rep := OpReport{
			Requests: o.requests, Errors: o.errors,
			LatencyMS: Latency{P50: o.latency.quantile(0.5), P95: o.latency.quantile(0.95), P99: o.latency.quantile(0.99), Max: toMS(o.latency.max)},
		}
		if secs > 0 && name != OpRegister {
			rep.RatePerSecond = float64(o.requests) / secs
			rep.RealRatePerSecond = rep.RatePerSecond / c.Scale
		}
		if len(o.codes) > 0 {
			rep.ErrorCodes = map[string]int64{}
			for k, v := range o.codes {
				rep.ErrorCodes[k] = v
			}
		}
		out.Operations[name] = rep
	}
	return out
}
