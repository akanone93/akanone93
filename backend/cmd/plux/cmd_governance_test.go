// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
)

// fakeGovernance records what the CLI asked and answers from memory.
type fakeGovernance struct {
	pluxv1connect.UnimplementedGovernanceServiceHandler
	pluxv1connect.UnimplementedOrgServiceHandler
	calls *[]string
}

func (f fakeGovernance) record(s string) { *f.calls = append(*f.calls, s) }

func (fakeGovernance) ListRoles(_ context.Context, r *connect.Request[pluxv1.ListRolesRequest]) (*connect.Response[pluxv1.ListRolesResponse], error) {
	if r.Header().Get("X-Plux-Organization") != "org-1" || r.Header().Get("Authorization") != "Bearer plux_pat_gov" {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}
	if r.Msg.GetPage().GetPageToken() == "" {
		return connect.NewResponse(&pluxv1.ListRolesResponse{
			Roles: []*pluxv1.RoleDefinition{{Key: "owner", Name: "Owner", Predefined: true, Permissions: []string{"org.manage"}}},
			Page:  &pluxv1.PageResult{NextPageToken: "p2"},
		}), nil
	}
	return connect.NewResponse(&pluxv1.ListRolesResponse{Roles: []*pluxv1.RoleDefinition{{Key: "log-reader", Name: "Log reader"}}}), nil
}

func (fakeGovernance) GetRole(_ context.Context, r *connect.Request[pluxv1.GetRoleRequest]) (*connect.Response[pluxv1.GetRoleResponse], error) {
	return connect.NewResponse(&pluxv1.GetRoleResponse{Role: &pluxv1.RoleDefinition{Key: r.Msg.GetKey(), Permissions: []string{"audit.read", "app.read"}}}), nil
}

func (f fakeGovernance) CreateRole(_ context.Context, r *connect.Request[pluxv1.CreateRoleRequest]) (*connect.Response[pluxv1.CreateRoleResponse], error) {
	f.record("create " + r.Msg.GetKey() + " " + strings.Join(r.Msg.GetPermissions(), "+"))
	if r.Msg.GetKey() == "refused" {
		return nil, connect.NewError(connect.CodeFailedPrecondition, nil)
	}
	return connect.NewResponse(&pluxv1.CreateRoleResponse{Role: &pluxv1.RoleDefinition{Key: r.Msg.GetKey(), Name: r.Msg.GetName(), Permissions: r.Msg.GetPermissions()}}), nil
}

func (f fakeGovernance) UpdateRole(_ context.Context, r *connect.Request[pluxv1.UpdateRoleRequest]) (*connect.Response[pluxv1.UpdateRoleResponse], error) {
	f.record("update " + r.Msg.GetKey() + " " + r.Msg.GetName())
	return connect.NewResponse(&pluxv1.UpdateRoleResponse{Role: &pluxv1.RoleDefinition{Key: r.Msg.GetKey()}}), nil
}

func (f fakeGovernance) DeleteRole(_ context.Context, r *connect.Request[pluxv1.DeleteRoleRequest]) (*connect.Response[pluxv1.DeleteRoleResponse], error) {
	f.record("delete role " + r.Msg.GetKey())
	return connect.NewResponse(&pluxv1.DeleteRoleResponse{}), nil
}

func (f fakeGovernance) ListGrants(_ context.Context, r *connect.Request[pluxv1.ListGrantsRequest]) (*connect.Response[pluxv1.ListGrantsResponse], error) {
	f.record("list grants app=" + r.Msg.GetAppId())
	return connect.NewResponse(&pluxv1.ListGrantsResponse{Grants: []*pluxv1.RoleGrant{
		{Id: "g1", UserId: "u1", Role: "owner", Scope: "organization"},
		{Id: "g2", TeamId: "t1", Role: "developer", Scope: "environment", AppId: "a1", EnvironmentId: "e1"},
	}}), nil
}

func (f fakeGovernance) CreateGrant(_ context.Context, r *connect.Request[pluxv1.CreateGrantRequest]) (*connect.Response[pluxv1.CreateGrantResponse], error) {
	m := r.Msg
	f.record("grant " + m.GetTeamId() + " " + m.GetRole() + " " + m.GetScope() + " " + m.GetAppId())
	return connect.NewResponse(&pluxv1.CreateGrantResponse{Grant: &pluxv1.RoleGrant{Id: "g3", TeamId: m.GetTeamId(), Role: m.GetRole(), Scope: m.GetScope(), AppId: m.GetAppId()}}), nil
}

func (f fakeGovernance) UpdateGrant(_ context.Context, r *connect.Request[pluxv1.UpdateGrantRequest]) (*connect.Response[pluxv1.UpdateGrantResponse], error) {
	f.record("regrant " + r.Msg.GetId() + " " + r.Msg.GetRole())
	return connect.NewResponse(&pluxv1.UpdateGrantResponse{Grant: &pluxv1.RoleGrant{Id: r.Msg.GetId(), Role: r.Msg.GetRole()}}), nil
}

func (f fakeGovernance) DeleteGrant(_ context.Context, r *connect.Request[pluxv1.DeleteGrantRequest]) (*connect.Response[pluxv1.DeleteGrantResponse], error) {
	f.record("revoke " + r.Msg.GetId())
	return connect.NewResponse(&pluxv1.DeleteGrantResponse{}), nil
}

func (f fakeGovernance) ListTeams(_ context.Context, r *connect.Request[pluxv1.ListTeamsRequest]) (*connect.Response[pluxv1.ListTeamsResponse], error) {
	f.record("list teams " + r.Msg.GetOrganizationId())
	return connect.NewResponse(&pluxv1.ListTeamsResponse{Teams: []*pluxv1.Team{{Id: "t1", Key: "web", Name: "Web"}}}), nil
}

func (f fakeGovernance) CreateTeam(_ context.Context, r *connect.Request[pluxv1.CreateTeamRequest]) (*connect.Response[pluxv1.CreateTeamResponse], error) {
	f.record("team " + r.Msg.GetKey() + " " + r.Msg.GetName())
	return connect.NewResponse(&pluxv1.CreateTeamResponse{Team: &pluxv1.Team{Id: "t2", Key: r.Msg.GetKey(), Name: r.Msg.GetName()}}), nil
}

func (f fakeGovernance) DeleteTeam(_ context.Context, r *connect.Request[pluxv1.DeleteTeamRequest]) (*connect.Response[pluxv1.DeleteTeamResponse], error) {
	f.record("delete team " + r.Msg.GetId())
	return connect.NewResponse(&pluxv1.DeleteTeamResponse{}), nil
}

func (f fakeGovernance) AddMember(_ context.Context, r *connect.Request[pluxv1.AddMemberRequest]) (*connect.Response[pluxv1.AddMemberResponse], error) {
	f.record("add " + r.Msg.GetTeamId() + " " + r.Msg.GetEmail())
	res := &pluxv1.AddMemberResponse{Member: &pluxv1.Member{UserId: "u9", TeamId: r.Msg.GetTeamId()}}
	if r.Msg.GetEmail() == "new@example.com" {
		res.Invitation = "inv-123"
	}
	return connect.NewResponse(res), nil
}

func (f fakeGovernance) RemoveMember(_ context.Context, r *connect.Request[pluxv1.RemoveMemberRequest]) (*connect.Response[pluxv1.RemoveMemberResponse], error) {
	f.record("remove " + r.Msg.GetTeamId() + " " + r.Msg.GetUserId())
	return connect.NewResponse(&pluxv1.RemoveMemberResponse{}), nil
}

// Verifies: GOV-002, GOV-003.
// plux roles, grants and teams call the governance and organisation
// services with the caller's credential and organisation, follow pages, and
// print text or JSON.
func TestGovernanceCommands_GOV_002(t *testing.T) { //nolint:paralleltest // the environment is process-wide
	t.Setenv(tokenEnv, "plux_pat_gov")
	t.Setenv(serverEnv, "")
	t.Setenv(orgEnv, "")
	var calls []string
	fake := fakeGovernance{calls: &calls}
	mux := http.NewServeMux()
	mux.Handle(pluxv1connect.NewGovernanceServiceHandler(fake))
	mux.Handle(pluxv1connect.NewOrgServiceHandler(fake))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	run := func(cmd func(env, []string) int, args ...string) (int, string, string) {
		var out, errOut bytes.Buffer
		e := newEnv(&out, &errOut)
		e.creds = credentials{dir: t.TempDir()}
		if len(args) > 0 {
			args = append([]string{args[0], "--server", srv.URL, "--org", "org-1"}, args[1:]...)
		}
		code := cmd(e, args)
		return code, out.String(), errOut.String()
	}

	code, out, stderr := run(env.roles, "list")
	if code != exitOK || !strings.Contains(out, "owner\tOwner\tpredefined\t1 permissions") || !strings.Contains(out, "log-reader\tLog reader\tcustom") {
		t.Errorf("roles list: %d %q %s", code, out, stderr)
	}
	code, out, _ = run(env.roles, "list", "--json")
	var listed struct{ Roles []map[string]any }
	if err := json.Unmarshal([]byte(out), &listed); code != exitOK || err != nil || len(listed.Roles) != 2 {
		t.Errorf("roles list --json: %d %v %s", code, err, out)
	}
	if code, out, _ = run(env.roles, "show", "auditor"); code != exitOK || !strings.Contains(out, "audit.read\napp.read") {
		t.Errorf("roles show: %d %q", code, out)
	}
	if code, out, _ = run(env.roles, "create", "--name", "Log reader", "--permissions", "audit.read, app.read,", "log-reader"); code != exitOK || !strings.Contains(out, "log-reader") {
		t.Errorf("roles create: %d %q", code, out)
	}
	if code, _, _ = run(env.roles, "create", "--name", "x", "refused"); code != exitFailed {
		t.Errorf("a refused create: %d", code)
	}
	if code, _, _ = run(env.roles, "update", "--name", "Reader", "log-reader"); code != exitOK {
		t.Errorf("roles update: %d", code)
	}
	if code, out, _ = run(env.roles, "delete", "log-reader"); code != exitOK || !strings.Contains(out, "Deleted role log-reader") {
		t.Errorf("roles delete: %d %q", code, out)
	}

	if code, out, _ = run(env.grants, "list", "--app", "a1"); code != exitOK || !strings.Contains(out, "g1\tuser u1\towner\torganization") || !strings.Contains(out, "g2\tteam t1\tdeveloper\tenvironment a1/e1") {
		t.Errorf("grants list: %d %q", code, out)
	}
	if code, out, _ = run(env.grants, "create", "--team", "t1", "--role", "developer", "--scope", "app", "--app", "a1"); code != exitOK || !strings.Contains(out, "g3\tteam t1\tdeveloper\tapp a1") {
		t.Errorf("grants create: %d %q", code, out)
	}
	if code, _, _ = run(env.grants, "update", "--role", "viewer", "g3"); code != exitOK {
		t.Errorf("grants update: %d", code)
	}
	if code, _, _ = run(env.grants, "delete", "g3"); code != exitOK {
		t.Errorf("grants delete: %d", code)
	}

	if code, out, _ = run(env.teams, "list"); code != exitOK || !strings.Contains(out, "t1\tweb\tWeb") {
		t.Errorf("teams list: %d %q", code, out)
	}
	if code, out, _ = run(env.teams, "create", "--name", "Mobile", "mobile"); code != exitOK || !strings.Contains(out, "Created team mobile (t2)") {
		t.Errorf("teams create: %d %q", code, out)
	}
	if code, out, _ = run(env.teams, "add", "--team", "t2", "ada@example.com"); code != exitOK || strings.Contains(out, "Invitation") {
		t.Errorf("teams add, an existing account: %d %q", code, out)
	}
	if code, out, _ = run(env.teams, "add", "--team", "t2", "new@example.com"); code != exitOK || !strings.Contains(out, "inv-123") {
		t.Errorf("teams add, a new account shows its invitation: %d %q", code, out)
	}
	if code, _, _ = run(env.teams, "remove", "--team", "t2", "u9"); code != exitOK {
		t.Errorf("teams remove: %d", code)
	}
	if code, _, _ = run(env.teams, "delete", "t2"); code != exitOK {
		t.Errorf("teams delete: %d", code)
	}

	want := []string{
		"create log-reader audit.read+app.read", "create refused ", "update log-reader Reader", "delete role log-reader",
		"list grants app=a1", "grant t1 developer app a1", "regrant g3 viewer", "revoke g3",
		"list teams org-1", "team mobile Mobile", "add t2 ada@example.com", "add t2 new@example.com", "remove t2 u9", "delete team t2",
	}
	if !slices.Equal(calls, want) {
		t.Errorf("calls = %q\nwant    %q", calls, want)
	}

	for name, c := range map[string]struct {
		cmd  func(env, []string) int
		args []string
	}{
		"roles without a subcommand":  {env.roles, nil},
		"grants without a subcommand": {env.grants, nil},
		"teams without a subcommand":  {env.teams, nil},
		"an unknown subcommand":       {env.roles, []string{"rename"}},
		"a missing key":               {env.roles, []string{"show"}},
		"a bad server":                {env.teams, []string{"list", "--server", "ftp://x"}},
	} {
		var errOut bytes.Buffer
		e := newEnv(&bytes.Buffer{}, &errOut)
		if code := c.cmd(e, c.args); code != exitUsage {
			t.Errorf("%s: %d %s", name, code, errOut.String())
		}
	}
	t.Setenv(tokenEnv, "")
	var errOut bytes.Buffer
	e := newEnv(&bytes.Buffer{}, &errOut)
	e.creds = credentials{dir: t.TempDir()}
	if code := e.grants([]string{"list", "--server", srv.URL}); code != exitAuth {
		t.Errorf("signed out: %d %s", code, errOut.String())
	}
}
