// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api_test

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
)

// scopedMember adds a person as a viewer, signs them in with a second
// factor, and returns the caller with their user identifier.
func (f *matrixFixture) scopedMember(t *testing.T) (caller, string) {
	t.Helper()
	ctx := context.Background()
	email := f.name() + "@example.com"
	added := must(f.w.org.AddMember(ctx, req(f.owner(), &pluxv1.AddMemberRequest{Email: email, Role: "viewer"})))(t)
	must(f.w.identity.AcceptInvitation(ctx, connect.NewRequest(&pluxv1.AcceptInvitationRequest{Invitation: added.GetInvitation(), DisplayName: "scoped", Password: "correct horse battery"})))(t)
	c := f.w.signIn(t, email)
	c.org = f.org
	f.w.withSecondFactor(t, c)
	return c, added.GetMember().GetUserId()
}

// Verifies: GOV-002, GOV-003, QA-008.
// Across the edge, a grant limited to one environment lets a person publish
// and promote there and refuses them on the other environment; a grant of a
// custom role written release.publish:production does the reverse.
func TestScopedGrantsAtTheEdge_QA_008(t *testing.T) {
	f := newMatrixFixture(t)
	ctx := context.Background()
	publish := func(c caller, env string) connect.Code {
		return f.w.post(t, c, "/plux.v1.PublishService/Publish", &pluxv1.PublishRequest{AppId: f.app, PluginId: f.plugin, EnvironmentId: env})
	}
	promote := func(c caller, env string) connect.Code {
		return f.w.post(t, c, "/plux.v1.ReleaseService/PromoteRelease", &pluxv1.PromoteReleaseRequest{AppId: f.app, Sequence: 1, EnvironmentId: env, ChannelKey: f.channel})
	}

	devOnly, devOnlyID := f.scopedMember(t)
	if c := publish(devOnly, f.dev); c != connect.CodePermissionDenied {
		t.Fatalf("a viewer published to development: %v", c)
	}
	grant := must(f.w.govern.CreateGrant(ctx, req(f.owner(), &pluxv1.CreateGrantRequest{
		UserId: devOnlyID, Role: string(auth.RoleReleaseManager), Scope: "environment", AppId: f.app, EnvironmentId: f.dev,
	})))(t).GetGrant()
	if grant.GetScope() != "environment" || grant.GetEnvironmentId() != f.dev {
		t.Errorf("grant = %v", grant)
	}
	for name, got := range map[string]connect.Code{
		"publish to development": publish(devOnly, f.dev),
		"promote to development": promote(devOnly, f.dev),
	} {
		if got == connect.CodePermissionDenied || got == connect.CodeUnauthenticated {
			t.Errorf("%s with a grant on development: %v", name, got)
		}
	}
	for name, got := range map[string]connect.Code{
		"publish to production": publish(devOnly, f.prod),
		"promote to production": promote(devOnly, f.prod),
	} {
		if got != connect.CodePermissionDenied {
			t.Errorf("%s with a grant on development: %v, want PermissionDenied", name, got)
		}
	}

	// Revoking the grant ends the access at the next call.
	must(f.w.govern.DeleteGrant(ctx, req(f.owner(), &pluxv1.DeleteGrantRequest{Id: grant.GetId()})))(t)
	if c := publish(devOnly, f.dev); c != connect.CodePermissionDenied {
		t.Errorf("publish to development after the revocation: %v", c)
	}

	// A custom role scoped by a permission string.
	prodOnly, prodOnlyID := f.scopedMember(t)
	must(f.w.govern.CreateRole(ctx, req(f.owner(), &pluxv1.CreateRoleRequest{
		Key: "prod-publisher", Name: "Production publisher", Permissions: []string{"app.read", "release.publish:production"},
	})))(t)
	must(f.w.govern.CreateGrant(ctx, req(f.owner(), &pluxv1.CreateGrantRequest{UserId: prodOnlyID, Role: "prod-publisher", Scope: "organization"})))(t)
	if c := publish(prodOnly, f.prod); c == connect.CodePermissionDenied {
		t.Errorf("publish to production with release.publish:production: %v", c)
	}
	if c := publish(prodOnly, f.dev); c != connect.CodePermissionDenied {
		t.Errorf("publish to development with release.publish:production: %v, want PermissionDenied", c)
	}

	// The error codes of the registry reach the caller.
	_, err := f.w.govern.CreateRole(ctx, req(f.owner(), &pluxv1.CreateRoleRequest{Key: "ghosts", Name: "Ghosts", Permissions: []string{"ghosts.manage"}}))
	if codeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("an unknown permission: %v", err)
	}
	_, err = f.w.govern.GetRole(ctx, req(f.owner(), &pluxv1.GetRoleRequest{Key: "ghosts"}))
	if codeOf(err) != connect.CodeNotFound {
		t.Errorf("an unknown role: %v", err)
	}
	_, err = f.w.govern.CreateGrant(ctx, req(f.owner(), &pluxv1.CreateGrantRequest{TeamId: f.team, Role: "viewer", Scope: "app"}))
	if codeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("a bad scope: %v", err)
	}
	perms := must(f.w.govern.ListPermissions(ctx, req(f.owner(), &pluxv1.ListPermissionsRequest{})))(t).GetPermissions()
	if len(perms) != len(auth.Catalogue()) || perms[0].GetName() != "a11y.waive" {
		t.Errorf("ListPermissions returned %d entries", len(perms))
	}
}
