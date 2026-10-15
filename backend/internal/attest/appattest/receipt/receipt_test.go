// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package receipt_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/attest/appattest"
	"github.com/nightCode42/plux3/backend/internal/attest/appattest/receipt"
	"github.com/nightCode42/plux3/backend/internal/attest/appattest/receipt/receipttest"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

const (
	appID  = "TEAMID.com.example.app"
	keyID  = "ABCDE12345"
	teamID = "TEAM123456"
)

var testNow = time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)

// rig is a client wired to a simulator for each Apple host.
type rig struct {
	signer    *receipttest.Signer
	prod, dev *receipttest.Server
	authority *receipttest.Authority
	deviceKey *ecdsa.PrivateKey
	client    *receipt.Client
	want      receipt.Expect
	initial   []byte
}

func newRig(t *testing.T, tweak func(*receipt.Config)) *rig {
	t.Helper()
	deviceKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	r := &rig{signer: receipttest.NewSigner(t), authority: receipttest.NewAuthority(t, testNow), deviceKey: deviceKey}
	now := func() time.Time { return testNow }
	sim := func() *receipttest.Server {
		return receipttest.New(t, receipttest.Config{
			Authority: r.authority, JWTKey: r.signer.Public(), KeyID: keyID, TeamID: teamID, AppID: appID, Key: &deviceKey.PublicKey, Now: now,
		})
	}
	r.prod, r.dev = sim(), sim()
	r.want = receipt.Expect{AppID: appID, Key: &deviceKey.PublicKey}
	r.initial = r.authority.Issue(t, receipttest.IssueOptions{AppID: appID, Key: &deviceKey.PublicKey, Risk: 2})
	cfg := receipt.Config{
		KeyID: keyID, TeamID: teamID, Signer: r.signer, HTTP: http.DefaultClient, Now: now,
		Hosts: receipt.Hosts{Production: r.prod.URL, Development: r.dev.URL}, Roots: r.authority.Roots(),
	}
	if tweak != nil {
		tweak(&cfg)
	}
	if r.client, err = receipt.New(cfg); err != nil {
		t.Fatal(err)
	}
	return r
}

func code(err error) plxerr.Code {
	c, _ := plxerr.CodeOf(err)
	return c
}

// Verifies: SEC-007, SEC-123.
// A refreshed receipt verifies and yields its risk metric in every
// encoding Apple's service might use: DER or BER with indefinite lengths,
// the attested key as a point, a SubjectPublicKeyInfo or a certificate,
// the metric as digits or as an INTEGER, the body in base64 or raw.
func TestRefreshEncodings_SEC_007(t *testing.T) {
	t.Parallel()
	for _, enc := range []receipttest.Encoding{receipttest.DER, receipttest.BERIndefinite} {
		for _, raw := range []bool{false, true} {
			r := newRig(t, nil)
			r.prod.SetEncoding(enc)
			r.prod.SetRawBody(raw)
			r.prod.SetRisk(3)
			got, err := r.client.Refresh(context.Background(), r.initial, appattest.Production, r.want)
			if err != nil {
				t.Fatalf("encoding %v raw %v: %v", enc, raw, err)
			}
			if got.RiskMetric != 3 || len(got.Raw) == 0 || !got.Expiration.After(testNow) || got.NotBefore.After(testNow) {
				t.Errorf("encoding %v raw %v: Refresh = %+v", enc, raw, got)
			}
			if _, err := r.client.Verify(got.Raw, r.want); err != nil {
				t.Errorf("encoding %v raw %v: the stored receipt does not verify again: %v", enc, raw, err)
			}
		}
	}
}

// Verifies: SEC-007.
// The attested key and the metric may be written in any documented form.
func TestVerifyForms_SEC_007(t *testing.T) {
	t.Parallel()
	r := newRig(t, nil)
	for _, form := range []receipttest.KeyForm{receipttest.KeyPoint, receipttest.KeySPKI, receipttest.KeyCertificate} {
		for _, asInt := range []bool{false, true} {
			for _, enc := range []receipttest.Encoding{receipttest.DER, receipttest.BERIndefinite} {
				raw := r.authority.Issue(t, receipttest.IssueOptions{
					AppID: appID, Key: &r.deviceKey.PublicKey, KeyForm: form, Risk: 17, RiskAsInteger: asInt, Encoding: enc,
				})
				got, err := r.client.Verify(raw, r.want)
				if err != nil || got.RiskMetric != 17 {
					t.Errorf("form %v integer %v encoding %v: %+v, %v", form, asInt, enc, got, err)
				}
			}
		}
	}
}

// Verifies: SEC-007.
// The development host serves development attestations and the production
// host production ones.
func TestRefreshHostByEnvironment_SEC_007(t *testing.T) {
	t.Parallel()
	r := newRig(t, nil)
	if _, err := r.client.Refresh(context.Background(), r.initial, appattest.Development, r.want); err != nil {
		t.Fatal(err)
	}
	if r.dev.Requests() != 1 || r.prod.Requests() != 0 {
		t.Errorf("development: dev %d prod %d requests", r.dev.Requests(), r.prod.Requests())
	}
	if _, err := r.client.Refresh(context.Background(), r.initial, appattest.Production, r.want); err != nil {
		t.Fatal(err)
	}
	if r.dev.Requests() != 1 || r.prod.Requests() != 1 {
		t.Errorf("production: dev %d prod %d requests", r.dev.Requests(), r.prod.Requests())
	}
	if string(r.prod.Received()) != string(r.initial) {
		t.Error("Apple was not sent the stored receipt")
	}
	if _, err := r.client.Refresh(context.Background(), r.initial, appattest.Environment(9), r.want); code(err) != plxerr.FraudReceiptUnavailable {
		t.Errorf("an unknown environment: %v", err)
	}
}

// Verifies: SEC-007.
// 304 keeps the receipt the client sent, which must still verify.
func TestRefreshNotModified_SEC_007(t *testing.T) {
	t.Parallel()
	r := newRig(t, nil)
	r.prod.SetMode(receipttest.ModeNotModified)
	got, err := r.client.Refresh(context.Background(), r.initial, appattest.Production, r.want)
	if err != nil || got.RiskMetric != 2 || string(got.Raw) != string(r.initial) {
		t.Errorf("304 = %+v, %v", got, err)
	}
	attest := r.authority.Issue(t, receipttest.IssueOptions{AppID: appID, Key: &r.deviceKey.PublicKey, Type: "ATTEST", OmitRisk: true})
	if _, err := r.client.Refresh(context.Background(), attest, appattest.Production, r.want); code(err) != plxerr.FraudReceiptUnavailable {
		t.Errorf("304 for an attestation receipt: %v", err)
	}
}

// Verifies: SEC-007.
// Every way Apple can fail, and every receipt that does not verify, is
// PLX-6080, and the message holds no token or receipt.
func TestRefreshFailures_SEC_007(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		mode receipttest.Mode
		why  string
	}{
		{"401", receipttest.ModeUnauthorized, "status 401"},
		{"429", receipttest.ModeTooManyRequests, "status 429"},
		{"500", receipttest.ModeServerError, "status 500"},
		{"tampered signature", receipttest.ModeTampered, "signature does not verify"},
		{"tampered content", receipttest.ModeTamperedContent, "does not match its signed digest"},
		{"expired", receipttest.ModeExpired, "has expired"},
		{"wrong app", receipttest.ModeWrongApp, "another app"},
		{"wrong key", receipttest.ModeWrongKey, "another key"},
		{"untrusted authority", receipttest.ModeUntrusted, "does not chain to a trusted root"},
	} {
		for _, enc := range []receipttest.Encoding{receipttest.DER, receipttest.BERIndefinite} {
			r := newRig(t, nil)
			r.prod.SetMode(c.mode)
			r.prod.SetEncoding(enc)
			_, err := r.client.Refresh(context.Background(), r.initial, appattest.Production, r.want)
			if code(err) != plxerr.FraudReceiptUnavailable {
				t.Errorf("%s (%v): %v", c.name, enc, err)
				continue
			}
			msg := err.Error()
			if !strings.Contains(msg, c.why) {
				t.Errorf("%s (%v): the failure is %q, want it to say %q", c.name, enc, msg, c.why)
			}
			if strings.Contains(msg, "Bearer") || strings.Contains(msg, "eyJ") || strings.Contains(msg, base64.StdEncoding.EncodeToString(r.initial[:12])) {
				t.Errorf("%s: the message leaks a credential: %s", c.name, msg)
			}
		}
	}
}

// Verifies: SEC-007.
// An answer that never comes ends at the timeout; the caller's context
// ends it earlier.
func TestRefreshTimeout_SEC_007(t *testing.T) {
	t.Parallel()
	r := newRig(t, func(c *receipt.Config) { c.Timeout = 150 * time.Millisecond })
	r.prod.SetMode(receipttest.ModeTimeout)
	start := time.Now()
	_, err := r.client.Refresh(context.Background(), r.initial, appattest.Production, r.want)
	if code(err) != plxerr.FraudReceiptUnavailable || time.Since(start) > 5*time.Second {
		t.Errorf("timeout: %v after %v", err, time.Since(start))
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.client.Refresh(ctx, r.initial, appattest.Production, r.want); code(err) != plxerr.FraudReceiptUnavailable {
		t.Errorf("a cancelled context: %v", err)
	}
}

// Verifies: SEC-007.
// A response beyond the bound is refused before it is parsed.
func TestRefreshResponseBound_SEC_007(t *testing.T) {
	t.Parallel()
	r := newRig(t, func(c *receipt.Config) { c.MaxBytes = 64 })
	if _, err := r.client.Refresh(context.Background(), r.initial, appattest.Production, r.want); code(err) != plxerr.FraudReceiptUnavailable {
		t.Errorf("an oversized response: %v", err)
	}
	if _, err := r.client.Verify(r.initial, r.want); code(err) != plxerr.FraudReceiptUnavailable {
		t.Errorf("an oversized receipt: %v", err)
	}
}

// Verifies: SEC-123, SEC-007.
// The request token is signed through the signer under the receipt role;
// a token Apple would refuse (wrong kid, wrong team, stale iat) and a
// signer that fails are PLX-6080.
func TestRefreshToken_SEC_123(t *testing.T) {
	t.Parallel()
	r := newRig(t, nil)
	if _, err := r.client.Refresh(context.Background(), r.initial, appattest.Production, r.want); err != nil {
		t.Fatal(err)
	}
	if roles := r.signer.Roles(); len(roles) != 1 || roles[0] != receipt.Role || receipt.Role != "apple-appattest-receipt" {
		t.Errorf("signing roles = %v", roles)
	}
	for name, tweak := range map[string]func(*receipt.Config){
		"wrong kid":  func(c *receipt.Config) { c.KeyID = "ZZZZZZZZZZ" },
		"wrong team": func(c *receipt.Config) { c.TeamID = "ZZZZZZZZZZ" },
		"stale iat":  func(c *receipt.Config) { c.Now = func() time.Time { return testNow.Add(-time.Hour) } },
	} {
		rr := newRig(t, tweak)
		if _, err := rr.client.Refresh(context.Background(), rr.initial, appattest.Production, rr.want); code(err) != plxerr.FraudReceiptUnavailable {
			t.Errorf("%s: %v", name, err)
		}
		if rr.prod.Requests() != 0 {
			t.Errorf("%s: the simulator accepted the token", name)
		}
	}
	r.signer.FailWith(errors.New("the key is not imported"))
	if _, err := r.client.Refresh(context.Background(), r.initial, appattest.Production, r.want); code(err) != plxerr.FraudReceiptUnavailable {
		t.Errorf("a failing signer: %v", err)
	}
}

// shortSigner returns a signature of the wrong length.
type shortSigner struct{}

func (shortSigner) SignES256(context.Context, string, []byte) ([]byte, error) { return []byte{1}, nil }

// Verifies: SEC-123.
// A signature that is not r‖s of 64 bytes is never sent.
func TestRefreshSignatureLength_SEC_123(t *testing.T) {
	t.Parallel()
	r := newRig(t, func(c *receipt.Config) { c.Signer = shortSigner{} })
	if _, err := r.client.Refresh(context.Background(), r.initial, appattest.Production, r.want); code(err) != plxerr.FraudReceiptUnavailable {
		t.Errorf("a short signature: %v", err)
	}
	if r.prod.Requests() != 0 {
		t.Error("a request was sent")
	}
}

// Verifies: SEC-007.
// The checks on a receipt: an expectation is required, the type must be
// RECEIPT with a risk metric, and the validity window is enforced at its
// edges.
func TestVerifyChecks_SEC_007(t *testing.T) {
	t.Parallel()
	r := newRig(t, nil)
	other, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	issue := func(o receipttest.IssueOptions) []byte {
		o.AppID, o.Key = appID, &r.deviceKey.PublicKey
		return r.authority.Issue(t, o)
	}
	for _, c := range []struct {
		name string
		raw  []byte
		want receipt.Expect
		ok   bool
	}{
		{"valid", issue(receipttest.IssueOptions{Risk: 1}), r.want, true},
		{"no app expected", issue(receipttest.IssueOptions{}), receipt.Expect{Key: r.want.Key}, false},
		{"no key expected", issue(receipttest.IssueOptions{}), receipt.Expect{AppID: appID}, false},
		{"other app expected", issue(receipttest.IssueOptions{}), receipt.Expect{AppID: "TEAMID.x", Key: r.want.Key}, false},
		{"other key expected", issue(receipttest.IssueOptions{}), receipt.Expect{AppID: appID, Key: &other.PublicKey}, false},
		{"attestation receipt", issue(receipttest.IssueOptions{Type: "ATTEST"}), r.want, false},
		{"unknown type", issue(receipttest.IssueOptions{Type: "OTHER"}), r.want, false},
		{"no risk metric", issue(receipttest.IssueOptions{OmitRisk: true}), r.want, false},
		{"not yet valid", issue(receipttest.IssueOptions{NotBefore: testNow.Add(time.Second), Expiration: testNow.Add(time.Hour)}), r.want, false},
		{"valid from now", issue(receipttest.IssueOptions{NotBefore: testNow, Expiration: testNow.Add(time.Hour)}), r.want, true},
		{"expires now", issue(receipttest.IssueOptions{NotBefore: testNow.Add(-time.Hour), Expiration: testNow}), r.want, false},
		{"empty", nil, r.want, false},
		{"garbage", []byte("not a receipt"), r.want, false},
	} {
		got, err := r.client.Verify(c.raw, c.want)
		if c.ok != (err == nil) || (err != nil && code(err) != plxerr.FraudReceiptUnavailable) {
			t.Errorf("%s: %+v, %v", c.name, got, err)
		}
	}
}

// Verifies: SEC-007.
// The root pool decides trust: the default pool is Apple's, which does
// not trust a test authority.
func TestDefaultRootsAreApples_SEC_007(t *testing.T) {
	t.Parallel()
	r := newRig(t, func(c *receipt.Config) { c.Roots = nil })
	if _, err := r.client.Verify(r.initial, r.want); code(err) != plxerr.FraudReceiptUnavailable {
		t.Errorf("a receipt from a test authority under Apple's root: %v", err)
	}
	pool := x509.NewCertPool()
	rr := newRig(t, func(c *receipt.Config) { c.Roots = pool })
	if _, err := rr.client.Verify(rr.initial, rr.want); code(err) != plxerr.FraudReceiptUnavailable {
		t.Errorf("an empty pool: %v", err)
	}
}

// Verifies: SEC-007.
// New refuses an incomplete configuration.
func TestNewRequiresConfiguration_SEC_007(t *testing.T) {
	t.Parallel()
	good := receipt.Config{KeyID: keyID, TeamID: teamID, Signer: receipttest.NewSigner(t), HTTP: http.DefaultClient}
	for name, tweak := range map[string]func(*receipt.Config){
		"key ID":  func(c *receipt.Config) { c.KeyID = "" },
		"team ID": func(c *receipt.Config) { c.TeamID = "" },
		"signer":  func(c *receipt.Config) { c.Signer = nil },
		"client":  func(c *receipt.Config) { c.HTTP = nil },
	} {
		c := good
		tweak(&c)
		if _, err := receipt.New(c); err == nil {
			t.Errorf("a configuration without a %s was accepted", name)
		}
	}
	if _, err := receipt.New(good); err != nil {
		t.Errorf("a complete configuration: %v", err)
	}
}

// Verifies: SEC-007.
// A client is safe for concurrent use.
func TestRefreshConcurrent_SEC_007(t *testing.T) {
	t.Parallel()
	r := newRig(t, nil)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := r.client.Refresh(context.Background(), r.initial, appattest.Production, r.want); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if r.prod.Requests() != 8 {
		t.Errorf("requests = %d", r.prod.Requests())
	}
}
