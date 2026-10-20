// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// expired is the refusal of a credential that is unknown, expired or
// revoked; the three are not told apart.
func expired() error {
	return plxerr.New(plxerr.AuthenticationRequired, "the credential is not valid; sign in again")
}

// AuthenticateToken resolves a bearer token: a personal access token, a
// `plux login` token or a CI token (SRV-064). The token is found by its
// hash before its organisation is known, which is the one thing the
// authentication scope is for.
func (s *Service) AuthenticateToken(ctx context.Context, secret string) (Identity, error) {
	if !strings.HasPrefix(secret, PrefixToken+"_") {
		return Identity{}, expired()
	}
	var id Identity
	err := s.inTx(ctx, storage.Tenant{Scope: storage.ScopeAuthentication}, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		row, err := q.GetAccessTokenByHash(ctx, HashSecret(secret))
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return expired()
			}
			return fmt.Errorf("auth: read a token: %w", err)
		}
		req := audit.RequestFrom(ctx)
		if err := q.TouchAccessToken(ctx, dbgen.TouchAccessTokenParams{ID: row.ID, LastIpPrefix: ipPrefix(req.SourceIP), LastUserAgent: req.UserAgent}); err != nil {
			return fmt.Errorf("auth: record a token's use: %w", err)
		}
		id = tokenIdentity(row)
		return nil
	})
	return id, err
}

// tokenIdentity describes a stored token's holder.
func tokenIdentity(row dbgen.GetAccessTokenByHashRow) Identity {
	// A scope that has left the catalogue grants nothing; the rest still
	// apply, and the slice is never nil, because a token is always
	// restricted by its scopes.
	scopes := make([]Permission, 0, len(row.Scopes))
	for _, v := range row.Scopes {
		if p, err := ParseScopes([]string{v}); err == nil {
			scopes = append(scopes, p...)
		}
	}
	id := Identity{
		Kind: KindToken, ID: storage.ID(row.ID), UserID: storage.ID(row.UserID),
		Display: row.UserDisplayName, OrganizationID: storage.ID(row.OrganizationID),
		Scopes: scopes, SecondFactor: true,
	}
	if row.Source == sourceCI {
		id.Kind, id.Display = KindCI, "ci: "+row.Subject
	}
	return id
}

// Resolve binds an identity to the organisation a call acts in and
// computes what it may do there (SEC-102). A token is bound to its own
// organisation and cannot name another; a session may name any
// organisation its user belongs to. An organisation the caller has no
// part in is reported as not found, not as forbidden, so that its
// existence is not disclosed.
func (s *Service) Resolve(ctx context.Context, id Identity, organizationID string) (Principal, error) {
	if id.OrganizationID != "" {
		if organizationID != "" && organizationID != id.OrganizationID {
			return Principal{}, plxerr.New(plxerr.PermissionDenied, "this token acts only in its own organisation")
		}
		organizationID = id.OrganizationID
	}
	if organizationID == "" {
		return Principal{}, plxerr.New(plxerr.InvalidFormat,
			"this call acts in an organisation; name it with the X-Plux-Organization header or the request")
	}
	org, err := parseID(organizationID, "organisation")
	if err != nil {
		return Principal{}, err
	}
	if id.Kind == KindCI {
		return resolve(id, organizationID, nil, nil, id.Scopes), nil
	}
	given, custom, err := s.grants(ctx, organizationID, org, storage.MustUUID(id.UserID))
	if err != nil {
		return Principal{}, err
	}
	return resolve(id, organizationID, given, custom, nil), nil
}

// grants reads a person's grants in an organisation, directly or through
// a team (GOV-001, GOV-003): the role of a direct membership is a grant on
// the organisation, a role on an app (app_access) a grant on that app, and
// role_grants hold grants at any scope. It reads the organisation's custom
// roles with them, to resolve grants of custom roles. A person with no
// grant and no membership is not part of the organisation, which is
// reported as not found.
func (s *Service) grants(ctx context.Context, organizationID string, org, user pgtype.UUID) ([]roleGrant, map[Role][]ScopedPermission, error) {
	var (
		given  []roleGrant
		custom map[Role][]ScopedPermission
	)
	err := s.inTx(ctx, storage.Tenant{OrganizationID: organizationID}, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		memberships, err := q.ListMembershipsForUser(ctx, dbgen.ListMembershipsForUserParams{OrganizationID: org, UserID: user})
		if err != nil {
			return fmt.Errorf("auth: read memberships: %w", err)
		}
		for _, m := range memberships {
			// A team membership grants the team's roles on apps, through
			// app_access and role_grants; the organisation-wide role is the
			// direct one.
			if !m.TeamID.Valid {
				given = append(given, roleGrant{Role: Role(m.Role), Scope: ScopeOrganization})
			}
		}
		appGrants, err := q.ListAppRolesForUser(ctx, dbgen.ListAppRolesForUserParams{OrganizationID: org, UserID: user})
		if err != nil {
			return fmt.Errorf("auth: read app access: %w", err)
		}
		for _, g := range appGrants {
			given = append(given, roleGrant{Role: Role(g.Role), Scope: ScopeApp, AppID: storage.ID(g.AppID)})
		}
		scoped, err := q.ListRoleGrantsForUser(ctx, dbgen.ListRoleGrantsForUserParams{OrganizationID: org, UserID: user})
		if err != nil {
			return fmt.Errorf("auth: read role grants: %w", err)
		}
		for _, g := range scoped {
			given = append(given, roleGrant{
				Role: Role(g.Role), Scope: Scope(g.Scope), AppID: storage.ID(g.AppID),
				Environment: deref(g.EnvironmentKey),
			})
		}
		if len(memberships) == 0 && len(appGrants) == 0 && len(scoped) == 0 {
			return plxerr.New(plxerr.ResourceNotFound, "no such organisation")
		}
		custom, err = s.customRoleTable(ctx, q, org, given)
		return err
	})
	return given, custom, err
}

// deref returns the string a nullable column holds, or "".
func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// CurrentUser returns the signed-in person with their memberships.
func (s *Service) CurrentUser(ctx context.Context, id Identity) (User, []dbgen.Membership, error) {
	if id.UserID == "" {
		return User{}, nil, plxerr.New(plxerr.PreconditionFailed, "this credential belongs to no person")
	}
	var (
		user        User
		memberships []dbgen.Membership
	)
	err := s.inTx(ctx, storage.Tenant{UserID: id.UserID}, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		uid := storage.MustUUID(id.UserID)
		row, err := q.GetUser(ctx, uid)
		if err != nil {
			return notFound(err, "user")
		}
		n, err := q.CountConfirmedFactors(ctx, uid)
		if err != nil {
			return fmt.Errorf("auth: read the factors: %w", err)
		}
		user = userOf(row, n > 0)
		memberships, err = q.ListAllMembershipsForUser(ctx, dbgen.ListAllMembershipsForUserParams{UserID: uid, PageSize: 1000})
		if err != nil {
			return fmt.Errorf("auth: read memberships: %w", err)
		}
		if id.OrganizationID != "" {
			// A token sees only its own organisation.
			kept := memberships[:0]
			for _, m := range memberships {
				if storage.ID(m.OrganizationID) == id.OrganizationID {
					kept = append(kept, m)
				}
			}
			memberships = kept
		}
		return nil
	})
	return user, memberships, err
}
