// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"math/big"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// Phones signed in to the Plux Dev app (DEV-002, ADR-0104).
//
// A person connects a phone once: Studio, in a session with a recent
// step-up, creates a sign-in whose QR code holds a single-use grant. The
// phone claims the grant with a DPoP proof of its hardware key; Studio then
// shows a two-digit number and the phone offers it among three. The right
// pick mints a phone token bound to the key: a token of its own source and
// prefix, refused as a bearer token, accepted only with a proof of that key
// and only by the procedures of a signed-in phone (api.PhoneProcedures). It
// lapses after devSession.phoneIdleDays without use. A wrong pick ends the
// sign-in, so a guess has one chance in three, once per QR code.

const (
	// sourcePhone is the source of a phone token.
	sourcePhone = "phone"
	// PrefixPhone is a phone token: never a bearer token.
	PrefixPhone = "plux_phn"
	// grantBytes is the size of a sign-in grant.
	grantBytes = 32
	// phoneNameMax and platformMax bound what a phone says about itself.
	phoneNameMax = 80
	platformMax  = 40
	// attemptWindow is the window of devSession.phoneAttempts.
	attemptWindow = 10 * time.Minute
	// numberOptions is how many numbers a phone offers, one of them right.
	numberOptions = 3
)

// phoneScopes are what a phone token may do at most: read the apps to
// preview and join dev sessions. The procedures a phone may call narrow it
// further, to joining its own person's Studio sessions.
var phoneScopes = []Permission{"app.read", "devsession.create"}

// Phone sign-in states.
const (
	PhoneSignInWaiting   = "waiting"
	PhoneSignInClaimed   = "claimed"
	PhoneSignInSignedIn  = "signed_in"
	PhoneSignInFailed    = "failed"
	PhoneSignInCancelled = "cancelled"
	// PhoneSignInExpired is reported for a waiting or claimed sign-in past
	// its expiry; it is never stored.
	PhoneSignInExpired = "expired"
)

// PhoneSignInGrant is a new sign-in: Grant goes into the QR code and is
// returned exactly once.
type PhoneSignInGrant struct {
	ID        string
	Grant     string
	ExpiresAt time.Time
}

// PhoneSignInStatus is where a sign-in stands, for the person who created
// it. Number is set only while a phone has claimed it.
type PhoneSignInStatus struct {
	State     string
	Number    int
	PhoneName string
	Platform  string
	ExpiresAt time.Time
}

// PhoneClaim is what a phone shows once it claimed a sign-in.
type PhoneClaim struct {
	Options          []int
	ExpiresAt        time.Time
	UserDisplay      string
	OrganizationName string
}

// PhoneToken is a phone's new token, returned exactly once.
type PhoneToken struct {
	Secret           string
	PhoneID          string
	OrganizationID   string
	UserDisplay      string
	OrganizationName string
}

// Phone is a signed-in phone.
type Phone struct {
	ID         string
	Name       string
	Platform   string
	SignedInAt time.Time
	LastUsedAt time.Time
	LapsesAt   time.Time
}

// DrawNumbers draws a number challenge from r: numberOptions distinct
// two-digit numbers in a uniformly random order, and the one among them,
// uniformly chosen, that a person must pick.
func DrawNumbers(r io.Reader) (int, []int, error) {
	options := make([]int, 0, numberOptions)
	for len(options) < numberOptions {
		n, err := rand.Int(r, big.NewInt(90))
		if err != nil {
			return 0, nil, fmt.Errorf("auth: draw a number: %w", err)
		}
		v := 10 + int(n.Int64())
		if !containsInt(options, v) {
			options = append(options, v)
		}
	}
	i, err := rand.Int(r, big.NewInt(numberOptions))
	if err != nil {
		return 0, nil, fmt.Errorf("auth: draw a number: %w", err)
	}
	return options[i.Int64()], options, nil
}

func containsInt(s []int, v int) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// CreatePhoneSignIn starts connecting a phone for the person of a browser
// session that stepped up recently (SEC-103).
func (s *Service) CreatePhoneSignIn(ctx context.Context, p Principal) (PhoneSignInGrant, error) {
	if p.Kind != KindUser || p.UserID == "" {
		return PhoneSignInGrant{}, plxerr.New(plxerr.PermissionDenied, "a phone is connected from a browser session")
	}
	if err := p.StepUpError(StepUpWindow); err != nil {
		return PhoneSignInGrant{}, err //nolint:wrapcheck // a domain error
	}
	raw := make([]byte, grantBytes)
	if _, err := rand.Read(raw); err != nil {
		return PhoneSignInGrant{}, fmt.Errorf("auth: draw a grant: %w", err)
	}
	grant := base64.RawURLEncoding.EncodeToString(raw)
	id, err := s.newID()
	if err != nil {
		return PhoneSignInGrant{}, err
	}
	expires := s.now().Add(time.Duration(s.o.Limits.Get(limits.DevSessionPhoneSignInSeconds)) * time.Second)
	err = s.inTx(ctx, storage.Tenant{OrganizationID: p.OrganizationID}, func(ctx context.Context, tx pgx.Tx) error {
		_, err := dbgen.New(tx).InsertPhoneSignIn(ctx, dbgen.InsertPhoneSignInParams{
			ID: storage.MustUUID(id), OrganizationID: storage.MustUUID(p.OrganizationID), UserID: storage.MustUUID(p.UserID),
			GrantHash: HashSecret(grant), ExpiresAt: storage.Timestamp(expires), CreatedAt: storage.Timestamp(s.now()),
		})
		if err != nil {
			return fmt.Errorf("auth: store a phone sign-in: %w", err)
		}
		return nil
	})
	if err != nil {
		return PhoneSignInGrant{}, err
	}
	return PhoneSignInGrant{ID: id, Grant: grant, ExpiresAt: expires}, nil
}

// GetPhoneSignIn reports one of the caller's sign-ins.
func (s *Service) GetPhoneSignIn(ctx context.Context, p Principal, signInID string) (PhoneSignInStatus, error) {
	id, err := parseID(signInID, "sign-in")
	if err != nil {
		return PhoneSignInStatus{}, err
	}
	var out PhoneSignInStatus
	err = s.inTx(ctx, storage.Tenant{OrganizationID: p.OrganizationID}, func(ctx context.Context, tx pgx.Tx) error {
		row, err := dbgen.New(tx).GetPhoneSignIn(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && storage.ID(row.UserID) != p.UserID) {
			return plxerr.New(plxerr.ResourceNotFound, "no such sign-in")
		}
		if err != nil {
			return fmt.Errorf("auth: read a phone sign-in: %w", err)
		}
		out = PhoneSignInStatus{
			State: row.State, PhoneName: row.PhoneName, Platform: row.Platform, ExpiresAt: row.ExpiresAt.Time,
		}
		open := row.State == PhoneSignInWaiting || row.State == PhoneSignInClaimed
		if open && !s.now().Before(row.ExpiresAt.Time) {
			out.State = PhoneSignInExpired
		}
		if out.State == PhoneSignInClaimed {
			out.Number = int(row.Number)
		}
		return nil
	})
	return out, err
}

// CancelPhoneSignIn ends one of the caller's sign-ins that is not finished.
func (s *Service) CancelPhoneSignIn(ctx context.Context, p Principal, signInID string) error {
	id, err := parseID(signInID, "sign-in")
	if err != nil {
		return err
	}
	return s.inTx(ctx, storage.Tenant{OrganizationID: p.OrganizationID}, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := dbgen.New(tx).CancelPhoneSignIn(ctx, dbgen.CancelPhoneSignInParams{ID: id, UserID: storage.MustUUID(p.UserID)}); err != nil {
			return fmt.Errorf("auth: cancel a phone sign-in: %w", err)
		}
		return nil
	})
}

// closed is the refusal of a grant that is unknown, expired, used,
// cancelled, or claimed by another key; they are not told apart.
func closed() error {
	return plxerr.New(plxerr.PhoneRequestClosed, "this QR code is no longer valid; show a new one in Studio")
}

// ClaimPhoneSignIn binds a waiting sign-in to the phone key jkt, the key of
// the call's DPoP proof, and draws its number. A key may claim
// devSession.phoneAttempts sign-ins in ten minutes.
func (s *Service) ClaimPhoneSignIn(ctx context.Context, grant, jkt, name, platform string) (PhoneClaim, error) {
	if jkt == "" || grant == "" {
		return PhoneClaim{}, closed()
	}
	number, options, err := DrawNumbers(rand.Reader)
	if err != nil {
		return PhoneClaim{}, err
	}
	var out PhoneClaim
	err = s.inTx(ctx, storage.Tenant{Scope: storage.ScopeAuthentication}, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		n, err := q.CountPhoneClaims(ctx, dbgen.CountPhoneClaimsParams{DpopThumbprint: jkt, Since: storage.Timestamp(s.now().Add(-attemptWindow))})
		if err != nil {
			return fmt.Errorf("auth: count a phone's sign-ins: %w", err)
		}
		if n >= s.o.Limits.Get(limits.DevSessionPhoneAttempts) {
			return plxerr.New(plxerr.RateLimited, "this phone started too many sign-ins; wait ten minutes")
		}
		row, err := q.GetPhoneSignInByGrantForUpdate(ctx, HashSecret(grant))
		if errors.Is(err, pgx.ErrNoRows) {
			return closed()
		}
		if err != nil {
			return fmt.Errorf("auth: read a phone sign-in: %w", err)
		}
		if row.State != PhoneSignInWaiting || !s.now().Before(row.ExpiresAt.Time) {
			return closed()
		}
		if _, err := q.ClaimPhoneSignIn(ctx, dbgen.ClaimPhoneSignInParams{
			ID: row.ID, DpopThumbprint: jkt, Number: int16(number), //nolint:gosec // two digits
			PhoneName: describe(name, phoneNameMax, "Phone"), Platform: describe(platform, platformMax, ""),
			ClaimedAt: storage.Timestamp(s.now()),
		}); err != nil {
			return fmt.Errorf("auth: claim a phone sign-in: %w", err)
		}
		out = PhoneClaim{Options: options, ExpiresAt: row.ExpiresAt.Time, UserDisplay: row.UserDisplayName, OrganizationName: row.OrganizationName}
		return nil
	})
	return out, err
}

// CompletePhoneSignIn is the phone's pick. The phone presents the grant
// with a proof of the key that claimed it; the right number mints the phone
// token, a wrong one ends the sign-in, which is recorded before the refusal
// returns.
func (s *Service) CompletePhoneSignIn(ctx context.Context, grant, jkt string, number int) (PhoneToken, error) {
	if jkt == "" || grant == "" {
		return PhoneToken{}, closed()
	}
	var out PhoneToken
	err := s.inTxRefusing(ctx, storage.Tenant{Scope: storage.ScopeAuthentication}, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		row, err := q.GetPhoneSignInByGrantForUpdate(ctx, HashSecret(grant))
		if errors.Is(err, pgx.ErrNoRows) {
			return closed()
		}
		if err != nil {
			return fmt.Errorf("auth: read a phone sign-in: %w", err)
		}
		if row.State != PhoneSignInClaimed || !s.now().Before(row.ExpiresAt.Time) ||
			subtle.ConstantTimeCompare([]byte(row.DpopThumbprint), []byte(jkt)) != 1 {
			return closed()
		}
		if number != int(row.Number) {
			if _, err := q.FinishPhoneSignIn(ctx, dbgen.FinishPhoneSignInParams{ID: row.ID, State: PhoneSignInFailed}); err != nil {
				return fmt.Errorf("auth: end a phone sign-in: %w", err)
			}
			return refuse(plxerr.New(plxerr.NumberMismatch, "that is not the number Studio showed; show a new QR code in Studio and start again"))
		}
		org := storage.ID(row.OrganizationID)
		if err := storage.Rescope(ctx, tx, storage.Tenant{OrganizationID: org}); err != nil {
			return fmt.Errorf("auth: %w", err)
		}
		token, err := s.mintPhone(ctx, tx, row)
		if err != nil {
			return err
		}
		if _, err := q.FinishPhoneSignIn(ctx, dbgen.FinishPhoneSignInParams{ID: row.ID, State: PhoneSignInSignedIn, TokenID: storage.MustUUID(token.PhoneID)}); err != nil {
			return fmt.Errorf("auth: end a phone sign-in: %w", err)
		}
		token.UserDisplay, token.OrganizationName = row.UserDisplayName, row.OrganizationName
		out = token
		return s.record(ctx, tx, audit.Entry{
			OrganizationID: org,
			Actor:          audit.Actor{Kind: KindUser, ID: storage.ID(row.UserID), Display: row.UserDisplayName},
			Action:         audit.DevPhoneSignedIn, TargetKind: "token", TargetID: token.PhoneID,
			Detail: fmt.Sprintf("%s (%s)", row.PhoneName, row.Platform),
		})
	})
	return out, err
}

// mintPhone writes the phone token of a claimed sign-in.
func (s *Service) mintPhone(ctx context.Context, tx pgx.Tx, row dbgen.GetPhoneSignInByGrantForUpdateRow) (PhoneToken, error) {
	secret, err := NewSecret(PrefixPhone, 32)
	if err != nil {
		return PhoneToken{}, err
	}
	id, err := s.newID()
	if err != nil {
		return PhoneToken{}, err
	}
	_, err = dbgen.New(tx).CreatePhoneToken(ctx, dbgen.CreatePhoneTokenParams{
		ID: storage.MustUUID(id), OrganizationID: row.OrganizationID, UserID: row.UserID,
		Name: row.PhoneName, Prefix: secret.Prefix, SecretHash: secret.Hash, Scopes: ScopeStrings(phoneScopes),
		// A phone's sign-in lapses after devSession.phoneIdleDays unused;
		// this is only its outer bound.
		ExpiresAt:      storage.Timestamp(s.now().Add(maxTokenTTL)),
		DpopThumbprint: row.DpopThumbprint, Platform: row.Platform, CreatedAt: storage.Timestamp(s.now()),
	})
	if err != nil {
		return PhoneToken{}, fmt.Errorf("auth: create a phone token: %w", err)
	}
	return PhoneToken{Secret: secret.Value, PhoneID: id, OrganizationID: storage.ID(row.OrganizationID)}, nil
}

// phoneIdle is how long a phone may go unused.
func (s *Service) phoneIdle() time.Duration {
	return time.Duration(s.o.Limits.Get(limits.DevSessionPhoneIdleDays)) * 24 * time.Hour
}

// lastUse is when a phone token was last used, or created.
func lastUse(createdAt, lastUsedAt pgtype.Timestamptz) time.Time {
	if lastUsedAt.Valid && lastUsedAt.Time.After(createdAt.Time) {
		return lastUsedAt.Time
	}
	return createdAt.Time
}

// PhoneKey is what a phone token says about its phone: the thumbprint of
// the key the call's DPoP proof must match, and the phone's name.
type PhoneKey struct {
	JKT  string
	Name string
}

// AuthenticatePhoneToken resolves a phone token with its PhoneKey. A token
// idle for longer than devSession.phoneIdleDays is refused like an unknown
// one.
func (s *Service) AuthenticatePhoneToken(ctx context.Context, secret string) (Identity, PhoneKey, error) {
	if !strings.HasPrefix(secret, PrefixPhone+"_") {
		return Identity{}, PhoneKey{}, expired()
	}
	var (
		id  Identity
		key PhoneKey
	)
	err := s.inTx(ctx, storage.Tenant{Scope: storage.ScopeAuthentication}, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		row, err := q.GetAccessTokenByHash(ctx, HashSecret(secret))
		if errors.Is(err, pgx.ErrNoRows) {
			return expired()
		}
		if err != nil {
			return fmt.Errorf("auth: read a token: %w", err)
		}
		if row.Source != sourcePhone || row.DpopThumbprint == "" || !s.now().Before(lastUse(row.CreatedAt, row.LastUsedAt).Add(s.phoneIdle())) {
			return expired()
		}
		req := audit.RequestFrom(ctx)
		if err := q.TouchPhoneToken(ctx, dbgen.TouchPhoneTokenParams{
			ID: row.ID, LastIpPrefix: ipPrefix(req.SourceIP), LastUserAgent: req.UserAgent, At: storage.Timestamp(s.now()),
		}); err != nil {
			return fmt.Errorf("auth: record a phone's use: %w", err)
		}
		id = tokenIdentity(row)
		id.Kind = KindPhone
		key = PhoneKey{JKT: row.DpopThumbprint, Name: row.Name}
		return nil
	})
	return id, key, err
}

// ListPhones lists the caller's signed-in phones that have not lapsed.
func (s *Service) ListPhones(ctx context.Context, p Principal) ([]Phone, error) {
	if p.UserID == "" {
		return nil, plxerr.New(plxerr.PermissionDenied, "only a person has phones")
	}
	var out []Phone
	err := s.inTx(ctx, storage.Tenant{OrganizationID: p.OrganizationID}, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := dbgen.New(tx).ListPhoneTokens(ctx, dbgen.ListPhoneTokensParams{
			OrganizationID: storage.MustUUID(p.OrganizationID), UserID: storage.MustUUID(p.UserID),
		})
		if err != nil {
			return fmt.Errorf("auth: list phones: %w", err)
		}
		for _, r := range rows {
			lapses := lastUse(r.CreatedAt, r.LastUsedAt).Add(s.phoneIdle())
			if !s.now().Before(lapses) || !s.now().Before(r.ExpiresAt.Time) {
				continue
			}
			out = append(out, Phone{
				ID: storage.ID(r.ID), Name: r.Name, Platform: r.Platform, SignedInAt: r.CreatedAt.Time,
				LastUsedAt: r.LastUsedAt.Time, LapsesAt: lapses,
			})
		}
		return nil
	})
	return out, err
}

// RevokePhone signs out one of the caller's phones.
func (s *Service) RevokePhone(ctx context.Context, p Principal, phoneID string) error {
	id, err := parseID(phoneID, "phone")
	if err != nil {
		return err
	}
	return s.inTx(ctx, storage.Tenant{OrganizationID: p.OrganizationID}, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		row, err := q.GetAccessToken(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && (row.Source != sourcePhone || storage.ID(row.UserID) != p.UserID)) {
			return plxerr.New(plxerr.ResourceNotFound, "no such phone")
		}
		if err != nil {
			return fmt.Errorf("auth: read a phone: %w", err)
		}
		return s.revokePhone(ctx, tx, row, p.Actor())
	})
}

// SignOutPhone revokes the calling phone's own token.
func (s *Service) SignOutPhone(ctx context.Context, id Identity) error {
	if id.Kind != KindPhone {
		return plxerr.New(plxerr.PermissionDenied, "only a phone signs itself out")
	}
	return s.inTx(ctx, storage.Tenant{OrganizationID: id.OrganizationID}, func(ctx context.Context, tx pgx.Tx) error {
		row, err := dbgen.New(tx).GetAccessToken(ctx, storage.MustUUID(id.ID))
		if err != nil {
			return fmt.Errorf("auth: read a phone: %w", err)
		}
		return s.revokePhone(ctx, tx, row, id.Actor())
	})
}

func (s *Service) revokePhone(ctx context.Context, tx pgx.Tx, row dbgen.AccessToken, actor audit.Actor) error {
	if _, err := dbgen.New(tx).RevokeAccessToken(ctx, row.ID); err != nil {
		return fmt.Errorf("auth: revoke a phone: %w", err)
	}
	return s.record(ctx, tx, audit.Entry{
		OrganizationID: storage.ID(row.OrganizationID), Actor: actor,
		Action: audit.DevPhoneRevoked, TargetKind: "token", TargetID: storage.ID(row.ID),
	})
}

// describe trims what a phone says about itself to max runes of printable
// text, or def when nothing is left.
func describe(s string, maxRunes int, def string) string {
	var b strings.Builder
	n := 0
	for _, r := range strings.TrimSpace(s) {
		if n == maxRunes {
			break
		}
		if r < 0x20 || r == 0x7f {
			continue
		}
		b.WriteRune(r)
		n++
	}
	if b.Len() == 0 {
		return def
	}
	return b.String()
}
