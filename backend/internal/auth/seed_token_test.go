// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package auth

import (
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// The seed token of `make dev` passes the step-up check; every other user
// token still needs a step-up of its own (SEC-103, DEP-020).
//
// Verifies: SEC-103.
func TestSeedTokenCountsAsSteppedUp(t *testing.T) {
	for _, tc := range []struct {
		source string
		want   plxerr.Code
	}{
		{sourceSeed, 0},
		{sourcePAT, plxerr.StepUpRequired},
		{sourceCLI, plxerr.StepUpRequired},
	} {
		id := tokenIdentity(dbgen.GetAccessTokenByHashRow{Source: tc.source})
		if got := errCode(id.StepUpError(StepUpWindow)); got != tc.want {
			t.Errorf("%s token: step-up error %v, want %v", tc.source, got, tc.want)
		}
		if id.Kind != KindToken {
			t.Errorf("%s token: kind %q, want %q", tc.source, id.Kind, KindToken)
		}
	}
}

// A seed token is only for the seeded user, and only short-lived.
func TestCreateSeedTokenRefusals(t *testing.T) {
	s := &Service{}
	user := Principal{Identity: Identity{Kind: KindUser, UserID: "u"}}
	for name, tc := range map[string]struct {
		p   Principal
		ttl time.Duration
	}{
		"another token": {Principal{Identity: Identity{Kind: KindToken, UserID: "u"}}, time.Minute},
		"no user":       {Principal{Identity: Identity{Kind: KindUser}}, time.Minute},
		"too long":      {user, SeedTokenTTL + time.Second},
		"no time":       {user, 0},
	} {
		if _, err := s.CreateSeedToken(t.Context(), tc.p, tc.ttl); err == nil {
			t.Errorf("%s: a seed token was created", name)
		}
	}
}
