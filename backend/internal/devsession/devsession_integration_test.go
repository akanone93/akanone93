// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package devsession_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/device/devicetest"
	"github.com/nightCode42/plux3/backend/internal/devsession"
	"github.com/nightCode42/plux3/backend/internal/governancetest"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/seccfg"
	"github.com/nightCode42/plux3/backend/internal/security/settings"
	"github.com/nightCode42/plux3/backend/internal/signing"
)

// fixture is a world with the dev-session service on its clock.
type fixture struct {
	*governancetest.World
	svc     *devsession.Service
	configs *seccfg.Service
	signer  *devicetest.TokenSigner
	// Developer holds devsession.create with a fresh step-up; Unstepped
	// is the same kind of person without one.
	Developer auth.Principal
	Unstepped auth.Principal
	// tight tightens limits for the organisation; a test sets it before
	// it starts anything that reads the limits.
	tight map[limits.Key]int64
}

// newFixture builds the world and the service; devices overrides
// devSession.devicesPerSession when it is not zero.
func newFixture(t *testing.T, devices int64) *fixture {
	t.Helper()
	w := governancetest.New(t)
	configs, err := seccfg.New(seccfg.Options{DB: w.DB, Audit: w.Log, Tenancy: w.Tenancy, Now: w.Clock})
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{World: w, configs: configs, signer: devicetest.NewTokenSigner(t), tight: map[limits.Key]int64{}}
	if devices != 0 {
		f.tight[limits.DevSessionDevicesPerSession] = devices
	}
	lim := func(ctx context.Context, tx pgx.Tx, org, _ string) (limits.Set, error) {
		set, err := w.Limits(ctx, tx, org)
		for k, v := range f.tight {
			if err != nil {
				break
			}
			set, err = set.Tighten(k, limits.ScopeOrganization, v)
		}
		return set, err //nolint:wrapcheck // a test
	}
	f.svc, err = devsession.New(devsession.Options{
		DB: w.DB, Audit: w.Log, IDs: w.Gen, Limits: lim, Tokens: f.signer, Environments: []string{"development"},
		PublicBaseURL: "https://plux.example.com", Now: w.Clock,
		StepUp: func(ctx context.Context, appID, environmentID string) (bool, error) {
			values, _, err := configs.Effective(ctx, appID, environmentID)
			if err != nil {
				return false, err //nolint:wrapcheck // a test
			}
			return values.Get(settings.DevSessionRequireStepUp).Bool(), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	f.Developer = f.member(t, "dev@example.com", true)
	f.Unstepped = f.member(t, "dev2@example.com", false)
	return f
}

// member adds a developer, signed in with or without a fresh step-up.
func (f *fixture) member(t *testing.T, email string, stepped bool) auth.Principal {
	t.Helper()
	ctx := context.Background()
	m, _, err := f.Tenancy.AddMember(ctx, f.Owner, "", email, "developer")
	if err != nil {
		t.Fatal(err)
	}
	id := auth.Identity{Kind: auth.KindUser, ID: m.UserID, UserID: m.UserID, Display: email, SecondFactor: true}
	if stepped {
		id = id.WithStepUp(0)
	}
	p, err := f.Auth.Resolve(ctx, id, f.Org)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// device is a development device of the world's app.
func (f *fixture) device(id, jkt string) devsession.Caller {
	return devsession.Caller{DeviceID: id, OrganizationID: f.Org, AppID: f.App, EnvironmentID: f.Envs["development"], Thumbprint: jkt}
}

// open creates a session in the development environment as the developer.
func (f *fixture) open(t *testing.T) (devsession.Session, devsession.Pairing) {
	t.Helper()
	s, p, err := f.svc.CreateSession(context.Background(), f.Developer, f.App, f.Envs["development"])
	if err != nil {
		t.Fatal(err)
	}
	return s, p
}

// pair pairs a device with a token.
func (f *fixture) pair(d devsession.Caller, token string) (devsession.Paired, error) {
	return f.svc.Pair(context.Background(), d, devsession.PairRequest{Token: token, Name: "Pixel", Platform: "android", RuntimeVersion: "1.0.0"})
}

// wantCode checks the code of a refusal.
func wantCode(t *testing.T, err error, want plxerr.Code, what string) {
	t.Helper()
	if code, ok := plxerr.CodeOf(err); !ok || code != want {
		t.Fatalf("%s: %v, want code %d", what, err, want)
	}
}

// Verifies: DEV-003.
// A dev session is refused in a production environment, and in one the
// configuration does not enable, with PLX-9203; nothing is stored.
func TestCreateSession_NonProductionOnly_DEV_003(t *testing.T) {
	f := newFixture(t, 0)
	ctx := context.Background()
	_, _, err := f.svc.CreateSession(ctx, f.Owner, f.App, f.Envs["production"])
	wantCode(t, err, plxerr.DevSessionEnvironment, "production")
	narrow, err := devsession.New(devsession.Options{
		DB: f.DB, Audit: f.Log, IDs: f.Gen, Tokens: f.signer, Now: f.Clock,
		Limits: func(ctx context.Context, tx pgx.Tx, org, _ string) (limits.Set, error) { return f.Limits(ctx, tx, org) },
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = narrow.CreateSession(ctx, f.Owner, f.App, f.Envs["development"])
	wantCode(t, err, plxerr.DevSessionEnvironment, "an environment that is not enabled")
	if rows := f.Audit(t, audit.DevSessionCreated); len(rows) != 0 {
		t.Errorf("refused sessions were audited: %+v", rows)
	}
	s, p := f.open(t)
	if s.EnvironmentKey != "development" || !s.ExpiresAt.Equal(f.Now.Add(480*time.Minute)) || s.Creator != f.Developer.UserID {
		t.Errorf("session = %+v", s)
	}
	if rows := f.Audit(t, audit.DevSessionCreated); len(rows) != 1 || rows[0].TargetID != s.ID || strings.Contains(rows[0].Detail, p.Token) {
		t.Errorf("audit entries = %+v", rows)
	}
}

// Verifies: DEV-002, SEC-103.
// Only a principal who holds devsession.create on the environment may open
// a session or ask for its pairing tokens.
func TestCreateSession_PermissionRequired_DEV_002(t *testing.T) {
	f := newFixture(t, 0)
	ctx := context.Background()
	_, _, err := f.svc.CreateSession(ctx, f.Viewer, f.App, f.Envs["development"])
	wantCode(t, err, plxerr.PermissionDenied, "an observer")
	s, _ := f.open(t)
	_, err = f.svc.GetPairing(ctx, f.Viewer, s.ID)
	wantCode(t, err, plxerr.PermissionDenied, "an observer asking for a pairing")
	_, err = f.svc.GetPairing(ctx, f.Unstepped, s.ID)
	wantCode(t, err, plxerr.PermissionDenied, "another developer asking for a pairing")
	if _, err := f.svc.ListDevices(ctx, f.Viewer, s.ID); err != nil {
		t.Errorf("an observer lists the devices: %v", err)
	}
	_, err = f.svc.EndSession(ctx, f.Unstepped, s.ID)
	wantCode(t, err, plxerr.PermissionDenied, "another developer ending the session")
}

// Verifies: DEV-002, SEC-103.
// Creating a session needs a step-up only when the environment's security
// configuration sets devSessionRequireStepUp.
func TestCreateSession_StepUpOnlyWhenConfigured_DEV_002(t *testing.T) {
	f := newFixture(t, 0)
	ctx := context.Background()
	dev := f.Envs["development"]
	if _, _, err := f.svc.CreateSession(ctx, f.Unstepped, f.App, dev); err != nil {
		t.Fatalf("without the setting: %v", err)
	}
	if _, err := f.configs.Set(ctx, f.Owner, seccfg.Change{
		AppID: f.App, EnvironmentID: dev, Profile: "standard", Overrides: []byte(`{"devSessionRequireStepUp":true}`),
	}); err != nil {
		t.Fatal(err)
	}
	_, _, err := f.svc.CreateSession(ctx, f.Unstepped, f.App, dev)
	wantCode(t, err, plxerr.StepUpRequired, "with the setting and no step-up")
	if _, _, err := f.svc.CreateSession(ctx, f.Developer, f.App, dev); err != nil {
		t.Fatalf("with the setting and a step-up: %v", err)
	}
}

// Verifies: DEV-002, SEC-021.
// A device pairs once with a token: the token is stored as a hash only, and
// the device is bound to its key and handed a ticket.
func TestPair_PairsAndBinds_DEV_002(t *testing.T) {
	f := newFixture(t, 0)
	ctx := context.Background()
	s, p := f.open(t)
	if !strings.HasPrefix(p.URI, "plux-dev://pair?") || !strings.Contains(p.URI, "server=https%3A%2F%2Fplux.example.com") || len(p.Code) != 8 {
		t.Errorf("pairing = %+v", p)
	}
	if !p.ExpiresAt.Equal(f.Now.Add(120 * time.Second)) {
		t.Errorf("token expires %s", p.ExpiresAt)
	}
	var stored []byte
	err := f.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT token_hash FROM dev_pairing_tokens WHERE session_id = $1`, s.ID).Scan(&stored) //nolint:wrapcheck // a test
	})
	if sum := sha256.Sum256([]byte(p.Token)); err != nil || !bytes.Equal(stored, sum[:]) {
		t.Fatalf("stored %x: %v", stored, err)
	}
	res, err := f.pair(f.device("device-1", "jkt-1"), p.Token)
	if err != nil {
		t.Fatal(err)
	}
	if res.SessionID != s.ID || res.Ticket == "" || !res.TicketExpiresAt.Equal(f.Now.Add(time.Minute)) {
		t.Errorf("paired = %+v", res)
	}
	devices, err := f.svc.ListDevices(ctx, f.Developer, s.ID)
	if err != nil || len(devices) != 1 || devices[0].ID != res.DeviceID || devices[0].DeviceID != "device-1" || devices[0].Platform != "android" {
		t.Fatalf("devices = %+v: %v", devices, err)
	}
	if rows := f.Audit(t, audit.DevSessionPaired); len(rows) != 1 || rows[0].TargetID != s.ID {
		t.Errorf("audit entries = %+v", rows)
	}
}

// Verifies: DEV-002.
// A pairing token is single-use (PLX-9202) and short-lived (PLX-9201).
func TestPair_TokenSingleUseAndExpiry_DEV_002(t *testing.T) {
	f := newFixture(t, 0)
	_, p := f.open(t)
	if _, err := f.pair(f.device("device-1", "jkt-1"), p.Token); err != nil {
		t.Fatal(err)
	}
	_, err := f.pair(f.device("device-2", "jkt-2"), p.Token)
	wantCode(t, err, plxerr.PairingTokenUsed, "a used token")

	s, _ := f.open(t)
	late, err := f.svc.GetPairing(context.Background(), f.Developer, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	f.Now = f.Now.Add(121 * time.Second)
	_, err = f.pair(f.device("device-3", "jkt-3"), late.Token)
	wantCode(t, err, plxerr.PairingTokenExpired, "an expired token")
}

// Verifies: DEV-002, DEV-003.
// A token with a signature that is not the server's development key, or for
// another environment, pairs nothing.
func TestPair_RefusesForeignTokens_DEV_002(t *testing.T) {
	f := newFixture(t, 0)
	s, p := f.open(t)
	forged := devsession.Claims{Session: s.ID, Subject: s.Creator, App: f.App, Environment: f.Envs["development"], Expiry: f.Now.Add(time.Minute).Unix(), Nonce: "x"}
	_, err := f.pair(f.device("device-1", "jkt-1"), sign(t, devicetest.NewTokenSigner(t), signing.TokenDevelopment, devsession.PairingType, forged))
	wantCode(t, err, plxerr.PermissionDenied, "a token of another signer")
	_, err = f.pair(f.device("device-1", "jkt-1"), sign(t, f.signer, signing.TokenProduction, devsession.PairingType, forged))
	wantCode(t, err, plxerr.PermissionDenied, "a token signed with the production key")
	other := f.device("device-1", "jkt-1")
	other.EnvironmentID = f.Envs["production"]
	_, err = f.pair(other, p.Token)
	wantCode(t, err, plxerr.DevSessionEnvironment, "a device of another environment")
	if _, err := f.pair(f.device("device-1", "jkt-1"), p.Token); err != nil {
		t.Fatalf("the refusals spent the token: %v", err)
	}
}

// Verifies: DEV-002.
// A session holds at most devSession.devicesPerSession devices (PLX-9208);
// pairing the same device again replaces its pairing.
func TestPair_DeviceLimit_DEV_002(t *testing.T) {
	f := newFixture(t, 2)
	ctx := context.Background()
	s, p := f.open(t)
	token := func() string {
		t.Helper()
		next, err := f.svc.GetPairing(ctx, f.Developer, s.ID)
		if err != nil {
			t.Fatal(err)
		}
		return next.Token
	}
	if _, err := f.pair(f.device("device-1", "jkt-1"), p.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pair(f.device("device-1", "jkt-1b"), token()); err != nil {
		t.Fatalf("pairing the same device again: %v", err)
	}
	if _, err := f.pair(f.device("device-2", "jkt-2"), token()); err != nil {
		t.Fatal(err)
	}
	_, err := f.pair(f.device("device-3", "jkt-3"), token())
	wantCode(t, err, plxerr.DevSessionDeviceLimit, "a third device")
	if devices, err := f.svc.ListDevices(ctx, f.Developer, s.ID); err != nil || len(devices) != 2 {
		t.Fatalf("devices = %+v: %v", devices, err)
	}
}

// Verifies: DEV-002.
// A session without traffic for devSession.idleMinutes is over (PLX-9200):
// it issues no token, pairs no device and cannot be revived by Touch.
func TestSession_Idle_DEV_002(t *testing.T) {
	f := newFixture(t, 0)
	ctx := context.Background()
	s, _ := f.open(t)
	f.Now = f.Now.Add(20 * time.Minute)
	if err := f.svc.Touch(ctx, f.Org, s.ID); err != nil {
		t.Fatalf("touching a live session: %v", err)
	}
	f.Now = f.Now.Add(29 * time.Minute)
	if _, err := f.svc.GetPairing(ctx, f.Developer, s.ID); err != nil {
		t.Fatalf("the touch did not keep the session alive: %v", err)
	}
	f.Now = f.Now.Add(31 * time.Minute)
	_, err := f.svc.GetPairing(ctx, f.Developer, s.ID)
	wantCode(t, err, plxerr.DevSessionExpired, "a pairing of an idle session")
	err = f.svc.Touch(ctx, f.Org, s.ID)
	wantCode(t, err, plxerr.DevSessionExpired, "touching an idle session")
}

// Verifies: DEV-002.
// Pairing with an idle session's token is PLX-9200.
func TestPair_IdleSession_DEV_002(t *testing.T) {
	f := newFixture(t, 0)
	_, p := f.open(t)
	f.Now = f.Now.Add(100 * time.Second)
	if _, err := f.pair(f.device("device-1", "jkt-1"), p.Token); err != nil {
		t.Fatal(err)
	}
	s, _ := f.open(t)
	later, err := f.svc.GetPairing(context.Background(), f.Developer, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE dev_sessions SET last_activity_at = $2 WHERE id = $1`, s.ID, f.Now.Add(-31*time.Minute))
		return err //nolint:wrapcheck // a test
	}); err != nil {
		t.Fatal(err)
	}
	_, err = f.pair(f.device("device-2", "jkt-2"), later.Token)
	wantCode(t, err, plxerr.DevSessionExpired, "pairing with an idle session")
}

// Verifies: DEV-002, SEC-021.
// A socket ticket is accepted once, before its expiry, with the key the
// device paired with; anything else is PLX-9205.
func TestConsumeSocketTicket_DEV_002(t *testing.T) {
	f := newFixture(t, 0)
	ctx := context.Background()
	s, p := f.open(t)
	res, err := f.pair(f.device("device-1", "jkt-1"), p.Token)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.svc.ConsumeSocketTicket(ctx, res.Ticket, "jkt-other")
	wantCode(t, err, plxerr.SocketTicketInvalid, "another key")
	_, err = f.svc.ConsumeSocketTicket(ctx, "not-a-ticket", "jkt-1")
	wantCode(t, err, plxerr.SocketTicketInvalid, "an unknown ticket")
	b, err := f.svc.ConsumeSocketTicket(ctx, res.Ticket, "jkt-1")
	if err != nil {
		t.Fatal(err)
	}
	if b.SessionID != s.ID || b.DeviceID != res.DeviceID || b.RegisteredDeviceID != "device-1" || b.OrganizationID != f.Org || b.Thumbprint != "jkt-1" {
		t.Errorf("binding = %+v", b)
	}
	_, err = f.svc.ConsumeSocketTicket(ctx, res.Ticket, "jkt-1")
	wantCode(t, err, plxerr.SocketTicketInvalid, "a used ticket")

	next, err := f.svc.GetPairing(ctx, f.Developer, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.pair(f.device("device-2", "jkt-2"), next.Token)
	if err != nil {
		t.Fatal(err)
	}
	f.Now = f.Now.Add(61 * time.Second)
	_, err = f.svc.ConsumeSocketTicket(ctx, second.Ticket, "jkt-2")
	wantCode(t, err, plxerr.SocketTicketInvalid, "an expired ticket")
}

// Verifies: DEV-002.
// Ending a session unpairs its devices: their tickets and the session's
// tokens stop working.
func TestEndSession_DEV_002(t *testing.T) {
	f := newFixture(t, 0)
	ctx := context.Background()
	s, p := f.open(t)
	res, err := f.pair(f.device("device-1", "jkt-1"), p.Token)
	if err != nil {
		t.Fatal(err)
	}
	next, err := f.svc.GetPairing(ctx, f.Developer, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	ended, err := f.svc.EndSession(ctx, f.Owner, s.ID)
	if err != nil || !ended.EndedAt.Equal(f.Now) {
		t.Fatalf("ended = %+v: %v", ended, err)
	}
	_, err = f.svc.ConsumeSocketTicket(ctx, res.Ticket, "jkt-1")
	wantCode(t, err, plxerr.SocketTicketInvalid, "the ticket of an ended session")
	_, err = f.pair(f.device("device-2", "jkt-2"), next.Token)
	wantCode(t, err, plxerr.DevSessionExpired, "a token of an ended session")
	if devices, err := f.svc.ListDevices(ctx, f.Developer, s.ID); err != nil || len(devices) != 0 {
		t.Errorf("devices = %+v: %v", devices, err)
	}
	if rows := f.Audit(t, audit.DevSessionEnded); len(rows) != 1 || rows[0].TargetID != s.ID {
		t.Errorf("audit entries = %+v", rows)
	}
	again, err := f.svc.EndSession(ctx, f.Developer, s.ID)
	if err != nil || !again.EndedAt.Equal(ended.EndedAt) {
		t.Errorf("ending again: %+v, %v", again, err)
	}
}
