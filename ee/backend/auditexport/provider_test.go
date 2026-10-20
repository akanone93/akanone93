// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package auditexport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/nightCode42/plux3/backend/edition"
	"github.com/nightCode42/plux3/backend/ext/export"
)

// Verifies: SEC-143.
// The provider serves the three enterprise kinds and validates each kind's
// address: CEF needs host:port, OTLP needs an https URL, and a kind it does
// not serve is refused.
func TestProviderValidate_SEC_143(t *testing.T) {
	t.Parallel()
	p := Provider{}
	if got := p.Kinds(); len(got) != 3 {
		t.Fatalf("Kinds() = %v, want cef, otlp and webhook", got)
	}
	secret := []byte(loadWebhookVectors(t).Secret)
	cases := []struct {
		name    string
		kind    string
		address string
		valid   bool
	}{
		{"CEF host and port", KindCEF, "siem.example.com:6514", true},
		{"CEF without a port", KindCEF, "siem.example.com", false},
		{"CEF with an empty host", KindCEF, ":6514", false},
		{"OTLP https URL", KindOTLP, "https://collector.example.com/v1/logs", true},
		{"OTLP plain http", KindOTLP, "http://collector.example.com/v1/logs", false},
		{"OTLP not a URL", KindOTLP, "collector", false},
		{"webhook https URL", KindWebhook, "https://hooks.example.com/plux", true},
		{"an unserved kind", "syslog-tls-v2", "siem.example.com:6514", false},
	}
	for _, tc := range cases {
		err := p.Validate(export.Destination{Kind: tc.kind, Address: tc.address}, secret)
		if (err == nil) != tc.valid {
			t.Errorf("%s: Validate = %v, want valid %t", tc.name, err, tc.valid)
		}
	}
}

// Verifies: SEC-143, GOV-030.
// The sinks write only while the licence lists the SIEM feature for
// writing: a valid licence and one in grace may write, an expired licence
// may not, and a licence without the feature may not.
func TestSinksWriteOnlyWhileLicensed_SEC_143(t *testing.T) {
	t.Parallel()
	var posts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posts.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	sink, err := newOTLPSink(export.Config{
		Destination: export.Destination{Kind: KindOTLP, Address: srv.URL},
		HTTP:        srv.Client(),
		Now:         fixedClock,
	})
	if err != nil {
		t.Fatal(err)
	}
	records := sampleRecords(1)
	for _, state := range []edition.State{edition.StateValid, edition.StateGrace} {
		if err := sink.Deliver(licenceCtx(state), records); err != nil {
			t.Errorf("state %s: Deliver = %v, want a write", state, err)
		}
	}
	unlicensed := edition.WithProvider(context.Background(), fixedLicence{status: edition.Status{State: edition.StateValid}})
	for name, ctx := range map[string]context.Context{"expired": licenceCtx(edition.StateExpired), "unlisted": unlicensed} {
		err := sink.Deliver(ctx, records)
		if err == nil || !strings.Contains(err.Error(), "PLX-8600") {
			t.Errorf("%s: Deliver = %v, want PLX-8600", name, err)
		}
	}
	if n := posts.Load(); n != 2 {
		t.Errorf("posts = %d, want only the two licensed deliveries", n)
	}
}
