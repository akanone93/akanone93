// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"errors"
	"net/http"

	"github.com/nightCode42/plux3/backend/edition"
	scimext "github.com/nightCode42/plux3/backend/ext/identity"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

// The two plain HTTP routes of identity federation (GOV-004). Both
// authenticate by what they carry — a signed logout token, a SCIM bearer
// token — and neither goes through the Connect interceptors.
const (
	// BackChannelLogoutRoute receives an OpenID Connect provider's logout
	// token (OpenID Connect Back-Channel Logout 1.0). {provider} is the
	// provider's identifier, or "default" for the one in the configuration
	// file.
	BackChannelLogoutRoute = "POST /sso/oidc/{provider}/backchannel-logout"
	// SCIMRoute is the SCIM 2.0 server's prefix (RFC 7644 §3).
	SCIMRoute = "/scim/v2/"
	// scimPrefix is SCIMRoute without its trailing slash, stripped before
	// the SCIM server routes the request.
	scimPrefix = "/scim/v2"
)

// BackChannelLogout handles a provider's logout token. It answers 200 when
// the token was accepted, whether or not it named a session here, and 400
// with an OAuth error body when it was not, as the specification asks.
func (h *Handlers) BackChannelLogout() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		r.Body = http.MaxBytesReader(w, r.Body, h.Limits.Get(limits.APIRequestSize))
		if err := r.ParseForm(); err != nil {
			logoutRefused(w)
			return
		}
		provider := r.PathValue("provider")
		if provider == "default" {
			provider = ""
		}
		_, err := h.Auth.BackChannelLogout(r.Context(), provider, r.PostForm.Get("logout_token"))
		switch {
		case err == nil:
			w.WriteHeader(http.StatusOK)
		case errors.Is(err, auth.ErrLogoutToken), errors.Is(err, auth.ErrLogoutReplayed), isCode(err, plxerr.AuthenticationRequired):
			logoutRefused(w)
		case isCode(err, plxerr.ResourceNotFound), isCode(err, plxerr.InvalidFormat):
			http.NotFound(w, r)
		default:
			http.Error(w, "the logout could not be processed", http.StatusInternalServerError)
		}
	})
}

// isCode reports whether err carries a Plux error code.
func isCode(err error, code plxerr.Code) bool {
	c, ok := plxerr.CodeOf(err)
	return ok && c == code
}

// logoutRefused answers a logout token that was not accepted.
func logoutRefused(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	_, _ = w.Write([]byte(`{"error":"invalid_request"}`))
}

// SCIM mounts the SCIM server over the core's directory, behind the licence:
// a request is served only while the licence lets the feature be read, and
// the request context carries the licence provider so that the server gates
// its writes. The body is bounded by api.requestSize.
func (h *Handlers) SCIM(svc scimext.SCIMService) http.Handler {
	inner := http.StripPrefix(scimPrefix, svc.Handler(h.Auth.SCIMDirectory()))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := edition.WithProvider(r.Context(), h.Edition)
		if err := edition.RequireRead(ctx, edition.FeatureSCIM); err != nil {
			scimext.WriteSCIMError(w, http.StatusForbidden, "", err)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, h.Limits.Get(limits.APIRequestSize))
		inner.ServeHTTP(w, r.WithContext(ctx))
	})
}
