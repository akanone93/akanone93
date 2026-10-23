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

// fakeRestrictions records what the CLI asked and answers from memory.
type fakeRestrictions struct {
	pluxv1connect.UnimplementedGovernanceServiceHandler
	calls *[]string
}

func (f fakeRestrictions) record(s string) { *f.calls = append(*f.calls, s) }

func (f fakeRestrictions) CreateRestriction(_ context.Context, r *connect.Request[pluxv1.CreateRestrictionRequest]) (*connect.Response[pluxv1.CreateRestrictionResponse], error) {
	m := r.Msg
	f.record("create " + m.GetSubjectKind() + " " + m.GetSubjectId() + " " + strings.Join(m.GetPatterns(), "+") + " " + m.GetAppId() + "/" + m.GetEnvironment() + " " + m.GetReason())
	if m.GetSubjectKind() == "refused" {
		return nil, connect.NewError(connect.CodeFailedPrecondition, nil)
	}
	return connect.NewResponse(&pluxv1.CreateRestrictionResponse{Restriction: &pluxv1.Restriction{
		Id: "r3", SubjectKind: m.GetSubjectKind(), SubjectId: m.GetSubjectId(), Patterns: m.GetPatterns(),
		AppId: m.GetAppId(), Environment: m.GetEnvironment(), Reason: m.GetReason(),
	}}), nil
}

func (f fakeRestrictions) ListRestrictions(_ context.Context, r *connect.Request[pluxv1.ListRestrictionsRequest]) (*connect.Response[pluxv1.ListRestrictionsResponse], error) {
	f.record("list " + r.Msg.GetSubjectId() + " " + r.Msg.GetAppId() + " " + r.Msg.GetPage().GetPageToken())
	if r.Msg.GetPage().GetPageToken() == "" {
		return connect.NewResponse(&pluxv1.ListRestrictionsResponse{
			Restrictions: []*pluxv1.Restriction{{Id: "r1", SubjectKind: "organization", Patterns: []string{"app.delete"}}},
			Page:         &pluxv1.PageResult{NextPageToken: "p2"},
		}), nil
	}
	return connect.NewResponse(&pluxv1.ListRestrictionsResponse{Restrictions: []*pluxv1.Restriction{
		{Id: "r2", SubjectKind: "user", SubjectId: "u1", Patterns: []string{"release.*", "plugin.edit"}, AppId: "a1", Environment: "production"},
	}}), nil
}

func (f fakeRestrictions) DeleteRestriction(_ context.Context, r *connect.Request[pluxv1.DeleteRestrictionRequest]) (*connect.Response[pluxv1.DeleteRestrictionResponse], error) {
	f.record("delete " + r.Msg.GetId())
	return connect.NewResponse(&pluxv1.DeleteRestrictionResponse{}), nil
}

func (f fakeRestrictions) ExplainAccess(_ context.Context, r *connect.Request[pluxv1.ExplainAccessRequest]) (*connect.Response[pluxv1.ExplainAccessResponse], error) {
	m := r.Msg
	line := "explain " + m.GetUserId() + " " + m.GetPermission() + " " + m.GetAppId() + "/" + m.GetEnvironment()
	if g := m.GetProposedGrant(); g != nil {
		line += " grant " + g.GetRole() + " " + g.GetScope() + " user=" + g.GetUserId() + " team=" + g.GetTeamId()
	}
	if p := m.GetProposedRestriction(); p != nil {
		line += " restrict " + p.GetSubjectKind() + " " + p.GetSubjectId() + " " + strings.Join(p.GetPatterns(), "+")
	}
	f.record(line)
	if m.GetPermission() == "app.read" {
		return connect.NewResponse(&pluxv1.ExplainAccessResponse{Decision: "allow", AllowingGrantIds: []string{"g1"}}), nil
	}
	return connect.NewResponse(&pluxv1.ExplainAccessResponse{
		Decision: "deny", AllowingGrantIds: []string{"g1"}, RemovingRestrictionIds: []string{"r1"}, OwnerFloor: m.GetPermission() == "members.manage",
	}), nil
}

// Verifies: GOV-006, GOV-007.
// plux restrictions and plux access call the governance service with the
// caller's credential, follow pages, print text or JSON, turn the flags into
// the request, and refuse a call that is not well formed before making it.
func TestRestrictionAndAccessCommands_GOV_007(t *testing.T) { //nolint:paralleltest // the environment is process-wide
	t.Setenv(tokenEnv, "plux_pat_gov")
	t.Setenv(serverEnv, "")
	t.Setenv(orgEnv, "")
	var calls []string
	mux := http.NewServeMux()
	mux.Handle(pluxv1connect.NewGovernanceServiceHandler(fakeRestrictions{calls: &calls}))
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

	code, out, stderr := run(env.restrictions, "list", "--subject", "u1", "--app", "a1")
	if code != exitOK || !strings.Contains(out, "r1\torganization\tapp.delete\torganization") || !strings.Contains(out, "r2\tuser u1\trelease.*,plugin.edit\ta1/production") {
		t.Errorf("restrictions list: %d %q %s", code, out, stderr)
	}
	code, out, _ = run(env.restrictions, "list", "--json")
	var listed struct{ Restrictions []map[string]any }
	if err := json.Unmarshal([]byte(out), &listed); code != exitOK || err != nil || len(listed.Restrictions) != 2 {
		t.Errorf("restrictions list --json: %d %v %s", code, err, out)
	}
	code, out, _ = run(env.restrictions, "create", "--subject", "u1", "--patterns", "plugin.*, release.publish,", "--app", "a1", "--environment", "production", "--reason", "freeze")
	if code != exitOK || !strings.Contains(out, "r3\tuser u1\tplugin.*,release.publish\ta1/production") {
		t.Errorf("restrictions create: %d %q", code, out)
	}
	if code, _, _ = run(env.restrictions, "create", "--subject-kind", "refused", "--patterns", "*"); code != exitFailed {
		t.Errorf("a refused create: %d", code)
	}
	if code, out, _ = run(env.restrictions, "delete", "r3"); code != exitOK || !strings.Contains(out, "Deleted restriction r3") {
		t.Errorf("restrictions delete: %d %q", code, out)
	}

	code, out, _ = run(env.access, "explain", "--permission", "plugin.edit", "--user", "u1", "--app", "a1")
	if code != exitOK || !strings.HasPrefix(out, "deny\n") || !strings.Contains(out, "allowed by grant g1") || !strings.Contains(out, "removed by restriction r1") {
		t.Errorf("access explain: %d %q", code, out)
	}
	code, out, _ = run(env.access, "explain", "--permission", "app.read", "--json")
	var explained struct {
		Decision         string
		AllowingGrantIDs []string `json:"allowingGrantIds"`
		Removing         []string `json:"removingRestrictionIds"`
	}
	if err := json.Unmarshal([]byte(out), &explained); code != exitOK || err != nil || explained.Decision != "allow" || len(explained.Removing) != 0 || explained.AllowingGrantIDs[0] != "g1" {
		t.Errorf("access explain --json: %d %v %s", code, err, out)
	}
	if code, out, _ = run(env.access, "explain", "--permission", "members.manage"); code != exitOK || !strings.Contains(out, "owner floor") {
		t.Errorf("an explanation that the owner floor kept: %d %q", code, out)
	}
	run(env.access, "explain", "--permission", "plugin.edit", "--user", "u1", "--app", "a1", "--environment", "production", "--grant-role", "developer", "--grant-team", "t1")
	run(env.access, "explain", "--permission", "plugin.edit", "--user", "u1", "--app", "a1", "--grant-role", "developer")
	run(env.access, "explain", "--permission", "plugin.edit", "--user", "u1", "--restrict", "plugin.*")
	run(env.access, "explain", "--permission", "plugin.edit", "--user", "u1", "--restrict", "plugin.*", "--restrict-kind", "team", "--restrict-subject", "t1")

	want := []string{
		"list u1 a1 ", "list u1 a1 p2", "list   ", "list   p2",
		"create user u1 plugin.*+release.publish a1/production freeze", "create refused  * / ",
		"delete r3",
		"explain u1 plugin.edit a1/", "explain  app.read /", "explain  members.manage /",
		"explain u1 plugin.edit a1/production grant developer environment user= team=t1",
		"explain u1 plugin.edit a1/ grant developer app user=u1 team=",
		"explain u1 plugin.edit / restrict user u1 plugin.*",
		"explain u1 plugin.edit / restrict team t1 plugin.*",
	}
	if !slices.Equal(calls, want) {
		t.Errorf("calls = %q\nwant    %q", calls, want)
	}

	for name, c := range map[string]struct {
		cmd  func(env, []string) int
		args []string
	}{
		"restrictions without a subcommand": {env.restrictions, nil},
		"access without a subcommand":       {env.access, nil},
		"an unknown subcommand":             {env.access, []string{"simulate"}},
		"delete without an id":              {env.restrictions, []string{"delete"}},
		"explain without a permission":      {env.access, []string{"explain", "--server", srv.URL}},
		"a grant and a restriction":         {env.access, []string{"explain", "--server", srv.URL, "--permission", "app.read", "--grant-role", "viewer", "--restrict", "app.*"}},
	} {
		var errOut bytes.Buffer
		e := newEnv(&bytes.Buffer{}, &errOut)
		e.creds = credentials{dir: t.TempDir()}
		if code := c.cmd(e, c.args); code != exitUsage {
			t.Errorf("%s: %d %s", name, code, errOut.String())
		}
	}
}
