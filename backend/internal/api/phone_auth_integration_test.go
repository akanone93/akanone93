// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api_test

import (
	"context"
	"crypto/ecdsa"
	"net/http"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/api"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/device/devicetest"
	"github.com/nightCode42/plux3/backend/internal/dpop"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// signInPhone connects a phone with key for the fixture's administrator,
// stepped up, and returns the phone token.
func (f *authFixture) signInPhone(t *testing.T, key *ecdsa.PrivateKey) string {
	t.Helper()
	ctx := context.Background()
	me := must(f.w.identity.GetCurrentUser(ctx, req(f.admin, &pluxv1.GetCurrentUserRequest{})))(t)
	uid := me.GetUser().GetId()
	id := auth.Identity{Kind: auth.KindUser, ID: uid, UserID: uid, Display: "Admin", SecondFactor: true}.WithStepUp(0)
	p, err := f.w.auth.Resolve(ctx, id, f.admin.org)
	if err != nil {
		t.Fatal(err)
	}
	jkt, err := dpop.Thumbprint(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	grant, err := f.w.auth.CreatePhoneSignIn(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.w.auth.ClaimPhoneSignIn(ctx, grant.Grant, jkt, "Pixel 8", "android"); err != nil {
		t.Fatal(err)
	}
	st, err := f.w.auth.GetPhoneSignIn(ctx, p, grant.ID)
	if err != nil {
		t.Fatal(err)
	}
	token, err := f.w.auth.CompletePhoneSignIn(ctx, grant.Grant, jkt, st.Number)
	if err != nil {
		t.Fatal(err)
	}
	return token.Secret
}

// Verifies: DEV-002, SEC-021.
// A phone token is accepted only by the procedures of a phone, only as a
// DPoP token, and only with a proof of the key it is bound to and the
// server's nonce; the call then runs as the phone's person.
func TestPhoneAuthentication_DEV_002(t *testing.T) {
	t.Parallel()
	f := newAuthFixture(t, nil)
	a := f.w.deviceAuth
	people := 0
	a.People = func(context.Context, api.Call, func(context.Context) error) error { people++; return nil }
	around := api.DeviceAuthentication(a)
	key, _ := devicetest.NewKey(t)
	token := f.signInPhone(t, key)
	nonce := a.Nonces.Current()
	proof := func(k *ecdsa.PrivateKey, procedure, tok string) string {
		return devicetest.SignProof(t, k, devicetest.Proof{Method: "POST", URL: f.w.base + procedure, AccessToken: tok, Nonce: nonce})
	}
	var (
		seen   auth.Identity
		caller api.PhoneCaller
	)
	call := func(procedure string, h http.Header) error {
		seen, caller = auth.Identity{}, api.PhoneCaller{}
		return around(context.Background(), api.Call{Procedure: procedure, Header: h, ResponseHeader: http.Header{}}, func(ctx context.Context) error {
			seen, _ = api.IdentityFrom(ctx)
			caller, _ = api.PhoneFrom(ctx)
			return nil
		})
	}
	apps := pluxv1connect.DevPhoneServiceListPhoneAppsProcedure
	if err := call(apps, header("DPoP "+token, proof(key, apps, token))); err != nil {
		t.Fatalf("a phone's call: %v", err)
	}
	if seen.Kind != auth.KindPhone || caller.Name != "Pixel 8" || seen.OrganizationID != f.admin.org {
		t.Fatalf("the call ran as %+v %+v", seen, caller)
	}
	// The proof must be of the phone's key.
	stranger, _ := devicetest.NewKey(t)
	if err := call(apps, header("DPoP "+token, proof(stranger, apps, token))); err == nil {
		t.Error("a proof of another key was accepted")
	}
	// A proof is single-use.
	replayed := proof(key, apps, token)
	if err := call(apps, header("DPoP "+token, replayed)); err != nil {
		t.Fatal(err)
	}
	if err := call(apps, header("DPoP "+token, replayed)); err == nil {
		t.Error("a replayed proof was accepted")
	}
	// Never a bearer token, and never without a proof.
	if err := call(apps, header("Bearer "+token, "")); err == nil {
		t.Error("a phone token was accepted as a bearer token")
	}
	if err := call(apps, header("DPoP "+token, "")); err == nil {
		t.Error("a phone call without a proof was accepted")
	}
	// A device token is not a phone token.
	if err := call(apps, header("DPoP "+f.dev.token, proof(f.dev.key, apps, f.dev.token))); err == nil {
		t.Error("a device token called a phone procedure")
	}
	// And a phone token calls nothing else.
	list := pluxv1connect.AppServiceListAppsProcedure
	if err := call(list, header("DPoP "+token, proof(key, list, token))); code(err) != plxerr.PermissionDenied || people != 0 {
		t.Errorf("a phone token on another procedure: %v (handed to people %d times)", err, people)
	}
	if _, err := f.w.auth.AuthenticateToken(context.Background(), token); err == nil {
		t.Error("the people's authentication accepted a phone token")
	}
}
