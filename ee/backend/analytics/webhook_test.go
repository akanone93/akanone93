// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package analytics

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/edition"
	"github.com/nightCode42/plux3/ee/backend/internal/stdwebhook"
)

// receivedMessage is a message the test receiver got.
type receivedMessage struct {
	header http.Header
	body   []byte
}

// receiver is a TLS webhook receiver that records messages and answers with
// a status the test sets.
type receiver struct {
	srv      *httptest.Server
	mu       sync.Mutex
	messages []receivedMessage
	status   int
}

func newReceiver(t *testing.T, status int) *receiver {
	t.Helper()
	r := &receiver{status: status}
	r.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Errorf("read the body: %v", err)
		}
		r.mu.Lock()
		defer r.mu.Unlock()
		r.messages = append(r.messages, receivedMessage{header: req.Header.Clone(), body: body})
		w.WriteHeader(r.status)
	}))
	t.Cleanup(r.srv.Close)
	return r
}

// vectorSecret is the secret of the published Standard Webhooks vectors.
func vectorSecret(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("../internal/stdwebhook/testdata/standard_webhooks_vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Secret string `json:"secret"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	return v.Secret
}

func sampleRecords(n int) []Record {
	out := make([]Record, n)
	for i := range out {
		out[i] = Record{
			ID: fmt.Sprintf("0197f000-0000-7000-8000-%012d", i+1), OrganizationID: testOrg, AppID: testApp, EnvironmentID: testEnv,
			DeviceID: testDevice, Name: "screen_view", Time: time.Date(2026, 10, 20, 10, i, 0, 0, time.UTC),
			ReleaseSequence: 3, Route: "/home", Category: "analytics", DeviceClass: "phone", Locale: "de-DE",
			Fields: json.RawMessage(`{"duration_ms":12}`),
		}
	}
	return out
}

func webhookFor(t *testing.T, r *receiver, batch int) *WebhookExporter {
	t.Helper()
	w, err := NewWebhookExporter(WebhookConfig{
		Client: r.srv.Client(), URL: r.srv.URL, Secret: vectorSecret(t), BatchSize: batch,
		Now: func() time.Time { return time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	return w
}

// Verifies: ANL-011, SEC-143.
// Events go out in messages of at most the batch size, each with the three
// Standard Webhooks headers, and the signature is the one the published
// algorithm gives for the id, timestamp and body that were sent.
func TestWebhookExport_SignsBatches_ANL_011(t *testing.T) {
	t.Parallel()
	r := newReceiver(t, http.StatusNoContent)
	key, err := stdwebhook.ParseSecret(vectorSecret(t))
	if err != nil {
		t.Fatal(err)
	}
	ctx := licensed(edition.StateValid, edition.FeatureWebhookExport)
	if err := webhookFor(t, r, 2).Export(ctx, testOrg, sampleRecords(5)); err != nil {
		t.Fatal(err)
	}
	if len(r.messages) != 3 {
		t.Fatalf("%d messages, want 3 batches of 2, 2 and 1 events", len(r.messages))
	}
	at := time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)
	ids := map[string]bool{}
	total := 0
	for _, m := range r.messages {
		id := m.header.Get("Webhook-Id")
		ids[id] = true
		if m.header.Get("Webhook-Timestamp") != strconv.FormatInt(at.Unix(), 10) {
			t.Errorf("webhook-timestamp = %q", m.header.Get("Webhook-Timestamp"))
		}
		if want := stdwebhook.Sign(key, id, at, m.body); m.header.Get("Webhook-Signature") != want {
			t.Errorf("webhook-signature = %q, want %q", m.header.Get("Webhook-Signature"), want)
		}
		if !strings.HasPrefix(m.header.Get("Webhook-Signature"), "v1,") {
			t.Errorf("signature %q lacks the v1 tag", m.header.Get("Webhook-Signature"))
		}
		var env webhookEnvelope
		if err := json.Unmarshal(m.body, &env); err != nil {
			t.Fatal(err)
		}
		if env.Type != "plux.analytics.events" || env.Data.OrganizationID != testOrg || len(env.Data.Events) == 0 {
			t.Errorf("envelope = %+v", env)
		}
		total += len(env.Data.Events)
	}
	if total != 5 || len(ids) != 3 {
		t.Errorf("%d events in %d distinct messages, want 5 in 3", total, len(ids))
	}
}

// Verifies: ANL-011.
// The message identifier follows from the events, so a redelivered batch is
// recognisable as a duplicate, and differs between batches.
func TestWebhookExport_MessageIDIsStable_ANL_011(t *testing.T) {
	t.Parallel()
	a, b := messageID(sampleRecords(2)), messageID(sampleRecords(2))
	if a != b || a == messageID(sampleRecords(3)) || !strings.HasPrefix(a, "msg_") {
		t.Errorf("message ids: %q, %q", a, messageID(sampleRecords(3)))
	}
}

// Verifies: ANL-011.
// A receiver that refuses a message stops the export there with an error
// that names the status and not the address.
func TestWebhookExport_StopsAtRefusal_ANL_011(t *testing.T) {
	t.Parallel()
	r := newReceiver(t, http.StatusInternalServerError)
	ctx := licensed(edition.StateValid, edition.FeatureWebhookExport)
	err := webhookFor(t, r, 1).Export(ctx, testOrg, sampleRecords(3))
	if err == nil || !strings.Contains(err.Error(), "500") || strings.Contains(err.Error(), r.srv.URL) {
		t.Fatalf("Export = %v", err)
	}
	if len(r.messages) != 1 {
		t.Errorf("%d messages were sent after the refusal, want 1 in all", len(r.messages))
	}
}

// Verifies: ANL-011, GOV-030.
// Without a licence that lists webhook export for writing, nothing is sent
// and the error is PLX-8600.
func TestWebhookExport_RefusedWithoutLicence_ANL_011(t *testing.T) {
	t.Parallel()
	r := newReceiver(t, http.StatusNoContent)
	w := webhookFor(t, r, 10)
	for name, ctx := range map[string]context.Context{
		"none":    context.Background(),
		"other":   licensed(edition.StateValid, edition.FeatureParquetExport),
		"expired": licensed(edition.StateExpired, edition.FeatureWebhookExport),
		"invalid": licensed(edition.StateInvalid, edition.FeatureWebhookExport),
	} {
		err := w.Export(ctx, testOrg, sampleRecords(1))
		if err == nil || !strings.Contains(err.Error(), "PLX-8600") {
			t.Errorf("%s: Export = %v, want PLX-8600", name, err)
		}
	}
	if len(r.messages) != 0 {
		t.Errorf("%d messages were sent", len(r.messages))
	}
}

// Verifies: ANL-011, SEC-143.
// A configuration the exporter cannot use is refused when it is made, and
// the error does not quote the secret.
func TestNewWebhookExporter_Refuses_ANL_011(t *testing.T) {
	t.Parallel()
	good := WebhookConfig{Client: &http.Client{}, URL: "https://hooks.example.com/plux", Secret: vectorSecret(t), BatchSize: 10, Now: time.Now}
	mutations := map[string]func(c *WebhookConfig){
		"no client":     func(c *WebhookConfig) { c.Client = nil },
		"no clock":      func(c *WebhookConfig) { c.Now = nil },
		"no batch size": func(c *WebhookConfig) { c.BatchSize = 0 },
		"http":          func(c *WebhookConfig) { c.URL = "http://hooks.example.com/plux" },
		"no host":       func(c *WebhookConfig) { c.URL = "https://" },
		"bad secret":    func(c *WebhookConfig) { c.Secret = "whsec_%%%s3cret" },
	}
	for name, mutate := range mutations {
		c := good
		mutate(&c)
		_, err := NewWebhookExporter(c)
		if err == nil {
			t.Errorf("%s: the configuration was accepted", name)
		} else if strings.Contains(err.Error(), "s3cret") {
			t.Errorf("%s: the error quotes the secret: %v", name, err)
		}
	}
	if _, err := NewWebhookExporter(good); err != nil {
		t.Errorf("the good configuration was refused: %v", err)
	}
}

// Verifies: ANL-011.
// A hostile organisation identifier is refused before anything is sent.
func TestWebhookExport_HostileOrganization_ANL_011(t *testing.T) {
	t.Parallel()
	r := newReceiver(t, http.StatusNoContent)
	ctx := licensed(edition.StateValid, edition.FeatureWebhookExport)
	if err := webhookFor(t, r, 10).Export(ctx, hostile, sampleRecords(1)); err == nil || len(r.messages) != 0 {
		t.Errorf("Export = %v with %d messages", err, len(r.messages))
	}
}
