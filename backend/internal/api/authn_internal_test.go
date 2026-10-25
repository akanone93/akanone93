// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"
	"net/http"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
)

// Verifies: GOV-005.
// Decision O-1: lock heartbeats and long-lived streams are background
// procedures; a call a person makes is not.
func TestBackgroundProcedure_GOV_005(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		procedure string
		want      bool
	}{
		{pluxv1connect.PluginServiceRenewLockProcedure, true},
		{pluxv1connect.PluginServiceGetLockProcedure, true},
		{pluxv1connect.GovernanceServiceWatchPermissionsProcedure, true},
		{pluxv1connect.DevSessionServiceWatchEventsProcedure, true},
		{"/plux.v1.NotificationService/ListInbox", false},
		{pluxv1connect.AppServiceCreateAppProcedure, false},
		{"", false},
	} {
		if got := backgroundProcedure(tc.procedure); got != tc.want {
			t.Errorf("backgroundProcedure(%q) = %v, want %v", tc.procedure, got, tc.want)
		}
	}
}

// recordingAuthenticator records which way a session was authenticated.
type recordingAuthenticator struct{ called string }

func (r *recordingAuthenticator) AuthenticateSession(context.Context, string) (auth.Identity, error) {
	r.called = "touching"
	return auth.Identity{Kind: auth.KindUser, ID: "u", CSRFToken: "plux_csrf_good"}, nil //nolint:gosec // a test value
}

func (r *recordingAuthenticator) AuthenticateSessionQuietly(context.Context, string) (auth.Identity, error) {
	r.called = "quiet"
	return auth.Identity{Kind: auth.KindUser, ID: "u", CSRFToken: "plux_csrf_good"}, nil //nolint:gosec // a test value
}

func (r *recordingAuthenticator) AuthenticateToken(context.Context, string) (auth.Identity, error) {
	r.called = "token"
	return auth.Identity{}, nil
}

// Verifies: GOV-005.
// Decision O-1: a session presented to a lock heartbeat is authenticated
// quietly, and one presented to a person's call is counted as a use.
func TestCredentialRoutesBackgroundCallsQuietly_GOV_005(t *testing.T) {
	t.Parallel()
	h := http.Header{}
	h.Add("Cookie", SessionCookie+"=plux_ses_good")
	h.Set(CSRFHeader, "plux_csrf_good")
	for procedure, want := range map[string]string{
		pluxv1connect.PluginServiceRenewLockProcedure: "quiet",
		pluxv1connect.AppServiceCreateAppProcedure:    "touching",
	} {
		a := &recordingAuthenticator{}
		if _, found, err := credential(context.Background(), a, procedure, h); err != nil || !found {
			t.Fatalf("credential(%s) = %v, %v", procedure, found, err)
		}
		if a.called != want {
			t.Errorf("credential(%s) authenticated %s, want %s", procedure, a.called, want)
		}
	}
}

// Verifies: SEC-101, STU-002.
// Only GetCurrentUser may be called with the cookie and no CSRF header; it
// hands the token back. Every other procedure still needs the header, and a
// bearer caller is never asked for one.
func TestCredentialCsrfExemption_SEC_101_STU_002(t *testing.T) {
	t.Parallel()
	h := http.Header{}
	h.Add("Cookie", SessionCookie+"=plux_ses_good")
	for procedure, wantOK := range map[string]bool{
		pluxv1connect.IdentityServiceGetCurrentUserProcedure: true,
		pluxv1connect.IdentityServiceLogoutProcedure:         false,
		pluxv1connect.AppServiceCreateAppProcedure:           false,
		pluxv1connect.PluginServiceRenewLockProcedure:        false,
	} {
		id, found, err := credential(context.Background(), &recordingAuthenticator{}, procedure, h)
		if (err == nil && found) != wantOK {
			t.Errorf("credential(%s) without the header = %v, %v", procedure, found, err)
		}
		if wantOK && id.CSRFToken != "plux_csrf_good" {
			t.Errorf("credential(%s) lost the session token", procedure)
		}
	}
	b := http.Header{}
	b.Set("Authorization", "Bearer plux_pat_good")
	if id, _, err := credential(context.Background(), &recordingAuthenticator{}, pluxv1connect.IdentityServiceGetCurrentUserProcedure, b); err != nil || id.CSRFToken != "" {
		t.Errorf("a bearer caller: %+v, %v", id, err)
	}
}
