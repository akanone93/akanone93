// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package webauthndriver

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

// relyingParty is a fake of the server's side of the ceremonies: the sign-in
// through the test provider, the registration, the sign-in and step-up
// assertions, and the token. It verifies what backend/internal/auth verifies
// (origin, challenge, relying party hash, presence, verification, signature
// and counter), so a driver that passes here passes there.
type relyingParty struct {
	t      *testing.T
	rpID   string
	origin string
	idp    *httptest.Server
	api    *httptest.Server

	mu         sync.Mutex
	pub        *ecdsa.PublicKey
	credential []byte
	counter    uint32
	challenges map[string][]byte
	sessions   map[string]*fakeSession
	calls      []string
	stepUps    []string
	token      string
}

// fakeSession is a session of the fake server.
type fakeSession struct {
	csrf         string
	secondFactor bool
}

const (
	testEmail  = "alice@walkthrough.example"
	testOrgID  = "00000000-0000-0000-0000-000000000001"
	testOrgKey = "acme"
)

// fakeAccessToken is what the fake server issues; it is not a credential.
const fakeAccessToken = "plux_pat_" + "example"

// newRelyingParty starts the fake provider and the fake server.
func newRelyingParty(t *testing.T) *relyingParty {
	t.Helper()
	rp := &relyingParty{
		t: t, rpID: "plux.test", origin: "https://plux.test", token: fakeAccessToken,
		challenges: map[string][]byte{}, sessions: map[string]*fakeSession{},
	}
	rp.idp = httptest.NewTLSServer(http.HandlerFunc(rp.authorize))
	rp.api = httptest.NewServer(http.HandlerFunc(rp.serve))
	t.Cleanup(rp.idp.Close)
	t.Cleanup(rp.api.Close)
	return rp
}

// authorize is the provider's /authorize: it vouches for the login hint and
// redirects back with a code and the state it was given.
func (*relyingParty) authorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if q.Get("login_hint") == "" {
		http.Error(w, "no login_hint", http.StatusBadRequest)
		return
	}
	back := url.Values{"code": {"code-for-" + q.Get("login_hint")}, "state": {q.Get("state")}}
	http.Redirect(w, r, "https://plux.test/callback?"+back.Encode(), http.StatusFound)
}

// serve is the server's ConnectRPC surface the driver uses.
func (rp *relyingParty) serve(w http.ResponseWriter, r *http.Request) {
	rp.mu.Lock()
	defer rp.mu.Unlock()
	var req map[string]any
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		rp.fail(w, "invalid_argument", "not JSON")
		return
	}
	procedure := strings.TrimPrefix(r.URL.Path, "/plux.v1.")
	rp.calls = append(rp.calls, procedure)
	handlers := map[string]func(*http.Request, map[string]any) (any, string, error){
		"FederationService/ListSignInProviders":      rp.listProviders,
		"IdentityService/StartOidcLogin":             rp.startLogin,
		"IdentityService/CompleteOidcLogin":          rp.completeLogin,
		"IdentityService/BeginWebAuthnRegistration":  rp.beginRegistration,
		"IdentityService/FinishWebAuthnRegistration": rp.finishRegistration,
		"IdentityService/BeginWebAuthnLogin":         rp.beginLogin,
		"IdentityService/CompleteWebAuthnLogin":      rp.completeWebAuthnLogin,
		"IdentityService/CreateAccessToken":          rp.createToken,
		"IdentityService/BeginStepUp":                rp.beginStepUp,
		"IdentityService/CompleteStepUp":             rp.completeStepUp,
	}
	h, ok := handlers[procedure]
	if !ok {
		rp.fail(w, "unimplemented", procedure)
		return
	}
	out, cookie, err := h(r, req)
	if err != nil {
		rp.fail(w, "failed_precondition", err.Error())
		return
	}
	if cookie != "" {
		http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: cookie, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// fail answers with a Connect error.
func (*relyingParty) fail(w http.ResponseWriter, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(map[string]string{"code": code, "message": message})
}

func (*relyingParty) listProviders(_ *http.Request, req map[string]any) (any, string, error) {
	if req["organizationKey"] != testOrgKey {
		return map[string]any{}, "", nil
	}
	return map[string]any{"providers": []map[string]string{{"id": "idp-1", "key": "walkthrough-idp", "kind": "oidc"}}}, "", nil
}

func (rp *relyingParty) startLogin(_ *http.Request, req map[string]any) (any, string, error) {
	if req["providerId"] != "idp-1" {
		return nil, "", errors.New("unknown provider")
	}
	return map[string]any{"url": rp.idp.URL + "/authorize?client_id=c&state=state-1", "state": "state-1"}, "", nil
}

func (rp *relyingParty) completeLogin(_ *http.Request, req map[string]any) (any, string, error) {
	if req["state"] != "state-1" || req["code"] != "code-for-"+testEmail {
		return nil, "", errors.New("the code is not the provider's")
	}
	if rp.pub != nil {
		return map[string]any{"challenge": map[string]any{"id": "challenge-1", "kinds": []string{"webauthn"}}}, "", nil
	}
	return rp.newSession(false)
}

// newSession opens a session and returns the reply that carries it.
func (rp *relyingParty) newSession(secondFactor bool) (any, string, error) {
	cookie, csrf := fmt.Sprintf("cookie-%d", len(rp.sessions)+1), fmt.Sprintf("csrf-%d", len(rp.sessions)+1)
	rp.sessions[cookie] = &fakeSession{csrf: csrf, secondFactor: secondFactor}
	return map[string]any{"session": map[string]any{"id": "s", "csrfToken": csrf, "secondFactor": secondFactor}}, cookie, nil
}

// session checks the cookie and the CSRF header of a call and returns the session.
func (rp *relyingParty) session(r *http.Request) (*fakeSession, error) {
	c, err := r.Cookie(sessionCookie)
	if err != nil || rp.sessions[c.Value] == nil {
		return nil, errors.New("no session")
	}
	s := rp.sessions[c.Value]
	if r.Header.Get(csrfHeader) != s.csrf {
		return nil, errors.New("the CSRF token does not match the session")
	}
	return s, nil
}

// challenge makes a challenge for a ceremony.
func (rp *relyingParty) challenge(id string) string {
	c := sha256.Sum256([]byte("challenge " + id + fmt.Sprint(len(rp.calls))))
	rp.challenges[id] = c[:]
	return base64.RawURLEncoding.EncodeToString(c[:])
}

func (rp *relyingParty) beginRegistration(r *http.Request, _ map[string]any) (any, string, error) {
	if _, err := rp.session(r); err != nil {
		return nil, "", err
	}
	options, _ := json.Marshal(map[string]any{
		"challenge": rp.challenge("factor-1"), "rp": map[string]string{"id": rp.rpID, "name": "Plux"},
	})
	return map[string]any{"factor": map[string]string{"id": "factor-1"}, "optionsJson": options}, "", nil
}

func (rp *relyingParty) finishRegistration(r *http.Request, req map[string]any) (any, string, error) {
	s, err := rp.session(r)
	if err != nil {
		return nil, "", err
	}
	clientData, object := bytesOf(req["clientDataJson"]), bytesOf(req["attestationObject"])
	if err := rp.checkClientData(clientData, "webauthn.create", rp.challenges["factor-1"]); err != nil {
		return nil, "", err
	}
	v, _, err := decodeCBOR(object)
	if err != nil {
		return nil, "", err
	}
	attestation, _ := v.(map[any]any)
	if attestation["fmt"] != "none" {
		return nil, "", errors.New("attestation is not none")
	}
	authData, _ := attestation["authData"].([]byte)
	if err := rp.checkAuthData(authData, flagUserPresent|flagAttested); err != nil {
		return nil, "", err
	}
	if err := rp.takeCredential(authData[37:]); err != nil {
		return nil, "", err
	}
	s.secondFactor = true
	return map[string]any{"factor": map[string]string{"id": "factor-1"}}, "", nil
}

// takeCredential reads the attested credential data and keeps the key.
func (rp *relyingParty) takeCredential(rest []byte) error {
	n := int(binary.BigEndian.Uint16(rest[16:18]))
	rp.credential = rest[18 : 18+n]
	v, _, err := decodeCBOR(rest[18+n:])
	if err != nil {
		return err
	}
	key, _ := v.(map[any]any)
	if key[int64(1)] != int64(2) || key[int64(3)] != int64(coseES256) || key[int64(-1)] != int64(1) {
		return errors.New("the key is not ES256 on P-256")
	}
	x, _ := key[int64(-2)].([]byte)
	y, _ := key[int64(-3)].([]byte)
	pub, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), append(append([]byte{4}, x...), y...))
	if err != nil {
		return errors.New("the key is not on the curve")
	}
	rp.pub = pub
	return nil
}

func (rp *relyingParty) beginLogin(_ *http.Request, req map[string]any) (any, string, error) {
	if req["challengeId"] != "challenge-1" {
		return nil, "", errors.New("unknown challenge")
	}
	return map[string]any{"optionsJson": []byte(rp.requestOptions("challenge-1", "preferred"))}, "", nil
}

// requestOptions are PublicKeyCredentialRequestOptions for a ceremony.
func (rp *relyingParty) requestOptions(id, verification string) string {
	out, _ := json.Marshal(map[string]any{
		"challenge": rp.challenge(id), "rpId": rp.rpID, "timeout": 300000, "userVerification": verification,
		"allowCredentials": []map[string]string{{"type": "public-key", "id": base64.RawURLEncoding.EncodeToString(rp.credential)}},
	})
	return string(out)
}

func (rp *relyingParty) completeWebAuthnLogin(_ *http.Request, req map[string]any) (any, string, error) {
	if err := rp.verifyAssertion("challenge-1", req, false); err != nil {
		return nil, "", err
	}
	return rp.newSession(true)
}

func (rp *relyingParty) createToken(r *http.Request, req map[string]any) (any, string, error) {
	s, err := rp.session(r)
	if err != nil {
		return nil, "", err
	}
	if !s.secondFactor {
		return nil, "", errors.New("creating a token requires a second factor in this session")
	}
	if req["organizationId"] != testOrgID || req["ttlSeconds"].(float64) <= 0 {
		return nil, "", errors.New("the token names no organisation or lifetime")
	}
	return map[string]any{"token": map[string]string{"id": "t"}, "secret": rp.token}, "", nil
}

func (rp *relyingParty) beginStepUp(r *http.Request, req map[string]any) (any, string, error) {
	if _, err := rp.session(r); err != nil {
		return nil, "", err
	}
	if req["cliRequestId"] != "request-1" {
		return nil, "", errors.New("no such step-up request")
	}
	return map[string]any{"requestId": "request-1", "optionsJson": rp.requestOptions("request-1", "required")}, "", nil
}

func (rp *relyingParty) completeStepUp(r *http.Request, req map[string]any) (any, string, error) {
	if _, err := rp.session(r); err != nil {
		return nil, "", err
	}
	if err := rp.verifyAssertion("request-1", req, true); err != nil {
		return nil, "", err
	}
	rp.stepUps = append(rp.stepUps, fmt.Sprint(req["requestId"]))
	return map[string]any{}, "", nil
}

// verifyAssertion checks an assertion as the server's verifyAssertion does.
func (rp *relyingParty) verifyAssertion(challenge string, req map[string]any, requireUV bool) error {
	clientData, authData, sig := bytesOf(req["clientDataJson"]), bytesOf(req["authenticatorData"]), bytesOf(req["signature"])
	if !bytes.Equal(bytesOf(req["credentialId"]), rp.credential) {
		return errors.New("unknown credential")
	}
	if err := rp.checkClientData(clientData, "webauthn.get", rp.challenges[challenge]); err != nil {
		return err
	}
	flags := byte(flagUserPresent)
	if requireUV {
		flags |= flagUserVerified
	}
	if err := rp.checkAuthData(authData, flags); err != nil {
		return err
	}
	digest := sha256.Sum256(clientData)
	signed := sha256.Sum256(append(append([]byte{}, authData...), digest[:]...))
	if !ecdsa.VerifyASN1(rp.pub, signed[:], sig) {
		return errors.New("the signature does not verify")
	}
	count := binary.BigEndian.Uint32(authData[33:37])
	if count <= rp.counter {
		return errors.New("the signature counter went backwards; the key may be cloned")
	}
	rp.counter = count
	return nil
}

// checkClientData verifies the ceremony type, challenge and origin.
func (rp *relyingParty) checkClientData(raw []byte, ceremony string, challenge []byte) error {
	var cd struct {
		Type, Challenge, Origin string
		CrossOrigin             bool
	}
	if err := json.Unmarshal(raw, &cd); err != nil {
		return err
	}
	got, err := base64.RawURLEncoding.DecodeString(cd.Challenge)
	switch {
	case err != nil || !bytes.Equal(got, challenge):
		return errors.New("the challenge does not match")
	case cd.Type != ceremony:
		return fmt.Errorf("a %q ceremony, not %q", cd.Type, ceremony)
	case cd.CrossOrigin || cd.Origin != rp.origin:
		return fmt.Errorf("origin %q is not Studio's", cd.Origin)
	}
	return nil
}

// checkAuthData verifies the relying party hash and that every flag in want is set.
func (rp *relyingParty) checkAuthData(authData []byte, want byte) error {
	hash := sha256.Sum256([]byte(rp.rpID))
	if len(authData) < 37 || !bytes.Equal(authData[:32], hash[:]) {
		return errors.New("the credential belongs to another site")
	}
	if authData[32]&want != want {
		return fmt.Errorf("flags %08b lack %08b", authData[32], want)
	}
	return nil
}

// bytesOf reads a bytes field of a request, which the driver sends as base64.
func bytesOf(v any) []byte {
	s, _ := v.(string)
	b, _ := base64.StdEncoding.DecodeString(s)
	return b
}

// noRedirects makes the client report a redirect instead of following it, as
// the command's provider client does.
func noRedirects(c *http.Client) *http.Client {
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return c
}
