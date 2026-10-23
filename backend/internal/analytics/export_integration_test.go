// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package analytics_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/analytics"
	"github.com/nightCode42/plux3/backend/internal/analytics/analyticstest"
	"github.com/nightCode42/plux3/backend/internal/httpx"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// collector is an OTLP/HTTP collector that records what it receives.
type collector struct {
	*httptest.Server
	mu       sync.Mutex
	bodies   [][]byte
	paths    []string
	status   int
	attempts int
}

func newCollector(t *testing.T) *collector {
	t.Helper()
	c := &collector{status: http.StatusOK}
	c.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		c.mu.Lock()
		defer c.mu.Unlock()
		c.attempts++
		if c.status == http.StatusOK {
			c.paths = append(c.paths, r.Method+" "+r.URL.Path+" "+r.Header.Get("Content-Type"))
			c.bodies = append(c.bodies, body)
		}
		w.WriteHeader(c.status)
	}))
	t.Cleanup(c.Close)
	return c
}

func (c *collector) setStatus(s int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.status = s
}

func (c *collector) requests() [][]byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([][]byte(nil), c.bodies...)
}

// loopbackOptions let the exporter reach an httptest server on 127.0.0.1.
func loopbackOptions(t *testing.T) analytics.ExportOptions {
	t.Helper()
	return analytics.ExportOptions{
		Client:    mustClient(t, httpx.Options{Destination: httpx.KindAnalyticsOTLP, Policy: httpx.Policy{AllowLoopback: true}, Timeout: 5 * time.Second}),
		AllowHTTP: true, Lag: 10 * time.Minute, Chunk: 50,
	}
}

// otlp is the part of an OTLP request the tests read.
type otlp struct {
	ResourceMetrics []struct {
		Resource struct {
			Attributes []struct {
				Key   string
				Value struct{ StringValue string }
			}
		}
		ScopeMetrics []struct {
			Metrics []struct {
				Name string
				Unit string
				Sum  *struct {
					DataPoints []struct{ AsInt string }
					IsMono     bool `json:"isMonotonic"`
					Temporal   int  `json:"aggregationTemporality"`
				}
				Histogram *struct {
					DataPoints []struct {
						Count          string
						BucketCounts   []string
						ExplicitBounds []float64
					}
				}
			}
		}
	}
}

// totals adds up the event counts and the durations' counts of requests.
func totals(t *testing.T, bodies [][]byte) (events, durations int64) {
	t.Helper()
	for _, b := range bodies {
		var req otlp
		if err := json.Unmarshal(b, &req); err != nil {
			t.Fatalf("the request is not JSON: %v\n%s", err, b)
		}
		for _, rm := range req.ResourceMetrics {
			for _, sm := range rm.ScopeMetrics {
				for _, m := range sm.Metrics {
					if m.Sum != nil {
						for _, p := range m.Sum.DataPoints {
							n, _ := strconv.ParseInt(p.AsInt, 10, 64)
							events += n
						}
					}
					if m.Histogram != nil {
						for _, p := range m.Histogram.DataPoints {
							n, _ := strconv.ParseInt(p.Count, 10, 64)
							durations += n
							if len(p.BucketCounts) != analytics.Buckets || len(p.ExplicitBounds) != len(analytics.Bounds) {
								t.Errorf("a histogram of %d buckets and %d bounds", len(p.BucketCounts), len(p.ExplicitBounds))
							}
						}
					}
				}
			}
		}
	}
	return events, durations
}

// Verifies: ANL-011.
// A destination is checked before it is saved: the server sends it an empty
// request, and one that does not answer, answers with an error, is not https,
// carries credentials or is on a private address is refused as PLX-8302 or as
// an invalid address, and nothing is stored.
func TestExportDestinationIsChecked_ANL_011(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	col := newCollector(t)
	f := analyticstest.New(t, analyticstest.Options{Export: loopbackOptions(t)})

	e, err := f.Analytics.SetExport(ctx, f.Owner, col.URL, true)
	if err != nil || e.Endpoint != col.URL+"/v1/metrics" || !e.Enabled {
		t.Fatalf("SetExport: %+v %v", e, err)
	}
	if reqs := col.requests(); len(reqs) != 1 || col.paths[0] != "POST /v1/metrics application/json" {
		t.Fatalf("the collector saw %d requests %v", len(reqs), col.paths)
	}
	if n, _ := totals(t, col.requests()); n != 0 {
		t.Errorf("the check carried %d events", n)
	}
	if got, err := f.Analytics.GetExport(ctx, f.Viewer()); err != nil || got.Endpoint != e.Endpoint {
		t.Errorf("GetExport: %+v %v", got, err)
	}

	col.setStatus(http.StatusInternalServerError)
	if _, err := f.Analytics.SetExport(ctx, f.Owner, col.URL+"/other", true); code(err) != plxerr.ExportDestinationUnreachable {
		t.Errorf("a collector that answers 500: %v", err)
	}
	if got, _ := f.Analytics.GetExport(ctx, f.Owner); got.Endpoint != e.Endpoint {
		t.Errorf("a refused destination replaced the saved one: %s", got.Endpoint)
	}
	userinfo := url.URL{Scheme: "https", User: url.UserPassword("u", "p"), Host: "x.example"}
	for name, endpoint := range map[string]string{
		"not a url": "::", "ftp": "ftp://x.example", "userinfo": userinfo.String(), "a query": "https://x.example/?token=1",
		"plain http": col.URL,
	} {
		strict := analyticstest.New(t, analyticstest.Options{})
		if _, err := strict.Analytics.SetExport(ctx, strict.Owner, endpoint, true); code(err) != plxerr.InvalidFormat {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := f.Analytics.SetExport(ctx, f.Viewer(), col.URL, true); code(err) != plxerr.PermissionDenied {
		t.Errorf("a viewer sets the destination: %v", err)
	}
	col.Close()
	if _, err := f.Analytics.SetExport(ctx, f.Owner, col.URL, true); code(err) != plxerr.ExportDestinationUnreachable {
		t.Errorf("a collector that is down: %v", err)
	}
}

// Verifies: ANL-011, SEC-105.
// With the default client the exporter cannot reach a private or loopback
// address, whatever the URL says.
func TestExportRefusesPrivateAddresses_ANL_011(t *testing.T) {
	t.Parallel()
	col := newCollector(t)
	opts := analytics.ExportOptions{AllowHTTP: true}
	f := analyticstest.New(t, analyticstest.Options{Export: opts})
	if _, err := f.Analytics.SetExport(context.Background(), f.Owner, col.URL, true); code(err) != plxerr.ExportDestinationUnreachable {
		t.Errorf("a loopback collector: %v", err)
	}
	if reqs := col.requests(); len(reqs) != 0 {
		t.Errorf("the collector received %d requests", len(reqs))
	}
}

// Verifies: ANL-011.
// New rollups are sent as OTLP delta metrics, once: every event is counted in
// exactly one request, durations as explicit-bucket histograms with the
// rollup's bounds, and nothing is sent twice.
func TestExportSendsRollupsOnce_ANL_011(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	col := newCollector(t)
	f := analyticstest.New(t, analyticstest.Options{Export: loopbackOptions(t)})
	if _, err := f.Analytics.SetExport(ctx, f.Owner, col.URL, true); err != nil {
		t.Fatal(err)
	}
	checks := len(col.requests())
	base := f.Clock.Now().Add(time.Minute)
	var samples []sample
	for i := range 90 {
		samples = append(samples, sample{at: base.Add(time.Duration(i) * 7 * time.Minute), release: int64(1 + i%3), page: []string{"/a", "/b"}[i%2], value: float64(i*13%800) + 1})
	}
	write(t, f, samples)
	rollUp(t, f, time.Now().Add(time.Hour))

	f.Clock.Advance(5 * time.Minute)
	if err := f.Analytics.ExportAll(ctx); err != nil || len(col.requests()) != checks {
		t.Fatalf("an export inside the lag sent %d requests: %v", len(col.requests())-checks, err)
	}
	f.Clock.Advance(24 * time.Hour)
	if err := f.Analytics.ExportAll(ctx); err != nil {
		t.Fatal(err)
	}
	sent := col.requests()[checks:]
	if len(sent) < 2 {
		t.Fatalf("%d requests; the points should not fit one chunk of 50", len(sent))
	}
	if events, durations := totals(t, sent); events != int64(len(samples)) || durations != int64(len(samples)) {
		t.Errorf("exported %d events and %d durations, want %d of each", events, durations, len(samples))
	}
	var first otlp
	if err := json.Unmarshal(sent[0], &first); err != nil {
		t.Fatal(err)
	}
	metrics := first.ResourceMetrics[0].ScopeMetrics[0].Metrics
	if metrics[0].Name != "plux.events" || metrics[0].Sum == nil || !metrics[0].Sum.IsMono || metrics[0].Sum.Temporal != 1 ||
		metrics[1].Name != "plux.event.duration" || metrics[1].Unit != "ms" {
		t.Errorf("metrics = %+v", metrics)
	}
	if err := f.Analytics.ExportAll(ctx); err != nil || len(col.requests()) != checks+len(sent) {
		t.Errorf("a second export sent %d more requests: %v", len(col.requests())-checks-len(sent), err)
	}
	if e, _ := f.Analytics.GetExport(ctx, f.Owner); e.LastError != "" || !e.ExportedUntil.After(base) {
		t.Errorf("export state: %+v", e)
	}
}

// Verifies: ANL-011.
// A destination that fails is PLX-8302: the failure is recorded on the
// configuration, the export does not move on, and when the destination
// recovers the same rollups are sent. A disabled or removed destination
// receives nothing.
func TestExportFailureAndRecovery_ANL_011(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	col := newCollector(t)
	f := analyticstest.New(t, analyticstest.Options{Export: loopbackOptions(t)})
	if _, err := f.Analytics.SetExport(ctx, f.Owner, col.URL, true); err != nil {
		t.Fatal(err)
	}
	checks := len(col.requests())
	write(t, f, []sample{{at: f.Clock.Now().Add(time.Minute), value: 5}, {at: f.Clock.Now().Add(2 * time.Minute), value: 6}})
	rollUp(t, f, time.Now().Add(time.Hour))
	f.Clock.Advance(time.Hour)

	col.setStatus(http.StatusServiceUnavailable)
	err := f.Analytics.ExportAll(ctx)
	if col.attempts == 0 {
		t.Error("the collector was not tried")
	}
	if code(err) != plxerr.ExportDestinationUnreachable {
		// ExportAll joins the errors of organisations; one organisation here.
		t.Fatalf("a failing destination: %v", err)
	}
	failed, _ := f.Analytics.GetExport(ctx, f.Owner)
	if failed.LastError == "" || failed.LastAttemptAt.IsZero() {
		t.Errorf("the failure was not recorded: %+v", failed)
	}
	col.setStatus(http.StatusOK)
	if err := f.Analytics.ExportAll(ctx); err != nil {
		t.Fatal(err)
	}
	recovered, _ := f.Analytics.GetExport(ctx, f.Owner)
	events, _ := totals(t, col.requests()[checks:])
	if recovered.LastError != "" || !recovered.ExportedUntil.After(failed.ExportedUntil) || events != 2 {
		t.Errorf("after recovery: %+v, %d events counted", recovered, events)
	}

	if _, err := f.Analytics.SetExport(ctx, f.Owner, col.URL, false); err != nil {
		t.Fatal(err)
	}
	sent := len(col.requests())
	write(t, f, []sample{{at: f.Clock.Now().Add(time.Minute), value: 5}})
	rollUp(t, f, time.Now().Add(2*time.Hour))
	f.Clock.Advance(time.Hour)
	if err := f.Analytics.ExportAll(ctx); err != nil || len(col.requests()) != sent+1 && len(col.requests()) != sent {
		t.Fatalf("a disabled destination: %v", err)
	}
	if len(col.requests()) != sent {
		t.Errorf("a disabled destination received %d requests", len(col.requests())-sent)
	}
	if err := f.Analytics.DeleteExport(ctx, f.Owner); err != nil {
		t.Fatal(err)
	}
	if err := f.Analytics.DeleteExport(ctx, f.Owner); code(err) != plxerr.ResourceNotFound {
		t.Errorf("deleting twice: %v", err)
	}
	if _, err := f.Analytics.GetExport(ctx, f.Owner); code(err) != plxerr.ResourceNotFound {
		t.Errorf("GetExport after delete: %v", err)
	}
	if got := auditActions(t, f); len(got) != 3 || got[0] != "analytics.export_set" || got[2] != "analytics.export_deleted" {
		t.Errorf("audit = %v", got)
	}
}

// mustClient builds an SSRF-safe client for a registered destination.
func mustClient(t *testing.T, o httpx.Options) *http.Client {
	t.Helper()
	c, err := httpx.NewClient(o)
	if err != nil {
		t.Fatalf("httpx.NewClient: %v", err)
	}
	return c
}
