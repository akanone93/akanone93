// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestLogin(t *testing.T) {
	srv := testServer(t, Options{})
	tests := []struct {
		name   string
		body   string
		status int
		code   string
	}{
		{"the demo user", `{"username":"demo","pin":"246810","otp":"123456"}`, http.StatusOK, ""},
		{"a wrong PIN", `{"username":"demo","pin":"000000","otp":"123456"}`, http.StatusUnauthorized, "invalid_credentials"},
		{"a wrong code", `{"username":"demo","pin":"246810","otp":"654321"}`, http.StatusUnauthorized, "invalid_credentials"},
		{"a wrong username", `{"username":"eve","pin":"246810","otp":"123456"}`, http.StatusUnauthorized, "invalid_credentials"},
		{"empty credentials", `{}`, http.StatusUnauthorized, "invalid_credentials"},
		{"a body that is no JSON", `not json`, http.StatusBadRequest, "bad_request"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := call(t, srv, http.MethodPost, "/bank/v1/login", aDevice, tc.body)
			if r.status != tc.status {
				t.Fatalf("status %d, want %d: %s", r.status, tc.status, r.body)
			}
			if tc.code != "" {
				if got := r.errorCode(t); got != tc.code {
					t.Fatalf("error code %q, want %q", got, tc.code)
				}
				return
			}
			var out struct {
				Token     string
				ExpiresIn int
			}
			r.json(t, &out)
			if len(out.Token) < 32 || out.ExpiresIn <= 0 {
				t.Fatalf("login answered %+v", out)
			}
		})
	}
}

// loginToken signs the demo user in and returns the token.
func loginToken(t *testing.T, srv *testAPI) string {
	t.Helper()
	r := call(t, srv, http.MethodPost, "/bank/v1/login", aDevice, `{"username":"demo","pin":"246810","otp":"123456"}`)
	if r.status != http.StatusOK {
		t.Fatalf("login: %d %s", r.status, r.body)
	}
	var out struct{ Token string }
	r.json(t, &out)
	return out.Token
}

func TestLoginIssuesADifferentTokenEachTime(t *testing.T) {
	srv := testServer(t, Options{})
	if a, b := loginToken(t, srv), loginToken(t, srv); a == b {
		t.Fatal("two logins share a token")
	}
}

// Verifies: SEC-026.
// The identity provider's token verifies against the key set it publishes,
// which is what the Plux server does before it exchanges the token, and
// carries the issuer and audience an environment registers.
func TestIdentityProviderTokenVerifiesWithItsPublishedKeys(t *testing.T) {
	srv := testServer(t, Options{})
	token := loginToken(t, srv)
	jws, err := parseJWS(token)
	if err != nil {
		t.Fatal(err)
	}
	r := call(t, srv, http.MethodGet, idpPath+"/jwks.json", nobody, "")
	var set jwkSet
	r.json(t, &set)
	if r.status != http.StatusOK || len(set.Keys) != 1 || set.Keys[0].Kid != jws.header.Kid {
		t.Fatalf("keys answered %d %s for the key %q", r.status, r.body, jws.header.Kid)
	}
	pub, err := set.Keys[0].key()
	if err != nil || !jws.verify(pub) {
		t.Fatalf("the token does not verify: %v", err)
	}
	var c idpClaims
	if err := json.Unmarshal(jws.payload, &c); err != nil {
		t.Fatal(err)
	}
	if c.Iss != srv.URL+idpPath || c.Aud != idpAudience || c.Sub != demoSubject || c.EXP <= c.IAT {
		t.Fatalf("claims %+v", c)
	}
}

// Verifies: SEC-031, SEC-026.
// The bank trusts the gateway's assertion and nothing else about the caller.
func TestBankEndpointsTrustOnlyTheGatewaysAssertion(t *testing.T) {
	srv := testServer(t, Options{})
	other, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	const body = `{"fromAccountId":"acct-001","toIban":"DE44500105175407324931","amount":"1.00","currency":"EUR"}`
	endpoints := []struct{ method, path string }{
		{http.MethodPost, "/bank/v1/login"},
		{http.MethodGet, "/bank/v1/accounts"},
		{http.MethodPost, "/bank/v1/transfers"},
		{http.MethodPost, "/bank/graphql"},
		{http.MethodGet, "/bank/v1/notifications"},
	}
	cases := map[string]func(ep struct{ method, path string }) string{
		"no assertion":        func(struct{ method, path string }) string { return "" },
		"a bearer token only": func(struct{ method, path string }) string { return "" },
		"an assertion signed with another key": func(ep struct{ method, path string }) string {
			g := &testGateway{key: other, kid: srv.gw.kid, now: time.Now}
			return g.assertion(t, aConfirmedUser, ep.method, ep.path, body)
		},
		"an unknown key": func(ep struct{ method, path string }) string {
			return srv.gw.assertion(t, aConfirmedUser, ep.method, ep.path, body, func(h *jwsHeader, _ map[string]any) { h.Kid = "nope" })
		},
		"another type (an access token)": func(ep struct{ method, path string }) string {
			return srv.gw.assertion(t, aConfirmedUser, ep.method, ep.path, body, func(h *jwsHeader, _ map[string]any) { h.Typ = "at+jwt" })
		},
		"another issuer": func(ep struct{ method, path string }) string {
			return srv.gw.assertion(t, aConfirmedUser, ep.method, ep.path, body, func(_ *jwsHeader, c map[string]any) { c["iss"] = "https://evil.example" })
		},
		"another audience": func(ep struct{ method, path string }) string {
			return srv.gw.assertion(t, aConfirmedUser, ep.method, ep.path, body, func(_ *jwsHeader, c map[string]any) { c["aud"] = "other" })
		},
		"an expired assertion": func(ep struct{ method, path string }) string {
			return srv.gw.assertion(t, aConfirmedUser, ep.method, ep.path, body, func(_ *jwsHeader, c map[string]any) {
				c["iat"], c["exp"] = time.Now().Add(-10*time.Minute).Unix(), time.Now().Add(-9*time.Minute).Unix()
			})
		},
		"an assertion minted for another request": func(ep struct{ method, path string }) string {
			return srv.gw.assertion(t, aConfirmedUser, ep.method, ep.path+"/elsewhere", body)
		},
	}
	for name, mint := range cases {
		for _, ep := range endpoints {
			t.Run(ep.method+" "+ep.path+" "+name, func(t *testing.T) {
				req, err := http.NewRequestWithContext(t.Context(), ep.method, srv.URL+ep.path, strings.NewReader(body))
				if err != nil {
					t.Fatal(err)
				}
				if a := mint(ep); a != "" {
					req.Header.Set(assertionHeader, a)
				}
				req.Header.Set("Authorization", "Bearer "+loginToken(t, srv))
				r := send(t, srv, req)
				if r.status != http.StatusUnauthorized || r.errorCode(t) != "unauthorised" {
					t.Fatalf("status %d %s, want 401 unauthorised", r.status, r.body)
				}
			})
		}
	}
}

// Verifies: SEC-026, SEC-032.
// An endpoint asks the assertion for the user, and a transfer for the
// confirmation the gateway verified.
func TestBankEndpointsAskTheAssertionForTheUserAndTheConfirmation(t *testing.T) {
	srv := testServer(t, Options{})
	const body = `{"fromAccountId":"acct-001","toIban":"DE44500105175407324931","amount":"1.00","currency":"EUR"}`
	for _, ep := range []string{"/bank/v1/accounts", "/bank/graphql", "/bank/v1/notifications", "/bank/v1/transfers"} {
		method, sent := http.MethodGet, ""
		if ep == "/bank/graphql" || ep == "/bank/v1/transfers" {
			method, sent = http.MethodPost, body
		}
		r := call(t, srv, method, ep, aDevice, sent)
		if r.status != http.StatusForbidden || r.errorCode(t) != "user_required" {
			t.Fatalf("%s for a device without a user: %d %s", ep, r.status, r.body)
		}
	}
	r := call(t, srv, http.MethodPost, "/bank/v1/transfers", aUser, body)
	if r.status != http.StatusForbidden || r.errorCode(t) != "sca_required" {
		t.Fatalf("a transfer without a confirmation: %d %s", r.status, r.body)
	}
	if r := call(t, srv, http.MethodGet, "/bank/v1/accounts", aUser, ""); r.status != http.StatusOK {
		t.Fatalf("accounts for a user: %d", r.status)
	}
}

func TestAccounts(t *testing.T) {
	srv := testServer(t, Options{})
	r := call(t, srv, http.MethodGet, "/bank/v1/accounts", aUser, "")
	if r.status != http.StatusOK {
		t.Fatalf("status %d", r.status)
	}
	var out struct{ Items []account }
	r.json(t, &out)
	want := []account{
		{ID: "acct-001", Name: "Checking", IBAN: "DE89370400440532013000", BalanceCents: 250_000, Currency: "EUR"},
		{ID: "acct-002", Name: "Savings", IBAN: "DE02120300000000202051", BalanceCents: 1_200_000, Currency: "EUR"},
	}
	if len(out.Items) != len(want) || out.Items[0] != want[0] || out.Items[1] != want[1] {
		t.Fatalf("accounts %+v, want %+v", out.Items, want)
	}
}

func TestCreateTransfer(t *testing.T) {
	const other = "DE44500105175407324931"
	tests := []struct {
		name   string
		body   string
		status int
		code   string
	}{
		{"a transfer", `{"fromAccountId":"acct-001","toIban":"` + other + `","amount":"10.00","currency":"EUR","reference":"Lunch"}`, http.StatusCreated, ""},
		{"an IBAN with spaces and lowercase", `{"fromAccountId":"acct-001","toIban":"de44 5001 0517 5407 3249 31","amount":"10.00","currency":"EUR"}`, http.StatusCreated, ""},
		{"the whole balance", `{"fromAccountId":"acct-001","toIban":"` + other + `","amount":"2500.00","currency":"EUR"}`, http.StatusCreated, ""},
		{"an unknown account", `{"fromAccountId":"acct-999","toIban":"` + other + `","amount":"10.00","currency":"EUR"}`, http.StatusUnprocessableEntity, "unknown_account"},
		{"a zero amount", `{"fromAccountId":"acct-001","toIban":"` + other + `","amount":"0.00","currency":"EUR"}`, http.StatusUnprocessableEntity, "invalid_amount"},
		{"a negative amount", `{"fromAccountId":"acct-001","toIban":"` + other + `","amount":"-0.05","currency":"EUR"}`, http.StatusUnprocessableEntity, "invalid_amount"},
		{"an amount over the balance", `{"fromAccountId":"acct-001","toIban":"` + other + `","amount":"2500.01","currency":"EUR"}`, http.StatusUnprocessableEntity, "insufficient_funds"},
		{"an IBAN of the wrong shape", `{"fromAccountId":"acct-001","toIban":"12345","amount":"10.00","currency":"EUR"}`, http.StatusUnprocessableEntity, "invalid_iban"},
		{"another currency than the account's", `{"fromAccountId":"acct-001","toIban":"` + other + `","amount":"10.00","currency":"USD"}`, http.StatusUnprocessableEntity, "invalid_currency"},
		{"an amount with three decimals", `{"fromAccountId":"acct-001","toIban":"` + other + `","amount":"10.001","currency":"EUR"}`, http.StatusUnprocessableEntity, "invalid_amount"},
		{"no IBAN", `{"fromAccountId":"acct-001","amount":"10.00","currency":"EUR"}`, http.StatusUnprocessableEntity, "invalid_iban"},
		{"a body that is no JSON", `[`, http.StatusBadRequest, "bad_request"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := testServer(t, Options{})
			r := call(t, srv, http.MethodPost, "/bank/v1/transfers", aConfirmedUser, tc.body)
			if r.status != tc.status {
				t.Fatalf("status %d, want %d: %s", r.status, tc.status, r.body)
			}
			if tc.code != "" {
				if got := r.errorCode(t); got != tc.code {
					t.Fatalf("error code %q, want %q", got, tc.code)
				}
				return
			}
			var out transferResult
			r.json(t, &out)
			if out.ID != "tr-0001" || out.Status != "completed" || out.BalanceCents >= 250_000 {
				t.Fatalf("transfer answered %+v", out)
			}
		})
	}
}

func TestTransferBooksTheMoney(t *testing.T) {
	srv := testServer(t, Options{})
	token := aConfirmedUser
	r := call(t, srv, http.MethodPost, "/bank/v1/transfers", token,
		`{"fromAccountId":"acct-001","toIban":"DE02120300000000202051","amount":"40.00","currency":"EUR","reference":"To savings"}`)
	var out transferResult
	r.json(t, &out)
	if out.BalanceCents != 246_000 {
		t.Fatalf("the new balance is %d", out.BalanceCents)
	}
	var accounts struct{ Items []account }
	call(t, srv, http.MethodGet, "/bank/v1/accounts", token, "").json(t, &accounts)
	if accounts.Items[0].BalanceCents != 246_000 || accounts.Items[1].BalanceCents != 1_204_000 {
		t.Fatalf("balances after the transfer: %+v", accounts.Items)
	}
}

func TestTransferIsIdempotentOnItsKey(t *testing.T) {
	srv := testServer(t, Options{})
	token := aConfirmedUser
	body := `{"fromAccountId":"acct-001","toIban":"DE44500105175407324931","amount":"50.00","currency":"EUR"}`
	first := call(t, srv, http.MethodPost, "/bank/v1/transfers", token, body, "Idempotency-Key", "key-1")
	replay := call(t, srv, http.MethodPost, "/bank/v1/transfers", token, body, "Idempotency-Key", "key-1")
	other := call(t, srv, http.MethodPost, "/bank/v1/transfers", token, body, "Idempotency-Key", "key-2")
	if first.status != http.StatusCreated || replay.status != http.StatusCreated {
		t.Fatalf("statuses %d and %d", first.status, replay.status)
	}
	if string(first.body) != string(replay.body) {
		t.Fatalf("the replay answered %s, the first %s", replay.body, first.body)
	}
	var a, c transferResult
	first.json(t, &a)
	other.json(t, &c)
	var accounts struct{ Items []account }
	call(t, srv, http.MethodGet, "/bank/v1/accounts", token, "").json(t, &accounts)
	if got := accounts.Items[0].BalanceCents; got != 250_000-2*5_000 {
		t.Fatalf("the balance is %d: the replay moved money", got)
	}
	if a.ID == c.ID {
		t.Fatal("another key shares the transfer")
	}
}

func TestGraphQLAccounts(t *testing.T) {
	srv := testServer(t, Options{})
	token := aUser
	for _, query := range []string{
		`query Accounts { accounts { id name iban balanceCents currency } }`,
		`{ accounts { id } }`,
		`query { accounts { id } }`,
	} {
		r := call(t, srv, http.MethodPost, "/bank/graphql", token, bodyOf(t, map[string]any{"query": query}))
		var out struct {
			Data struct{ Accounts []account }
		}
		r.json(t, &out)
		if r.status != http.StatusOK || len(out.Data.Accounts) != 2 || out.Data.Accounts[0].ID != "acct-001" {
			t.Fatalf("%q answered %d %s", query, r.status, r.body)
		}
	}
}

func TestGraphQLTransactionsPaginate(t *testing.T) {
	srv := testServer(t, Options{})
	token := aUser
	const query = `query Transactions($accountId: String!, $first: Int!, $after: String) { transactions(accountId: $accountId, first: $first, after: $after) { items { id } next more } }`
	var seen []string
	var after any
	for page := 0; ; page++ {
		if page > 5 {
			t.Fatal("pagination does not end")
		}
		r := call(t, srv, http.MethodPost, "/bank/graphql", token, bodyOf(t, map[string]any{
			"query":     query,
			"variables": map[string]any{"accountId": "acct-001", "first": 10, "after": after},
		}))
		var out struct {
			Data struct {
				Transactions transactionsPage
			}
		}
		r.json(t, &out)
		p := out.Data.Transactions
		for _, tx := range p.Items {
			seen = append(seen, tx.ID)
		}
		wantMore := page < 2
		if p.More != wantMore || (p.Next != nil) != wantMore {
			t.Fatalf("page %d: more=%v next=%v", page, p.More, p.Next)
		}
		if !p.More {
			break
		}
		after = *p.Next
	}
	if len(seen) != 25 || seen[0] != "txn-001" || seen[24] != "txn-025" {
		t.Fatalf("the pages held %d bookings: %v", len(seen), seen)
	}
}

func TestGraphQLErrors(t *testing.T) {
	srv := testServer(t, Options{})
	token := aUser
	tests := []struct {
		name    string
		body    string
		message string
	}{
		{"a mutation", `{"query":"mutation M { doIt }"}`, "only queries are supported"},
		{"an unknown query", `{"query":"{ secrets { id } }"}`, "the query secrets is not supported"},
		{"an unknown account", `{"query":"{ transactions { items { id } } }","variables":{"accountId":"nope"}}`, "the account does not exist"},
		{"an unknown cursor", `{"query":"{ transactions { items { id } } }","variables":{"accountId":"acct-001","after":"nope"}}`, "the cursor is unknown"},
		{"a page size out of range", `{"query":"{ transactions { items { id } } }","variables":{"accountId":"acct-001","first":0}}`, "first must be between 1 and 50"},
		{"malformed variables", `{"query":"{ transactions { items { id } } }","variables":[1]}`, "the variables are malformed"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := call(t, srv, http.MethodPost, "/bank/graphql", token, tc.body)
			var out struct {
				Data   any
				Errors []graphqlError
			}
			r.json(t, &out)
			if r.status != http.StatusOK || out.Data != nil || len(out.Errors) != 1 || out.Errors[0].Message != tc.message {
				t.Fatalf("answered %d %s, want the error %q", r.status, r.body, tc.message)
			}
		})
	}
	t.Run("a body that is no JSON", func(t *testing.T) {
		if r := call(t, srv, http.MethodPost, "/bank/graphql", token, `{`); r.status != http.StatusBadRequest {
			t.Fatalf("status %d", r.status)
		}
	})
}

// sseStream reads the blocks of a server-sent events response.
type sseStream struct {
	br *bufio.Reader
}

// block reads lines up to the next blank one.
func (s *sseStream) block(t *testing.T) string {
	t.Helper()
	var lines []string
	for {
		line, err := s.br.ReadString('\n')
		if err != nil {
			t.Fatalf("reading the stream: %v (after %q)", err, lines)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			return strings.Join(lines, "\n")
		}
		lines = append(lines, line)
	}
}

func TestNotificationsStreamTransfersAndResume(t *testing.T) {
	srv := testServer(t, Options{})
	token := aConfirmedUser
	transfer := func() {
		r := call(t, srv, http.MethodPost, "/bank/v1/transfers", token,
			`{"fromAccountId":"acct-001","toIban":"DE44500105175407324931","amount":"1.00","currency":"EUR"}`)
		if r.status != http.StatusCreated {
			t.Fatalf("transfer: %d %s", r.status, r.body)
		}
	}
	open := func(lastID string) *sseStream {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/bank/v1/notifications", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set(assertionHeader, srv.gw.assertion(t, token, http.MethodGet, "/bank/v1/notifications", ""))
		if lastID != "" {
			req.Header.Set("Last-Event-ID", lastID)
		}
		res, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = res.Body.Close() })
		if got := res.Header.Get("Content-Type"); got != "text/event-stream" {
			t.Fatalf("content type %q", got)
		}
		return &sseStream{br: bufio.NewReader(res.Body)}
	}

	// A new stream starts after what happened before it.
	transfer()
	live := open("")
	transfer()
	want := "id: 2\nevent: transfer\ndata: "
	if got := live.block(t); !strings.HasPrefix(got, want) || !strings.Contains(got, `"transferId":"tr-0002"`) || !strings.Contains(got, `"balanceCents":249800`) {
		t.Fatalf("the first event is %q", got)
	}

	// A reconnection with Last-Event-ID is sent what it missed, in order.
	transfer()
	resumed := open("1")
	for _, id := range []string{"2", "3"} {
		if got := resumed.block(t); !strings.HasPrefix(got, "id: "+id+"\n") {
			t.Fatalf("the resumed stream sent %q, want event %s", got, id)
		}
	}
	// The stream that was never closed has them too.
	for _, id := range []string{"3"} {
		if got := live.block(t); !strings.HasPrefix(got, "id: "+id+"\n") {
			t.Fatalf("the live stream sent %q, want event %s", got, id)
		}
	}

	// Caught up, the next transfer is the next event.
	caughtUp := open("3")
	transfer()
	if got := caughtUp.block(t); !strings.HasPrefix(got, "id: 4\n") {
		t.Fatalf("after catching up the stream sent %q", got)
	}
}

func TestNotificationsSendHeartbeats(t *testing.T) {
	srv := testServer(t, Options{Heartbeat: 5 * time.Millisecond})
	token := aConfirmedUser
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/bank/v1/notifications", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set(assertionHeader, srv.gw.assertion(t, token, http.MethodGet, "/bank/v1/notifications", ""))
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if got := (&sseStream{br: bufio.NewReader(res.Body)}).block(t); got != ": heartbeat" {
		t.Fatalf("the first block is %q", got)
	}
}

func TestParseAmount(t *testing.T) {
	for in, want := range map[string]int64{"0": 0, "12": 1200, "12.5": 1250, "12.50": 1250, "0.05": 5, "999999999.99": 99999999999} {
		if got, ok := parseAmount(in); !ok || got != want {
			t.Errorf("parseAmount(%q) = %d, %v, want %d", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "-1", "1.", ".5", "1.234", "1e3", "1 000", "9999999999", "12,50"} {
		if _, ok := parseAmount(in); ok {
			t.Errorf("parseAmount(%q) was accepted", in)
		}
	}
}

// Verifies: SEC-032.
// The bank keeps the gateway's proof of the user's confirmation with the
// payment, and nothing of a signature.
func TestTransferKeepsTheConfirmationAsEvidence(t *testing.T) {
	srv := testServer(t, Options{})
	r := call(t, srv, http.MethodPost, "/bank/v1/transfers", aConfirmedUser,
		`{"fromAccountId":"acct-001","toIban":"DE44500105175407324931","amount":"3.00","currency":"EUR"}`)
	if r.status != http.StatusCreated {
		t.Fatalf("transfer: %d %s", r.status, r.body)
	}
	srv.api.store.mu.Lock()
	defer srv.api.store.mu.Unlock()
	want := transferEvidence{TransferID: "tr-0001", User: demoSubject, Device: "device-1", ChallengeID: "challenge-1", SummaryHash: "summary-hash-1"}
	if got := srv.api.store.bank.evidence; len(got) != 1 || got[0] != want {
		t.Fatalf("evidence %+v, want %+v", got, want)
	}
}
