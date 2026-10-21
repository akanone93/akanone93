// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package analytics

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/nightCode42/plux3/backend/edition"
	"github.com/nightCode42/plux3/ee/backend/internal/stdwebhook"
)

const (
	// webhookEventType is the type of every delivered message.
	webhookEventType = "plux.analytics.events"
	// messageIDHexLen is how many hexadecimal digits of the batch digest
	// make the message identifier.
	messageIDHexLen = 32
	// maxResponseBytes bounds how much of a receiver's answer is read; the
	// status is what matters.
	maxResponseBytes = 64 << 10
)

// WebhookConfig configures a WebhookExporter.
type WebhookConfig struct {
	// Client is the core's HTTP client: SSRF-safe and registered with the
	// egress policy. The exporter opens no connection of its own.
	Client *http.Client
	// URL is the https address of the receiver.
	URL string
	// Secret is the Standard Webhooks signing secret, "whsec_" and base64.
	Secret string
	// BatchSize is the most events in one message; it comes from the
	// installation's configuration, not from this package.
	BatchSize int
	// Now is the clock that stamps messages.
	Now func() time.Time
}

// WebhookExporter posts batches of events to a URL as signed Standard
// Webhooks messages (ANL-011). The message identifier is a digest of the
// event identifiers it carries, so a receiver that sees a batch again after
// a failed attempt can drop the duplicate.
type WebhookExporter struct {
	client    *http.Client
	endpoint  string
	key       []byte
	batchSize int
	now       func() time.Time
}

// NewWebhookExporter validates a configuration and returns the exporter.
func NewWebhookExporter(c WebhookConfig) (*WebhookExporter, error) {
	if c.Client == nil || c.Now == nil {
		return nil, errors.New("analytics: a webhook exporter needs an HTTP client and a clock")
	}
	if c.BatchSize <= 0 {
		return nil, errors.New("analytics: a webhook exporter needs a positive batch size")
	}
	if u, err := url.Parse(c.URL); err != nil || u.Scheme != "https" || u.Host == "" {
		return nil, errors.New("analytics: a webhook address is an https URL")
	}
	key, err := stdwebhook.ParseSecret(c.Secret)
	if err != nil {
		return nil, fmt.Errorf("analytics: the webhook secret: %w", err)
	}
	return &WebhookExporter{client: c.Client, endpoint: c.URL, key: key, batchSize: c.BatchSize, now: c.Now}, nil
}

// webhookEnvelope is the body of a message.
type webhookEnvelope struct {
	Type      string      `json:"type"`
	Timestamp time.Time   `json:"timestamp"`
	Data      webhookData `json:"data"`
}

// webhookData is the payload of a message.
type webhookData struct {
	OrganizationID string   `json:"organization_id"`
	Events         []Record `json:"events"`
}

// Export posts the events in messages of at most BatchSize events, in order,
// and stops at the first message the receiver does not accept; the caller
// redelivers from there. It refuses the write without a licence that lists
// edition.FeatureWebhookExport.
func (w *WebhookExporter) Export(ctx context.Context, org string, events []Record) error {
	if err := requireFeature(ctx, edition.FeatureWebhookExport); err != nil {
		return err
	}
	org, err := canonicalUUID(org)
	if err != nil {
		return fmt.Errorf("analytics: export events: the organisation: %w", err)
	}
	for start := 0; start < len(events); start += w.batchSize {
		end := min(start+w.batchSize, len(events))
		if err := w.post(ctx, org, events[start:end]); err != nil {
			return err
		}
	}
	return nil
}

// post sends one signed message.
func (w *WebhookExporter) post(ctx context.Context, org string, batch []Record) error {
	at := w.now()
	body, err := json.Marshal(webhookEnvelope{
		Type: webhookEventType, Timestamp: at.UTC(), Data: webhookData{OrganizationID: org, Events: batch},
	})
	if err != nil {
		return fmt.Errorf("analytics: encode the webhook message: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.endpoint, bytes.NewReader(body))
	if err != nil {
		return errors.New("analytics: build the webhook request")
	}
	req.Header.Set("Content-Type", "application/json")
	for name, values := range stdwebhook.Headers(w.key, messageID(batch), at, body) {
		req.Header[name] = values
	}
	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("analytics: post to the webhook: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBytes))
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("analytics: the webhook answered %d", resp.StatusCode)
	}
	return nil
}

// messageID derives the identifier of a message from the events it carries.
func messageID(batch []Record) string {
	h := sha256.New()
	for _, e := range batch {
		h.Write([]byte(e.ID))
		h.Write([]byte{0})
	}
	return "msg_" + hex.EncodeToString(h.Sum(nil))[:messageIDHexLen]
}

// requireFeature refuses a write of a feature the licence does not list for
// writing. The error carries PLX-8600 (GOV-030).
func requireFeature(ctx context.Context, f edition.Feature) error {
	if edition.EnabledForWrite(ctx, f) {
		return nil
	}
	return edition.Require(ctx, f) //nolint:wrapcheck // carries PLX-8600
}
