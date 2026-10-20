// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package auditexport

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/nightCode42/plux3/backend/ext/export"
)

// otlpCollectorRequest is the OTLP shape a test reads back. It is written
// out independently of the sink's types, so the test checks the wire format.
type otlpCollectorRequest struct {
	ResourceLogs []struct {
		Resource struct {
			Attributes []otlpCollectorAttribute `json:"attributes"`
		} `json:"resource"`
		ScopeLogs []struct {
			Scope struct {
				Name string `json:"name"`
			} `json:"scope"`
			LogRecords []struct {
				TimeUnixNano string `json:"timeUnixNano"`
				SeverityText string `json:"severityText"`
				Body         struct {
					StringValue string `json:"stringValue"`
				} `json:"body"`
				Attributes []otlpCollectorAttribute `json:"attributes"`
			} `json:"logRecords"`
		} `json:"scopeLogs"`
	} `json:"resourceLogs"`
}

// otlpReceived is what the collector saw of one request.
type otlpReceived struct {
	method      string
	contentType string
	body        otlpCollectorRequest
}

// otlpCollectorAttribute is one key and its string value.
type otlpCollectorAttribute struct {
	Key   string `json:"key"`
	Value struct {
		StringValue string `json:"stringValue"`
	} `json:"value"`
}

// otlpSinkFor returns an OTLP sink that posts to the collector at url.
func otlpSinkFor(t *testing.T, url string) *otlpSink {
	t.Helper()
	sink, err := newOTLPSink(export.Config{
		Destination: export.Destination{Kind: KindOTLP, Address: url},
		HTTP:        &http.Client{},
		Hostname:    "plux-1",
		Version:     "1.0.0",
		Now:         fixedClock,
	})
	if err != nil {
		t.Fatal(err)
	}
	return sink
}

// Verifies: SEC-143.
// A batch is one OTLP/HTTP JSON export: one resource, one scope and one log
// record per entry, with the entry's time, its JSON as the body and its
// fields as attributes.
func TestOTLPPayload_SEC_143(t *testing.T) {
	t.Parallel()
	received := make(chan otlpReceived, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var got otlpCollectorRequest
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("the body is not JSON: %v", err)
		}
		received <- otlpReceived{method: r.Method, contentType: r.Header.Get("Content-Type"), body: got}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	records := sampleRecords(2)
	if err := otlpSinkFor(t, srv.URL).Deliver(licensedCtx(), records); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	rec := <-received
	got := rec.body
	if rec.method != http.MethodPost || rec.contentType != "application/json" {
		t.Errorf("request = %s %s, want a POST of application/json", rec.method, rec.contentType)
	}
	if len(got.ResourceLogs) != 1 || len(got.ResourceLogs[0].ScopeLogs) != 1 {
		t.Fatalf("resourceLogs = %+v, want one resource with one scope", got.ResourceLogs)
	}
	group := got.ResourceLogs[0]
	if group.ScopeLogs[0].Scope.Name != otlpScopeName {
		t.Errorf("scope = %q, want %q", group.ScopeLogs[0].Scope.Name, otlpScopeName)
	}
	logs := group.ScopeLogs[0].LogRecords
	if len(logs) != len(records) {
		t.Fatalf("logRecords = %d, want %d", len(logs), len(records))
	}
	first := logs[0]
	if want := strconv.FormatInt(records[0].At.UnixNano(), 10); first.TimeUnixNano != want {
		t.Errorf("timeUnixNano = %s, want %s", first.TimeUnixNano, want)
	}
	if first.SeverityText != otlpSeverityText {
		t.Errorf("severityText = %q, want %q", first.SeverityText, otlpSeverityText)
	}
	if !strings.Contains(first.Body.StringValue, `"sequence":1`) || !strings.Contains(first.Body.StringValue, `"entryHash":"hash-1"`) {
		t.Errorf("body = %s, want the record's JSON", first.Body.StringValue)
	}
	if !hasAttribute(first.Attributes, "plux.action", "team.created") || !hasAttribute(first.Attributes, "plux.sequence", "1") {
		t.Errorf("attributes = %+v, want the action and the sequence", first.Attributes)
	}
	if !hasAttribute(group.Resource.Attributes, "service.version", "1.0.0") {
		t.Errorf("resource attributes = %+v, want the service version", group.Resource.Attributes)
	}
}

// hasAttribute reports whether an attribute holds the key and the value.
func hasAttribute(attrs []otlpCollectorAttribute, key, value string) bool {
	for _, a := range attrs {
		if a.Key == key && a.Value.StringValue == value {
			return true
		}
	}
	return false
}

// Verifies: SEC-143.
// A collector that does not answer with a success status fails the batch,
// so the core keeps the destination's cursor where it was.
func TestOTLPCollectorRefusalFails_SEC_143(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)
	if err := otlpSinkFor(t, srv.URL).Deliver(licensedCtx(), sampleRecords(1)); err == nil {
		t.Error("Deliver accepted a 503 answer")
	}
}

// Verifies: SEC-143, GOV-030.
// Without a licence the OTLP sink refuses the write with PLX-8600 and sends
// nothing.
func TestOTLPRefusedWithoutLicence_SEC_143(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("the sink posted without a licence")
	}))
	t.Cleanup(srv.Close)
	err := otlpSinkFor(t, srv.URL).Deliver(licenceCtx("expired"), sampleRecords(1))
	if err == nil || !strings.Contains(err.Error(), "PLX-8600") {
		t.Errorf("Deliver with an expired licence = %v, want PLX-8600", err)
	}
}
