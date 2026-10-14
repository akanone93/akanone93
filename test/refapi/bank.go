// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"crypto/subtle"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// The reference user: constant test credentials, never real ones. The code
// stands for the one-time password an SMS would carry.
const (
	demoUsername = "demo"
	demoPIN      = "246810"
	demoOTP      = "123456"
)

// ibanShape is the shape of an IBAN: two letters, two check digits and up
// to 30 alphanumerics. The check digits are not verified.
var ibanShape = regexp.MustCompile(`^[A-Z]{2}[0-9]{2}[A-Z0-9]{11,30}$`)

// What a bank endpoint demands of the assertion it trusts.
type need int

const (
	// needDevice: a call the gateway forwarded for a device, with or
	// without a user (the sign-in itself).
	needDevice need = iota
	// needUser: a call made for a user the identity provider vouched for.
	needUser
	// needTransaction: a call made for a user who confirmed it with the
	// device's SCA key.
	needTransaction
)

type assertionKey struct{}

// gated answers 401 unless the request carries the gateway's assertion, 403
// unless the assertion says what the endpoint needs, and otherwise hands the
// assertion to next. The bank trusts nothing else about the caller: not a
// bearer token, not a device credential (ADR-0056).
func (a *api) gated(n need, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		as, err := a.verify.verify(r.Context(), r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorised", "the request did not come through the Plux gateway")
			return
		}
		switch {
		case n >= needUser && as.User == nil:
			writeError(w, http.StatusForbidden, "user_required", "sign in to continue")
			return
		case n >= needTransaction && as.SCA == nil:
			writeError(w, http.StatusForbidden, "sca_required", "the user has not confirmed this transaction")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), assertionKey{}, as)))
	}
}

// assertionOf is the assertion gated verified.
func assertionOf(r *http.Request) *assertion {
	as, _ := r.Context().Value(assertionKey{}).(*assertion)
	return as
}

// demoSubject is the subject of the demo user at the identity provider.
const demoSubject = "user-demo"

// demoTier is the customer tier of the demo user: the standard one, whose
// transfers cost 0.1 percent (sdk/fn/examples/fee).
const demoTier = "standard"

// login signs the demo user in at the identity provider and returns the
// token it issued. The bank keeps no session: the token is what the host
// hands the Plux runtime, which has the gateway exchange it.
func (a *api) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username string `json:"username"`
		PIN      string `json:"pin"`
		OTP      string `json:"otp"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	user := subtle.ConstantTimeCompare([]byte(in.Username), []byte(demoUsername))
	pin := subtle.ConstantTimeCompare([]byte(in.PIN), []byte(demoPIN))
	otp := subtle.ConstantTimeCompare([]byte(in.OTP), []byte(demoOTP))
	if user&pin&otp != 1 {
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "the username, PIN or code is wrong")
		return
	}
	token, err := a.idp.issue(demoSubject, "Demo User", demoTier)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "a token could not be issued")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": token, "expiresIn": int(idpTokenTTL.Seconds())})
}

// listAccounts lists the accounts.
func (a *api) listAccounts(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"items": a.accountsSnapshot()})
}

// accountsSnapshot copies the accounts.
func (a *api) accountsSnapshot() []account {
	a.store.mu.Lock()
	defer a.store.mu.Unlock()
	out := make([]account, 0, len(a.store.bank.accounts))
	for _, acc := range a.store.bank.accounts {
		out = append(out, *acc)
	}
	return out
}

// transferRequest is the body of a transfer. The amount, the currency and
// the payee (toIban) are the three fields the Plux fixture maps for the
// user's confirmation, so what the user saw is what the bank books.
type transferRequest struct {
	FromAccountID string `json:"fromAccountId"`
	ToIBAN        string `json:"toIban"`
	Amount        string `json:"amount"`
	Currency      string `json:"currency"`
	Reference     string `json:"reference"`
}

// amountShape is a decimal amount: up to nine digits and two decimals.
var amountShape = regexp.MustCompile(`^([0-9]{1,9})(?:\.([0-9]{1,2}))?$`)

// parseAmount reads a decimal amount in cents.
func parseAmount(s string) (int64, bool) {
	m := amountShape.FindStringSubmatch(s)
	if m == nil {
		return 0, false
	}
	var cents int64
	for _, c := range m[1] + (m[2] + "00")[:2] {
		cents = cents*10 + int64(c-'0')
	}
	return cents, true
}

// createTransfer moves money out of an account, for a user who confirmed
// it: the assertion carries the proof (`sca`) the gateway verified, and the
// bank keeps its challenge and summary hash with the payment as evidence. A
// request that repeats an Idempotency-Key (or an assertion) answers what the
// first one did, without moving money again.
func (a *api) createTransfer(w http.ResponseWriter, r *http.Request) {
	var in transferRequest
	if !readJSON(w, r, &in) {
		return
	}
	as := assertionOf(r)
	key := r.Header.Get("Idempotency-Key")
	if key == "" {
		key = "jti:" + as.JTI
	}
	cents, amountOK := parseAmount(in.Amount)
	a.store.mu.Lock()
	defer a.store.mu.Unlock()
	d := &a.store.bank
	if prior, ok := d.transfers[key]; ok {
		writeJSON(w, http.StatusCreated, prior)
		return
	}
	from := d.account(in.FromAccountID)
	iban := strings.ReplaceAll(strings.ToUpper(in.ToIBAN), " ", "")
	switch {
	case from == nil:
		writeError(w, http.StatusUnprocessableEntity, "unknown_account", "the account does not exist")
		return
	case !ibanShape.MatchString(iban):
		writeError(w, http.StatusUnprocessableEntity, "invalid_iban", "the recipient's IBAN is malformed")
		return
	case !amountOK || cents <= 0:
		writeError(w, http.StatusUnprocessableEntity, "invalid_amount", "the amount must be positive")
		return
	case in.Currency != from.Currency:
		writeError(w, http.StatusUnprocessableEntity, "invalid_currency", "the currency is not the account's")
		return
	case cents > from.BalanceCents:
		writeError(w, http.StatusUnprocessableEntity, "insufficient_funds", "the amount is over the balance")
		return
	}
	d.transferSeq++
	id := fmt.Sprintf("tr-%04d", d.transferSeq)
	// Bookings after the seed are dated from it, a minute apart.
	at := epoch.Add(time.Duration(d.transferSeq) * time.Minute).Format(time.RFC3339)
	from.BalanceCents -= cents
	d.book(from, transaction{ID: id, Description: orDefault(in.Reference, "Transfer"), AmountCents: -cents, PostedAt: at})
	if to := d.accountByIBAN(iban); to != nil {
		to.BalanceCents += cents
		d.book(to, transaction{ID: id + "-in", Description: orDefault(in.Reference, "Transfer"), AmountCents: cents, PostedAt: at})
	}
	result := transferResult{ID: id, Status: "completed", BalanceCents: from.BalanceCents}
	d.transfers[key] = result
	d.evidence = append(d.evidence, transferEvidence{
		TransferID: id, User: as.User.Subject, Device: as.Device, ChallengeID: as.SCA.ChallengeID, SummaryHash: as.SCA.SummaryHash,
	})
	d.publish(notification{TransferID: id, AccountID: from.ID, ToIBAN: iban, AmountCents: cents, BalanceCents: from.BalanceCents})
	writeJSON(w, http.StatusCreated, result)
}

// orDefault returns s, or fallback when s is blank.
func orDefault(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}

// account finds an account by ID.
func (d *bankData) account(id string) *account {
	for _, a := range d.accounts {
		if a.ID == id {
			return a
		}
	}
	return nil
}

// accountByIBAN finds an account by IBAN.
func (d *bankData) accountByIBAN(iban string) *account {
	for _, a := range d.accounts {
		if a.IBAN == iban {
			return a
		}
	}
	return nil
}

// book records t on acc, newest first.
func (d *bankData) book(acc *account, t transaction) {
	t.AccountID, t.BalanceCents = acc.ID, acc.BalanceCents
	d.transactions[acc.ID] = append([]transaction{t}, d.transactions[acc.ID]...)
}

// publish appends a notification and wakes the streams waiting for one. The
// store's mutex is held.
func (d *bankData) publish(n notification) {
	n.ID = len(d.events) + 1
	d.events = append(d.events, n)
	if d.signal != nil {
		close(d.signal)
		d.signal = nil
	}
}
