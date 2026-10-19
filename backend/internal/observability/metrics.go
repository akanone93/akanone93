// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package observability

import (
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds the process's metrics, exactly those catalogued in
// Appendix G.1 that this phase produces (OBS-002). Each field is
// unexported so that the only way to record a value is a method here,
// which fixes the label set and keeps cardinality bounded.
type Metrics struct {
	registry *prometheus.Registry

	rpcRequests      *prometheus.CounterVec
	rpcDuration      *prometheus.HistogramVec
	manifestRequests *prometheus.CounterVec
	deltaBytes       *prometheus.HistogramVec
	deltaGeneration  *prometheus.HistogramVec
	publishJobs      *prometheus.CounterVec
	publishDuration  *prometheus.HistogramVec
	jobsQueueDepth   *prometheus.GaugeVec

	replayDegraded prometheus.Gauge
	replayFallback prometheus.Counter

	functionRequests *prometheus.CounterVec
	functionSeconds  prometheus.Histogram
	functionReplays  prometheus.Counter
	functionPrepares *prometheus.CounterVec

	dpopRejections     *prometheus.CounterVec
	attestationResults *prometheus.CounterVec

	// auditVerified exists only once a verification has run, so that the
	// metric never claims a chain holds before anyone looked.
	auditMu       sync.Mutex
	auditVerified prometheus.Gauge

	devicesMu sync.Mutex
	devices   *devicesByAssurance
}

// NewMetrics builds the registry with the process collectors and the
// Plux metrics of Appendix G.1.
func NewMetrics() *Metrics {
	r := prometheus.NewRegistry()
	r.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	m := &Metrics{
		registry: r,
		rpcRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "plux_rpc_requests_total",
			Help: "RPC requests by service, method and result code.",
		}, []string{"service", "method", "code"}),
		rpcDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "plux_rpc_duration_seconds",
			Help:    "RPC duration by service and method.",
			Buckets: prometheus.DefBuckets,
		}, []string{"service", "method"}),
		manifestRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "plux_manifest_requests_total",
			Help: "Manifest requests by environment and result.",
		}, []string{"environment", "result"}),
		deltaBytes: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "plux_delta_bytes",
			Help:    "Bytes a device is asked to download, by kind.",
			Buckets: prometheus.ExponentialBuckets(1024, 4, 10),
		}, []string{"kind"}),
		deltaGeneration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "plux_delta_generation_seconds",
			Help:    "Time to produce a delta, by mode.",
			Buckets: prometheus.DefBuckets,
		}, []string{"mode"}),
		publishJobs: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "plux_publish_jobs_total",
			Help: "Publish jobs by result.",
		}, []string{"result"}),
		publishDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "plux_publish_duration_seconds",
			Help:    "Time spent in each stage of the publish pipeline.",
			Buckets: prometheus.ExponentialBuckets(0.01, 3, 10),
		}, []string{"stage"}),
		jobsQueueDepth: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "plux_jobs_queue_depth",
			Help: "Jobs waiting in each queue.",
		}, []string{"queue"}),
		replayDegraded: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "plux_dpop_replay_cache_degraded",
			Help: "1 while the shared DPoP replay cache is unreachable and this replica decides alone, 0 otherwise.",
		}),
		replayFallback: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "plux_dpop_replay_cache_fallback_total",
			Help: "DPoP replay checks the per-replica store decided because the shared cache failed.",
		}),
	}
	m.dpopRejections = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "plux_dpop_rejections_total",
		Help: "Device calls refused for their DPoP proof, by reason.",
	}, []string{"reason"})
	m.attestationResults = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "plux_attestation_results_total",
		Help: "Attestation verifications by platform and verdict.",
	}, []string{"platform", "verdict"})
	m.newFunctionMetrics()
	r.MustRegister(m.dpopRejections, m.attestationResults)
	r.MustRegister(m.rpcRequests, m.rpcDuration, m.manifestRequests, m.deltaBytes,
		m.deltaGeneration, m.publishJobs, m.publishDuration, m.jobsQueueDepth,
		m.replayDegraded, m.replayFallback)
	return m
}

// newFunctionMetrics creates and registers the metrics of the api role's
// function invocations.
func (m *Metrics) newFunctionMetrics() {
	// The api role's series are named apart from the fnrunner's
	// plux_function_invocations_total, which shares this registry when one
	// process runs both roles.
	m.functionRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "plux_function_requests_total",
		Help: "Function invocations as the api role answered them, by result: ok, business_error or the lower-case reason of the error.",
	}, []string{"result"})
	m.functionSeconds = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "plux_function_request_seconds",
		Help:    "Time from a device's function invocation to the api role's answer.",
		Buckets: []float64{0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30},
	})
	m.functionPrepares = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "plux_function_prepares_total",
		Help: "Requests from the api role to compile a deployed function module ahead of its first invocation, by result: ok or failed.",
	}, []string{"result"})
	m.functionPrepares.WithLabelValues("ok")
	m.functionPrepares.WithLabelValues("failed")
	m.functionReplays = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "plux_function_replays_total",
		Help: "Function invocations answered with the stored result of an idempotency key.",
	})
	m.registry.MustRegister(m.functionRequests, m.functionSeconds, m.functionReplays, m.functionPrepares)
}

// Handler serves the metrics endpoint.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{Registry: m.registry})
}

// Registry exposes the registry so that a component can register a
// collector of its own; the catalogued metrics stay behind the methods.
func (m *Metrics) Registry() *prometheus.Registry { return m.registry }

// RPC records one finished call. code is the Connect code, or "ok".
func (m *Metrics) RPC(service, method, code string, d time.Duration) {
	m.rpcRequests.WithLabelValues(service, method, code).Inc()
	m.rpcDuration.WithLabelValues(service, method).Observe(d.Seconds())
}

// Manifest records one manifest request; result is "not_modified",
// "updated" or "error".
func (m *Metrics) Manifest(environment, result string) {
	m.manifestRequests.WithLabelValues(environment, result).Inc()
}

// Download records the size a device is asked to fetch; kind is "delta"
// or "full".
func (m *Metrics) Download(kind string, bytes int64) {
	m.deltaBytes.WithLabelValues(kind).Observe(float64(bytes))
}

// DeltaGenerated records the time to produce a delta; mode is
// "precomputed" or "on_demand".
func (m *Metrics) DeltaGenerated(mode string, d time.Duration) {
	m.deltaGeneration.WithLabelValues(mode).Observe(d.Seconds())
}

// PublishFinished records a finished publish job; result is "succeeded",
// "failed" or "cancelled".
func (m *Metrics) PublishFinished(result string) { m.publishJobs.WithLabelValues(result).Inc() }

// PublishStage records the time one pipeline stage took.
func (m *Metrics) PublishStage(stage string, d time.Duration) {
	m.publishDuration.WithLabelValues(stage).Observe(d.Seconds())
}

// QueueDepth records how many jobs wait in a queue.
func (m *Metrics) QueueDepth(queue string, n int) {
	m.jobsQueueDepth.WithLabelValues(queue).Set(float64(n))
}

// ReplayCacheFallback records one DPoP replay check that the per-replica
// store decided because the shared cache failed (SEC-023). It raises the
// degraded gauge, which ReplayCacheRecovered lowers.
func (m *Metrics) ReplayCacheFallback() {
	m.replayFallback.Inc()
	m.replayDegraded.Set(1)
}

// ReplayCacheRecovered records that the shared DPoP replay cache answered
// again after a fallback (SEC-023).
func (m *Metrics) ReplayCacheRecovered() { m.replayDegraded.Set(0) }

// LicenseDaysRemaining exposes plux_license_days_remaining, the whole days
// until the enterprise licence expires, negative during its grace period
// (GOV-030). days is read at each scrape. Call it once per Metrics.
func (m *Metrics) LicenseDaysRemaining(days func() float64) {
	m.registry.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "plux_license_days_remaining",
		Help: "Days until the enterprise licence expires; negative during the grace period, 0 without a verified licence.",
	}, days))
}

// DPoP rejection reasons: the closed set of plux_dpop_rejections_total's
// reason label (SEC-021, SEC-023, SEC-024).
const (
	// DPoPNonceRequired is a proof without the server's current nonce.
	DPoPNonceRequired = "nonce_required"
	// DPoPProofInvalid is a malformed, wrongly signed, misbound or
	// out-of-window proof.
	DPoPProofInvalid = "proof_invalid"
	// DPoPReplay is a proof that was used before.
	DPoPReplay = "replay"
	// DPoPBindingMismatch is a proof from another key than the token's.
	DPoPBindingMismatch = "binding_mismatch"
	// DPoPReplayCacheUnavailable is a proof refused because the replay
	// cache is down and the environment fails closed.
	DPoPReplayCacheUnavailable = "replay_cache_unavailable"
)

var dpopReasons = map[string]bool{
	DPoPNonceRequired: true, DPoPProofInvalid: true, DPoPReplay: true,
	DPoPBindingMismatch: true, DPoPReplayCacheUnavailable: true,
}

// DPoPRejected records one device call refused for its DPoP proof. A reason
// outside the set above is counted as "other", so the label stays bounded.
func (m *Metrics) DPoPRejected(reason string) {
	if !dpopReasons[reason] {
		reason = "other"
	}
	m.dpopRejections.WithLabelValues(reason).Inc()
}

// Attestation verdicts: the closed set of plux_attestation_results_total's
// verdict label (SEC-009).
const (
	// AttestationVerified is evidence that passed every check.
	AttestationVerified = "verified"
	// AttestationFailed is evidence that failed a check.
	AttestationFailed = "failed"
	// AttestationUnavailable is a verification that could not be made:
	// the provider is down or the app has no attestation configured.
	AttestationUnavailable = "unavailable"
)

var (
	attestationPlatforms = map[string]bool{"android": true, "ios": true, "development": true}
	attestationVerdicts  = map[string]bool{AttestationVerified: true, AttestationFailed: true, AttestationUnavailable: true}
)

// AttestationResult records one verification of attestation evidence.
// Values outside the sets of platforms and verdicts are counted as
// "other".
func (m *Metrics) AttestationResult(platform, verdict string) {
	if !attestationPlatforms[platform] {
		platform = "other"
	}
	if !attestationVerdicts[verdict] {
		verdict = "other"
	}
	m.attestationResults.WithLabelValues(platform, verdict).Inc()
}

// AuditChainVerified records the outcome of the last verification of the
// audit chains: true when no break was found (SEC-140, SEC-141).
func (m *Metrics) AuditChainVerified(ok bool) {
	m.auditMu.Lock()
	defer m.auditMu.Unlock()
	if m.auditVerified == nil {
		m.auditVerified = prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "plux_audit_chain_verified",
			Help: "1 when the last verification of the audit chains found no break, 0 when it found one.",
		})
		m.registry.MustRegister(m.auditVerified)
	}
	if ok {
		m.auditVerified.Set(1)
		return
	}
	m.auditVerified.Set(0)
}

// Invoked records one function invocation as the api role answered it
// (FN-032): its result, its time end to end, and whether the answer was the
// stored one of an idempotency key. The series carry no app, user, function
// or input.
func (m *Metrics) Invoked(result string, d time.Duration, replayed bool) {
	m.functionRequests.WithLabelValues(result).Inc()
	m.functionSeconds.Observe(d.Seconds())
	if replayed {
		m.functionReplays.Inc()
	}
}

// Prepared records one request to the runner to compile a deployed
// module (FN-023) by its result, ok or failed.
func (m *Metrics) Prepared(result string) {
	m.functionPrepares.WithLabelValues(result).Inc()
}
