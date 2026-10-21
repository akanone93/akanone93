// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package auth_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/edition"
	"github.com/nightCode42/plux3/backend/ext/identity"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

// scimOrganisation is an organisation with an owner, a developer and a SCIM
// client: a token for the organisation's directory, and the directory. Its
// context carries a licence that includes SCIM.
type scimOrganisation struct {
	*fixture
	org   string
	owner auth.Principal
	ctx   context.Context
	dir   identity.Directory
	token string
}

// scimLicensed is a context whose licence includes SCIM.
func scimLicensed() context.Context {
	return edition.WithProvider(context.Background(), licence{
		State: edition.StateValid, Features: []edition.Feature{edition.FeatureSCIM, edition.FeatureSAML},
	})
}

// newSCIMOrganisation builds the organisation and mints its first SCIM token.
func newSCIMOrganisation(t *testing.T) *scimOrganisation {
	t.Helper()
	f := newFixture(t, nil)
	ada := f.person(t, "ada@example.com")
	carol := f.person(t, "carol@example.com")
	org := f.organisation(t, "acme", map[string]auth.Role{ada: auth.RoleOwner, carol: auth.RoleDeveloper})
	s := &scimOrganisation{fixture: f, org: org, owner: f.principalOf(t, "ada@example.com", org), ctx: scimLicensed(), dir: f.svc.SCIMDirectory()}
	_, secret, err := f.svc.CreateScimToken(s.ctx, s.owner, "okta")
	if err != nil {
		t.Fatalf("CreateScimToken: %v", err)
	}
	s.token = secret
	return s
}

// scimUser is the directory's view of an email address, created through SCIM.
func (s *scimOrganisation) scimUser(t *testing.T, email string) identity.User {
	t.Helper()
	u, err := s.dir.CreateUser(s.ctx, s.org, identity.UserSpec{UserName: email, DisplayName: "Person", Active: true})
	if err != nil {
		t.Fatalf("CreateUser(%s): %v", email, err)
	}
	return u
}

// userIDOf is the account identifier of an email address.
func (s *scimOrganisation) userIDOf(t *testing.T, email string) string {
	t.Helper()
	var id string
	if err := s.db.Pool().QueryRow(context.Background(), `SELECT id::text FROM users WHERE email = $1`, email).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// membershipCount counts the rows of the organisation's memberships that
// match a condition on the user or the team.
func (s *scimOrganisation) membershipCount(t *testing.T, where string, args ...any) int {
	t.Helper()
	var n int
	scanIn(t, s.fixture, storage.Tenant{OrganizationID: s.org}, `SELECT count(*) FROM memberships WHERE organization_id = $1 AND `+where,
		[]any{&n}, append([]any{s.org}, args...)...)
	return n
}

// Verifies: GOV-004, SEC-092.
// A SCIM token is stored as a hash only, is accepted by its organisation's
// directory, and is refused when it is unknown or malformed. Only an
// organisation's administrator may mint one, and only with the licence.
func TestSCIMTokenStoredAsAHash_GOV_004(t *testing.T) {
	t.Parallel()
	s := newSCIMOrganisation(t)
	tokens, err := s.svc.ListScimTokens(s.ctx, s.owner, storage.Cursor{}, 10)
	if err != nil || len(tokens) != 1 {
		t.Fatalf("ListScimTokens = %+v, %v", tokens, err)
	}
	if strings.Contains(fmt.Sprintf("%+v", tokens), s.token) {
		t.Error("the token listing carries the secret")
	}
	var hash []byte
	var row string
	scanIn(t, s.fixture, storage.Tenant{OrganizationID: s.org}, `SELECT secret_hash, row_to_json(t)::text FROM scim_tokens t WHERE id = $1`,
		[]any{&hash, &row}, tokens[0].ID)
	if !bytes.Equal(hash, auth.HashSecret(s.token)) {
		t.Error("the stored hash is not the hash of the token")
	}
	if strings.Contains(row, s.token) {
		t.Error("the token's row carries the secret")
	}
	if org, err := s.dir.Authenticate(context.Background(), s.token); err != nil || org != s.org {
		t.Errorf("Authenticate = %q, %v; want the organisation", org, err)
	}
	for _, bad := range []string{"", "not-a-token", auth.PrefixSCIM + "_forged"} {
		if _, err := s.dir.Authenticate(context.Background(), bad); !errors.Is(err, identity.ErrUnauthorized) {
			t.Errorf("Authenticate(%q) = %v; want ErrUnauthorized", bad, err)
		}
	}
	bob := s.principalOf(t, "carol@example.com", s.org)
	if _, _, err := s.svc.CreateScimToken(s.ctx, bob, "rogue"); code(err) != plxerr.PermissionDenied {
		t.Errorf("a developer minted a SCIM token: %v", err)
	}
	if _, _, err := s.svc.CreateScimToken(context.Background(), s.owner, "unlicensed"); code(err) != plxerr.EnterpriseFeatureNotLicensed {
		t.Errorf("a SCIM token without the licence: %v", err)
	}
}

// endTokenNow moves a token's stored end into the past, as the end of its
// overlap would. Authentication compares the end with the database's clock,
// which the test's injected clock does not move, so the test cannot wait for
// it.
func (s *scimOrganisation) endTokenNow(t *testing.T, id string) {
	t.Helper()
	if err := s.db.InTx(context.Background(), storage.Tenant{OrganizationID: s.org}, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE scim_tokens SET expires_at = now() - interval '1 second' WHERE id = $1`, id)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

// Verifies: GOV-004, SEC-092.
// Rotating a SCIM token keeps the old one working for the overlap, bounded by
// auth.scimTokenOverlapSeconds, and then refuses it. A revoked token is
// refused at once.
func TestSCIMTokenRotation_GOV_004(t *testing.T) {
	t.Parallel()
	s := newSCIMOrganisation(t)
	ctx := context.Background()
	tokens, err := s.svc.ListScimTokens(s.ctx, s.owner, storage.Cursor{}, 10)
	if err != nil || len(tokens) != 1 {
		t.Fatalf("ListScimTokens = %+v, %v", tokens, err)
	}
	bound := time.Duration(limits.Defaults().Get(limits.AuthScimTokenOverlapSeconds)) * time.Second
	next, nextSecret, previous, err := s.svc.RotateScimToken(s.ctx, s.owner, tokens[0].ID, 0)
	if err != nil || nextSecret == "" || next.ID == tokens[0].ID {
		t.Fatalf("RotateScimToken = %+v, %v", next, err)
	}
	if want := s.clock.now().Add(bound); !previous.ExpiresAt.Equal(want) {
		t.Errorf("the old token ends at %s; want the overlap bound %s", previous.ExpiresAt, want)
	}
	for name, secret := range map[string]string{"the old token": s.token, "the new token": nextSecret} {
		if _, err := s.dir.Authenticate(ctx, secret); err != nil {
			t.Errorf("during the overlap, %s: %v", name, err)
		}
	}
	s.endTokenNow(t, previous.ID)
	if _, err := s.dir.Authenticate(ctx, s.token); !errors.Is(err, identity.ErrUnauthorized) {
		t.Errorf("the old token accepted after the overlap: %v", err)
	}
	if _, err := s.dir.Authenticate(ctx, nextSecret); err != nil {
		t.Errorf("the new token refused: %v", err)
	}
	if err := s.svc.RevokeScimToken(s.ctx, s.owner, next.ID); err != nil {
		t.Fatalf("RevokeScimToken: %v", err)
	}
	if _, err := s.dir.Authenticate(ctx, nextSecret); !errors.Is(err, identity.ErrUnauthorized) {
		t.Errorf("a revoked token accepted: %v", err)
	}
	if err := s.svc.RevokeScimToken(s.ctx, s.owner, next.ID); code(err) != plxerr.ResourceNotFound {
		t.Errorf("a token revoked twice: %v", err)
	}
	if _, _, _, err := s.svc.RotateScimToken(s.ctx, s.owner, next.ID, 0); code(err) != plxerr.ResourceNotFound {
		t.Errorf("a revoked token rotated: %v", err)
	}
}

// Verifies: GOV-004, SEC-092.
// A directory creates, changes and deletes users, and refuses what it must:
// a duplicate, a malformed address, a stale version, a change of address to an
// account SCIM did not create, and another organisation's users.
func TestSCIMUsers_GOV_004(t *testing.T) {
	t.Parallel()
	s := newSCIMOrganisation(t)
	ctx := s.ctx
	eve, err := s.dir.CreateUser(ctx, s.org, identity.UserSpec{ExternalID: "ext-eve", UserName: "Eve@Example.com", DisplayName: "Eve", Active: true})
	if err != nil || !strings.EqualFold(eve.UserName, "eve@example.com") || !eve.Active {
		t.Fatalf("CreateUser = %+v, %v", eve, err)
	}
	if _, err := s.dir.CreateUser(ctx, s.org, identity.UserSpec{UserName: "eve@example.com", Active: true}); !errors.Is(err, identity.ErrConflict) {
		t.Errorf("a duplicate user: %v", err)
	}
	if _, err := s.dir.CreateUser(ctx, s.org, identity.UserSpec{UserName: "not-an-address", Active: true}); !errors.Is(err, identity.ErrInvalid) {
		t.Errorf("a malformed address: %v", err)
	}
	if _, err := s.dir.CreateUser(context.Background(), s.org, identity.UserSpec{UserName: "frank@example.com", Active: true}); code(err) != plxerr.EnterpriseFeatureNotLicensed {
		t.Errorf("a user without the licence: %v", err)
	}
	spec := identity.UserSpec{ExternalID: "ext-eve", UserName: "eve@example.com", DisplayName: "Eve Two", Active: true}
	if _, err := s.dir.ReplaceUser(ctx, s.org, eve.ID, spec, eve.Version+5); !errors.Is(err, identity.ErrVersionMismatch) {
		t.Errorf("a stale version: %v", err)
	}
	updated, err := s.dir.ReplaceUser(ctx, s.org, eve.ID, spec, eve.Version)
	if err != nil || updated.DisplayName != "Eve Two" || updated.Version <= eve.Version {
		t.Errorf("ReplaceUser = %+v, %v", updated, err)
	}
	carol := s.scimUser(t, "carol@example.com")
	if _, err := s.dir.ReplaceUser(ctx, s.org, carol.ID, identity.UserSpec{UserName: "elsewhere@example.com", Active: true}, 0); !errors.Is(err, identity.ErrInvalid) {
		t.Errorf("an account the directory did not create changed address: %v", err)
	}
	other := s.organisation(t, "other", map[string]auth.Role{s.userIDOf(t, "ada@example.com"): auth.RoleOwner})
	if _, err := s.dir.GetUser(ctx, other, eve.ID); !errors.Is(err, identity.ErrNotFound) {
		t.Errorf("another organisation reads a user: %v", err)
	}
	if page, err := s.dir.ListUsers(ctx, s.org, identity.ListQuery{Count: 10}); err != nil || page.Total != 2 {
		t.Errorf("ListUsers = %+v, %v; want the two users", page, err)
	}
	if err := s.dir.DeleteUser(ctx, s.org, eve.ID); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	if _, err := s.dir.GetUser(ctx, s.org, eve.ID); !errors.Is(err, identity.ErrNotFound) {
		t.Errorf("a deleted user is still read: %v", err)
	}
}

// Verifies: GOV-004, SEC-100.
// Deprovisioning a person ends their access at once: their sessions and their
// access tokens stop authenticating, their sign-in is refused, and their
// membership is removed. Reactivating them restores sign-in. The last owner
// of an organisation cannot be deprovisioned.
func TestDeprovisioningEndsAccessAtOnce_GOV_004(t *testing.T) {
	t.Parallel()
	s := newSCIMOrganisation(t)
	ctx := context.Background()
	carol := s.scimUser(t, "carol@example.com")
	session, _, err := s.svc.StartPasswordLogin(ctx, "carol@example.com", password)
	if err != nil {
		t.Fatalf("StartPasswordLogin: %v", err)
	}
	id, err := s.svc.AuthenticateSession(ctx, session.Secret)
	if err != nil {
		t.Fatalf("AuthenticateSession: %v", err)
	}
	_, id = s.enrol(t, id)
	minted, err := s.svc.CreateAccessToken(ctx, mustResolve(t, s, id), "ci", []string{string(auth.PluginRead)}, time.Hour)
	if err != nil {
		t.Fatalf("CreateAccessToken: %v", err)
	}
	if _, err := s.svc.AuthenticateToken(ctx, minted.Secret); err != nil {
		t.Fatalf("the token before deprovisioning: %v", err)
	}

	if _, err := s.dir.ReplaceUser(s.ctx, s.org, carol.ID, identity.UserSpec{UserName: "carol@example.com", Active: false}, 0); err != nil {
		t.Fatalf("ReplaceUser(inactive): %v", err)
	}
	if _, err := s.svc.AuthenticateSession(ctx, session.Secret); err == nil {
		t.Error("a session of a deprovisioned person still authenticates")
	}
	if _, err := s.svc.AuthenticateToken(ctx, minted.Secret); err == nil {
		t.Error("a token of a deprovisioned person still authenticates")
	}
	if _, _, err := s.svc.StartPasswordLogin(ctx, "carol@example.com", password); err == nil {
		t.Error("a deprovisioned person signs in")
	}
	if n := s.membershipCount(t, `user_id = $2`, carol.ID); n != 0 {
		t.Errorf("a deprovisioned person keeps %d memberships", n)
	}
	if u, err := s.dir.GetUser(ctx, s.org, carol.ID); err != nil || u.Active {
		t.Errorf("GetUser after deprovisioning = %+v, %v", u, err)
	}

	if _, err := s.dir.ReplaceUser(s.ctx, s.org, carol.ID, identity.UserSpec{UserName: "carol@example.com", Active: true}, 0); err != nil {
		t.Fatalf("ReplaceUser(active): %v", err)
	}
	if _, _, err := s.svc.StartPasswordLogin(ctx, "carol@example.com", password); err != nil {
		t.Errorf("a reactivated person cannot sign in: %v", err)
	}

	ada := s.scimUser(t, "ada@example.com")
	if _, err := s.dir.ReplaceUser(s.ctx, s.org, ada.ID, identity.UserSpec{UserName: "ada@example.com", Active: false}, 0); !errors.Is(err, identity.ErrRefused) {
		t.Errorf("the last owner was deprovisioned: %v", err)
	}
}

// mustResolve resolves the principal of an identity in the organisation.
func mustResolve(t *testing.T, s *scimOrganisation, id auth.Identity) auth.Principal {
	t.Helper()
	p, err := s.svc.Resolve(context.Background(), id, s.org)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	return p
}

// Verifies: GOV-004.
// SCIM groups are teams. A group's members are the team's members with the
// team's member role, which grants no role; a replaced group keeps exactly the
// members it is given; a group with an unknown member is refused; a deleted
// group is gone with its team.
func TestSCIMGroupsAreTeams_GOV_004(t *testing.T) {
	t.Parallel()
	s := newSCIMOrganisation(t)
	ctx := s.ctx
	carol := s.scimUser(t, "carol@example.com")
	eve := s.scimUser(t, "eve@example.com")
	group, err := s.dir.CreateGroup(ctx, s.org, identity.GroupSpec{ExternalID: "grp-web", DisplayName: "Web Engineers", MemberIDs: []string{carol.ID, eve.ID}})
	if err != nil || len(group.Members) != 2 {
		t.Fatalf("CreateGroup = %+v, %v", group, err)
	}
	if n := s.membershipCount(t, `team_id = $2 AND role = 'member'`, group.ID); n != 2 {
		t.Errorf("the team has %d member rows with the member role; want 2", n)
	}
	var grants int
	scanIn(t, s.fixture, storage.Tenant{OrganizationID: s.org}, `SELECT count(*) FROM role_grants WHERE user_id = ANY($1::uuid[])`,
		[]any{&grants}, []string{carol.ID, eve.ID})
	if grants != 0 {
		t.Errorf("a SCIM group granted %d roles; SCIM never grants a role", grants)
	}
	if _, err := s.dir.CreateGroup(ctx, s.org, identity.GroupSpec{DisplayName: "Ghosts", MemberIDs: []string{s.userIDOf(t, "ada@example.com"), "not-a-user"}}); !errors.Is(err, identity.ErrInvalid) {
		t.Errorf("a group with an unknown member: %v", err)
	}
	if _, err := s.dir.ReplaceGroup(ctx, s.org, group.ID, identity.GroupSpec{DisplayName: "Web Engineers", MemberIDs: []string{eve.ID}}, group.Version+5); !errors.Is(err, identity.ErrVersionMismatch) {
		t.Errorf("a stale group version: %v", err)
	}
	replaced, err := s.dir.ReplaceGroup(ctx, s.org, group.ID, identity.GroupSpec{DisplayName: "Web Engineers", MemberIDs: []string{eve.ID}}, group.Version)
	if err != nil || len(replaced.Members) != 1 || replaced.Members[0].UserID != eve.ID {
		t.Errorf("ReplaceGroup = %+v, %v; want only eve", replaced, err)
	}
	if n := s.membershipCount(t, `team_id = $2`, group.ID); n != 1 {
		t.Errorf("the team keeps %d members after the replacement; want 1", n)
	}
	second, err := s.dir.CreateGroup(ctx, s.org, identity.GroupSpec{DisplayName: "Web Engineers"})
	if err != nil || second.ID == group.ID {
		t.Errorf("a second group of the same name = %+v, %v", second, err)
	}
	if err := s.dir.DeleteGroup(ctx, s.org, group.ID); err != nil {
		t.Fatalf("DeleteGroup: %v", err)
	}
	if _, err := s.dir.GetGroup(ctx, s.org, group.ID); !errors.Is(err, identity.ErrNotFound) {
		t.Errorf("a deleted group is still read: %v", err)
	}
	if n := s.membershipCount(t, `team_id = $2`, group.ID); n != 0 {
		t.Errorf("a deleted group's team keeps %d members", n)
	}
}
