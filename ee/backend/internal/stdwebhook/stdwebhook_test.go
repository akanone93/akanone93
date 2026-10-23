// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package stdwebhook

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

// vectors are the Standard Webhooks test vectors, copied unmodified from
// backend/internal/notify/webhook/testdata; the file names its source.
type vectors struct {
	Secret            string   `json:"secret"`
	ID                string   `json:"id"`
	Timestamp         int64    `json:"timestamp"`
	Payload           string   `json:"payload"`
	Signature         string   `json:"signature"`
	InvalidSignatures []string `json:"invalidSignatures"`
	ValidMultiple     string   `json:"validMultiple"`
}

func loadVectors(t *testing.T) vectors {
	t.Helper()
	raw, err := os.ReadFile("testdata/standard_webhooks_vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var v vectors
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

// Verifies: SEC-143, ANL-011.
// Sign reproduces the published signature, none of the published invalid
// ones matches, and the published valid list holds it.
func TestSign_Vectors_ANL_011(t *testing.T) {
	t.Parallel()
	v := loadVectors(t)
	key, err := ParseSecret(v.Secret)
	if err != nil {
		t.Fatal(err)
	}
	got := Sign(key, v.ID, time.Unix(v.Timestamp, 0), []byte(v.Payload))
	if got != v.Signature {
		t.Errorf("Sign = %q, want %q", got, v.Signature)
	}
	for _, bad := range v.InvalidSignatures {
		if bad == got {
			t.Errorf("the invalid signature %q matches", bad)
		}
	}
	if !strings.Contains(" "+v.ValidMultiple+" ", " "+got+" ") {
		t.Errorf("validMultiple %q holds no signature for the payload", v.ValidMultiple)
	}
}

// Verifies: SEC-143, ANL-011.
// Headers carries the id, the timestamp in seconds and the signature.
func TestHeaders_ANL_011(t *testing.T) {
	t.Parallel()
	v := loadVectors(t)
	key, err := ParseSecret(v.Secret)
	if err != nil {
		t.Fatal(err)
	}
	h := Headers(key, v.ID, time.Unix(v.Timestamp, 0), []byte(v.Payload))
	if h.Get("Webhook-Id") != v.ID || h.Get("Webhook-Timestamp") != "1614265330" || h.Get("Webhook-Signature") != v.Signature {
		t.Errorf("Headers = %v", h)
	}
}

// Verifies: SEC-143.
// A malformed secret is refused without being quoted.
func TestParseSecret_Refuses_SEC_143(t *testing.T) {
	t.Parallel()
	for _, s := range []string{"", "MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw", "whsec_", "whsec_!!!", "whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw "} {
		_, err := ParseSecret(s)
		if err == nil {
			t.Errorf("ParseSecret(%q) accepted the secret", s)
		} else if rest := strings.TrimPrefix(s, SecretPrefix); rest != "" && strings.Contains(err.Error(), rest) {
			t.Errorf("the error quotes the secret: %v", err)
		}
	}
}
