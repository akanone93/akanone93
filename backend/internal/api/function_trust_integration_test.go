// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"testing"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/attest/binding"
	"github.com/nightCode42/plux3/backend/internal/device/devicetest"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
)

const (
	declAttested = `{"name":"needs.attestation","placement":"server","assurance":"AL0","attestation":true}`
	declSCA      = `{"name":"needs.sca","placement":"server","assurance":"AL0","sca":true}`
	inputA       = `{"amount":"5"}`
	inputB       = `{"amount":"5000"}`
)

// invokeWithID calls FunctionService.Invoke as the device with a proof
// whose identifier the test chooses, so that it can compute what the
// platform evidence is bound to.
func (f *invokeFixture) invokeWithID(t *testing.T, m *pluxv1.FunctionServiceInvokeRequest, jti string) (*pluxv1.FunctionServiceInvokeResponse, error) {
	t.Helper()
	r := connect.NewRequest(m)
	r.Header().Set("Authorization", "DPoP "+f.dev.token)
	r.Header().Set("DPoP", devicetest.SignProof(t, f.dev.key, devicetest.Proof{
		Method: "POST", URL: f.w.base + pluxv1connect.FunctionServiceInvokeProcedure, AccessToken: f.dev.token, Nonce: f.dev.nonce, ID: jti,
	}))
	res, err := f.fn.Invoke(t.Context(), r)
	if err != nil {
		return nil, err //nolint:wrapcheck // the code is what the test reads
	}
	return res.Msg, nil
}

// asIOS makes the device an iOS device with an attested App Attest key.
func (f *invokeFixture) asIOS(t *testing.T) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	f.exec(t, `UPDATE devices SET platform = 'ios', app_attest_public_key = $2, app_attest_counter = 0 WHERE id = $1`, f.dev.id, der)
}

// asAndroid makes the device an Android device.
func (f *invokeFixture) asAndroid(t *testing.T) {
	t.Helper()
	f.exec(t, `UPDATE devices SET platform = 'android', app_attest_public_key = NULL WHERE id = $1`, f.dev.id)
}

func requestHash(h [32]byte) string { return base64.RawURLEncoding.EncodeToString(h[:]) }

// Verifies: FN-020, SEC-004.
// A function that declares attestation runs on iOS only with an App Attest
// assertion over the function, version, input and the request's DPoP proof
// identifier, whose counter strictly increases; every other case is
// PLX-6001 and no module runs.
func TestFunctionInvokeAppAttest_FN_020(t *testing.T) {
	t.Parallel()
	f := newInvokeFixture(t)
	attested := f.deployPinned(t, "needs.attestation", declAttested)
	plain := f.deployPinned(t, "plain.fn", `{"name":"plain.fn","placement":"server","assurance":"AL0"}`)
	runner := f.w.fnrt.runner
	assert := f.w.fakes.assert
	bind := func(input, jti string) [32]byte {
		return binding.InvocationHash(attested.GetName(), attested.GetVersion(), []byte(input), jti)
	}
	with := func(input string) *pluxv1.FunctionServiceInvokeRequest {
		m := invokeOf(attested, input)
		m.AppAttestAssertion = []byte("assertion")
		return m
	}

	// A device with no platform evidence cannot satisfy the function.
	if _, err := f.invokeWithID(t, with(inputA), "jti-linux"); reasonOf(t, err) != "ATTESTATION_FAILED" {
		t.Errorf("a device that cannot attest: %v", err)
	}

	f.asIOS(t)
	assert.Counter = 1
	want := bind(inputA, "jti-1")
	assert.Want = &want
	if _, err := f.invokeWithID(t, with(inputA), "jti-1"); err != nil {
		t.Fatalf("a valid assertion: %v", err)
	}
	if assert.ClientDataHash != want || assert.AppID != "TEAMID.com.example.app" || assert.LastCounter != 0 {
		t.Errorf("the assertion was checked as %x %q %d", assert.ClientDataHash, assert.AppID, assert.LastCounter)
	}
	if runner.calls() != 1 {
		t.Fatalf("runner calls = %d", runner.calls())
	}

	// The counter does not advance: a replayed assertion is refused.
	want = bind(inputA, "jti-2")
	if _, err := f.invokeWithID(t, with(inputA), "jti-2"); reasonOf(t, err) != "ATTESTATION_FAILED" {
		t.Errorf("a replayed counter: %v", err)
	}

	// An assertion over another input is refused, and spends no counter.
	assert.Counter = 2
	want = bind(inputA, "jti-3")
	if _, err := f.invokeWithID(t, with(inputB), "jti-3"); reasonOf(t, err) != "ATTESTATION_FAILED" {
		t.Errorf("an assertion over another input: %v", err)
	}
	if _, err := f.invokeWithID(t, invokeOf(attested, inputA), "jti-4"); reasonOf(t, err) != "ATTESTATION_FAILED" {
		t.Errorf("no assertion: %v", err)
	}
	if runner.calls() != 1 {
		t.Errorf("a refused call reached the runner (%d calls)", runner.calls())
	}
	want = bind(inputA, "jti-5")
	if _, err := f.invokeWithID(t, with(inputA), "jti-5"); err != nil {
		t.Errorf("the counter was spent by a refused call: %v", err)
	}

	// The proof identifier is single-use, so the evidence bound to it is.
	assert.Counter = 3
	want = bind(inputA, "jti-6")
	if _, err := f.invokeWithID(t, with(inputA), "jti-6"); err != nil {
		t.Fatalf("a fresh proof: %v", err)
	}
	assert.Counter = 4
	if _, err := f.invokeWithID(t, with(inputA), "jti-6"); reasonOf(t, err) != "DPOP_REPLAY" {
		t.Errorf("a reused jti: %v", err)
	}

	// A function that declares nothing needs no evidence.
	if _, err := f.invoke(t, invokeOf(plain, `{}`)); err != nil {
		t.Errorf("a function without the flag: %v", err)
	}
}

// Verifies: FN-020, SEC-003.
// A function that declares attestation runs on Android only with a Play
// Integrity token whose requestHash is the invocation's binding.
func TestFunctionInvokePlayIntegrity_FN_020(t *testing.T) {
	t.Parallel()
	f := newInvokeFixture(t)
	attested := f.deployPinned(t, "needs.attestation", declAttested)
	f.asAndroid(t)
	play := f.w.fakes.play
	hash := func(input, jti string) string {
		return binding.InvocationRequestHash(attested.GetName(), attested.GetVersion(), []byte(input), jti)
	}
	with := func(input string) *pluxv1.FunctionServiceInvokeRequest {
		m := invokeOf(attested, input)
		m.PlayIntegrityToken = "token"
		return m
	}

	play.RequestHash = hash(inputA, "jti-1")
	if _, err := f.invokeWithID(t, with(inputA), "jti-1"); err != nil {
		t.Fatalf("a bound token: %v", err)
	}
	if play.Expect.RequestHash != play.RequestHash || play.Expect.PackageName != "com.example.app" {
		t.Errorf("the token was checked as %+v", play.Expect)
	}
	runner := f.w.fnrt.runner
	before := runner.calls()

	play.RequestHash = hash(inputA, "jti-2")
	if _, err := f.invokeWithID(t, with(inputB), "jti-2"); reasonOf(t, err) != "ATTESTATION_FAILED" {
		t.Errorf("a token over another input: %v", err)
	}
	play.RequestHash = hash(inputA, "another-jti")
	if _, err := f.invokeWithID(t, with(inputA), "jti-3"); reasonOf(t, err) != "ATTESTATION_FAILED" {
		t.Errorf("a token bound to another proof: %v", err)
	}
	play.RequestHash = hash(inputA, "jti-4")
	if _, err := f.invokeWithID(t, invokeOf(attested, inputA), "jti-4"); reasonOf(t, err) != "ATTESTATION_FAILED" {
		t.Errorf("no token: %v", err)
	}
	if runner.calls() != before {
		t.Error("a refused call reached the runner")
	}
	if _, err := f.invokeWithID(t, with(inputA), "jti-1"); reasonOf(t, err) != "DPOP_REPLAY" {
		t.Errorf("a reused jti: %v", err)
	}
}

// challengeFor asks for the SCA challenge of an invocation as the device
// does and signs it with the device's SCA key.
func (f *invokeFixture) challengeFor(t *testing.T, v *pluxv1.FunctionVersion, input string) (*pluxv1.CreateTransactionChallengeResponse, []byte, error) {
	t.Helper()
	sum := sha256.Sum256([]byte(input))
	res, err := f.w.sca.CreateTransactionChallenge(t.Context(), bound(t, f.dev, pluxv1connect.ScaServiceCreateTransactionChallengeProcedure,
		&pluxv1.CreateTransactionChallengeRequest{
			Function: v.GetName(), FunctionVersion: v.GetVersion(), InputSha256: sum[:], TransactionSummaryJcs: []byte(`{"summary":"Pay 5 EUR"}`),
		}))
	if err != nil {
		return nil, nil, err //nolint:wrapcheck // the code is what the test reads
	}
	c := res.Msg
	return c, f.w.fnrt.sca.sign(t, c.GetChallenge(), c.GetTransactionSummaryJcs()), nil
}

// Verifies: FN-020, SEC-027, SEC-028.
// A function that declares sca runs only with a challenge issued for it
// and signed over the summary that binds the input; a challenge for
// another input, a forged signature, a spent challenge and a missing one
// are refused with the gateway's codes and no module runs.
func TestFunctionInvokeSCA_SEC_027(t *testing.T) {
	t.Parallel()
	f := newInvokeFixture(t)
	sca := f.deployPinned(t, "needs.sca", declSCA)
	plain := f.deployPinned(t, "plain.fn", `{"name":"plain.fn","placement":"server","assurance":"AL0"}`)
	runner := f.w.fnrt.runner
	confirmed := func(c *pluxv1.CreateTransactionChallengeResponse, sig []byte, input string) *pluxv1.FunctionServiceInvokeRequest {
		m := invokeOf(sca, input)
		m.TransactionChallengeId, m.TransactionSignature = c.GetChallengeId(), sig
		return m
	}

	c, sig, err := f.challengeFor(t, sca, inputA)
	if err != nil {
		t.Fatalf("CreateTransactionChallenge: %v", err)
	}
	var summary map[string]string
	if err := json.Unmarshal(c.GetTransactionSummaryJcs(), &summary); err != nil ||
		summary["function"] != "needs.sca" || summary["summary"] != "Pay 5 EUR" || summary["version"] == "" || summary["inputSha256"] == "" {
		t.Errorf("summary = %s (%v)", c.GetTransactionSummaryJcs(), err)
	}
	if _, err := f.invoke(t, confirmed(c, sig, inputA)); err != nil {
		t.Fatalf("a confirmed invocation: %v", err)
	}
	if runner.calls() != 1 {
		t.Fatalf("runner calls = %d", runner.calls())
	}
	if _, err := f.invoke(t, confirmed(c, sig, inputA)); reasonOf(t, err) != "SCA_CHALLENGE_INVALID" {
		t.Errorf("a spent challenge: %v", err)
	}

	other, otherSig, err := f.challengeFor(t, sca, inputA)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.invoke(t, confirmed(other, otherSig, inputB)); reasonOf(t, err) != "SCA_DYNAMIC_LINKING_MISMATCH" {
		t.Errorf("a challenge for another input: %v", err)
	}
	forged, _, err := f.challengeFor(t, sca, inputA)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.invoke(t, confirmed(forged, []byte("forged"), inputA)); reasonOf(t, err) != "SCA_SIGNATURE_INVALID" {
		t.Errorf("a forged signature: %v", err)
	}
	if _, err := f.invoke(t, invokeOf(sca, inputA)); reasonOf(t, err) != "SCA_CHALLENGE_INVALID" {
		t.Errorf("no challenge: %v", err)
	}
	if runner.calls() != 1 {
		t.Errorf("a refused call reached the runner (%d calls)", runner.calls())
	}

	// A challenge is issued only for a function that declares sca, and
	// for a function alone.
	if _, _, err := f.challengeFor(t, plain, inputA); reasonOf(t, err) != "PRECONDITION_FAILED" {
		t.Errorf("a challenge for a function without sca: %v", err)
	}
	missing := &pluxv1.FunctionVersion{Name: "no.such", Version: 1}
	if _, _, err := f.challengeFor(t, missing, inputA); reasonOf(t, err) != "FUNCTION_NOT_FOUND" {
		t.Errorf("a challenge for an unknown function: %v", err)
	}
	sum := sha256.Sum256([]byte(inputA))
	both := &pluxv1.CreateTransactionChallengeRequest{
		Function: "needs.sca", FunctionVersion: sca.GetVersion(), InputSha256: sum[:], DataSource: "bank", Operation: "transfer", TransactionSummaryJcs: []byte(`{}`),
	}
	if _, err := f.w.sca.CreateTransactionChallenge(t.Context(), bound(t, f.dev, pluxv1connect.ScaServiceCreateTransactionChallengeProcedure, both)); reasonOf(t, err) != "INVALID_FORMAT" {
		t.Errorf("a challenge for a function and an upstream: %v", err)
	}

	// A function that declares nothing needs neither.
	if _, err := f.invoke(t, invokeOf(plain, `{}`)); err != nil {
		t.Errorf("a function without the flags: %v", err)
	}
}
