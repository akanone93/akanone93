// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Package webhook signs and verifies messages as the Standard Webhooks
// specification 1.0.0 describes (SRV-063, FN-025, ADR-0081).
//
// A message carries three headers. webhook-id is stable across retries, so
// a receiver can de-duplicate; webhook-timestamp is the time of the attempt,
// in Unix seconds, so a captured message cannot be replayed later; and
// webhook-signature lists "v1,<base64>" signatures, space separated, of
// HMAC-SHA256 over "id.timestamp.body" with a secret. A secret is
// "whsec_" followed by the base64 of its key. The list carries one signature
// per valid secret, which is how a secret is rotated with an overlap: both
// the old and the new secret verify until the old one expires.
package webhook

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// The headers of a message.
const (
	HeaderID        = "webhook-id"
	HeaderTimestamp = "webhook-timestamp"
	HeaderSignature = "webhook-signature"
)

// SecretPrefix starts every secret, so that one is recognisable in a
// configuration file or a leak scan.
const SecretPrefix = "whsec_"

// signatureVersion is the identifier of a symmetric signature.
const signatureVersion = "v1"

// secretBytes is the length of a generated key.
const secretBytes = 32

// DefaultTolerance is how far a message's timestamp may be from the
// receiver's clock.
const DefaultTolerance = 5 * time.Minute

// NewSecret returns a new random secret.
func NewSecret() (string, error) {
	key := make([]byte, secretBytes)
	if _, err := rand.Read(key); err != nil {
		return "", fmt.Errorf("webhook: generate a secret: %w", err)
	}
	return SecretPrefix + base64.StdEncoding.EncodeToString(key), nil
}

// key decodes a secret. The prefix is optional, as in the reference
// libraries, so a bare base64 key works too.
func key(secret string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(secret, SecretPrefix))
	if err != nil || len(raw) == 0 {
		return nil, plxerr.New(plxerr.ChannelMisconfigured, "a webhook secret is %s followed by base64 text", SecretPrefix)
	}
	return raw, nil
}

// Sign returns the v1 signature of a message, as "v1,<base64>".
func Sign(secret, id string, at time.Time, body []byte) (string, error) {
	k, err := key(secret)
	if err != nil {
		return "", err
	}
	return signatureVersion + "," + base64.StdEncoding.EncodeToString(mac(k, id, at.Unix(), body)), nil
}

func mac(k []byte, id string, unix int64, body []byte) []byte {
	h := hmac.New(sha256.New, k)
	h.Write([]byte(id))
	h.Write([]byte{'.'})
	h.Write([]byte(strconv.FormatInt(unix, 10)))
	h.Write([]byte{'.'})
	h.Write(body)
	return h.Sum(nil)
}

// Headers returns the three headers of a message, signed by every secret
// given: more than one while a secret is being rotated.
func Headers(secrets []string, id string, at time.Time, body []byte) (http.Header, error) {
	if len(secrets) == 0 {
		return nil, plxerr.New(plxerr.ChannelMisconfigured, "a webhook needs a secret to sign with")
	}
	sigs := make([]string, 0, len(secrets))
	for _, s := range secrets {
		sig, err := Sign(s, id, at, body)
		if err != nil {
			return nil, err
		}
		sigs = append(sigs, sig)
	}
	h := http.Header{}
	h.Set(HeaderID, id)
	h.Set(HeaderTimestamp, strconv.FormatInt(at.Unix(), 10))
	h.Set(HeaderSignature, strings.Join(sigs, " "))
	return h, nil
}

// Message is what a receiver verified: the identifier to de-duplicate on and
// the attempt's time.
type Message struct {
	ID string
	At time.Time
}

// Verify checks a received message against the secrets that are valid now.
// A malformed or missing header, a wrong signature and an unknown version are
// PLX-7151; a timestamp outside the tolerance is PLX-7152, because that is
// what a replayed capture looks like. The comparison is constant time, and
// the signature is checked before the timestamp so that a forger learns
// nothing about the clock.
func Verify(secrets []string, h http.Header, body []byte, now time.Time, tolerance time.Duration) (Message, error) {
	id, ts, sigs := h.Get(HeaderID), h.Get(HeaderTimestamp), h.Get(HeaderSignature)
	if id == "" || ts == "" || sigs == "" {
		return Message{}, plxerr.New(plxerr.WebhookSignatureInvalid, "the message lacks %s, %s or %s", HeaderID, HeaderTimestamp, HeaderSignature)
	}
	unix, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return Message{}, plxerr.New(plxerr.WebhookSignatureInvalid, "the timestamp is not a number of seconds")
	}
	if !matches(secrets, id, unix, body, sigs) {
		return Message{}, plxerr.New(plxerr.WebhookSignatureInvalid, "the signature does not match")
	}
	at := time.Unix(unix, 0).UTC()
	if tolerance <= 0 {
		tolerance = DefaultTolerance
	}
	if d := now.Sub(at); d > tolerance || d < -tolerance {
		return Message{}, plxerr.New(plxerr.WebhookReplayed, "the timestamp is outside the tolerance of %s", tolerance)
	}
	return Message{ID: id, At: at}, nil
}

// matches reports whether any presented v1 signature was made by any secret.
func matches(secrets []string, id string, unix int64, body []byte, header string) bool {
	var presented [][]byte
	for _, part := range strings.Fields(header) {
		version, value, ok := strings.Cut(part, ",")
		if !ok || version != signatureVersion {
			continue
		}
		if raw, err := base64.StdEncoding.DecodeString(value); err == nil {
			presented = append(presented, raw)
		}
	}
	found := false
	for _, s := range secrets {
		k, err := key(s)
		if err != nil {
			continue
		}
		want := mac(k, id, unix, body)
		for _, got := range presented {
			// No early exit: the time taken does not depend on which
			// secret or signature matched.
			if hmac.Equal(want, got) {
				found = true
			}
		}
	}
	return found
}
