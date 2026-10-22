// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package webauthndriver

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// The procedures the driver calls.
const (
	identityService   = "/plux.v1.IdentityService/"
	federationService = "/plux.v1.FederationService/"
	orgService        = "/plux.v1.OrgService/"
	appService        = "/plux.v1.AppService/"
)

// tokenLifetime is how long the access token of a walkthrough lasts: longer
// than a run, short enough that a leaked token is no lasting harm.
const tokenLifetime = 6 * time.Hour

// Config says where the driver acts and as whom.
type Config struct {
	// Server is the installation's public base URL.
	Server string
	// Origin is the origin the browser would report for the relying party,
	// which must be one the server accepts; empty uses the scheme and host of
	// Server.
	Origin string
	// OrgID is the organisation a token is created in; OrgKey names it for
	// the list of sign-in providers.
	OrgID, OrgKey string
	// ProviderKey is the key of the identity provider to sign in through.
	ProviderKey string
	// Actor names the person in the store; Email is the address the test
	// provider vouches for.
	Actor, Email string
	// Password, when set, signs the person in with Email and Password
	// instead of through the test provider: the bootstrapped administrator,
	// who has no account there; the driver enrols their key at the
	// bootstrap, or at their first step-up when it did not run.
	Password string
	// Store keeps the credential between the token and the step-up.
	Store FileStore
	// ServerHTTP reaches the installation and IDPHTTP the test provider; each
	// trusts the CA of its endpoint. IDPHTTP must not follow redirects.
	ServerHTTP, IDPHTTP *http.Client
	// Random supplies the randomness of keys and signatures; nil uses
	// crypto/rand.
	Random io.Reader
}

// Driver is a person's browser with a security key: it carries out the
// ceremonies the walkthrough needs.
type Driver struct {
	cfg    Config
	api    connectClient
	origin string
	random io.Reader
}

// New returns a driver for the configuration.
func New(cfg Config) (*Driver, error) {
	if cfg.Server == "" || cfg.Actor == "" || cfg.Email == "" || cfg.ServerHTTP == nil || cfg.IDPHTTP == nil {
		return nil, errors.New("webauthndriver.New: the server, the person, their email and both HTTP clients are required")
	}
	origin := cfg.Origin
	if origin == "" {
		u, err := url.Parse(cfg.Server)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return nil, fmt.Errorf("webauthndriver.New: %q is not an absolute URL", cfg.Server)
		}
		origin = u.Scheme + "://" + u.Host
	}
	random := cfg.Random
	if random == nil {
		random = rand.Reader
	}
	return &Driver{cfg: cfg, api: connectClient{baseURL: cfg.Server, http: cfg.ServerHTTP}, origin: origin, random: random}, nil
}

// Token signs the person in and returns an access token. A person with no
// security key yet is signed in through the test provider and enrols the
// driver's key; a person with one signs in with it, because creating a token
// needs a second factor in the session (SRV-064).
func (d *Driver) Token(ctx context.Context) (string, error) {
	var s *session
	cred, err := d.cfg.Store.Load(d.cfg.Actor)
	switch {
	case errors.Is(err, ErrNoCredential):
		s, err = d.enrol(ctx)
	case err != nil:
		return "", err
	default:
		s, err = d.secondFactorSession(ctx, AuthenticatorFor(cred, d.random))
	}
	if err != nil {
		return "", err
	}
	return d.mintToken(ctx, s, d.cfg.OrgID)
}

// mintToken creates an access token in the organisation from a session with
// a second factor.
func (d *Driver) mintToken(ctx context.Context, s *session, orgID string) (string, error) {
	var minted struct {
		Secret string `json:"secret"`
	}
	_, err := d.api.call(ctx, s, identityService+"CreateAccessToken", map[string]any{
		"name": "walkthrough " + d.cfg.Actor, "organizationId": orgID, "ttlSeconds": int64(tokenLifetime / time.Second),
	}, &minted)
	if err != nil {
		return "", err
	}
	if minted.Secret == "" {
		return "", errors.New("webauthndriver.Driver.Token: the server returned no token")
	}
	return minted.Secret, nil
}

// Installation is what Bootstrap prepared: the organisation, its app and the
// administrator's access token in it.
type Installation struct {
	OrgID, AppID, Token string
}

// Bootstrap prepares a new installation the way its operator does: it accepts
// the invitation `plux-server bootstrap` printed with Config.Password, enrols
// the driver's key as the administrator's second factor, creates the
// organisation and the app, and mints the administrator's token in the
// organisation. The key stays in the store, so later step-ups of the
// administrator sign in with it.
func (d *Driver) Bootstrap(ctx context.Context, invitation, orgKey, appKey string) (Installation, error) {
	if d.cfg.Password == "" || invitation == "" {
		return Installation{}, errors.New("webauthndriver.Driver.Bootstrap: an invitation and a password are required")
	}
	_, err := d.api.call(ctx, nil, identityService+"AcceptInvitation", map[string]any{
		"invitation": invitation, "displayName": "Administrator", "password": d.cfg.Password,
	}, &struct{}{})
	if err != nil {
		return Installation{}, err
	}
	s, err := d.enrol(ctx)
	if err != nil {
		return Installation{}, err
	}
	var org struct {
		Organization struct {
			ID string `json:"id"`
		} `json:"organization"`
	}
	if _, err := d.api.call(ctx, s, orgService+"CreateOrganization", map[string]any{"key": orgKey, "name": orgKey}, &org); err != nil {
		return Installation{}, err
	}
	var app struct {
		App struct {
			ID string `json:"id"`
		} `json:"app"`
	}
	_, err = d.api.call(ctx, s, appService+"CreateApp", map[string]any{"organizationId": org.Organization.ID, "key": appKey, "name": appKey}, &app)
	if err != nil {
		return Installation{}, err
	}
	if org.Organization.ID == "" || app.App.ID == "" {
		return Installation{}, errors.New("webauthndriver.Driver.Bootstrap: the server returned no organisation or app")
	}
	token, err := d.mintToken(ctx, s, org.Organization.ID)
	if err != nil {
		return Installation{}, err
	}
	return Installation{OrgID: org.Organization.ID, AppID: app.App.ID, Token: token}, nil
}

// StepUp completes the step-up request the plux CLI printed the URL of, in a
// session of the person it was made for, with the security key the person
// enrolled (SEC-103, ADR-0075).
func (d *Driver) StepUp(ctx context.Context, stepUpURL string) error {
	requestID, err := requestOf(stepUpURL)
	if err != nil {
		return err
	}
	s, key, err := d.stepUpSession(ctx)
	if err != nil {
		return err
	}
	var begun struct {
		RequestID   string `json:"requestId"`
		OptionsJSON string `json:"optionsJson"`
	}
	if _, err := d.api.call(ctx, s, identityService+"BeginStepUp", map[string]any{"cliRequestId": requestID}, &begun); err != nil {
		return err
	}
	options, err := ParseRequestOptions([]byte(begun.OptionsJSON))
	if err != nil {
		return err
	}
	assertion, err := d.assert(key, options)
	if err != nil {
		return err
	}
	var done struct{}
	_, err = d.api.call(ctx, s, identityService+"CompleteStepUp", assertionRequest(map[string]any{"requestId": begun.RequestID}, assertion), &done)
	return err
}

// stepUpSession signs the person in with their security key. A person who
// signs in with a password and has no key yet enrols one first; through the
// test provider the key comes from Token, and its absence is an error.
func (d *Driver) stepUpSession(ctx context.Context) (*session, *Authenticator, error) {
	cred, err := d.cfg.Store.Load(d.cfg.Actor)
	if errors.Is(err, ErrNoCredential) && d.cfg.Password != "" {
		s, err := d.enrol(ctx)
		if err != nil {
			return nil, nil, err
		}
		if cred, err = d.cfg.Store.Load(d.cfg.Actor); err != nil {
			return nil, nil, err
		}
		return s, AuthenticatorFor(cred, d.random), nil
	}
	if err != nil {
		return nil, nil, err
	}
	key := AuthenticatorFor(cred, d.random)
	s, err := d.secondFactorSession(ctx, key)
	return s, key, err
}

// requestOf reads the step-up request out of the URL the CLI printed.
func requestOf(stepUpURL string) (string, error) {
	u, err := url.Parse(stepUpURL)
	if err != nil || u.Query().Get("request") == "" {
		return "", fmt.Errorf("webauthndriver.Driver.StepUp: %q is not a step-up URL", stepUpURL)
	}
	return u.Query().Get("request"), nil
}

// enrol signs the person in, which must end in a session because no key is
// enrolled yet, and registers a new security key as their second factor. The
// credential is saved before the server is asked to confirm it: a key the
// server confirmed and the driver forgot would lock the person out, while a
// saved key the server never confirmed fails the run visibly.
func (d *Driver) enrol(ctx context.Context) (*session, error) {
	in, err := d.signIn(ctx)
	if err != nil {
		return nil, err
	}
	if in.session == nil {
		return nil, fmt.Errorf("webauthndriver.Driver.enrol: %s already has a second factor this driver does not hold", d.cfg.Actor)
	}
	key, err := NewAuthenticator(d.random)
	if err != nil {
		return nil, err
	}
	var begun struct {
		Factor struct {
			ID string `json:"id"`
		} `json:"factor"`
		OptionsJSON string `json:"optionsJson"`
	}
	if _, err := d.api.call(ctx, in.session, identityService+"BeginWebAuthnRegistration", map[string]any{"label": "walkthrough"}, &begun); err != nil {
		return nil, err
	}
	raw, err := decodeBytes(begun.OptionsJSON)
	if err != nil {
		return nil, err
	}
	options, err := ParseCreationOptions(raw)
	if err != nil {
		return nil, err
	}
	attestation, err := key.Create(options, d.origin)
	if err != nil {
		return nil, err
	}
	if err := d.cfg.Store.Save(d.cfg.Actor, key.Credential()); err != nil {
		return nil, err
	}
	var done struct{}
	_, err = d.api.call(ctx, in.session, identityService+"FinishWebAuthnRegistration", map[string]any{
		"factorId": begun.Factor.ID, "clientDataJson": attestation.ClientDataJSON, "attestationObject": attestation.AttestationObject,
	}, &done)
	return in.session, err
}

// secondFactorSession signs the person in (see signIn) and
// answers the second-factor challenge with the key.
func (d *Driver) secondFactorSession(ctx context.Context, key *Authenticator) (*session, error) {
	in, err := d.signIn(ctx)
	if err != nil {
		return nil, err
	}
	if in.challengeID == "" {
		return nil, fmt.Errorf("webauthndriver.Driver: %s signed in without being asked for the security key that was enrolled", d.cfg.Actor)
	}
	var begun struct {
		OptionsJSON string `json:"optionsJson"`
	}
	if _, err := d.api.call(ctx, nil, identityService+"BeginWebAuthnLogin", map[string]any{"challengeId": in.challengeID}, &begun); err != nil {
		return nil, err
	}
	raw, err := decodeBytes(begun.OptionsJSON)
	if err != nil {
		return nil, err
	}
	options, err := ParseRequestOptions(raw)
	if err != nil {
		return nil, err
	}
	assertion, err := d.assert(key, options)
	if err != nil {
		return nil, err
	}
	var done sessionReply
	cookie, err := d.api.call(ctx, nil, identityService+"CompleteWebAuthnLogin", assertionRequest(map[string]any{"challengeId": in.challengeID}, assertion), &done)
	if err != nil {
		return nil, err
	}
	return done.open(cookie)
}

// assert answers a ceremony and saves the advanced counter before the answer
// is used, so that a failure to save never leaves the server ahead of the file.
func (d *Driver) assert(key *Authenticator, options RequestOptions) (Assertion, error) {
	assertion, err := key.Get(options, d.origin)
	if err != nil {
		return Assertion{}, err
	}
	if err := d.cfg.Store.Save(d.cfg.Actor, key.Credential()); err != nil {
		return Assertion{}, err
	}
	return assertion, nil
}

// assertionRequest adds an assertion's fields to a request message.
func assertionRequest(m map[string]any, a Assertion) map[string]any {
	m["credentialId"], m["clientDataJson"] = a.CredentialID, a.ClientDataJSON
	m["authenticatorData"], m["signature"] = a.AuthenticatorData, a.Signature
	return m
}
