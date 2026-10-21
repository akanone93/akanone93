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
		loaded, err := s.restrictionsOf(ctx, organizationID, org, pgtype.UUID{})
		if err != nil {
			return Principal{}, err
		}
		p := resolve(id, organizationID, nil, nil, id.Scopes, loaded.restrictions...)
		p.Version = loaded.version
		return p, nil
	}
	loaded, err := s.grants(ctx, organizationID, org, storage.MustUUID(id.UserID))
	if err != nil {
		return Principal{}, err
	}
	p := resolve(id, organizationID, loaded.given, loaded.custom, nil, loaded.restrictions...)
	p.Version = loaded.version
	return p, nil
}

// loadedGrants is what the resolver reads for a person: their grants, the
// custom roles those use, the restrictions that apply to them, and the
// organisation's permissions version they were read at. It is immutable
// once loaded, so a cache may share it between calls.
type loadedGrants struct {
	given        []roleGrant
	custom       map[Role][]ScopedPermission
	restrictions []Restriction
	version      int64
}

// grants returns a person's loadedGrants, from this replica's permission
// cache when it holds a fresh entry (GOV-007), and otherwise as loadGrants
// reads them. Without a change notifier there is no cache: nothing would
// tell the replica that an entry had gone stale.
func (s *Service) grants(ctx context.Context, organizationID string, org, user pgtype.UUID) (loadedGrants, error) {
	if s.bus == nil {
		return s.loadGrants(ctx, organizationID, org, user)
	}
	key := permissionKey{user: storage.ID(user), organization: organizationID}
	if loaded, ok := s.bus.lookup(key); ok {
		return loaded, nil
	}
	t := s.bus.begin()
	loaded, err := s.loadGrants(ctx, organizationID, org, user)
	if err == nil {
		s.bus.store(t, key, loaded)
	}
	return loaded, err
}

// loadGrants reads a person's loadedGrants in a transaction of its own; see
// readGrants.
func (s *Service) loadGrants(ctx context.Context, organizationID string, org, user pgtype.UUID) (loadedGrants, error) {
	var loaded loadedGrants
	err := s.inTx(ctx, storage.Tenant{OrganizationID: organizationID}, func(ctx context.Context, tx pgx.Tx) (err error) {
		loaded, err = s.readGrants(ctx, dbgen.New(tx), org, user)
		return err
	})
	return loaded, err
}

// readGrants reads a person's grants in an organisation, directly or
// through a team (GOV-001, GOV-003), with the organisation's custom roles
// they use, and the restrictions that apply to the person, to their teams
// and to the organisation (GOV-006). It reads the permissions version first:
// whatever it reads after is at least as new as the version it is labelled
// with (GOV-007). A person with no grant and no membership is not part of the
// organisation, which is reported as not found.
func (s *Service) readGrants(ctx context.Context, q *dbgen.Queries, org, user pgtype.UUID) (loadedGrants, error) {
	var (
		loaded loadedGrants
		err    error
	)
	if loaded.version, err = q.GetPermissionsVersion(ctx, org); err != nil {
		return loadedGrants{}, fmt.Errorf("auth: read the permissions version: %w", err)
	}
	if loaded.given, err = readGivenGrants(ctx, q, org, user); err != nil {
		return loadedGrants{}, err
	}
	if loaded.custom, err = s.customRoleTable(ctx, q, org, loaded.given); err != nil {
		return loadedGrants{}, err
	}
	if loaded.restrictions, err = restrictionsApplying(ctx, q, org, user); err != nil {
		return loadedGrants{}, err
	}
	return loaded, nil
}

// readGivenGrants reads the grants of a person: the role of a direct
// membership is a grant on the organisation, a role on an app (app_access) a
// grant on that app, and role_grants hold grants at any scope. A team
// membership grants the team's roles on apps, through app_access and
// role_grants; the organisation-wide role is the direct one.
func readGivenGrants(ctx context.Context, q *dbgen.Queries, org, user pgtype.UUID) ([]roleGrant, error) {
	var given []roleGrant
	memberships, err := q.ListMembershipsForUser(ctx, dbgen.ListMembershipsForUserParams{OrganizationID: org, UserID: user})
	if err != nil {
		return nil, fmt.Errorf("auth: read memberships: %w", err)
	}
	for _, m := range memberships {
		if !m.TeamID.Valid {
			given = append(given, roleGrant{ID: storage.ID(m.ID), Role: Role(m.Role), Scope: ScopeOrganization})
		}
	}
	appGrants, err := q.ListAppRolesForUser(ctx, dbgen.ListAppRolesForUserParams{OrganizationID: org, UserID: user})
	if err != nil {
		return nil, fmt.Errorf("auth: read app access: %w", err)
	}
	for _, g := range appGrants {
		given = append(given, roleGrant{ID: storage.ID(g.ID), Role: Role(g.Role), Scope: ScopeApp, AppID: storage.ID(g.AppID)})
	}
	scoped, err := q.ListRoleGrantsForUser(ctx, dbgen.ListRoleGrantsForUserParams{OrganizationID: org, UserID: user})
	if err != nil {
		return nil, fmt.Errorf("auth: read role grants: %w", err)
	}
	for _, g := range scoped {
		given = append(given, roleGrant{
			ID: storage.ID(g.ID), Role: Role(g.Role), Scope: Scope(g.Scope), AppID: storage.ID(g.AppID),
			Environment: deref(g.EnvironmentKey),
		})
	}
	if len(memberships) == 0 && len(appGrants) == 0 && len(scoped) == 0 {
		return nil, plxerr.New(plxerr.ResourceNotFound, "no such organisation")
	}
	return given, nil
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
