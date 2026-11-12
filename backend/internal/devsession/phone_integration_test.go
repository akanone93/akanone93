// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package devsession_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/devsession"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

// signInPhone connects a phone with the key jkt-a for the developer and
// returns its token.
func (f *fixture) signInPhone(t *testing.T, dev auth.Principal) string {
	const jkt = "jkt-a"
	t.Helper()
	ctx := context.Background()
	grant, err := f.phones.CreatePhoneSignIn(ctx, dev)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.phones.ClaimPhoneSignIn(ctx, grant.Grant, jkt, "Pixel 8", "android"); err != nil {
		t.Fatal(err)
	}
	st, err := f.phones.GetPhoneSignIn(ctx, dev, grant.ID)
	if err != nil {
		t.Fatal(err)
	}
	token, err := f.phones.CompletePhoneSignIn(ctx, grant.Grant, jkt, st.Number)
	if err != nil {
		t.Fatal(err)
	}
	return token.Secret
}

// phone resolves a phone token as the procedures of a phone do.
func (f *fixture) phone(t *testing.T, token string) (auth.Principal, auth.PhoneKey) {
	t.Helper()
	ctx := context.Background()
	id, key, err := f.phones.AuthenticatePhoneToken(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	p, err := f.Auth.Resolve(ctx, id, "")
	if err != nil {
		t.Fatal(err)
	}
	return p, key
}

// openStudio opens the developer's Studio session in development.
func (f *fixture) openStudio(t *testing.T, dev auth.Principal) devsession.Session {
	t.Helper()
	s, _, err := f.svc.CreateSessionOfKind(context.Background(), dev, f.App, f.Envs["development"], devsession.KindStudio)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// joinEvents returns the join request events a session's watchers saw.
func (f *fixture) joinEvents(t *testing.T, dev auth.Principal, sessionID string) []*pluxv1.JoinRequestEvent {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	var out []*pluxv1.JoinRequestEvent
	err := f.svc.Watch(ctx, dev, sessionID, nil, 0, func(ev *pluxv1.DevEvent) error {
		if j := ev.GetJoinRequest(); j != nil {
			out = append(out, j)
		}
		return nil
	})
	if err != nil && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	return out
}

func TestPhoneSignIn_NumberMatching_DEV_002(t *testing.T) {
	f := newFixture(t, 0)
	ctx := context.Background()
	grant, err := f.phones.CreatePhoneSignIn(ctx, f.Developer)
	if err != nil {
		t.Fatal(err)
	}
	if st, _ := f.phones.GetPhoneSignIn(ctx, f.Developer, grant.ID); st.State != auth.PhoneSignInWaiting || st.Number != 0 {
		t.Fatalf("a new sign-in = %+v, want waiting with no number", st)
	}
	claim, err := f.phones.ClaimPhoneSignIn(ctx, grant.Grant, "jkt-a", "Pixel 8", "android")
	if err != nil {
		t.Fatal(err)
	}
	if len(claim.Options) != 3 || claim.UserDisplay == "" {
		t.Fatalf("claim = %+v, want three numbers and the person", claim)
	}
	for _, n := range claim.Options {
		if n < 10 || n > 99 {
			t.Fatalf("option %d is not two digits", n)
		}
	}
	st, err := f.phones.GetPhoneSignIn(ctx, f.Developer, grant.ID)
	if err != nil {
		t.Fatal(err)
	}
	if st.State != auth.PhoneSignInClaimed || !slices.Contains(claim.Options, st.Number) || st.PhoneName != "Pixel 8" {
		t.Fatalf("claimed sign-in = %+v, want Studio's number among %v", st, claim.Options)
	}
	// Another person of the organisation cannot read the number.
	if _, err := f.phones.GetPhoneSignIn(ctx, f.Unstepped, grant.ID); err == nil {
		t.Fatal("another person read the sign-in")
	}
	token, err := f.phones.CompletePhoneSignIn(ctx, grant.Grant, "jkt-a", st.Number)
	if err != nil {
		t.Fatal(err)
	}
	// The grant works once.
	_, err = f.phones.CompletePhoneSignIn(ctx, grant.Grant, "jkt-a", st.Number)
	wantCode(t, err, plxerr.PhoneRequestClosed, "a second completion")
	if st, _ := f.phones.GetPhoneSignIn(ctx, f.Developer, grant.ID); st.State != auth.PhoneSignInSignedIn || st.Number != 0 {
		t.Fatalf("finished sign-in = %+v, want signed in, no number", st)
	}
	// The token is the phone's, bound to its key, and never a bearer token.
	id, key, err := f.phones.AuthenticatePhoneToken(ctx, token.Secret)
	if err != nil {
		t.Fatal(err)
	}
	if id.Kind != auth.KindPhone || key.JKT != "jkt-a" || key.Name != "Pixel 8" || id.UserID != f.Developer.UserID {
		t.Fatalf("phone identity = %+v %+v", id, key)
	}
	if _, err := f.Auth.AuthenticateToken(ctx, token.Secret); err == nil {
		t.Fatal("a phone token was accepted as a bearer token")
	}
	phones, err := f.phones.ListPhones(ctx, f.Developer)
	if err != nil || len(phones) != 1 || phones[0].ID != token.PhoneID || phones[0].Platform != "android" {
		t.Fatalf("phones = %+v, %v", phones, err)
	}
	if rows := f.Audit(t, "devphone.signed_in"); len(rows) != 1 {
		t.Fatalf("audit = %+v, want one sign-in", rows)
	}
	// Revoked, it is refused.
	if err := f.phones.RevokePhone(ctx, f.Developer, token.PhoneID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.phones.AuthenticatePhoneToken(ctx, token.Secret); err == nil {
		t.Fatal("a revoked phone was accepted")
	}
}

func TestPhoneSignIn_WrongNumberEndsIt_DEV_002(t *testing.T) {
	f := newFixture(t, 0)
	ctx := context.Background()
	grant, err := f.phones.CreatePhoneSignIn(ctx, f.Developer)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := f.phones.ClaimPhoneSignIn(ctx, grant.Grant, "jkt-a", "Pixel 8", "android")
	if err != nil {
		t.Fatal(err)
	}
	st, _ := f.phones.GetPhoneSignIn(ctx, f.Developer, grant.ID)
	wrong := claim.Options[0]
	if wrong == st.Number {
		wrong = claim.Options[1]
	}
	_, err = f.phones.CompletePhoneSignIn(ctx, grant.Grant, "jkt-a", wrong)
	wantCode(t, err, plxerr.NumberMismatch, "a wrong number")
	// The failure is kept: the right number no longer helps.
	_, err = f.phones.CompletePhoneSignIn(ctx, grant.Grant, "jkt-a", st.Number)
	wantCode(t, err, plxerr.PhoneRequestClosed, "the right number after a wrong one")
	if st, _ := f.phones.GetPhoneSignIn(ctx, f.Developer, grant.ID); st.State != auth.PhoneSignInFailed {
		t.Fatalf("state = %q, want failed", st.State)
	}
}

func TestPhoneSignIn_KeyStepUpAndExpiry_DEV_002(t *testing.T) {
	f := newFixture(t, 0)
	ctx := context.Background()
	// A step-up is needed, and a browser session.
	if _, err := f.phones.CreatePhoneSignIn(ctx, f.Unstepped); err == nil {
		t.Fatal("a sign-in was created without a step-up")
	}
	grant, err := f.phones.CreatePhoneSignIn(ctx, f.Developer)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.phones.ClaimPhoneSignIn(ctx, grant.Grant, "jkt-a", "Pixel", "android"); err != nil {
		t.Fatal(err)
	}
	// One claim per grant, and the pick comes from the key that claimed.
	_, err = f.phones.ClaimPhoneSignIn(ctx, grant.Grant, "jkt-b", "Other", "android")
	wantCode(t, err, plxerr.PhoneRequestClosed, "a second claim")
	st, _ := f.phones.GetPhoneSignIn(ctx, f.Developer, grant.ID)
	_, err = f.phones.CompletePhoneSignIn(ctx, grant.Grant, "jkt-b", st.Number)
	wantCode(t, err, plxerr.PhoneRequestClosed, "a pick from another key")
	// Past its lifetime nothing completes it.
	f.Now = f.Now.Add(61 * time.Second)
	_, err = f.phones.CompletePhoneSignIn(ctx, grant.Grant, "jkt-a", st.Number)
	wantCode(t, err, plxerr.PhoneRequestClosed, "an expired sign-in")
	if st, _ := f.phones.GetPhoneSignIn(ctx, f.Developer, grant.ID); st.State != auth.PhoneSignInExpired || st.Number != 0 {
		t.Fatalf("expired sign-in = %+v", st)
	}
	_, err = f.phones.ClaimPhoneSignIn(ctx, "not-a-grant", "jkt-a", "Pixel", "android")
	wantCode(t, err, plxerr.PhoneRequestClosed, "an unknown grant")
}

func TestPhoneSignIn_LapsesWhenIdle_DEV_002(t *testing.T) {
	f := newFixture(t, 0)
	ctx := context.Background()
	token := f.signInPhone(t, f.Developer)
	f.Now = f.Now.Add(29 * 24 * time.Hour)
	f.phone(t, token) // used: the idle time starts again
	f.Now = f.Now.Add(29 * 24 * time.Hour)
	f.phone(t, token)
	f.Now = f.Now.Add(31 * 24 * time.Hour)
	if _, _, err := f.phones.AuthenticatePhoneToken(ctx, token); err == nil {
		t.Fatal("a phone idle for 31 days was accepted")
	}
	if phones, _ := f.phones.ListPhones(ctx, f.Developer); len(phones) != 0 {
		t.Fatalf("a lapsed phone is listed: %+v", phones)
	}
}

func TestStudioSession_OnePerPersonWhileLive_DEV_002(t *testing.T) {
	f := newFixture(t, 0)
	a := f.openStudio(t, f.Developer)
	if b := f.openStudio(t, f.Developer); b.ID != a.ID {
		t.Fatalf("Studio opened anew made session %s, want %s", b.ID, a.ID)
	}
	other := f.member(t, "dev3@example.com", true)
	if c := f.openStudio(t, other); c.ID == a.ID {
		t.Fatal("another person shares the session")
	}
	cli, _ := f.open(t)
	if cli.ID == a.ID {
		t.Fatal("a CLI session reused the Studio one")
	}
	f.Now = f.Now.Add(31 * time.Minute)
	if d := f.openStudio(t, f.Developer); d.ID == a.ID {
		t.Fatal("an idle session was reused")
	}
}

func TestJoin_NumberMatching_DEV_002(t *testing.T) {
	f := newFixture(t, 0)
	ctx := context.Background()
	session := f.openStudio(t, f.Developer)
	phone, key := f.phone(t, f.signInPhone(t, f.Developer))
	jr, err := f.svc.RequestJoin(ctx, phone, phone.ID, key.Name, f.App)
	if err != nil {
		t.Fatal(err)
	}
	if jr.SessionID != session.ID || jr.Environment != "development" || len(jr.Options) != 3 || jr.Pairing != nil {
		t.Fatalf("request = %+v", jr)
	}
	st, err := f.svc.GetJoinRequest(ctx, f.Developer, jr.ID)
	if err != nil {
		t.Fatal(err)
	}
	if st.State != pluxv1.DevJoinState_DEV_JOIN_STATE_WAITING || !slices.Contains(jr.Options, st.Number) || st.PhoneName != "Pixel 8" {
		t.Fatalf("Studio's view = %+v", st)
	}
	// Only the session's person reads the number.
	other := f.member(t, "dev3@example.com", true)
	if _, err := f.svc.GetJoinRequest(ctx, other, jr.ID); err == nil {
		t.Fatal("another person read the number")
	}
	joined, err := f.svc.CompleteJoin(ctx, phone, phone.ID, jr.ID, st.Number)
	if err != nil {
		t.Fatal(err)
	}
	if joined.SessionID != session.ID || joined.Pairing.Token == "" {
		t.Fatalf("joined = %+v", joined)
	}
	// The pairing token pairs the phone's runtime as a scan would.
	if _, err := f.pair(f.device("phone-device", "jkt-device"), joined.Pairing.Token); err != nil {
		t.Fatal(err)
	}
	// The same phone joins the same live session again without a number.
	again, err := f.svc.RequestJoin(ctx, phone, phone.ID, key.Name, f.App)
	if err != nil {
		t.Fatal(err)
	}
	if again.Pairing == nil || len(again.Options) != 0 {
		t.Fatalf("a second request = %+v, want a pairing and no number", again)
	}
	events := f.joinEvents(t, f.Developer, session.ID)
	if len(events) != 2 || events[0].GetState() != pluxv1.DevJoinState_DEV_JOIN_STATE_WAITING ||
		events[1].GetState() != pluxv1.DevJoinState_DEV_JOIN_STATE_JOINED || events[0].GetRequestId() != jr.ID {
		t.Fatalf("join events = %v", events)
	}
	if rows := f.Audit(t, "devsession.joined"); len(rows) != 1 {
		t.Fatalf("audit = %+v, want one join", rows)
	}
}

func TestJoin_WrongNumberKeepsThePhoneOut_DEV_002(t *testing.T) {
	f := newFixture(t, 0)
	ctx := context.Background()
	f.openStudio(t, f.Developer)
	phone, key := f.phone(t, f.signInPhone(t, f.Developer))
	jr, err := f.svc.RequestJoin(ctx, phone, phone.ID, key.Name, f.App)
	if err != nil {
		t.Fatal(err)
	}
	st, _ := f.svc.GetJoinRequest(ctx, f.Developer, jr.ID)
	wrong := jr.Options[0]
	if wrong == st.Number {
		wrong = jr.Options[1]
	}
	_, err = f.svc.CompleteJoin(ctx, phone, phone.ID, jr.ID, wrong)
	wantCode(t, err, plxerr.NumberMismatch, "a wrong number")
	_, err = f.svc.CompleteJoin(ctx, phone, phone.ID, jr.ID, st.Number)
	wantCode(t, err, plxerr.PhoneRequestClosed, "the right number after a wrong one")
	// One guess: the phone may not ask again until the person lets it.
	_, err = f.svc.RequestJoin(ctx, phone, phone.ID, key.Name, f.App)
	wantCode(t, err, plxerr.NumberMismatch, "a request after a wrong number")
	if err := f.svc.AllowJoinRetry(ctx, f.Developer, jr.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.RequestJoin(ctx, phone, phone.ID, key.Name, f.App); err != nil {
		t.Fatal(err)
	}
}

func TestJoin_DenyAndExpiry_DEV_002(t *testing.T) {
	f := newFixture(t, 0)
	ctx := context.Background()
	f.openStudio(t, f.Developer)
	phone, key := f.phone(t, f.signInPhone(t, f.Developer))
	jr, err := f.svc.RequestJoin(ctx, phone, phone.ID, key.Name, f.App)
	if err != nil {
		t.Fatal(err)
	}
	st, _ := f.svc.GetJoinRequest(ctx, f.Developer, jr.ID)
	if err := f.svc.DenyJoinRequest(ctx, f.Developer, jr.ID); err != nil {
		t.Fatal(err)
	}
	_, err = f.svc.CompleteJoin(ctx, phone, phone.ID, jr.ID, st.Number)
	wantCode(t, err, plxerr.PhoneRequestClosed, "a denied request")
	// A refusal is not a guess: the phone may ask again.
	jr, err = f.svc.RequestJoin(ctx, phone, phone.ID, key.Name, f.App)
	if err != nil {
		t.Fatal(err)
	}
	st, _ = f.svc.GetJoinRequest(ctx, f.Developer, jr.ID)
	f.Now = f.Now.Add(61 * time.Second)
	_, err = f.svc.CompleteJoin(ctx, phone, phone.ID, jr.ID, st.Number)
	wantCode(t, err, plxerr.PhoneRequestClosed, "an expired request")
	if st, _ := f.svc.GetJoinRequest(ctx, f.Developer, jr.ID); st.State != pluxv1.DevJoinState_DEV_JOIN_STATE_EXPIRED || st.Number != 0 {
		t.Fatalf("expired request = %+v", st)
	}
}

func TestJoin_OnlyTheOwnLiveStudioSession_DEV_002(t *testing.T) {
	f := newFixture(t, 0)
	ctx := context.Background()
	phone, key := f.phone(t, f.signInPhone(t, f.Developer))
	_, err := f.svc.RequestJoin(ctx, phone, phone.ID, key.Name, f.App)
	wantCode(t, err, plxerr.NoLiveStudioSession, "no session")
	f.open(t) // a CLI session is not joined this way
	_, err = f.svc.RequestJoin(ctx, phone, phone.ID, key.Name, f.App)
	wantCode(t, err, plxerr.NoLiveStudioSession, "a CLI session")
	f.openStudio(t, f.member(t, "dev3@example.com", true))
	_, err = f.svc.RequestJoin(ctx, phone, phone.ID, key.Name, f.App)
	wantCode(t, err, plxerr.NoLiveStudioSession, "another person's session")
	// A person, not a phone, does not join this way.
	_, err = f.svc.RequestJoin(ctx, f.Developer, f.Developer.ID, "", f.App)
	wantCode(t, err, plxerr.PermissionDenied, "a person")
	live, err := f.svc.LiveApps(ctx, phone, []string{f.App})
	if err != nil || live[f.App] {
		t.Fatalf("live apps = %v, %v; want none", live, err)
	}
	f.openStudio(t, f.Developer)
	if live, _ := f.svc.LiveApps(ctx, phone, []string{f.App}); !live[f.App] {
		t.Fatal("the open app is not live")
	}
}

func TestJoin_RateLimited_DEV_002(t *testing.T) {
	f := newFixture(t, 0)
	f.tight[limits.DevSessionPhoneAttempts] = 2
	ctx := context.Background()
	f.openStudio(t, f.Developer)
	phone, key := f.phone(t, f.signInPhone(t, f.Developer))
	for range 2 {
		if _, err := f.svc.RequestJoin(ctx, phone, phone.ID, key.Name, f.App); err != nil {
			t.Fatal(err)
		}
	}
	_, err := f.svc.RequestJoin(ctx, phone, phone.ID, key.Name, f.App)
	wantCode(t, err, plxerr.RateLimited, "a third request")
}

func TestJoin_StudioListsOpenRequestsAndKeepsItsSession_DEV_002(t *testing.T) {
	f := newFixture(t, 0)
	ctx := context.Background()
	session := f.openStudio(t, f.Developer)
	phone, key := f.phone(t, f.signInPhone(t, f.Developer))
	jr, err := f.svc.RequestJoin(ctx, phone, phone.ID, key.Name, f.App)
	if err != nil {
		t.Fatal(err)
	}
	list, err := f.svc.ListJoinRequests(ctx, f.Developer, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].RequestID != jr.ID || !slices.Contains(jr.Options, list[0].Number) || list[0].PhoneName != "Pixel 8" {
		t.Fatalf("Studio's listing = %+v", list)
	}
	// Another person lists nothing of it, and cannot touch the session.
	if _, err := f.svc.ListJoinRequests(ctx, f.member(t, "dev3@example.com", true), session.ID); err == nil {
		t.Fatal("another person listed the requests")
	}
	// Studio asking keeps its session live past the idle timeout, so the
	// phone still joins it.
	for range 4 {
		f.Now = f.Now.Add(20 * time.Minute)
		if _, err := f.svc.ListJoinRequests(ctx, f.Developer, session.ID); err != nil {
			t.Fatal(err)
		}
	}
	again, err := f.svc.RequestJoin(ctx, phone, phone.ID, key.Name, f.App)
	if err != nil || again.SessionID != session.ID {
		t.Fatalf("the phone joined %v (%v), want Studio's open session %s", again.SessionID, err, session.ID)
	}
}
