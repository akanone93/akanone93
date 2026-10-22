// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package webauthndriver

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

// sessionReply is the part of a sign-in answer that holds a session.
type sessionReply struct {
	Session *struct {
		CSRFToken string `json:"csrfToken"`
	} `json:"session"`
}

// open turns the answer into a session using the cookie the answer set.
func (r sessionReply) open(cookie string) (*session, error) {
	if r.Session == nil || cookie == "" || r.Session.CSRFToken == "" {
		return nil, errors.New("webauthndriver: the server returned no session")
	}
	return &session{cookie: cookie, csrf: r.Session.CSRFToken}, nil
}

// signedIn is where a sign-in through the provider ends: a session, or the
// challenge for a second factor.
type signedIn struct {
	session     *session
	challengeID string
}

// signInThroughOIDC plays a browser's part in the authorisation code flow with
// PKCE: the server's sign-in URL, the provider's code (the provider vouches for
// the person named by login_hint), the server's completion.
func (d *Driver) signInThroughOIDC(ctx context.Context) (signedIn, error) {
	providerID, err := d.providerID(ctx)
	if err != nil {
		return signedIn{}, err
	}
	var start struct {
		URL   string `json:"url"`
		State string `json:"state"`
	}
	if _, err := d.api.call(ctx, nil, identityService+"StartOidcLogin", map[string]any{"providerId": providerID}, &start); err != nil {
		return signedIn{}, err
	}
	code, err := d.authorize(ctx, start.URL, start.State)
	if err != nil {
		return signedIn{}, err
	}
	var done struct {
		sessionReply
		Challenge *struct {
			ID string `json:"id"`
		} `json:"challenge"`
	}
	cookie, err := d.api.call(ctx, nil, identityService+"CompleteOidcLogin", map[string]any{"state": start.State, "code": code}, &done)
	if err != nil {
		return signedIn{}, err
	}
	if done.Challenge != nil {
		return signedIn{challengeID: done.Challenge.ID}, nil
	}
	s, err := done.open(cookie)
	return signedIn{session: s}, err
}

// providerID finds the identity provider among those a person may sign in with.
func (d *Driver) providerID(ctx context.Context) (string, error) {
	var listed struct {
		Providers []struct {
			ID  string `json:"id"`
			Key string `json:"key"`
		} `json:"providers"`
	}
	if _, err := d.api.call(ctx, nil, federationService+"ListSignInProviders", map[string]any{"organizationKey": d.cfg.OrgKey}, &listed); err != nil {
		return "", err
	}
	for _, p := range listed.Providers {
		if p.Key == d.cfg.ProviderKey {
			return p.ID, nil
		}
	}
	return "", fmt.Errorf("webauthndriver: the installation offers no sign-in provider %q", d.cfg.ProviderKey)
}

// authorize visits the provider's sign-in URL as the person and returns the
// authorisation code it redirects back with, after checking that the state is
// the one the server issued.
func (d *Driver) authorize(ctx context.Context, signInURL, state string) (string, error) {
	u, err := url.Parse(signInURL)
	if err != nil {
		return "", fmt.Errorf("webauthndriver: the server's sign-in URL is malformed: %w", err)
	}
	q := u.Query()
	q.Set("login_hint", d.cfg.Email)
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", fmt.Errorf("webauthndriver: %w", err)
	}
	res, err := d.cfg.IDPHTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("webauthndriver: the test provider: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	back, err := res.Location()
	if err != nil {
		return "", fmt.Errorf("webauthndriver: the test provider answered HTTP %d without redirecting back", res.StatusCode)
	}
	if back.Query().Get("state") != state || back.Query().Get("code") == "" {
		return "", errors.New("webauthndriver: the test provider's redirect carries no code for this sign-in")
	}
	return back.Query().Get("code"), nil
}
