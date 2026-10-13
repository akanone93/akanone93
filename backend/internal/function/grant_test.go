// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package function

import (
	"bytes"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// fixedClock is a clock a test moves.
type fixedClock struct{ t time.Time }

func (c *fixedClock) now() time.Time { return c.t }

// counterReader yields a different byte every read, so nonces differ.
type counterReader struct{ n byte }

func (r *counterReader) Read(p []byte) (int, error) {
	for i := range p {
		r.n++
		p[i] = r.n
	}
	return len(p), nil
}

func testGrants(t *testing.T, key byte, clock *fixedClock) *Grants {
	t.Helper()
	g, err := NewGrants(bytes.Repeat([]byte{key}, 32), clock.now, &counterReader{})
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func sampleClaims() GrantClaims {
	return GrantClaims{
		OrganizationID: "org", AppID: "app", EnvironmentID: "env", Function: "payments.charge",
		Version: 3, ModuleSHA256: strings.Repeat("ab", 32), Depth: 1, Assurance: "AL2", UserClaims: []byte(`{"sub":"u1"}`),
	}
}

// Verifies: FN-012, FN-018.
// A grant carries exactly the scope it was issued for, and verification
// needs the installation's secret.
func TestGrantRoundTrip_FN_012(t *testing.T) {
	t.Parallel()
	clock := &fixedClock{t: time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)}
	g := testGrants(t, 1, clock)
	token, issued, err := g.Issue(sampleClaims(), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	got, err := g.Verify(token)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, issued) || got.Assurance != "AL2" || string(got.UserClaims) != `{"sub":"u1"}` || got.Function != "payments.charge" || got.Version != 3 || got.Depth != 1 || got.AppID != "app" || got.EnvironmentID != "env" {
		t.Errorf("claims = %+v, issued %+v", got, issued)
	}
	if want := (KVScope{OrganizationID: "org", AppID: "app", EnvironmentID: "env", Function: "payments.charge"}); got.KVScope() != want {
		t.Errorf("scope = %+v", got.KVScope())
	}
	other := testGrants(t, 2, clock)
	if _, err := other.Verify(token); code(err) != plxerr.AuthenticationRequired {
		t.Errorf("a grant under another secret: %v", err)
	}
}

// Verifies: FN-012.
// A grant lives for the invocation's timeout plus five seconds and never
// more than sixty (ADR-0064); it is refused from its expiry on.
func TestGrantLifetime_FN_012(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		timeout time.Duration
		life    time.Duration
	}{
		{"timeout plus margin", 2 * time.Second, 7 * time.Second},
		{"capped at sixty seconds", 58 * time.Second, MaxGrantLifetime},
		{"far above the cap", time.Hour, MaxGrantLifetime},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			clock := &fixedClock{t: time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)}
			g := testGrants(t, 1, clock)
			token, c, err := g.Issue(sampleClaims(), tc.timeout)
			if err != nil {
				t.Fatal(err)
			}
			if got := c.ExpiresAt.Sub(clock.t); got != tc.life {
				t.Errorf("lifetime = %v, want %v", got, tc.life)
			}
			clock.t = clock.t.Add(tc.life - time.Millisecond)
			if _, err := g.Verify(token); err != nil {
				t.Errorf("just before expiry: %v", err)
			}
			clock.t = clock.t.Add(time.Millisecond)
			if _, err := g.Verify(token); code(err) != plxerr.AuthenticationRequired {
				t.Errorf("at expiry: %v", err)
			}
		})
	}
	g := testGrants(t, 1, &fixedClock{t: time.Now()})
	if _, _, err := g.Issue(sampleClaims(), 0); err == nil {
		t.Error("a grant without a timeout was issued")
	}
	if _, _, err := g.Issue(GrantClaims{}, time.Second); err == nil {
		t.Error("a grant without a scope was issued")
	}
}

// Verifies: FN-012.
// A grant that was changed in any byte, or is not a grant, is refused with
// one generic error.
func TestGrantTampering_FN_012(t *testing.T) {
	t.Parallel()
	clock := &fixedClock{t: time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)}
	g := testGrants(t, 1, clock)
	token, _, err := g.Issue(sampleClaims(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	for i := range len(token) {
		flipped := []byte(token)
		flipped[i] ^= 1
		if _, err := g.Verify(Grant(flipped)); err == nil {
			t.Fatalf("a grant changed at byte %d verified", i)
		}
	}
	body, sig, _ := strings.Cut(string(token), ".")
	for _, bad := range []Grant{"", ".", "a.b", Grant(body), Grant(body + "." + sig + "x"), Grant(sig + "." + body), Grant(strings.Repeat("A", 4096))} {
		_, err := g.Verify(bad)
		if code(err) != plxerr.AuthenticationRequired || !strings.Contains(err.Error(), "the grant is not valid") {
			t.Errorf("Verify(%.12q) = %v", string(bad), err)
		}
	}
}

// Verifies: FN-012.
// A grant is a credential: it prints as a marker through fmt and slog.
func TestGrantNeverPrinted_FN_012(t *testing.T) {
	t.Parallel()
	g := testGrants(t, 1, &fixedClock{t: time.Now()})
	token, _, err := g.Issue(sampleClaims(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	slog.New(slog.NewTextHandler(&out, nil)).Info("invoke", "grant", token)
	fmt.Fprintf(&out, " %v %s %#v %+v", token, token, token, struct{ G Grant }{token})
	if strings.Contains(out.String(), string(token)) || strings.Contains(out.String(), strings.SplitN(string(token), ".", 2)[0]) {
		t.Errorf("a grant was printed: %s", out.String())
	}
}

// Verifies: FN-012.
// Two grants for the same scope differ, and a short secret is refused.
func TestGrantUniqueness_FN_012(t *testing.T) {
	t.Parallel()
	clock := &fixedClock{t: time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)}
	g := testGrants(t, 1, clock)
	a, _, _ := g.Issue(sampleClaims(), time.Second)
	b, _, _ := g.Issue(sampleClaims(), time.Second)
	if a == b {
		t.Error("two grants for one scope are the same")
	}
	if _, err := NewGrants([]byte("short"), nil, nil); err == nil {
		t.Error("a short secret was accepted")
	}
}
