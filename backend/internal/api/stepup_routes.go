// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/dpop"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// The routes of the CLI's step-up (maintainer decision Q15, ADR-0075).
const (
	// StepUpRequestsPath creates a request: POST with the CLI's token and
	// a DPoP proof of its key.
	StepUpRequestsPath = "/stepup/requests"
	// StepUpPollPath polls one: POST StepUpRequestsPath + "/{id}/poll".
	StepUpPollPath = "/stepup/requests/{id}/poll"
	// StepUpPagePath is the page the person opens in a browser.
	StepUpPagePath = "/stepup"
)

// stepUpPollSeconds is how often the CLI is asked to poll.
const stepUpPollSeconds = 2

// StepUpRoutes serves the CLI's step-up over plain HTTP: the two calls the
// CLI makes, authenticated by its token and a DPoP proof of its key, and the
// page a person completes the ceremony in. Nothing here decides anything:
// the auth service does, and the ceremony itself is the identity service's
// BeginStepUp and CompleteStepUp, which the page calls with the person's
// browser session (L-1).
type StepUpRoutes struct {
	Auth    *auth.Service
	Proofs  ProofChecker
	BaseURL string
	// Trusted are the proxies whose X-Forwarded-For is believed.
	Trusted []netip.Prefix
	// Limiter and PerPrincipal rate limit the CLI calls (SRV-065).
	Limiter      RateLimiter
	PerPrincipal int64
	page, notice *template.Template
}

// NewStepUpRoutes returns the routes, with their page templates parsed.
func NewStepUpRoutes(r StepUpRoutes) (*StepUpRoutes, error) {
	var err error
	if r.page, err = template.New("page").Parse(stepUpPage); err != nil {
		return nil, fmt.Errorf("api: step-up page: %w", err)
	}
	if r.notice, err = template.New("notice").Parse(stepUpNotice); err != nil {
		return nil, fmt.Errorf("api: step-up notice: %w", err)
	}
	return &r, nil
}

// Create handles POST /stepup/requests.
func (s *StepUpRoutes) Create(w http.ResponseWriter, r *http.Request) {
	ctx := audit.WithRequest(r.Context(), s.auditRequest(r))
	id, proof, ok := s.authenticate(ctx, w, r)
	if !ok {
		return
	}
	req, err := s.Auth.CreateCLIStepUp(ctx, id, proof.JKT)
	if err != nil {
		writeGatewayError(w, err)
		return
	}
	writeStepUpJSON(w, http.StatusCreated, map[string]any{
		"requestId": req.RequestID, "secret": req.Secret, "expiresAt": req.ExpiresAt.UTC().Format(time.RFC3339),
		"url":                 strings.TrimSuffix(s.BaseURL, "/") + StepUpPagePath + "?request=" + req.RequestID,
		"pollIntervalSeconds": stepUpPollSeconds,
	})
}

// Poll handles POST /stepup/requests/{id}/poll.
func (s *StepUpRoutes) Poll(w http.ResponseWriter, r *http.Request) {
	ctx := audit.WithRequest(r.Context(), s.auditRequest(r))
	id, proof, ok := s.authenticate(ctx, w, r)
	if !ok {
		return
	}
	grant, err := s.Auth.PollCLIStepUp(ctx, id, r.PathValue("id"), r.Header.Get(StepUpProofHeader), proof.JKT)
	if err != nil {
		writeGatewayError(w, err)
		return
	}
	body := map[string]any{"approved": grant.Approved}
	if grant.Approved {
		body["grant"], body["expiresAt"] = grant.Grant, grant.ExpiresAt.UTC().Format(time.RFC3339)
	}
	writeStepUpJSON(w, http.StatusOK, body)
}

// auditRequest describes the call for the audit trail.
func (s *StepUpRoutes) auditRequest(r *http.Request) audit.Request {
	return audit.Request{
		SourceIP:  ClientAddress(r.RemoteAddr, r.Header, s.Trusted),
		UserAgent: truncate(r.Header.Get("User-Agent"), 512),
	}
}

// authenticate checks the CLI's token and the proof of its key, and counts
// the call against the token's allowance. It answers the refusal itself.
func (s *StepUpRoutes) authenticate(ctx context.Context, w http.ResponseWriter, r *http.Request) (auth.Identity, dpop.Proof, bool) {
	fail := func(err error) (auth.Identity, dpop.Proof, bool) {
		writeGatewayError(w, err)
		return auth.Identity{}, dpop.Proof{}, false
	}
	token, ok := bearerToken(r.Header)
	if !ok {
		return fail(plxerr.New(plxerr.AuthenticationRequired, "this call needs the token of plux login or a personal access token"))
	}
	id, err := s.Auth.AuthenticateToken(ctx, token)
	if err != nil {
		return fail(err)
	}
	proof, err := s.Proofs.Check(ctx, r.Header, r.Method, r.URL.EscapedPath(), token)
	if err != nil {
		return fail(err)
	}
	if s.Limiter.Count != nil {
		if _, err := s.Limiter.Allow(ctx, "rate:principal:"+id.Kind+":"+id.ID, s.PerPrincipal); err != nil {
			return fail(err)
		}
	}
	return id, proof, true
}

// writeStepUpJSON answers a call of the CLI. The answers carry secrets, so
// nothing may keep them.
func writeStepUpJSON(w http.ResponseWriter, status int, body map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// pageData is what the step-up page shows.
type pageData struct {
	Nonce   string
	Request string
	CSRF    string
	Message string
}

// Page handles GET /stepup?request=<id>: the page on which a person, signed
// in to Studio in this browser, confirms the CLI's request with a security
// key. Its script runs under a CSP that allows nothing but this page's own
// nonce, calls only this origin, and has no inline handlers. The person must
// be the one the request was made for, so a link to someone else's request
// shows nothing to approve (SEC-103, SEC-040).
func (s *StepUpRoutes) Page(w http.ResponseWriter, r *http.Request) {
	nonce, err := newNonce()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	h := w.Header()
	h.Set("Content-Security-Policy", "default-src 'none'; script-src 'nonce-"+nonce+"'; style-src 'nonce-"+nonce+
		"'; connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cross-Origin-Opener-Policy", "same-origin")
	data := pageData{Nonce: nonce, Request: r.URL.Query().Get("request")}
	status, tmpl := s.decide(r, &data)
	w.WriteHeader(status)
	_ = tmpl.Execute(w, data)
}

// decide picks the page for a visitor: the ceremony for the person the
// request belongs to, and a notice for anyone else.
func (s *StepUpRoutes) decide(r *http.Request, data *pageData) (int, *template.Template) {
	secret, ok := cookie(r.Header, SessionCookie)
	if !ok {
		data.Message = "Sign in to Plux Studio in this browser, then open this link again."
		return http.StatusUnauthorized, s.notice
	}
	id, err := s.Auth.AuthenticateSession(r.Context(), secret)
	if err != nil {
		data.Message = "Your Studio session is not valid. Sign in again, then open this link again."
		return http.StatusUnauthorized, s.notice
	}
	if _, err := s.Auth.PendingStepUp(r.Context(), id, data.Request); err != nil {
		data.Message = "There is no step-up request waiting for you here. Run the command again."
		return http.StatusNotFound, s.notice
	}
	data.CSRF = id.CSRFToken
	return http.StatusOK, s.page
}

// newNonce returns a fresh CSP nonce.
func newNonce() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("api: %w", err)
	}
	return base64.RawStdEncoding.EncodeToString(b), nil
}

// stepUpNotice is the page for a visitor who cannot approve anything.
const stepUpNotice = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>Plux step-up</title>
<style nonce="{{.Nonce}}">body{font:16px/1.5 system-ui,sans-serif;max-width:32rem;margin:4rem auto;padding:0 1rem}</style>
</head><body><main><h1>Plux step-up</h1><p>{{.Message}}</p></main></body></html>`

// stepUpPage is the page of the ceremony. The script is static: what varies
// travels in data attributes, which the template escapes.
const stepUpPage = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>Plux step-up</title>
<style nonce="{{.Nonce}}">body{font:16px/1.5 system-ui,sans-serif;max-width:32rem;margin:4rem auto;padding:0 1rem}button{font:inherit;padding:.6rem 1rem}</style>
</head><body>
<main id="app" data-request="{{.Request}}" data-csrf="{{.CSRF}}">
<h1>Confirm with your security key</h1>
<p>The Plux command line asks you to confirm an action with your security key. Continue only if you just started it.</p>
<button id="go" type="button">Use my security key</button>
<p id="status" role="status"></p>
</main>
<script nonce="{{.Nonce}}">
(() => {
  const app = document.getElementById('app');
  const status = document.getElementById('status');
  const button = document.getElementById('go');
  const fromB64u = (s) => Uint8Array.from(atob(s.replace(/-/g, '+').replace(/_/g, '/')), (c) => c.charCodeAt(0));
  const toB64 = (buf) => btoa(String.fromCharCode(...new Uint8Array(buf)));
  const call = async (method, body) => {
    const res = await fetch('/plux.v1.IdentityService/' + method, {
      method: 'POST', credentials: 'same-origin',
      headers: {'Content-Type': 'application/json', 'Connect-Protocol-Version': '1', 'X-CSRF-Token': app.dataset.csrf},
      body: JSON.stringify(body),
    });
    const data = await res.json();
    if (!res.ok) { throw new Error(data.message || 'The request was refused.'); }
    return data;
  };
  button.addEventListener('click', async () => {
    button.disabled = true;
    status.textContent = 'Waiting for your security key...';
    try {
      const begun = await call('BeginStepUp', {cliRequestId: app.dataset.request});
      const o = JSON.parse(begun.optionsJson);
      const cred = await navigator.credentials.get({publicKey: {
        challenge: fromB64u(o.challenge), rpId: o.rpId, timeout: o.timeout, userVerification: o.userVerification,
        allowCredentials: o.allowCredentials.map((c) => ({type: c.type, id: fromB64u(c.id)})),
      }});
      const r = cred.response;
      await call('CompleteStepUp', {
        requestId: begun.requestId, credentialId: toB64(cred.rawId), clientDataJson: toB64(r.clientDataJSON),
        authenticatorData: toB64(r.authenticatorData), signature: toB64(r.signature),
      });
      status.textContent = 'Confirmed. You can return to the terminal.';
    } catch (e) {
      status.textContent = 'Not confirmed: ' + e.message;
      button.disabled = false;
    }
  });
})();
</script>
</body></html>`
