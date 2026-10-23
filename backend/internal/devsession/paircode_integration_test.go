// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package devsession_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/devsession"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// pairCode pairs a device with a typed code.
func (f *fixture) pairCode(d devsession.Caller, code string) (devsession.Paired, error) {
	return f.svc.Pair(context.Background(), d, devsession.PairRequest{Code: code, Name: "Pixel", Platform: "android", RuntimeVersion: "1.0.0"})
}

// Verifies: DEV-002.
// A device pairs with the short code typed in any case and without its
// dash, and the code is single-use (PLX-9202).
func TestPairCode_PairsOnce_DEV_002(t *testing.T) {
	f := newFixture(t, 0)
	s, p := f.open(t)
	if len(p.Code) != 9 || p.Code[4] != '-' || p.Code != devsession.FormatCode(devsession.Code(p.Token)) {
		t.Fatalf("code = %q", p.Code)
	}
	typed := strings.ToLower(strings.ReplaceAll(p.Code, "-", ""))
	res, err := f.pairCode(f.device("device-1", "jkt-1"), typed)
	if err != nil {
		t.Fatal(err)
	}
	if res.SessionID != s.ID || res.Ticket == "" {
		t.Errorf("paired = %+v", res)
	}
	_, err = f.pairCode(f.device("device-2", "jkt-2"), p.Code)
	wantCode(t, err, plxerr.PairingTokenUsed, "a used code")
	_, err = f.pair(f.device("device-2", "jkt-2"), p.Token)
	wantCode(t, err, plxerr.PairingTokenUsed, "the token of a used code")
}

// Verifies: DEV-002.
// A code expires with its token (PLX-9201), and a pairing naming both a
// token and a code, or a code of the wrong form, is refused.
func TestPairCode_ExpiryAndForm_DEV_002(t *testing.T) {
	f := newFixture(t, 0)
	_, p := f.open(t)
	_, err := f.svc.Pair(context.Background(), f.device("device-1", "jkt-1"), devsession.PairRequest{Token: p.Token, Code: p.Code})
	wantCode(t, err, plxerr.InvalidFormat, "a token and a code")
	_, err = f.pairCode(f.device("device-1", "jkt-1"), "ABC-12")
	wantCode(t, err, plxerr.InvalidFormat, "a short code")
	f.Now = f.Now.Add(121 * time.Second)
	_, err = f.pairCode(f.device("device-1", "jkt-1"), p.Code)
	wantCode(t, err, plxerr.PairingTokenExpired, "an expired code")
}

// Verifies: DEV-002.
// A code only pairs a device of the session's organisation, app and
// environment; to any other it is unknown (PLX-9201), and the refusals do
// not spend it.
func TestPairCode_OnlyItsEnvironment_DEV_002(t *testing.T) {
	f := newFixture(t, 0)
	ctx := context.Background()
	_, p := f.open(t)
	admin := auth.Identity{Kind: auth.KindUser, ID: f.Owner.UserID, UserID: f.Owner.UserID, Display: "Admin", SecondFactor: true, InstallationAdmin: true}.WithStepUp(0)
	org, err := f.Tenancy.CreateOrganization(ctx, admin, "globex", "Globex")
	if err != nil {
		t.Fatal(err)
	}
	app, err := f.Tenancy.CreateApp(ctx, f.Owner, "other", "Other")
	if err != nil {
		t.Fatal(err)
	}
	otherOrg := f.device("device-1", "jkt-1")
	otherOrg.OrganizationID = org.ID
	otherApp := f.device("device-2", "jkt-2")
	otherApp.AppID = app.ID
	otherEnv := f.device("device-3", "jkt-3")
	otherEnv.EnvironmentID = f.Envs["production"]
	for what, d := range map[string]devsession.Caller{"another organisation": otherOrg, "another app": otherApp, "another environment": otherEnv} {
		_, err := f.pairCode(d, p.Code)
		wantCode(t, err, plxerr.PairingTokenExpired, "a device of "+what)
	}
	if _, err := f.pairCode(f.device("device-4", "jkt-4"), p.Code); err != nil {
		t.Fatalf("the refusals spent the code: %v", err)
	}
}

// Verifies: DEV-002.
// A device that failed devSession.pairingCodeAttempts (5) codes in ten
// minutes is refused, even with the right code, until the window passes;
// another device is not affected.
func TestPairCode_BoundsFailedAttempts_DEV_002(t *testing.T) {
	f := newFixture(t, 0)
	_, p := f.open(t)
	guesser := f.device("device-1", "jkt-1")
	wrong := "0000-0000"
	if p.Code == wrong {
		wrong = "1111-1111"
	}
	for range 5 {
		_, err := f.pairCode(guesser, wrong)
		wantCode(t, err, plxerr.PairingTokenExpired, "a wrong code")
		f.Now = f.Now.Add(time.Second)
	}
	_, err := f.pairCode(guesser, p.Code)
	wantCode(t, err, plxerr.RateLimited, "the sixth attempt")
	if _, err := f.pairCode(f.device("device-2", "jkt-2"), p.Code); err != nil {
		t.Fatalf("another device: %v", err)
	}
	f.Now = f.Now.Add(10 * time.Minute)
	_, again := f.open(t)
	if _, err := f.pairCode(guesser, again.Code); err != nil {
		t.Fatalf("after the window: %v", err)
	}
}
