// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package stdwebhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	// SecretPrefix opens a signing secret, which is the prefix followed by
	// the base64 of the key.
	SecretPrefix = "whsec_"
	// signatureVersion is the version tag of the webhook-signature header.
	signatureVersion = "v1"
)

// ErrSecret says a signing secret is malformed. It never quotes the secret.
var ErrSecret = errors.New("stdwebhook: a webhook secret is " + SecretPrefix + " followed by base64 text")

// ParseSecret decodes a signing secret into its key. It must be "whsec_"
// followed by the base64 of a non-empty key, as the Standard Webhooks
// libraries write it.
func ParseSecret(secret string) ([]byte, error) {
	if !strings.HasPrefix(secret, SecretPrefix) {
		return nil, ErrSecret
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(secret, SecretPrefix))
	if err != nil || len(key) == 0 {
		return nil, ErrSecret
	}
	return key, nil
}

// Sign returns the v1 signature of a message: the base64 of HMAC-SHA256 over
// "id.timestamp.body" with the key, written as "v1,<base64>".
func Sign(key []byte, id string, at time.Time, body []byte) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(id + "." + strconv.FormatInt(at.Unix(), 10) + "."))
	mac.Write(body)
	return signatureVersion + "," + base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// Headers returns the three headers that carry a signed message.
func Headers(key []byte, id string, at time.Time, body []byte) http.Header {
	return http.Header{
		"Webhook-Id":        []string{id},
		"Webhook-Timestamp": []string{strconv.FormatInt(at.Unix(), 10)},
		"Webhook-Signature": []string{Sign(key, id, at, body)},
	}
}
