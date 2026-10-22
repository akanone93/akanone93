// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package license_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/edition"
	"github.com/nightCode42/plux3/backend/edition/licensefile"
	"github.com/nightCode42/plux3/ee/backend/license"
)

// testSeed derives the test signing key. The key is a test fixture, derived
// from a constant in code so that no private key is committed.
func testSeed(b byte) ed25519.PrivateKey {
	return ed25519.NewKeyFromSeed(bytes.Repeat([]byte{b}, ed25519.SeedSize))
}

var (
	issuerKey = testSeed(1)
	otherKey  = testSeed(2)
	epoch     = time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
)

func publicOf(k ed25519.PrivateKey) ed25519.PublicKey {
	pub, _ := k.Public().(ed25519.PublicKey)
	return pub
}

// payload is a licence valid from epoch-30d for 365 days.
func payload() licensefile.Payload {
	return licensefile.Payload{
		LicenseID: "LIC-2026-0001", Organization: "Acme AG", Edition: licensefile.Edition,
		IssuedAt:  epoch.AddDate(0, 0, -30).Format(time.RFC3339),
		NotBefore: epoch.AddDate(0, 0, -30).Format(time.RFC3339),
		ExpiresAt: epoch.AddDate(0, 0, 335).Format(time.RFC3339),
		Features:  []string{"saml", "scim", "custom-roles"},
		Limits:    map[string]int64{"page.nodes": 1000},
	}
}

func signed(t *testing.T, p licensefile.Payload, key ed25519.PrivateKey) []byte {
	t.Helper()
	raw, err := license.Sign(p, key)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func verifierAt(at time.Time) license.Verifier {
	return license.Verifier{Keys: licensefile.NewKeySet(publicOf(issuerKey)), Now: func() time.Time { return at }}
}

func TestLicenseStates_GOV_030(t *testing.T) {
	t.Parallel()
	base := payload()
	expires := epoch.AddDate(0, 0, 335)
	wrongKey := signed(t, base, otherKey)
	good := signed(t, base, issuerKey)

	unknownLimit := base
	unknownLimit.Limits = map[string]int64{"no.such.limit": 5}
	// Signed by the trusted key, so only the registry check can refuse it:
	// build the file by hand, since Sign refuses such a payload.
	unknownLimitFile := mustMarshal(t, unknownLimit, issuerKey)

	tamperedPayload := bytes.Replace(good, []byte(`"Acme AG"`), []byte(`"Evil AG"`), 1)
	if bytes.Equal(tamperedPayload, good) {
		t.Fatal("the tamper did not change the file")
	}

	bound := base
	bound.InstallationID = "inst-1"

	tests := []struct {
		name    string
		raw     []byte
		at      time.Time
		install string
		want    edition.State
		days    int
	}{
		{"valid", good, epoch, "", edition.StateValid, 335},
		{"valid on the last second", good, expires.Add(-time.Second), "", edition.StateValid, 0},
		{"grace begins at expiry", good, expires, "", edition.StateGrace, 0},
		{"grace, ten days in", good, expires.AddDate(0, 0, 10), "", edition.StateGrace, -10},
		{"expired when grace has ended", good, expires.AddDate(0, 0, 30), "", edition.StateExpired, -30},
		{"tampered payload", tamperedPayload, epoch, "", edition.StateInvalid, 0},
		{"signed by a key that is not trusted", wrongKey, epoch, "", edition.StateInvalid, 0},
		{"unknown limit key", unknownLimitFile, epoch, "", edition.StateInvalid, 0},
		{"not yet valid", good, epoch.AddDate(0, 0, -60), "", edition.StateInvalid, 0},
		{"garbage", []byte("not json"), epoch, "", edition.StateInvalid, 0},
		{"bound to another installation", signed(t, bound, issuerKey), epoch, "inst-2", edition.StateInvalid, 0},
		{"bound to this installation", signed(t, bound, issuerKey), epoch, "inst-1", edition.StateValid, 335},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			st := verifierAt(tc.at).EvaluateBytes(tc.raw, tc.install)
			if st.State != tc.want {
				t.Fatalf("state = %s (%s), want %s", st.State, st.Message, tc.want)
			}
			if tc.want == edition.StateInvalid {
				if len(st.Features) != 0 || st.LicenseID != "" || st.Organization != "" {
					t.Errorf("an invalid licence exposes content: %+v", st)
				}
				return
			}
			if st.DaysRemaining != tc.days {
				t.Errorf("DaysRemaining = %d, want %d", st.DaysRemaining, tc.days)
			}
			if len(st.Features) != 3 || st.Limits["page.nodes"] != 1000 || st.Organization != "Acme AG" {
				t.Errorf("status = %+v", st)
			}
		})
	}
}

func mustMarshal(t *testing.T, p licensefile.Payload, key ed25519.PrivateKey) []byte {
	t.Helper()
	g := licensefile.DefaultGraceDays
	p.GraceDays = &g
	canonical, err := licensefile.Canonical(p)
	if err != nil {
		t.Fatal(err)
	}
	sig := licensefile.Signature{KeyID: licensefile.KeyID(publicOf(key)), Alg: licensefile.AlgEd25519, Sig: signBase64(key, canonical)}
	raw, err := licensefile.Marshal(p, []licensefile.Signature{sig})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func signBase64(key ed25519.PrivateKey, msg []byte) string {
	return base64.StdEncoding.EncodeToString(ed25519.Sign(key, msg))
}

func TestGraceDaysDefaultAndExplicit_GOV_030(t *testing.T) {
	t.Parallel()
	p := payload()
	zero := 0
	p.GraceDays = &zero
	st := verifierAt(epoch.AddDate(0, 0, 335)).EvaluateBytes(signed(t, p, issuerKey), "")
	if st.State != edition.StateExpired {
		t.Errorf("graceDays 0: state = %s, want expired at the expiry instant", st.State)
	}
	seven := 7
	p.GraceDays = &seven
	st = verifierAt(epoch.AddDate(0, 0, 335+6)).EvaluateBytes(signed(t, p, issuerKey), "")
	if st.State != edition.StateGrace {
		t.Errorf("graceDays 7, day 6: state = %s, want grace", st.State)
	}
}

func TestExpiredFeaturesAreReadOnly_GOV_030(t *testing.T) {
	t.Parallel()
	raw := signed(t, payload(), issuerKey)
	at := epoch.AddDate(0, 0, 335+31)
	st := verifierAt(at).EvaluateBytes(raw, "")
	ctx := edition.WithProvider(t.Context(), staticProvider{st})
	if edition.EnabledForWrite(ctx, edition.FeatureSAML) || edition.Enabled(ctx, edition.FeatureSAML) {
		t.Error("an expired licence still enables writes")
	}
	if !edition.Readable(ctx, edition.FeatureSAML) {
		t.Error("an expired licence must keep the data readable")
	}
	if edition.Readable(ctx, edition.FeatureClickHouse) {
		t.Error("a feature the licence does not list is readable")
	}
	if err := edition.Require(ctx, edition.FeatureSAML); err == nil || !strings.Contains(err.Error(), "PLX-8600") {
		t.Errorf("Require = %v, want PLX-8600", err)
	}
}

type staticProvider struct{ st edition.Status }

func (p staticProvider) Status() edition.Status { return p.st }

func TestEvaluateFileStates_GOV_030(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	good := filepath.Join(dir, "license.json")
	if err := os.WriteFile(good, signed(t, payload(), issuerKey), 0o600); err != nil {
		t.Fatal(err)
	}
	v := verifierAt(epoch)
	if st := v.Evaluate("", ""); st.State != edition.StateAbsent {
		t.Errorf("no path: %s", st.State)
	}
	if st := v.Evaluate(filepath.Join(dir, "missing.json"), ""); st.State != edition.StateAbsent {
		t.Errorf("missing file: %s", st.State)
	}
	if st := v.Evaluate(good, ""); st.State != edition.StateValid {
		t.Errorf("good file: %s", st.State)
	}
	if st := v.Evaluate(dir, ""); st.State != edition.StateInvalid {
		t.Errorf("a directory: %s", st.State)
	}
	v.ReadFile = func(string) ([]byte, error) { return nil, fs.ErrPermission }
	if st := v.Evaluate(good, ""); st.State != edition.StateInvalid || strings.Contains(st.Message, dir) {
		t.Errorf("unreadable: %+v", st)
	}
}

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

func TestProviderReverifiesAndWarns_GOV_030(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "license.json")
	var mu sync.Mutex
	at := epoch
	v := license.Verifier{
		Keys: licensefile.NewKeySet(publicOf(issuerKey)), Interval: 5 * time.Millisecond,
		Now: func() time.Time { mu.Lock(); defer mu.Unlock(); return at },
	}
	var logs lockedBuffer
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	p, err := v.Open(ctx, edition.Settings{File: path, Log: slog.New(slog.NewTextHandler(&logs, nil))})
	if err != nil {
		t.Fatal(err)
	}
	if p.Status().State != edition.StateAbsent {
		t.Fatalf("before the file exists: %s", p.Status().State)
	}
	if err := os.WriteFile(path, signed(t, payload(), issuerKey), 0o600); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return p.Status().State == edition.StateValid })
	mu.Lock()
	at = epoch.AddDate(0, 0, 340)
	mu.Unlock()
	waitFor(t, func() bool { return p.Status().State == edition.StateGrace })
	waitFor(t, func() bool { return strings.Contains(logs.String(), "level=WARN") })
	if strings.Contains(logs.String(), "signature") {
		t.Errorf("the log quotes the licence: %s", logs.String())
	}
	if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return p.Status().State == edition.StateInvalid })
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("the condition did not hold in time")
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func TestDefaultsAndLoggingDiscard_GOV_030(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	p, err := license.Verifier{}.Open(ctx, edition.Settings{})
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	if p.Status().State != edition.StateAbsent {
		t.Errorf("state = %s", p.Status().State)
	}
	// The build trusts exactly the pinned production key (2026-10-22).
	if licensefile.TrustedKeys().Len() != 1 {
		t.Errorf("the build trusts %d production keys, want 1", licensefile.TrustedKeys().Len())
	}
}

func TestSignRefusesWhatTheVerifierRefuses_GOV_030(t *testing.T) {
	t.Parallel()
	bad := payload()
	bad.Limits = map[string]int64{"no.such.limit": 1}
	if _, err := license.Sign(bad, issuerKey); !errors.Is(err, licensefile.ErrLimitKey) {
		t.Errorf("unknown limit: %v", err)
	}
	if _, err := license.Sign(payload(), ed25519.PrivateKey("short")); err == nil {
		t.Error("a short key was accepted")
	}
	pub, priv, err := license.GenerateKey(nil)
	if err != nil || len(pub) != ed25519.PublicKeySize || len(priv) != ed25519.PrivateKeySize {
		t.Errorf("GenerateKey: %v", err)
	}
}
