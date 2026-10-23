// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package function

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// Verifies: FN-025, GOV-003.
// The first check of a call that names a trigger or a hook by identifier
// passes for a caller who holds function.deploy anywhere, and for no one else.
func TestAuthorizeSome_FN_025(t *testing.T) {
	t.Parallel()
	deploy := []auth.Permission{auth.FunctionDeploy}
	cases := map[string]struct {
		p    auth.Principal
		deny bool
	}{
		"organisation-wide": {p: auth.Principal{Permissions: deploy}},
		"on one app":        {p: auth.Principal{AppPermissions: map[string][]auth.Permission{"app": deploy}}},
		"on one environment": {p: auth.Principal{
			EnvPermissions: map[auth.EnvScope][]auth.Permission{{AppID: "app", Environment: "development"}: deploy},
		}},
		"nowhere": {p: auth.Principal{Permissions: []auth.Permission{auth.AppRead}}, deny: true},
		"only another permission on an app": {
			p:    auth.Principal{AppPermissions: map[string][]auth.Permission{"app": {auth.AppRead}}},
			deny: true,
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := authorizeSome(c.p); (err != nil) != c.deny {
				t.Fatalf("authorizeSome = %v, deny %v", err, c.deny)
			}
		})
	}
}

// Verifies: FN-025, FN-007.
// A production trigger needs function.deploy.production beside function.deploy.
func TestAuthorizeEnv_ProductionNeedsItsOwnPermission_FN_007(t *testing.T) {
	t.Parallel()
	p := auth.Principal{OrganizationID: "org", Permissions: []auth.Permission{auth.FunctionDeploy}}
	if err := authorizeEnv(p, "app", dbgen.Environment{}); err != nil {
		t.Errorf("a development environment: %v", err)
	}
	if err := authorizeEnv(p, "app", dbgen.Environment{Production: true}); err == nil {
		t.Error("a production environment needed no function.deploy.production")
	}
	if err := authorizeEnv(auth.Principal{}, "app", dbgen.Environment{}); err == nil {
		t.Error("a caller without function.deploy changed a trigger")
	}
}

// Verifies: FN-025.
// An error is written as the status a webhook sender understands, with the
// PLX code and nothing about the cause.
func TestReplyStatus_FN_025(t *testing.T) {
	t.Parallel()
	cases := []struct {
		err  error
		want int
		body string
	}{
		{plxerr.New(plxerr.WebhookSignatureInvalid, "bad"), http.StatusUnauthorized, "PLX-7151"},
		{plxerr.New(plxerr.WebhookReplayed, "again"), http.StatusConflict, "PLX-7152"},
		{plxerr.New(plxerr.FunctionNotFound, "none"), http.StatusNotFound, ""},
		{plxerr.New(plxerr.FunctionInputTooLarge, "big"), http.StatusRequestEntityTooLarge, ""},
		{plxerr.New(plxerr.FunctionRateLimited, "fast"), http.StatusTooManyRequests, ""},
		{plxerr.New(plxerr.AssuranceInsufficient, "weak"), http.StatusForbidden, ""},
		{plxerr.New(plxerr.InvalidFormat, "odd"), http.StatusBadRequest, ""},
		{plxerr.New(plxerr.PreconditionFailed, "other"), http.StatusBadGateway, ""},
		{context.Canceled, http.StatusInternalServerError, "internal"},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		reply(rec, c.err)
		if rec.Code != c.want || !strings.Contains(rec.Body.String(), c.body) {
			t.Errorf("%v: %d %s, want %d %q", c.err, rec.Code, rec.Body, c.want, c.body)
		}
		if strings.Contains(rec.Body.String(), "canceled") {
			t.Errorf("the cause leaked: %s", rec.Body)
		}
	}
}

// Verifies: FN-025.
// The standard verifier is built in, a provider verifier is looked up by
// name, and an unknown name has none.
func TestHookVerifierLookup_FN_025(t *testing.T) {
	t.Parallel()
	s := &Service{o: Options{Verifiers: map[string]Verifier{"acme": standardVerifier{}}}}
	if s.verifier(StandardWebhooks) == nil || s.verifier("acme") == nil {
		t.Error("a known verifier was not found")
	}
	if s.verifier("unknown") != nil {
		t.Error("an unknown verifier was found")
	}
	if _, err := s.hookSecrets(context.Background(), nil, dbgen.FunctionHook{}); err == nil {
		t.Error("a server that keeps no secrets verified a hook")
	}
}
