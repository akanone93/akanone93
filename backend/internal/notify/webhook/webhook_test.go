// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package webhook_test

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/notify/webhook"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

type vectors struct {
	Secret            string   `json:"secret"`
	ID                string   `json:"id"`
	Timestamp         int64    `json:"timestamp"`
	Payload           string   `json:"payload"`
	Signature         string   `json:"signature"`
	InvalidSignatures []string `json:"invalidSignatures"`
	ValidMultiple     string   `json:"validMultiple"`
}

func load(t *testing.T) vectors {
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

func code(err error) plxerr.Code {
	c, _ := plxerr.CodeOf(err)
	return c
}

// SRV-063: the signature matches the one the Standard Webhooks reference
// library publishes.
func TestSRV063PublishedSignatureVector(t *testing.T) {
	v := load(t)
	got, err := webhook.Sign(v.Secret, v.ID, time.Unix(v.Timestamp, 0), []byte(v.Payload))
	if err != nil {
		t.Fatal(err)
	}
	if got != v.Signature {
		t.Fatalf("signature = %s, want the published %s", got, v.Signature)
	}
	// The prefix is optional, as in the reference libraries.
	bare, err := webhook.Sign(strings.TrimPrefix(v.Secret, webhook.SecretPrefix), v.ID, time.Unix(v.Timestamp, 0), []byte(v.Payload))
	if err != nil || bare != v.Signature {
		t.Fatalf("a secret without its prefix signs differently: %s %v", bare, err)
	}
}

// SRV-063, FN-025: verification accepts the published message and refuses
// the published invalid ones.
func TestSRV063FN025VerifyPublishedVectors(t *testing.T) {
	v := load(t)
	now := time.Unix(v.Timestamp, 0).Add(time.Minute)
	header := func(sig string) http.Header {
		h := http.Header{}
		h.Set(webhook.HeaderID, v.ID)
		h.Set(webhook.HeaderTimestamp, "1614265330")
		h.Set(webhook.HeaderSignature, sig)
		return h
	}
	msg, err := webhook.Verify([]string{v.Secret}, header(v.Signature), []byte(v.Payload), now, 0)
	if err != nil || msg.ID != v.ID || msg.At.Unix() != v.Timestamp {
		t.Fatalf("Verify = %+v, %v", msg, err)
	}
	if _, err := webhook.Verify([]string{v.Secret}, header(v.ValidMultiple), []byte(v.Payload), now, 0); err != nil {
		t.Fatalf("a list holding one valid signature: %v", err)
	}
	for _, bad := range v.InvalidSignatures {
		if _, err := webhook.Verify([]string{v.Secret}, header(bad), []byte(v.Payload), now, 0); code(err) != plxerr.WebhookSignatureInvalid {
			t.Errorf("signature %q: %v, want PLX-7151", bad, err)
		}
	}
	if _, err := webhook.Verify([]string{v.Secret}, header(v.Signature), []byte(v.Payload+" "), now, 0); code(err) != plxerr.WebhookSignatureInvalid {
		t.Errorf("a changed body: %v, want PLX-7151", err)
	}
	for _, drop := range []string{webhook.HeaderID, webhook.HeaderTimestamp, webhook.HeaderSignature} {
		h := header(v.Signature)
		h.Del(drop)
		if _, err := webhook.Verify([]string{v.Secret}, h, []byte(v.Payload), now, 0); code(err) != plxerr.WebhookSignatureInvalid {
			t.Errorf("without %s: %v, want PLX-7151", drop, err)
		}
	}
}

// FN-025: a correctly signed message outside the tolerance is a replay.
func TestFN025TimestampTolerance(t *testing.T) {
	v := load(t)
	at := time.Unix(v.Timestamp, 0)
	h, err := webhook.Headers([]string{v.Secret}, v.ID, at, []byte(v.Payload))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		skew time.Duration
		want plxerr.Code
	}{
		{0, 0},
		{4*time.Minute + 59*time.Second, 0},
		{-(4*time.Minute + 59*time.Second), 0},
		{5*time.Minute + time.Second, plxerr.WebhookReplayed},
		{-(5*time.Minute + time.Second), plxerr.WebhookReplayed},
	} {
		_, err := webhook.Verify([]string{v.Secret}, h, []byte(v.Payload), at.Add(c.skew), 0)
		if c.want == 0 && err != nil || c.want != 0 && code(err) != c.want {
			t.Errorf("skew %s: %v, want %v", c.skew, err, c.want)
		}
	}
}

// SRV-063: during a rotation both secrets verify, and a receiver that holds
// either one accepts the message.
func TestSRV063RotationOverlap(t *testing.T) {
	oldSecret, err := webhook.NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	newSecret, err := webhook.NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(newSecret, webhook.SecretPrefix) || newSecret == oldSecret {
		t.Fatalf("secrets %q %q", oldSecret, newSecret)
	}
	now := time.Unix(1_700_000_000, 0)
	body := []byte(`{"type":"x"}`)
	h, err := webhook.Headers([]string{newSecret, oldSecret}, "msg_1", now, body)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(strings.Fields(h.Get(webhook.HeaderSignature))); n != 2 {
		t.Fatalf("%d signatures, want one per secret", n)
	}
	for name, receiver := range map[string][]string{"old": {oldSecret}, "new": {newSecret}, "both": {newSecret, oldSecret}} {
		if _, err := webhook.Verify(receiver, h, body, now, 0); err != nil {
			t.Errorf("receiver with the %s secret: %v", name, err)
		}
	}
	other, _ := webhook.NewSecret()
	if _, err := webhook.Verify([]string{other}, h, body, now, 0); code(err) != plxerr.WebhookSignatureInvalid {
		t.Errorf("an unrelated secret: %v", err)
	}
	if _, err := webhook.Headers(nil, "msg_1", now, body); err == nil {
		t.Error("signing with no secret succeeded")
	}
	if _, err := webhook.Sign("whsec_!!!", "msg_1", now, body); code(err) != plxerr.ChannelMisconfigured {
		t.Errorf("a secret that is not base64: %v", err)
	}
}

func FuzzVerify(f *testing.F) {
	f.Add("msg", "1614265330", "v1,AAAA", []byte("{}"))
	f.Fuzz(func(t *testing.T, id, ts, sig string, body []byte) {
		h := http.Header{}
		h.Set(webhook.HeaderID, id)
		h.Set(webhook.HeaderTimestamp, ts)
		h.Set(webhook.HeaderSignature, sig)
		_, _ = webhook.Verify([]string{"whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw"}, h, body, time.Unix(1614265330, 0), 0)
	})
}
