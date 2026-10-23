// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package devsession

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/devtoken"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/signing"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// nonceBytes is the size of a pairing token's nonce.
const nonceBytes = 16

// IDs generates identifiers.
type IDs interface{ New() (string, error) }

// LimitsFunc returns the limits in force for an organisation and an app,
// read in the transaction.
type LimitsFunc func(ctx context.Context, tx pgx.Tx, organizationID, appID string) (limits.Set, error)

// StepUpPolicy reports whether creating a dev session in an environment
// needs a recent step-up: the environment's devSessionRequireStepUp
// security setting (ADR-0075 Revision).
type StepUpPolicy func(ctx context.Context, appID, environmentID string) (bool, error)

// Options configures a Service.
type Options struct {
	DB     *storage.DB
	Audit  *audit.Log
	IDs    IDs
	Limits LimitsFunc
	// Tokens signs pairing tokens with the development token key and lists
	// the keys that verify them.
	Tokens signing.TokenSigner
	// Environments are the environment keys that may host a dev session,
	// the configuration's devSession.enabledEnvironments. A production
	// environment is refused whatever is listed (DEV-003).
	Environments []string
	// PublicBaseURL is the server's externally reachable base URL, which
	// the pairing link names.
	PublicBaseURL string
	// StepUp, when set, says whether an environment's sessions need a
	// step-up; nil means none does.
	StepUp StepUpPolicy
	// PingInterval is how often the relay pings a device socket, and
	// IdleTimeout how long a socket may stay silent before it is closed
	// (devSession.pingInterval, devSession.idleTimeout); zero takes the
	// configuration's defaults.
	PingInterval time.Duration
	IdleTimeout  time.Duration
	// ServerVersion is the server's version, which an exported trace
	// names.
	ServerVersion string
	// Random is the source of nonces and tickets; nil uses crypto/rand.
	Random io.Reader
	// Now is the clock; nil uses time.Now.
	Now func() time.Time
}

// Service opens dev sessions, pairs devices with them and relays their
// sockets.
type Service struct {
	o       Options
	now     func() time.Time
	random  io.Reader
	compact devtoken.Compact
	hub     *Hub
}

// New returns the service.
func New(o Options) (*Service, error) {
	if o.DB == nil || o.Audit == nil || o.IDs == nil || o.Limits == nil || o.Tokens == nil {
		return nil, errors.New("devsession: a database, an audit log, identifiers, limits and a token signer are required")
	}
	s := &Service{o: o, now: o.Now, random: o.Random, hub: NewHub()}
	if s.now == nil {
		s.now = time.Now
	}
	if s.random == nil {
		s.random = rand.Reader
	}
	return s, nil
}

// Session is a dev session.
type Session struct {
	ID             string
	OrganizationID string
	AppID          string
	EnvironmentID  string
	EnvironmentKey string
	// Creator is the person who created the session, whom its pairing
	// tokens name (DEV-002).
	Creator        string
	CreatedBy      audit.Actor
	CreatedAt      time.Time
	ExpiresAt      time.Time
	LastActivityAt time.Time
	EndedAt        time.Time
}

// Live reports whether the session is live at the instant now: not ended,
// within its lifetime, and with traffic within idle (PLX-9200).
func (s Session) Live(now time.Time, idle time.Duration) bool {
	return s.EndedAt.IsZero() && now.Before(s.ExpiresAt) && now.Before(s.LastActivityAt.Add(idle))
}

// Pairing is an issued pairing token with the ways to show it. Token is a
// credential: it goes to the person who asked and nowhere else.
type Pairing struct {
	Token string
	// URI is the link the QR code holds.
	URI string
	// Code is the short code a person may type instead.
	Code      string
	ExpiresAt time.Time
}

// Device is a device paired with a session.
type Device struct {
	// ID identifies the device inside the session.
	ID string
	// DeviceID is the device's registered identifier.
	DeviceID       string
	Name           string
	Platform       string
	RuntimeVersion string
	PairedAt       time.Time
}

// CreateSession opens a dev session for an app's environment and issues its
// first pairing token (DEV-002, DEV-003).
func (s *Service) CreateSession(ctx context.Context, p auth.Principal, appID, environmentID string) (Session, Pairing, error) {
	if err := p.AuthorizeSomeEnvironment(auth.DevSessionCreate, appID); err != nil {
		return Session{}, Pairing{}, fmt.Errorf("devsession: %w", err)
	}
	app, err := parseID(appID, "app")
	if err != nil {
		return Session{}, Pairing{}, err
	}
	env, err := parseID(environmentID, "environment")
	if err != nil {
		return Session{}, Pairing{}, err
	}
	var key string
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		key, err = s.checkEnvironment(ctx, tx, p, app, env)
		return err
	})
	if err != nil {
		return Session{}, Pairing{}, err
	}
	if err := s.checkStepUp(ctx, p, appID, environmentID); err != nil {
		return Session{}, Pairing{}, err
	}
	var (
		session Session
		pairing Pairing
	)
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		session, pairing, err = s.create(ctx, tx, p, app, env, key)
		return err
	})
	return session, pairing, err
}

// checkEnvironment authorises the principal on the environment and refuses
// one that may not host a dev session; it returns the environment's key.
func (s *Service) checkEnvironment(ctx context.Context, tx pgx.Tx, p auth.Principal, app, env pgtype.UUID) (string, error) {
	row, err := dbgen.New(tx).GetEnvironment(ctx, env)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && row.AppID != app) {
		return "", plxerr.New(plxerr.ResourceNotFound, "no such environment")
	}
	if err != nil {
		return "", fmt.Errorf("devsession: find the environment: %w", err)
	}
	if err := p.AuthorizeEnvironment(auth.DevSessionCreate, storage.ID(app), row.Key); err != nil {
		return "", fmt.Errorf("devsession: %w", err)
	}
	if row.Production {
		return "", plxerr.New(plxerr.DevSessionEnvironment, "a dev session cannot be opened in the production environment %q", row.Key)
	}
	if !slices.Contains(s.o.Environments, row.Key) {
		return "", plxerr.New(plxerr.DevSessionEnvironment, "the environment %q is not enabled for dev sessions (devSession.enabledEnvironments)", row.Key)
	}
	return row.Key, nil
}

// checkStepUp asks for a recent step-up when the environment's security
// configuration says a session needs one.
func (s *Service) checkStepUp(ctx context.Context, p auth.Principal, appID, environmentID string) error {
	if s.o.StepUp == nil {
		return nil
	}
	required, err := s.o.StepUp(ctx, appID, environmentID)
	if err != nil {
		return fmt.Errorf("devsession: read the security configuration: %w", err)
	}
	if !required {
		return nil
	}
	return p.StepUpError(auth.StepUpWindow) //nolint:wrapcheck // a domain error
}

// create stores a session, issues its first pairing token and audits it.
func (s *Service) create(ctx context.Context, tx pgx.Tx, p auth.Principal, app, env pgtype.UUID, key string) (Session, Pairing, error) {
	set, err := s.o.Limits(ctx, tx, p.OrganizationID, storage.ID(app))
	if err != nil {
		return Session{}, Pairing{}, fmt.Errorf("devsession: read the limits: %w", err)
	}
	id, err := s.o.IDs.New()
	if err != nil {
		return Session{}, Pairing{}, fmt.Errorf("devsession: %w", err)
	}
	now := s.now()
	actor := p.Actor()
	row, err := dbgen.New(tx).InsertDevSession(ctx, dbgen.InsertDevSessionParams{
		ID: storage.MustUUID(id), OrganizationID: storage.MustUUID(p.OrganizationID), AppID: app, EnvironmentID: env, EnvironmentKey: key,
		CreatedByUser: creatorOf(p), CreatedByKind: actor.Kind, CreatedByID: actor.ID, CreatedBy: actor.Display,
		CreatedAt: storage.Timestamp(now), ExpiresAt: storage.Timestamp(now.Add(minutes(set, limits.DevSessionLifetimeMinutes))),
	})
	if err != nil {
		return Session{}, Pairing{}, fmt.Errorf("devsession: store the session: %w", err)
	}
	session := sessionOf(row)
	pairing, err := s.issue(ctx, tx, session, set, now)
	if err != nil {
		return Session{}, Pairing{}, err
	}
	_, err = s.o.Audit.Append(ctx, tx, audit.Entry{
		OrganizationID: p.OrganizationID, Actor: actor, Action: audit.DevSessionCreated, TargetKind: "dev_session", TargetID: id,
		Detail: fmt.Sprintf("app %s, environment %s", session.AppID, key),
	})
	if err != nil {
		return Session{}, Pairing{}, fmt.Errorf("devsession: %w", err)
	}
	return session, pairing, nil
}

// GetPairing issues a fresh pairing token for a live session. Only the
// session's creator may (DEV-002).
func (s *Service) GetPairing(ctx context.Context, p auth.Principal, sessionID string) (Pairing, error) {
	if err := authorizeAnywhere(p, auth.DevSessionCreate); err != nil {
		return Pairing{}, err
	}
	id, err := parseID(sessionID, "session")
	if err != nil {
		return Pairing{}, err
	}
	var out Pairing
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		session, err := s.authorized(ctx, tx, p, id, auth.DevSessionCreate, false)
		if err != nil {
			return err
		}
		if session.Creator != creatorOf(p) {
			return plxerr.New(plxerr.PermissionDenied, "only the person who created a dev session may pair devices with it")
		}
		set, err := s.o.Limits(ctx, tx, p.OrganizationID, session.AppID)
		if err != nil {
			return fmt.Errorf("devsession: read the limits: %w", err)
		}
		now := s.now()
		if !session.Live(now, minutes(set, limits.DevSessionIdleMinutes)) {
			return expired()
		}
		out, err = s.issue(ctx, tx, session, set, now)
		return err
	})
	return out, err
}

// issue signs a pairing token for the session and stores its hash.
func (s *Service) issue(ctx context.Context, tx pgx.Tx, session Session, set limits.Set, now time.Time) (Pairing, error) {
	nonce := make([]byte, nonceBytes)
	if _, err := io.ReadFull(s.random, nonce); err != nil {
		return Pairing{}, fmt.Errorf("devsession: draw a nonce: %w", err)
	}
	expires := now.Add(time.Duration(set.Get(limits.DevSessionPairingTokenSeconds)) * time.Second)
	payload, err := json.Marshal(Claims{
		Session: session.ID, Subject: session.Creator, App: session.AppID, Environment: session.EnvironmentID,
		Expiry: expires.Unix(), Nonce: base64.RawURLEncoding.EncodeToString(nonce),
	})
	if err != nil {
		return Pairing{}, fmt.Errorf("devsession: encode the claims: %w", err)
	}
	token, err := s.compact.Sign(ctx, s.o.Tokens, signing.TokenDevelopment, PairingType, payload)
	if err != nil {
		return Pairing{}, fmt.Errorf("devsession: sign the pairing token: %w", err)
	}
	err = dbgen.New(tx).InsertDevPairingToken(ctx, dbgen.InsertDevPairingTokenParams{
		TokenHash: digest(token), OrganizationID: storage.MustUUID(session.OrganizationID), SessionID: storage.MustUUID(session.ID),
		ExpiresAt: storage.Timestamp(expires),
	})
	if err != nil {
		return Pairing{}, fmt.Errorf("devsession: store the pairing token: %w", err)
	}
	return Pairing{Token: token, URI: s.link(token), Code: Code(token), ExpiresAt: expires}, nil
}

// link is the pairing link a QR code holds.
func (s *Service) link(token string) string {
	q := url.Values{}
	q.Set("server", s.o.PublicBaseURL)
	q.Set("token", token)
	return "plux-dev://pair?" + q.Encode()
}

// ListDevices lists the devices paired with a session.
func (s *Service) ListDevices(ctx context.Context, p auth.Principal, sessionID string) ([]Device, error) {
	if err := authorizeAnywhere(p, auth.DevSessionObserve); err != nil {
		return nil, err
	}
	id, err := parseID(sessionID, "session")
	if err != nil {
		return nil, err
	}
	var out []Device
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := s.authorized(ctx, tx, p, id, auth.DevSessionObserve, false); err != nil {
			return err
		}
		rows, err := dbgen.New(tx).ListDevSessionDevices(ctx, id)
		if err != nil {
			return fmt.Errorf("devsession: list the devices: %w", err)
		}
		out = make([]Device, 0, len(rows))
		for _, r := range rows {
			out = append(out, Device{
				ID: storage.ID(r.ID), DeviceID: r.DeviceID, Name: r.Name, Platform: r.Platform, RuntimeVersion: r.RuntimeVersion, PairedAt: r.PairedAt.Time,
			})
		}
		return nil
	})
	return out, err
}

// EndSession ends a session and unpairs its devices; its pairing tokens and
// socket tickets stop working. The creator or an administrator of the
// organisation may end it; ending an ended session changes nothing.
func (s *Service) EndSession(ctx context.Context, p auth.Principal, sessionID string) (Session, error) {
	if err := authorizeAnywhere(p, auth.DevSessionCreate); err != nil {
		return Session{}, err
	}
	id, err := parseID(sessionID, "session")
	if err != nil {
		return Session{}, err
	}
	var out Session
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		session, err := s.authorized(ctx, tx, p, id, auth.DevSessionCreate, true)
		if err != nil {
			return err
		}
		if session.Creator != creatorOf(p) && !p.Holds(auth.OrganizationManage) {
			return plxerr.New(plxerr.PermissionDenied, "only the person who created a dev session, or an administrator, may end it")
		}
		if !session.EndedAt.IsZero() {
			out = session
			return nil
		}
		out, err = s.end(ctx, tx, p, id)
		return err
	})
	if err == nil {
		s.hub.End(out.ID, plxerr.DevSessionExpired)
	}
	return out, err
}

// end marks a session ended, unpairs its devices and audits it.
func (s *Service) end(ctx context.Context, tx pgx.Tx, p auth.Principal, id pgtype.UUID) (Session, error) {
	q := dbgen.New(tx)
	at := storage.Timestamp(s.now())
	row, err := q.EndDevSession(ctx, dbgen.EndDevSessionParams{ID: id, EndedAt: at})
	if err != nil {
		return Session{}, fmt.Errorf("devsession: end the session: %w", err)
	}
	if err := q.RevokeDevSessionDevices(ctx, dbgen.RevokeDevSessionDevicesParams{SessionID: id, RevokedAt: at}); err != nil {
		return Session{}, fmt.Errorf("devsession: unpair the devices: %w", err)
	}
	_, err = s.o.Audit.Append(ctx, tx, audit.Entry{
		OrganizationID: p.OrganizationID, Actor: p.Actor(), Action: audit.DevSessionEnded, TargetKind: "dev_session", TargetID: storage.ID(id),
		Detail: "environment " + row.EnvironmentKey,
	})
	if err != nil {
		return Session{}, fmt.Errorf("devsession: %w", err)
	}
	return sessionOf(row), nil
}

// authorized reads a session, locking it when lock is set, and authorises
// the principal on its environment.
func (*Service) authorized(ctx context.Context, tx pgx.Tx, p auth.Principal, id pgtype.UUID, want auth.Permission, lock bool) (Session, error) {
	q := dbgen.New(tx)
	var (
		row dbgen.DevSession
		err error
	)
	if lock {
		row, err = q.GetDevSessionForUpdate(ctx, id)
	} else {
		row, err = q.GetDevSession(ctx, id)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, plxerr.New(plxerr.ResourceNotFound, "no such dev session")
	}
	if err != nil {
		return Session{}, fmt.Errorf("devsession: find the session: %w", err)
	}
	session := sessionOf(row)
	if err := p.AuthorizeEnvironment(want, session.AppID, session.EnvironmentKey); err != nil {
		return Session{}, fmt.Errorf("devsession: %w", err)
	}
	return session, nil
}

// authorizeAnywhere is the first check of a call that names a session whose
// environment is not yet known: the principal must hold the permission
// somewhere in the organisation. The call then checks the session's
// environment.
func authorizeAnywhere(p auth.Principal, want auth.Permission) error {
	if p.OrganizationID == "" {
		return plxerr.New(plxerr.AuthenticationRequired, "this call acts in an organisation, and none was named")
	}
	if p.Holds(want) {
		return nil
	}
	for _, perms := range p.AppPermissions {
		if slices.Contains(perms, want) {
			return nil
		}
	}
	for _, perms := range p.EnvPermissions {
		if slices.Contains(perms, want) {
			return nil
		}
	}
	return plxerr.New(plxerr.PermissionDenied, "%s is required", want)
}

// creatorOf names the person behind a principal, also when it acts through
// an access token, as a session records its creator.
func creatorOf(p auth.Principal) string {
	if p.UserID != "" {
		return p.UserID
	}
	return p.Kind + ":" + p.ID
}

// sessionOf converts a stored session.
func sessionOf(r dbgen.DevSession) Session {
	return Session{
		ID: storage.ID(r.ID), OrganizationID: storage.ID(r.OrganizationID), AppID: storage.ID(r.AppID), EnvironmentID: storage.ID(r.EnvironmentID),
		EnvironmentKey: r.EnvironmentKey, Creator: r.CreatedByUser,
		CreatedBy: audit.Actor{Kind: r.CreatedByKind, ID: r.CreatedByID, Display: r.CreatedBy},
		CreatedAt: r.CreatedAt.Time, ExpiresAt: r.ExpiresAt.Time, LastActivityAt: r.LastActivityAt.Time, EndedAt: r.EndedAt.Time,
	}
}

// minutes reads a limit counted in minutes.
func minutes(set limits.Set, k limits.Key) time.Duration {
	return time.Duration(set.Get(k)) * time.Minute
}

// expired is the refusal of a session that is no longer live.
func expired() error {
	return plxerr.New(plxerr.DevSessionExpired, "the dev session has ended")
}

// parseID parses an identifier a caller gave.
func parseID(id, what string) (pgtype.UUID, error) {
	v, err := storage.UUID(id)
	if err != nil {
		return pgtype.UUID{}, plxerr.New(plxerr.InvalidFormat, "the %s identifier is not valid", what)
	}
	return v, nil
}

// inOrg runs f in a transaction of the principal's organisation.
func (s *Service) inOrg(ctx context.Context, p auth.Principal, f func(context.Context, pgx.Tx) error) error {
	return s.inTenant(ctx, storage.Tenant{OrganizationID: p.OrganizationID, UserID: p.UserID}, f)
}

// inTenant runs f in a transaction bound to a tenant.
func (s *Service) inTenant(ctx context.Context, t storage.Tenant, f func(context.Context, pgx.Tx) error) error {
	return s.o.DB.InTx(ctx, t, f) //nolint:wrapcheck // InTx wraps its own failures
}
