// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package webauthndriver

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// newTestDriver returns a driver for alice against the fakes, with a store in
// a temporary directory.
func newTestDriver(t *testing.T, rp *relyingParty, mutate ...func(*Config)) *Driver {
	t.Helper()
	cfg := Config{
		Server: rp.api.URL, Origin: rp.origin, OrgID: testOrgID, OrgKey: testOrgKey, ProviderKey: "walkthrough-idp",
		Actor: "alice", Email: testEmail, Store: FileStore{Dir: filepath.Join(t.TempDir(), "keys")},
		ServerHTTP: rp.api.Client(), IDPHTTP: noRedirects(rp.idp.Client()), Random: rand.Reader,
	}
	for _, m := range mutate {
		m(&cfg)
	}
	d, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// TestDriverToken_SEC103 signs a person in through the test provider, enrols
// the key and gets a token; a second call signs in with the key.
//
// Verifies: SEC-103, GOV-030.
func TestDriverToken_SEC103(t *testing.T) {
	rp := newRelyingParty(t)
	d := newTestDriver(t, rp)
	token, err := d.Token(t.Context())
	if err != nil || token != rp.token {
		t.Fatalf("Token = %q, %v; want %q", token, err, rp.token)
	}
	if rp.pub == nil {
		t.Fatal("the key was not enrolled")
	}
	again, err := d.Token(t.Context())
	if err != nil || again != rp.token {
		t.Fatalf("second Token = %q, %v", again, err)
	}
	for _, want := range []string{"IdentityService/BeginWebAuthnRegistration", "IdentityService/BeginWebAuthnLogin"} {
		if !slices.Contains(rp.calls, want) {
			t.Errorf("the server never saw %s: %v", want, rp.calls)
		}
	}
}

// TestDriverStepUp_SEC103 completes the step-up the CLI asked for, with user
// verification, in a second process (a new driver) with the stored key.
//
// Verifies: SEC-103.
func TestDriverStepUp_SEC103(t *testing.T) {
	rp := newRelyingParty(t)
	dir := filepath.Join(t.TempDir(), "keys")
	withDir := func(c *Config) { c.Store = FileStore{Dir: dir} }
	if _, err := newTestDriver(t, rp, withDir).Token(t.Context()); err != nil {
		t.Fatal(err)
	}
	counterAfterToken := rp.counter
	if err := newTestDriver(t, rp, withDir).StepUp(t.Context(), "https://plux.test/stepup?request=request-1"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(rp.stepUps, []string{"request-1"}) {
		t.Fatalf("approved step-ups = %v", rp.stepUps)
	}
	if rp.counter <= counterAfterToken {
		t.Fatalf("the counter did not advance across processes: %d after %d", rp.counter, counterAfterToken)
	}
}

// TestDriverPasswordStepUp_SEC103 completes the step-ups of a person who
// signs in with a password and meets the driver first at a step-up (the
// seeded administrator): the first enrols the key, later ones sign in with it.
//
// Verifies: SEC-103.
func TestDriverPasswordStepUp_SEC103(t *testing.T) {
	rp := newRelyingParty(t)
	dir := filepath.Join(t.TempDir(), "keys")
	withPassword := func(c *Config) { c.Store, c.Password = FileStore{Dir: dir}, testPassword }
	if err := newTestDriver(t, rp, withPassword).StepUp(t.Context(), "https://plux.test/stepup?request=request-1"); err != nil {
		t.Fatal(err)
	}
	if rp.pub == nil {
		t.Fatal("the key was not enrolled")
	}
	if err := newTestDriver(t, rp, withPassword).StepUp(t.Context(), "https://plux.test/stepup?request=request-1"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(rp.stepUps, []string{"request-1", "request-1"}) || slices.Contains(rp.calls, "IdentityService/StartOidcLogin") {
		t.Fatalf("approved step-ups = %v, calls = %v", rp.stepUps, rp.calls)
	}
	wrong := newTestDriver(t, rp, func(c *Config) { c.Store, c.Password = FileStore{Dir: dir}, "wrong" })
	if err := wrong.StepUp(t.Context(), "https://plux.test/stepup?request=request-1"); err == nil || !strings.Contains(err.Error(), "wrong email or password") {
		t.Fatalf("StepUp with a wrong password = %v", err)
	}
}

// TestDriverBootstrap_SEC103 prepares an installation the way its operator
// does: it accepts the invitation with the password, enrols the key, creates
// the organisation and the app, and mints a token in the organisation; the
// administrator's later step-up signs in with the enrolled key.
//
// Verifies: SEC-103.
func TestDriverBootstrap_SEC103(t *testing.T) {
	rp := newRelyingParty(t)
	dir := filepath.Join(t.TempDir(), "keys")
	withPassword := func(c *Config) { c.Store, c.Password = FileStore{Dir: dir}, testPassword }
	in, err := newTestDriver(t, rp, withPassword).Bootstrap(t.Context(), testInvitation, testOrgKey, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if in.OrgID != testOrgID || in.AppID != "app-1" || in.Token != rp.token || rp.pub == nil {
		t.Fatalf("Bootstrap = %+v, key enrolled %v", in, rp.pub != nil)
	}
	if err := newTestDriver(t, rp, withPassword).StepUp(t.Context(), "https://plux.test/stepup?request=request-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := newTestDriver(t, rp, withPassword).Bootstrap(t.Context(), "", testOrgKey, "demo"); err == nil {
		t.Error("Bootstrap without an invitation succeeded")
	}
	if _, err := newTestDriver(t, rp, withPassword).Bootstrap(t.Context(), "invitation-2", testOrgKey, "demo"); err == nil || !strings.Contains(err.Error(), "no such invitation") {
		t.Errorf("Bootstrap with an unknown invitation = %v", err)
	}
}

// TestDriverRefusals_SEC103 covers the ways a ceremony must fail loudly: an
// origin the server does not accept, a missing key, an unusable URL, a
// provider the server does not offer, and a key the server does not allow.
//
// Verifies: SEC-103.
func TestDriverRefusals_SEC103(t *testing.T) {
	t.Run("the server refuses another origin", func(t *testing.T) {
		rp := newRelyingParty(t)
		d := newTestDriver(t, rp, func(c *Config) { c.Origin = "https://evil.example" })
		if _, err := d.Token(t.Context()); err == nil || !strings.Contains(err.Error(), "is not Studio's") {
			t.Fatalf("Token = %v, want the origin refusal", err)
		}
	})
	t.Run("a step-up needs an enrolled key", func(t *testing.T) {
		rp := newRelyingParty(t)
		err := newTestDriver(t, rp).StepUp(t.Context(), "https://plux.test/stepup?request=request-1")
		if !errors.Is(err, ErrNoCredential) {
			t.Fatalf("StepUp = %v, want ErrNoCredential", err)
		}
	})
	t.Run("a step-up URL names a request", func(t *testing.T) {
		rp := newRelyingParty(t)
		if err := newTestDriver(t, rp).StepUp(t.Context(), "https://plux.test/stepup"); err == nil {
			t.Fatal("StepUp accepted a URL with no request")
		}
	})
	t.Run("an unknown provider", func(t *testing.T) {
		rp := newRelyingParty(t)
		d := newTestDriver(t, rp, func(c *Config) { c.ProviderKey = "other" })
		if _, err := d.Token(t.Context()); err == nil || !strings.Contains(err.Error(), `no sign-in provider "other"`) {
			t.Fatalf("Token = %v", err)
		}
	})
	t.Run("a request for another request ID", func(t *testing.T) {
		rp := newRelyingParty(t)
		d := newTestDriver(t, rp)
		if _, err := d.Token(t.Context()); err != nil {
			t.Fatal(err)
		}
		if err := d.StepUp(t.Context(), "https://plux.test/stepup?request=request-2"); err == nil || !strings.Contains(err.Error(), "no such step-up request") {
			t.Fatalf("StepUp = %v", err)
		}
		if len(rp.stepUps) != 0 {
			t.Fatalf("a step-up was approved: %v", rp.stepUps)
		}
	})
	t.Run("a person whose key the driver does not hold", func(t *testing.T) {
		rp := newRelyingParty(t)
		if _, err := newTestDriver(t, rp).Token(t.Context()); err != nil {
			t.Fatal(err)
		}
		if _, err := newTestDriver(t, rp).Token(t.Context()); err == nil || !strings.Contains(err.Error(), "already has a second factor") {
			t.Fatalf("Token with a fresh store = %v", err)
		}
	})
}

// TestDriverProviderRedirect_SEC103 checks what the driver does with a
// provider that answers wrongly: no redirect, and a redirect for another state.
//
// Verifies: SEC-103.
func TestDriverProviderRedirect_SEC103(t *testing.T) {
	rp := newRelyingParty(t)
	d := newTestDriver(t, rp)
	for name, signIn := range map[string]string{
		"no redirect":   rp.api.URL + "/plux.v1.NoSuchService/NoSuchCall",
		"another state": rp.idp.URL + "/authorize?state=other",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := d.authorize(t.Context(), signIn, "state-1"); err == nil {
				t.Fatal("authorize accepted the answer")
			}
		})
	}
}

// TestNew_SEC103 rejects a configuration that cannot work.
//
// Verifies: SEC-103.
func TestNew_SEC103(t *testing.T) {
	rp := newRelyingParty(t)
	for name, mutate := range map[string]func(*Config){
		"no server":   func(c *Config) { c.Server = "" },
		"no actor":    func(c *Config) { c.Actor = "" },
		"no client":   func(c *Config) { c.IDPHTTP = nil },
		"relative":    func(c *Config) { c.Server, c.Origin = "plux.test", "" },
		"empty email": func(c *Config) { c.Email = "" },
	} {
		cfg := Config{
			Server: rp.api.URL, Actor: "alice", Email: testEmail, ServerHTTP: rp.api.Client(), IDPHTTP: rp.idp.Client(),
		}
		mutate(&cfg)
		if _, err := New(cfg); err == nil {
			t.Errorf("%s: New accepted the configuration", name)
		}
	}
}

// TestAuthenticator_SEC103 checks the key itself: signatures verify, user
// verification is asserted, the counter advances, and a key the options do not
// allow refuses to answer.
//
// Verifies: SEC-103.
func TestAuthenticator_SEC103(t *testing.T) {
	key, err := NewAuthenticator(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	challenge := base64.RawURLEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	options := RequestOptions{Challenge: challenge, RPID: "plux.test"}
	first, err := key.Get(options, "https://plux.test")
	if err != nil {
		t.Fatal(err)
	}
	second, err := key.Get(options, "https://plux.test")
	if err != nil {
		t.Fatal(err)
	}
	if first.AuthenticatorData[32]&(flagUserPresent|flagUserVerified) != flagUserPresent|flagUserVerified {
		t.Errorf("flags = %08b, want presence and verification", first.AuthenticatorData[32])
	}
	if c1, c2 := counterOf(first), counterOf(second); c2 <= c1 || key.Credential().Counter != c2 {
		t.Errorf("counters %d then %d, stored %d", c1, c2, key.Credential().Counter)
	}
	pub := key.Credential().Key.PublicKey
	digest := sha256.Sum256(first.ClientDataJSON)
	signed := sha256.Sum256(append(append([]byte{}, first.AuthenticatorData...), digest[:]...))
	if !ecdsa.VerifyASN1(&pub, signed[:], first.Signature) {
		t.Error("the signature does not verify")
	}
	options.AllowCredentials = append(options.AllowCredentials, struct {
		ID string `json:"id"`
	}{ID: base64.RawURLEncoding.EncodeToString([]byte("another key"))})
	if _, err := key.Get(options, "https://plux.test"); err == nil {
		t.Error("the key answered options that do not allow it")
	}
}

// counterOf reads the signature counter of an assertion.
func counterOf(a Assertion) uint32 {
	d := a.AuthenticatorData
	return uint32(d[33])<<24 | uint32(d[34])<<16 | uint32(d[35])<<8 | uint32(d[36])
}

// TestParseOptions_SEC103 rejects options an authenticator cannot act on.
//
// Verifies: SEC-103.
func TestParseOptions_SEC103(t *testing.T) {
	for _, raw := range []string{``, `{}`, `{"challenge":"x"}`, `[]`} {
		if _, err := ParseRequestOptions([]byte(raw)); err == nil {
			t.Errorf("ParseRequestOptions(%q) succeeded", raw)
		}
		if _, err := ParseCreationOptions([]byte(raw)); err == nil {
			t.Errorf("ParseCreationOptions(%q) succeeded", raw)
		}
	}
}

// TestFileStore_SEC103 checks the store: user-only files, a faithful round
// trip, and names that cannot leave the directory.
//
// Verifies: SEC-103.
func TestFileStore_SEC103(t *testing.T) {
	store := FileStore{Dir: filepath.Join(t.TempDir(), "keys")}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	want := Credential{ID: []byte("credential"), Key: key, Counter: 7}
	if err := store.Save("alice", want); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(store.Dir, "alice.json"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("credential file: %v, %v; want mode 0600", info, err)
	}
	if dir, err := os.Stat(store.Dir); err != nil || dir.Mode().Perm() != 0o700 {
		t.Fatalf("directory: %v, %v; want mode 0700", dir, err)
	}
	got, err := store.Load("alice")
	if err != nil || string(got.ID) != "credential" || got.Counter != 7 || !got.Key.Equal(key) {
		t.Fatalf("Load = %+v, %v", got, err)
	}
	if _, err := store.Load("bob"); !errors.Is(err, ErrNoCredential) {
		t.Errorf("Load of a stranger = %v, want ErrNoCredential", err)
	}
	for _, name := range []string{"", "../alice", "a/b", ".hidden", "Alice", strings.Repeat("a", 65)} {
		if err := store.Save(name, want); err == nil {
			t.Errorf("Save(%q) accepted the name", name)
		}
	}
	if err := os.WriteFile(filepath.Join(store.Dir, "carol.json"), []byte(`{"id":"AQ==","key":"AQ=="}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load("carol"); err == nil || strings.Contains(err.Error(), "AQ") {
		t.Errorf("Load of a damaged file = %v, want an error that quotes nothing", err)
	}
}
