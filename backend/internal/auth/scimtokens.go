// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/edition"
	"github.com/nightCode42/plux3/backend/ext/identity"
	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// This file administers the bearer tokens of an organisation's SCIM client
// (GOV-004). A token is shown once and stored as its hash. Rotating makes a
// new token and lets the old one work for an overlap, so that the identity
// provider can be switched over without a gap; revoking ends a token at
// once.

// ScimToken is a SCIM bearer token without its secret.
type ScimToken struct {
	ID             string
	OrganizationID string
	Name           string
	// Prefix identifies the token without revealing it.
	Prefix     string
	CreatedAt  time.Time
	ExpiresAt  time.Time
	RevokedAt  time.Time
	LastUsedAt time.Time
}

// scimTokenOf converts a stored token.
func scimTokenOf(row dbgen.ScimToken) ScimToken {
	return ScimToken{
		ID: storage.ID(row.ID), OrganizationID: storage.ID(row.OrganizationID), Name: row.Name, Prefix: row.Prefix,
		CreatedAt: storage.Time(row.CreatedAt), ExpiresAt: storage.Time(row.ExpiresAt),
		RevokedAt: storage.Time(row.RevokedAt), LastUsedAt: storage.Time(row.LastUsedAt),
	}
}

// scimAdmin authorises the administration of SCIM tokens: sso.manage, and
// the licence for SCIM when the call changes anything.
func scimAdmin(ctx context.Context, p Principal, write bool) error {
	if err := p.Authorize(SSOManage); err != nil {
		return err
	}
	if write {
		return edition.Require(ctx, edition.FeatureSCIM) //nolint:wrapcheck // a domain error
	}
	return edition.RequireRead(ctx, edition.FeatureSCIM) //nolint:wrapcheck // a domain error
}

// mintScimToken stores a new token and returns it with its secret.
func (s *Service) mintScimToken(ctx context.Context, tx pgx.Tx, p Principal, name string) (ScimToken, string, error) {
	secret, err := NewSecret(PrefixSCIM, 32)
	if err != nil {
		return ScimToken{}, "", err
	}
	id, err := s.newID()
	if err != nil {
		return ScimToken{}, "", err
	}
	user, _ := storage.NullUUID(p.UserID)
	row, err := dbgen.New(tx).CreateScimToken(ctx, dbgen.CreateScimTokenParams{
		ID: storage.MustUUID(id), OrganizationID: storage.MustUUID(p.OrganizationID), Name: name,
		Prefix: secret.Prefix, SecretHash: secret.Hash, CreatedBy: user,
	})
	if err != nil {
		return ScimToken{}, "", fmt.Errorf("auth: create a SCIM token: %w", err)
	}
	return scimTokenOf(row), secret.Value, nil
}

// checkTokenName validates the name of a SCIM token.
func checkTokenName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 120 {
		return "", plxerr.New(plxerr.InvalidFormat, "a token has a name of up to 120 characters")
	}
	return name, nil
}

// CreateScimToken makes a token for the organisation's SCIM client and
// returns it with its secret, which is not kept and cannot be shown again.
func (s *Service) CreateScimToken(ctx context.Context, p Principal, name string) (ScimToken, string, error) {
	if err := scimAdmin(ctx, p, true); err != nil {
		return ScimToken{}, "", err
	}
	name, err := checkTokenName(name)
	if err != nil {
		return ScimToken{}, "", err
	}
	var (
		out    ScimToken
		secret string
	)
	err = s.inTx(ctx, storage.Tenant{OrganizationID: p.OrganizationID}, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		if out, secret, err = s.mintScimToken(ctx, tx, p, name); err != nil {
			return err
		}
		return s.record(ctx, tx, audit.Entry{
			OrganizationID: p.OrganizationID, Actor: p.Actor(), Action: audit.ScimAccessCreated,
			TargetKind: "scim_token", TargetID: out.ID, Detail: "name=" + name,
		})
	})
	return out, secret, err
}

// ListScimTokens lists the organisation's SCIM tokens.
func (s *Service) ListScimTokens(ctx context.Context, p Principal, after storage.Cursor, limit int32) ([]ScimToken, error) {
	if err := scimAdmin(ctx, p, false); err != nil {
		return nil, err
	}
	var out []ScimToken
	err := s.inTx(ctx, storage.Tenant{OrganizationID: p.OrganizationID}, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := dbgen.New(tx).ListScimTokens(ctx, dbgen.ListScimTokensParams{
			OrganizationID: storage.MustUUID(p.OrganizationID), AfterTime: after.AfterTime(), AfterID: after.AfterID(), PageSize: limit,
		})
		if err != nil {
			return fmt.Errorf("auth: list SCIM tokens: %w", err)
		}
		for _, row := range rows {
			out = append(out, scimTokenOf(row))
		}
		return nil
	})
	return out, err
}

// RotateScimToken replaces a token: it makes a new one, and the old one
// stops working after the overlap. An overlap of zero is the limit
// auth.scimTokenOverlapSeconds; a longer one is cut to it. It returns the
// new token with its secret, and the old token with its end time.
func (s *Service) RotateScimToken(ctx context.Context, p Principal, id string, overlap time.Duration) (next ScimToken, secret string, previous ScimToken, err error) {
	if err := scimAdmin(ctx, p, true); err != nil {
		return ScimToken{}, "", ScimToken{}, err
	}
	tid, err := parseID(id, "token")
	if err != nil {
		return ScimToken{}, "", ScimToken{}, err
	}
	bound := time.Duration(s.o.Limits.Get(limits.AuthScimTokenOverlapSeconds)) * time.Second
	if overlap <= 0 || overlap > bound {
		overlap = bound
	}
	err = s.inTx(ctx, storage.Tenant{OrganizationID: p.OrganizationID}, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		old, err := q.GetScimToken(ctx, tid)
		if err != nil || old.RevokedAt.Valid {
			if err == nil {
				err = pgx.ErrNoRows
			}
			return notFound(err, "SCIM token")
		}
		if next, secret, err = s.mintScimToken(ctx, tx, p, old.Name); err != nil {
			return err
		}
		ended, err := q.ExpireScimToken(ctx, dbgen.ExpireScimTokenParams{ID: old.ID, ExpiresAt: storage.Timestamp(s.now().Add(overlap))})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("auth: end the old SCIM token: %w", err)
		}
		if err == nil {
			previous = scimTokenOf(ended)
		} else {
			previous = scimTokenOf(old)
		}
		return s.record(ctx, tx, audit.Entry{
			OrganizationID: p.OrganizationID, Actor: p.Actor(), Action: audit.ScimAccessRotated,
			TargetKind: "scim_token", TargetID: next.ID, Detail: fmt.Sprintf("replaces %s, which works for %s more", previous.ID, overlap),
		})
	})
	return next, secret, previous, err
}

// RevokeScimToken ends a token at once.
func (s *Service) RevokeScimToken(ctx context.Context, p Principal, id string) error {
	if err := scimAdmin(ctx, p, false); err != nil {
		return err
	}
	tid, err := parseID(id, "token")
	if err != nil {
		return err
	}
	return s.inTx(ctx, storage.Tenant{OrganizationID: p.OrganizationID}, func(ctx context.Context, tx pgx.Tx) error {
		n, err := dbgen.New(tx).RevokeScimToken(ctx, tid)
		if err != nil {
			return fmt.Errorf("auth: revoke a SCIM token: %w", err)
		}
		if n == 0 {
			return plxerr.New(plxerr.ResourceNotFound, "no such SCIM token")
		}
		return s.record(ctx, tx, audit.Entry{
			OrganizationID: p.OrganizationID, Actor: p.Actor(), Action: audit.ScimAccessRevoked,
			TargetKind: "scim_token", TargetID: id,
		})
	})
}

// authenticateScim resolves a SCIM bearer token to its organisation. A
// token that is unknown, expired or revoked is not told apart.
func (s *Service) authenticateScim(ctx context.Context, token string) (string, error) {
	if !strings.HasPrefix(token, PrefixSCIM+"_") {
		return "", identity.ErrUnauthorized
	}
	var row dbgen.ScimToken
	err := s.inTx(ctx, storage.Tenant{Scope: storage.ScopeAuthentication}, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		row, err = dbgen.New(tx).GetScimTokenByHash(ctx, HashSecret(token))
		if err != nil {
			return identity.ErrUnauthorized
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return s.touchScimToken(ctx, row)
}

// touchScimToken records a SCIM token's use in its organisation's own
// transaction: the row's policy admits a write only into the organisation
// it belongs to, not into the authentication scope it was found in.
func (s *Service) touchScimToken(ctx context.Context, row dbgen.ScimToken) (string, error) {
	org := storage.ID(row.OrganizationID)
	err := s.inTx(ctx, storage.Tenant{OrganizationID: org}, func(ctx context.Context, tx pgx.Tx) error {
		if err := dbgen.New(tx).TouchScimToken(ctx, row.ID); err != nil {
			return fmt.Errorf("auth: record a SCIM token's use: %w", err)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return org, nil
}
