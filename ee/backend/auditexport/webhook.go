// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package auditexport

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/nightCode42/plux3/backend/ext/export"
)

const (
	// secretPrefix opens a Standard Webhooks signing secret, which is the
	// prefix followed by the base64 of the key.
	secretPrefix = "whsec_"
	// signatureVersion is the version tag of the webhook-signature header.
	signatureVersion = "v1"
	// webhookEventType is the type of every delivered message.
	webhookEventType = "plux.audit.entry"
)

// webhookSink delivers each record as one Standard Webhooks message, signed
// with the destination's secret (SEC-143). The message id is derived from
// the record, so a receiver that sees a record again after a failed batch
// can drop the duplicate.
type webhookSink struct {
	client   *http.Client
	endpoint string
	key      []byte
	now      func() time.Time
}

// newWebhookSink returns the webhook sink of a destination.
func newWebhookSink(c export.Config) (*webhookSink, error) {
	if c.HTTP == nil || c.Now == nil {
		return nil, errors.New("auditexport: a webhook sink needs an HTTP client and a clock")
	}
	key, err := webhookKey(string(c.Secret))
	if err != nil {
		return nil, err
	}
	return &webhookSink{client: c.HTTP, endpoint: c.Destination.Address, key: key, now: c.Now}, nil
}

// Name identifies the sink kind.
func (*webhookSink) Name() string { return KindWebhook }

// Deliver posts each record of the batch in order. It refuses the write
// without a licence that lists the SIEM feature, and stops at the first
// record the receiver does not accept, so the core redelivers from the
// start of the batch.
func (s *webhookSink) Deliver(ctx context.Context, records []export.Record) error {
	if err := requireLicence(ctx); err != nil {
		return err
	}
	for _, r := range records {
		if err := s.post(ctx, r); err != nil {
			return err
		}
	}
	return nil
}

// post sends one record as a signed message.
func (s *webhookSink) post(ctx context.Context, r export.Record) error {
	data, err := recordJSON(r)
	if err != nil {
		return err
	}
	body, err := json.Marshal(webhookEnvelope{
		Type:      webhookEventType,
		Timestamp: r.At.UTC(),
		Data:      json.RawMessage(data),
	})
	if err != nil {
		return fmt.Errorf("auditexport: encode the webhook message: %w", err)
	}
	id := "msg_" + r.ID
	at := s.now()
	headers := http.Header{
		"Webhook-Id":        []string{id},
		"Webhook-Timestamp": []string{strconv.FormatInt(at.Unix(), 10)},
		"Webhook-Signature": []string{sign(s.key, id, at, body)},
	}
	return postJSON(ctx, s.client, s.endpoint, headers, body)
}

// webhookEnvelope is the body of a message: its type, the time it was sent
// and the record.
type webhookEnvelope struct {
	Type      string          `json:"type"`
	Timestamp time.Time       `json:"timestamp"`
	Data      json.RawMessage `json:"data"`
}

// webhookKey decodes a signing secret. It must be "whsec_" followed by the
// base64 of a non-empty key, as the Standard Webhooks libraries write it.
func webhookKey(secret string) ([]byte, error) {
	if !strings.HasPrefix(secret, secretPrefix) {
		return nil, fmt.Errorf("a webhook secret is %s followed by base64 text", secretPrefix)
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(secret, secretPrefix))
	if err != nil || len(key) == 0 {
		return nil, fmt.Errorf("a webhook secret is %s followed by base64 text", secretPrefix)
	}
	return key, nil
}

// sign returns the Standard Webhooks v1 signature of a message: the base64
// of HMAC-SHA256 over "id.timestamp.body", with the key, written as
// "v1,<base64>".
func sign(key []byte, id string, at time.Time, body []byte) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(id + "." + strconv.FormatInt(at.Unix(), 10) + "."))
	mac.Write(body)
	return signatureVersion + "," + base64.StdEncoding.EncodeToString(mac.Sum(nil))
}
