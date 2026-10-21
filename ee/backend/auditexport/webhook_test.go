// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package auditexport

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/ext/export"
	"github.com/nightCode42/plux3/ee/backend/internal/stdwebhook"
)

// webhookVectors are the Standard Webhooks test vectors, copied unmodified
// from backend/internal/notify/webhook/testdata. Their source is named in
// the file.
type webhookVectors struct {
	Secret            string   `json:"secret"`
	ID                string   `json:"id"`
	Timestamp         int64    `json:"timestamp"`
	Payload           string   `json:"payload"`
	Signature         string   `json:"signature"`
	InvalidSignatures []string `json:"invalidSignatures"`
	ValidMultiple     string   `json:"validMultiple"`
}

// loadWebhookVectors reads the vectors from testdata.
func loadWebhookVectors(t *testing.T) webhookVectors {
	t.Helper()
	raw, err := os.ReadFile("testdata/standard_webhooks_vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var v webhookVectors
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

// webhookSinkFor returns a webhook sink that posts to the receiver at url,
// signed with secret.
func webhookSinkFor(t *testing.T, url, secret string) *webhookSink {
	t.Helper()
	sink, err := newWebhookSink(export.Config{
		Destination: export.Destination{Kind: KindWebhook, Address: url},
		Secret:      []byte(secret),
		HTTP:        &http.Client{},
		Now:         fixedClock,
	})
	if err != nil {
		t.Fatal(err)
	}
	return sink
}

// Verifies: SEC-143.
// The signer reproduces the published Standard Webhooks signature, rejects
// the published invalid ones, and accepts the published valid list.
func TestWebhookSignatureVectors_SEC_143(t *testing.T) {
	t.Parallel()
	v := loadWebhookVectors(t)
	key, err := stdwebhook.ParseSecret(v.Secret)
	if err != nil {
		t.Fatal(err)
	}
	got := stdwebhook.Sign(key, v.ID, time.Unix(v.Timestamp, 0), []byte(v.Payload))
	if got != v.Signature {
		t.Errorf("sign = %q, want %q", got, v.Signature)
	}
	for _, bad := range v.InvalidSignatures {
		if bad == got {
			t.Errorf("the invalid signature %q matches", bad)
		}
	}
	matched := false
	for _, sig := range strings.Fields(v.ValidMultiple) {
		if sig == got {
			matched = true
		}
	}
	if !matched {
		t.Errorf("validMultiple %q holds no signature for the payload", v.ValidMultiple)
	}
}

// Verifies: SEC-143.
// Each message carries the Standard Webhooks headers, and the signature in
// webhook-signature is the signature of the id, the timestamp and the body
// it was sent with.
func TestWebhookSinkSignsMessages_SEC_143(t *testing.T) {
	t.Parallel()
	v := loadWebhookVectors(t)
	key, err := stdwebhook.ParseSecret(v.Secret)
	if err != nil {
		t.Fatal(err)
	}
	var messages atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read the body: %v", err)
		}
		id := r.Header.Get("Webhook-Id")
		at := sampleAt()
		if ts := r.Header.Get("Webhook-Timestamp"); ts != strconv.FormatInt(at.Unix(), 10) {
			t.Errorf("webhook-timestamp = %q, want the sink's clock", ts)
		}
		if want := stdwebhook.Sign(key, id, at, body); r.Header.Get("Webhook-Signature") != want {
			t.Errorf("webhook-signature = %q, want %q", r.Header.Get("Webhook-Signature"), want)
		}
		if !strings.HasPrefix(id, "msg_rec-") {
			t.Errorf("webhook-id = %q, want the record's id", id)
		}
		messages.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)

	if err := webhookSinkFor(t, srv.URL, v.Secret).Deliver(licensedCtx(), sampleRecords(2)); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if n := messages.Load(); n != 2 {
		t.Errorf("messages = %d, want one per record", n)
	}
}

// Verifies: SEC-143.
// A secret must be whsec_ followed by the base64 of a non-empty key, and a
// webhook address must be https, so a destination cannot be registered
// with a secret the signer cannot use.
func TestWebhookSecretRefused_SEC_143(t *testing.T) {
	t.Parallel()
	bad := []string{"", "MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw", "whsec_", "whsec_!!!", "whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw "}
	for _, secret := range bad {
		if _, err := stdwebhook.ParseSecret(secret); err == nil {
			t.Errorf("stdwebhook.ParseSecret(%q) accepted the secret", secret)
		}
		d := export.Destination{Kind: KindWebhook, Address: "https://hooks.example.com/plux"}
		if err := (Provider{}).Validate(d, []byte(secret)); err == nil {
			t.Errorf("Validate accepted the webhook secret %q", secret)
		}
	}
	d := export.Destination{Kind: KindWebhook, Address: "http://hooks.example.com/plux"}
	if err := (Provider{}).Validate(d, []byte(loadWebhookVectors(t).Secret)); err == nil {
		t.Error("Validate accepted a webhook address that is not https")
	}
}

// Verifies: SEC-143, GOV-030.
// Without a licence the webhook sink refuses the write with PLX-8600 and
// posts nothing.
func TestWebhookRefusedWithoutLicence_SEC_143(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("the sink posted without a licence")
	}))
	t.Cleanup(srv.Close)
	err := webhookSinkFor(t, srv.URL, loadWebhookVectors(t).Secret).Deliver(context.Background(), sampleRecords(1))
	if err == nil || !strings.Contains(err.Error(), "PLX-8600") {
		t.Errorf("Deliver without a licence = %v, want PLX-8600", err)
	}
}
