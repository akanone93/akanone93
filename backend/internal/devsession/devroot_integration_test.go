// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package devsession_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/devsession"
	"github.com/nightCode42/plux3/backend/internal/release"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// rootCertifier stands in for the release service: one organisation root, and
// a record of the environments it certified.
type rootCertifier struct {
	pub  ed25519.PublicKey
	priv ed25519.PrivateKey
	mu   sync.Mutex
	envs []string
}

func newRootCertifier(t *testing.T) *rootCertifier {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return &rootCertifier{pub: pub, priv: priv}
}

func (c *rootCertifier) DevelopmentRoot(context.Context, string) (ed25519.PublicKey, error) {
	return c.pub, nil
}

func (c *rootCertifier) CertifyDevelopmentKey(_ context.Context, env dbgen.Environment) (release.DevKeyCertificate, error) {
	c.mu.Lock()
	c.envs = append(c.envs, storage.ID(env.ID))
	c.mu.Unlock()
	statement := []byte(`{"environmentId":"` + storage.ID(env.ID) + `"}`)
	return release.DevKeyCertificate{Statement: statement, Signature: ed25519.Sign(c.priv, statement)}, nil
}

// TestJoin_CertifiedDevelopmentKey_DEV_003 checks that a phone joining a
// session receives the development key of that session's own environment,
// certified by the organisation's root, on the first join after the number
// and again on a rejoin without one (ADR-0109); and that the sign-in link
// carries the root the phone pins.
// Verifies: DEV-003, DEV-002.
func TestJoin_CertifiedDevelopmentKey_DEV_003(t *testing.T) {
	cert := newRootCertifier(t)
	f := newFixtureWith(t, 0, func(o *devsession.Options) { o.Certifier = cert })
	ctx := context.Background()
	session := f.openStudio(t, f.Developer)
	phone, key := f.phone(t, f.signInPhone(t, f.Developer))
	jr, err := f.svc.RequestJoin(ctx, phone, phone.ID, key.Name, f.App)
	if err != nil {
		t.Fatal(err)
	}
	if jr.Key != nil {
		t.Fatal("a key was given before the number was picked")
	}
	st, err := f.svc.GetJoinRequest(ctx, f.Developer, jr.ID)
	if err != nil {
		t.Fatal(err)
	}
	joined, err := f.svc.CompleteJoin(ctx, phone, phone.ID, jr.ID, st.Number)
	if err != nil {
		t.Fatal(err)
	}
	if joined.Key == nil || !ed25519.Verify(cert.pub, joined.Key.Statement, joined.Key.Signature) {
		t.Fatalf("the join gave no key certified by the root: %+v", joined.Key)
	}
	again, err := f.svc.RequestJoin(ctx, phone, phone.ID, key.Name, f.App)
	if err != nil {
		t.Fatal(err)
	}
	if again.Pairing == nil || again.Key == nil || !ed25519.Verify(cert.pub, again.Key.Statement, again.Key.Signature) {
		t.Fatalf("the rejoin gave no certified key: %+v", again)
	}
	cert.mu.Lock()
	envs := append([]string(nil), cert.envs...)
	cert.mu.Unlock()
	if len(envs) != 2 || envs[0] != session.EnvironmentID || envs[1] != session.EnvironmentID {
		t.Fatalf("certified environments = %v, want the session's %s twice", envs, session.EnvironmentID)
	}
	// The sign-in link carries the root, for the phone to pin.
	root, err := f.svc.DevelopmentRoot(ctx, f.Developer.OrganizationID)
	if err != nil {
		t.Fatal(err)
	}
	link, err := url.Parse(f.svc.SignInLink("grant", root))
	if err != nil || !strings.HasPrefix(link.String(), "plux-dev://signin?") {
		t.Fatalf("link = %v, %v", link, err)
	}
	pinned, err := base64.RawURLEncoding.DecodeString(link.Query().Get("root"))
	if err != nil || !ed25519.PublicKey(pinned).Equal(cert.pub) {
		t.Fatalf("the link's root = %q, want the organisation's", link.Query().Get("root"))
	}
}
