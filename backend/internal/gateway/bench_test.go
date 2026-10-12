// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package gateway

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/cache"
	"github.com/nightCode42/plux3/backend/internal/device/devicetest"
	"github.com/nightCode42/plux3/backend/internal/devtoken"
	"github.com/nightCode42/plux3/backend/internal/dpop"
	"github.com/nightCode42/plux3/backend/internal/signing"
)

// p95 reports the 95th percentile of the samples, in milliseconds, under
// the metric name.
func p95(b *testing.B, name string, samples []time.Duration) {
	b.Helper()
	if len(samples) == 0 {
		return
	}
	slices.Sort(samples)
	b.ReportMetric(float64(samples[len(samples)*95/100])/float64(time.Millisecond), name)
}

// BenchmarkGatewayOverhead measures what the gateway adds to a call
// without the upstream's time (NFR-025, SEC-031): every check, the
// signature of the assertion, and the forwarding over a pooled
// connection to an upstream that answers at once.
//
// authorise-p95-ms is the time of Begin and Finish alone, the work the
// gateway does before it sends. added-p95-ms is the 95th percentile of a
// whole call through the gateway minus the median of the same request made
// straight to the upstream, which is the overhead the device sees. The
// budget is 5 ms at 1,000 requests per second per replica; the sequential
// runs below are well under that rate on one core, and the parallel one
// shows the cost under contention.
func BenchmarkGatewayOverhead(b *testing.B) {
	f := newFixture(b)
	f.gw.o.Count = nil // the benchmark sends far more than a device may
	u := f.ups["bank"]
	u.Operations = nil
	f.ups["bank"] = u
	f.handler = func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, `{"ok":true}`) }
	go func() { // the benchmark does not look at what the upstream saw
		for range f.requests {
		}
	}()
	b.Cleanup(func() { close(f.requests) })
	req := Request{Upstream: "bank", Method: "GET", Target: "/accounts/1?expand=history", Header: http.Header{"Accept": {"application/json"}}}
	ctx := context.Background()

	b.Run("authorise", func(b *testing.B) {
		samples := make([]time.Duration, 0, b.N)
		for b.Loop() {
			start := time.Now()
			p, err := f.gw.Begin(ctx, f.dev, req)
			if err != nil {
				b.Fatal(err)
			}
			if _, err := p.Finish(ctx, nil); err != nil {
				b.Fatal(err)
			}
			samples = append(samples, time.Since(start))
		}
		p95(b, "authorise-p95-ms", samples)
	})

	b.Run("authorise-parallel", func(b *testing.B) {
		results := make(chan []time.Duration, 64)
		b.RunParallel(func(pb *testing.PB) {
			var mine []time.Duration
			for pb.Next() {
				start := time.Now()
				p, err := f.gw.Begin(ctx, f.dev, req)
				if err != nil {
					b.Error(err)
					return
				}
				if _, err := p.Finish(ctx, nil); err != nil {
					b.Error(err)
					return
				}
				mine = append(mine, time.Since(start))
			}
			results <- mine
		})
		close(results)
		var all []time.Duration
		for s := range results {
			all = append(all, s...)
		}
		p95(b, "authorise-p95-ms", all)
	})

	b.Run("end-to-end", func(b *testing.B) {
		direct := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, `{"ok":true}`) }))
		b.Cleanup(direct.Close)
		client := &http.Client{Transport: &http.Transport{MaxIdleConnsPerHost: 64}}
		baseline := make([]time.Duration, 0, 2000)
		for range 2000 {
			start := time.Now()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, direct.URL+"/api/accounts/1?expand=history", nil)
			if err != nil {
				b.Fatal(err)
			}
			resp, err := client.Do(req)
			if err != nil {
				b.Fatal(err)
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			baseline = append(baseline, time.Since(start))
		}
		slices.Sort(baseline)
		median := baseline[len(baseline)/2]
		samples := make([]time.Duration, 0, b.N)
		for b.Loop() {
			start := time.Now()
			p, err := f.gw.Begin(ctx, f.dev, req)
			if err != nil {
				b.Fatal(err)
			}
			plan, err := p.Finish(ctx, nil)
			if err != nil {
				b.Fatal(err)
			}
			if _, err := f.gw.Do(ctx, plan); err != nil {
				b.Fatal(err)
			}
			samples = append(samples, time.Since(start))
		}
		slices.Sort(samples)
		added := samples[len(samples)*95/100] - median
		b.ReportMetric(float64(added)/float64(time.Millisecond), "added-p95-ms")
	})
}

// BenchmarkDeviceVerification measures the server's check of a device
// request: the access token, the DPoP proof bound to it with its nonce,
// and the replay check (NFR-025, SEC-022). The device lookup, a cached
// revocation check, is not part of it. verify-p95-ms must stay under 1 ms.
func BenchmarkDeviceVerification(b *testing.B) {
	f := newFixture(b)
	now := time.Now()
	issuer := &devtoken.Issuer{Signer: signing.TokenOnly(f.signer), Issuer: testIssuer, Audience: testIssuer, Lifetime: 5 * time.Minute}
	key, _ := devicetest.NewKey(b)
	jkt, err := dpop.Thumbprint(&key.PublicKey)
	if err != nil {
		b.Fatal(err)
	}
	token, _, err := issuer.Issue(context.Background(), devtoken.Claims{
		DeviceID: "d", AppID: "a", Environment: "e", OrganizationID: "o", Assurance: "AL2", JKT: jkt,
	})
	if err != nil {
		b.Fatal(err)
	}
	keys := (&tokenKeyCache{signer: signing.TokenOnly(f.signer)}).Keys
	verifier := &devtoken.Verifier{Keys: keys, Issuer: testIssuer, Audience: testIssuer}
	nonces, err := dpop.NewNonces(make([]byte, 32), 5*time.Minute, time.Now)
	if err != nil {
		b.Fatal(err)
	}
	proofs := &dpop.Verifier{Nonces: nonces}
	const url = "https://plux.example/gateway/v1/bank/accounts"
	proof := devicetest.SignProof(b, key, devicetest.Proof{Method: "GET", URL: url, AccessToken: token, Nonce: nonces.Current(), IssuedAt: now})
	replay := dpop.NewReplay(cache.NewMemory(nil), dpop.FailClosed, 1<<16, time.Now, nil)
	ctx := context.Background()

	samples := make([]time.Duration, 0, b.N)
	i := 0
	for b.Loop() {
		i++
		start := time.Now()
		claims, err := verifier.Verify(ctx, token, false)
		if err != nil {
			b.Fatal(err)
		}
		p, err := proofs.Verify(proof, dpop.Expect{Method: "GET", URL: url, AccessToken: token, JKT: claims.JKT, Window: time.Minute})
		if err != nil {
			b.Fatal(err)
		}
		// Each request carries a fresh identifier; the proof is verified
		// as it is, and the replay check sees a new one every time.
		if _, err := replay.Check(ctx, p.JKT, strconv.Itoa(i), 3*time.Minute); err != nil {
			b.Fatal(err)
		}
		samples = append(samples, time.Since(start))
	}
	p95(b, "verify-p95-ms", samples)
}

// BenchmarkConfigOverhead measures what server-held configuration adds to
// a gateway call (DAT-092, ADR-0061, NFR-025): the check of the epoch,
// which is one read of the cache store, and the whole authorisation of a
// call that binds a value into its body.
//
// epoch-check-p95-ms is the time of Snapshot on a warm replica. The cache
// store here is in memory; against Valkey it adds the network round trip of
// one GET, which the budget of 0.2 ms p95 allows for on a local network.
// bound-p95-ms is Begin and Finish of an operation with a JSON Pointer
// binding, and unbound-p95-ms the same for one without, so their
// difference is the cost of the binding itself.
func BenchmarkConfigOverhead(b *testing.B) {
	f := newFixture(b)
	src := f.withConfig()
	f.gw.o.Count = nil // the benchmark sends far more than a device may
	src.set(b, bound("poolAccount", "POOL-42"))
	ctx := context.Background()
	body := []byte(`{"amount":"20.00","note":"rent"}`)

	b.Run("epoch-check", func(b *testing.B) {
		samples := make([]time.Duration, 0, b.N)
		for b.Loop() {
			start := time.Now()
			if _, err := f.config.Snapshot(ctx, f.dev.OrganizationID, f.dev.AppID, f.dev.EnvironmentID); err != nil {
				b.Fatal(err)
			}
			samples = append(samples, time.Since(start))
		}
		p95(b, "epoch-check-p95-ms", samples)
	})

	for _, tc := range []struct {
		name, metric string
		req          Request
		body         []byte
	}{
		{"unbound", "unbound-p95-ms", Request{Upstream: "bank", Operation: "balance", Method: "GET", Target: "/accounts/1"}, nil},
		{"bound", "bound-p95-ms", Request{Upstream: "bank", Operation: "pay", Method: "POST", Target: "/payments", Header: http.Header{"Content-Type": {"application/json"}}}, body},
	} {
		b.Run(tc.name, func(b *testing.B) {
			samples := make([]time.Duration, 0, b.N)
			for b.Loop() {
				start := time.Now()
				p, err := f.gw.Begin(ctx, f.dev, tc.req)
				if err != nil {
					b.Fatal(err)
				}
				if _, err := p.Finish(ctx, tc.body); err != nil {
					b.Fatal(err)
				}
				samples = append(samples, time.Since(start))
			}
			p95(b, tc.metric, samples)
		})
	}
}
