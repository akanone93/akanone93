// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"fmt"
	"strings"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
)

const (
	rolesUsage  = "Usage: plux roles list|show|create|update|delete [flags]\n"
	grantsUsage = "Usage: plux grants list|create|update|delete [flags]\n"
	teamsUsage  = "Usage: plux teams list|create|delete|add|remove [flags]\n"
)

// governanceCmd parses a governance command's flags: the common ones, the
// subcommand's usage and positional count, and extra flags. Without a
// subcommand it prints usage.
func (e env) governanceCmd(cmd, usage string, args []string, usages map[string]subUsage, extra func(*flag.FlagSet)) (common, []string, int, bool) {
	var c common
	if len(args) == 0 {
		_, _ = fmt.Fprint(e.stderr, usage)
		return c, nil, exitUsage, false
	}
	u, known := usages[args[0]]
	if !known {
		_, _ = fmt.Fprintf(e.stderr, "%s %s: unknown subcommand %q\n", name, cmd, args[0])
		return c, nil, exitUsage, false
	}
	set := flag.NewFlagSet(cmd+" "+args[0], flag.ContinueOnError)
	c.register(set, false)
	extra(set)
	rest, code, ok := parse(set, args[1:], e.stderr, u.usage, u.positional)
	if !ok {
		return c, nil, code, false
	}
	if err := c.resolve(); err != nil {
		return c, nil, e.fail(cmd, err), false
	}
	return c, rest, exitOK, true
}

// subUsage is a subcommand's usage line and its number of positional
// arguments.
type subUsage struct {
	usage      string
	positional int
}

// governanceClients connects to the governance and organisation services.
func (e env) governanceClients(c common) (pluxv1connect.GovernanceServiceClient, pluxv1connect.OrgServiceClient, error) {
	token, err := e.creds.load(c.server)
	if err != nil {
		return nil, nil, err
	}
	if token == "" {
		return nil, nil, authError("not signed in to " + c.server + ": run plux login or set " + tokenEnv)
	}
	opts := connect.WithInterceptors(headers{token: token, org: c.org})
	return pluxv1connect.NewGovernanceServiceClient(e.httpClient, c.server, opts),
		pluxv1connect.NewOrgServiceClient(e.httpClient, c.server, opts), nil
}

// eachPage calls fetch with successive page tokens, starting with none,
// until it returns no next token.
func eachPage(fetch func(token string) (string, error)) error {
	token := ""
	for {
		next, err := fetch(token)
		if err != nil || next == "" {
			return err
		}
		token = next
	}
}

// splitList splits a comma-separated flag value, dropping empty items.
func splitList(v string) []string {
	out := []string{}
	for _, s := range strings.Split(v, ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func roleView(r *pluxv1.RoleDefinition) map[string]any {
	return map[string]any{
		"key": r.GetKey(), "name": r.GetName(), "description": r.GetDescription(),
		"predefined": r.GetPredefined(), "permissions": nonNil(r.GetPermissions()),
	}
}

func roleLine(r *pluxv1.RoleDefinition) string {
	kind := "custom"
	if r.GetPredefined() {
		kind = "predefined"
	}
	return fmt.Sprintf("%s\t%s\t%s\t%d permissions\n", r.GetKey(), r.GetName(), kind, len(r.GetPermissions()))
}

// roles lists, shows, creates, updates and deletes roles (GOV-002). Custom
// roles are an enterprise feature; the server refuses them without a
// licence.
func (e env) roles(args []string) int {
	var roleName, description, permissions string
	c, rest, code, ok := e.governanceCmd("roles", rolesUsage, args, map[string]subUsage{
		"list":   {"Usage: plux roles list [--json]\n\nLists the predefined and custom roles of the organisation.", 0},
		"show":   {"Usage: plux roles show [--json] <key>\n\nShows a role and its permissions.", 1},
		"create": {"Usage: plux roles create --name text --permissions a,b [--description text] [--json] <key>\n\nCreates a custom role of permissions you hold (enterprise).", 1},
		"update": {"Usage: plux roles update --name text --permissions a,b [--description text] [--json] <key>\n\nReplaces a custom role's name, description and permissions (enterprise).", 1},
		"delete": {"Usage: plux roles delete <key>\n\nDeletes a custom role nobody is granted.", 1},
	}, func(set *flag.FlagSet) {
		set.StringVar(&roleName, "name", "", "the role's display `name`")
		set.StringVar(&description, "description", "", "the role's `description`")
		set.StringVar(&permissions, "permissions", "", "the role's permissions, comma-separated (`list`)")
	})
	if !ok {
		return code
	}
	gov, _, err := e.governanceClients(c)
	if err != nil {
		return e.fail("roles", err)
	}
	ctx := context.Background()
	switch args[0] {
	case "list":
		views := []map[string]any{}
		var text strings.Builder
		if err := eachPage(func(token string) (string, error) {
			res, err := gov.ListRoles(ctx, connect.NewRequest(&pluxv1.ListRolesRequest{Page: &pluxv1.Page{PageSize: 100, PageToken: token}}))
			if err != nil {
				return "", err //nolint:wrapcheck // e.fail names the command
			}
			for _, r := range res.Msg.GetRoles() {
				views = append(views, roleView(r))
				text.WriteString(roleLine(r))
			}
			return res.Msg.GetPage().GetNextPageToken(), nil
		}); err != nil {
			return e.fail("roles list", err)
		}
		return e.emit(c.json, map[string]any{"roles": views}, text.String())
	case "show":
		res, err := gov.GetRole(ctx, connect.NewRequest(&pluxv1.GetRoleRequest{Key: rest[0]}))
		if err != nil {
			return e.fail("roles show", err)
		}
		r := res.Msg.GetRole()
		return e.emit(c.json, roleView(r), roleLine(r)+strings.Join(r.GetPermissions(), "\n"))
	case "create":
		res, err := gov.CreateRole(ctx, connect.NewRequest(&pluxv1.CreateRoleRequest{Key: rest[0], Name: roleName, Description: description, Permissions: splitList(permissions)}))
		if err != nil {
			return e.fail("roles create", err)
		}
		return e.emit(c.json, roleView(res.Msg.GetRole()), roleLine(res.Msg.GetRole()))
	case "update":
		res, err := gov.UpdateRole(ctx, connect.NewRequest(&pluxv1.UpdateRoleRequest{Key: rest[0], Name: roleName, Description: description, Permissions: splitList(permissions)}))
		if err != nil {
			return e.fail("roles update", err)
		}
		return e.emit(c.json, roleView(res.Msg.GetRole()), roleLine(res.Msg.GetRole()))
	default: // delete
		if _, err := gov.DeleteRole(ctx, connect.NewRequest(&pluxv1.DeleteRoleRequest{Key: rest[0]})); err != nil {
			return e.fail("roles delete", err)
		}
		return e.emit(c.json, map[string]any{"deleted": rest[0]}, "Deleted role "+rest[0])
	}
}

func grantView(g *pluxv1.RoleGrant) map[string]any {
	return map[string]any{
		"id": g.GetId(), "userId": g.GetUserId(), "teamId": g.GetTeamId(), "role": g.GetRole(),
		"scope": g.GetScope(), "appId": g.GetAppId(), "environmentId": g.GetEnvironmentId(),
	}
}

func grantLine(g *pluxv1.RoleGrant) string {
	holder := "user " + g.GetUserId()
	if g.GetTeamId() != "" {
		holder = "team " + g.GetTeamId()
	}
	at := g.GetScope()
	switch {
	case g.GetEnvironmentId() != "":
		at += " " + g.GetAppId() + "/" + g.GetEnvironmentId()
	case g.GetAppId() != "":
		at += " " + g.GetAppId()
	}
	return fmt.Sprintf("%s\t%s\t%s\t%s\n", g.GetId(), holder, g.GetRole(), at)
}

// grants lists, creates, updates and deletes role grants: a role held by a
// user or a team on the organisation, an app or an environment (GOV-003).
func (e env) grants(args []string) int {
	var user, team, app, environment, role, scope string
	c, rest, code, ok := e.governanceCmd("grants", grantsUsage, args, map[string]subUsage{
		"list":   {"Usage: plux grants list [--user id] [--team id] [--app id] [--json]\n\nLists role grants, optionally of one holder or app.", 0},
		"create": {"Usage: plux grants create --role key (--user id | --team id) [--scope organization|app|environment] [--app id] [--environment id] [--json]\n\nGrants a role to a user or a team at a scope.", 0},
		"update": {"Usage: plux grants update --role key [--json] <id>\n\nChanges the role of a grant.", 1},
		"delete": {"Usage: plux grants delete <id>\n\nRevokes a grant.", 1},
	}, func(set *flag.FlagSet) {
		set.StringVar(&user, "user", "", "the user `id`")
		set.StringVar(&team, "team", "", "the team `id`")
		set.StringVar(&app, "app", "", "the app `id`")
		set.StringVar(&environment, "environment", "", "the environment `id`")
		set.StringVar(&role, "role", "", "the role `key`")
		set.StringVar(&scope, "scope", "organization", "the `scope`: organization, app or environment")
	})
	if !ok {
		return code
	}
	gov, _, err := e.governanceClients(c)
	if err != nil {
		return e.fail("grants", err)
	}
	ctx := context.Background()
	switch args[0] {
	case "list":
		views := []map[string]any{}
		var text strings.Builder
		if err := eachPage(func(token string) (string, error) {
			res, err := gov.ListGrants(ctx, connect.NewRequest(&pluxv1.ListGrantsRequest{UserId: user, TeamId: team, AppId: app, Page: &pluxv1.Page{PageSize: 100, PageToken: token}}))
			if err != nil {
				return "", err //nolint:wrapcheck // e.fail names the command
			}
			for _, g := range res.Msg.GetGrants() {
				views = append(views, grantView(g))
				text.WriteString(grantLine(g))
			}
			return res.Msg.GetPage().GetNextPageToken(), nil
		}); err != nil {
			return e.fail("grants list", err)
		}
		return e.emit(c.json, map[string]any{"grants": views}, text.String())
	case "create":
		res, err := gov.CreateGrant(ctx, connect.NewRequest(&pluxv1.CreateGrantRequest{
			UserId: user, TeamId: team, Role: role, Scope: scope, AppId: app, EnvironmentId: environment,
		}))
		if err != nil {
			return e.fail("grants create", err)
		}
		return e.emit(c.json, grantView(res.Msg.GetGrant()), grantLine(res.Msg.GetGrant()))
	case "update":
		res, err := gov.UpdateGrant(ctx, connect.NewRequest(&pluxv1.UpdateGrantRequest{Id: rest[0], Role: role}))
		if err != nil {
			return e.fail("grants update", err)
		}
		return e.emit(c.json, grantView(res.Msg.GetGrant()), grantLine(res.Msg.GetGrant()))
	default: // delete
		if _, err := gov.DeleteGrant(ctx, connect.NewRequest(&pluxv1.DeleteGrantRequest{Id: rest[0]})); err != nil {
			return e.fail("grants delete", err)
		}
		return e.emit(c.json, map[string]any{"deleted": rest[0]}, "Revoked grant "+rest[0])
	}
}

// teams lists, creates and deletes teams and adds and removes their
// members (GOV-003); roles are granted to a team with plux grants.
func (e env) teams(args []string) int {
	var teamName, team string
	c, rest, code, ok := e.governanceCmd("teams", teamsUsage, args, map[string]subUsage{
		"list":   {"Usage: plux teams list [--json]\n\nLists the organisation's teams.", 0},
		"create": {"Usage: plux teams create --name text [--json] <key>\n\nCreates a team.", 1},
		"delete": {"Usage: plux teams delete <id>\n\nDeletes a team and its grants.", 1},
		"add":    {"Usage: plux teams add --team id [--json] <email>\n\nAdds a person to a team.", 1},
		"remove": {"Usage: plux teams remove --team id <user-id>\n\nRemoves a person from a team.", 1},
	}, func(set *flag.FlagSet) {
		set.StringVar(&teamName, "name", "", "the team's display `name`")
		set.StringVar(&team, "team", "", "the team `id`")
	})
	if !ok {
		return code
	}
	_, org, err := e.governanceClients(c)
	if err != nil {
		return e.fail("teams", err)
	}
	ctx := context.Background()
	switch args[0] {
	case "list":
		views := []map[string]any{}
		var text strings.Builder
		if err := eachPage(func(token string) (string, error) {
			res, err := org.ListTeams(ctx, connect.NewRequest(&pluxv1.ListTeamsRequest{OrganizationId: c.org, Page: &pluxv1.Page{PageSize: 100, PageToken: token}}))
			if err != nil {
				return "", err //nolint:wrapcheck // e.fail names the command
			}
			for _, t := range res.Msg.GetTeams() {
				views = append(views, map[string]any{"id": t.GetId(), "key": t.GetKey(), "name": t.GetName()})
				_, _ = fmt.Fprintf(&text, "%s\t%s\t%s\n", t.GetId(), t.GetKey(), t.GetName())
			}
			return res.Msg.GetPage().GetNextPageToken(), nil
		}); err != nil {
			return e.fail("teams list", err)
		}
		return e.emit(c.json, map[string]any{"teams": views}, text.String())
	case "create":
		res, err := org.CreateTeam(ctx, connect.NewRequest(&pluxv1.CreateTeamRequest{OrganizationId: c.org, Key: rest[0], Name: teamName}))
		if err != nil {
			return e.fail("teams create", err)
		}
		t := res.Msg.GetTeam()
		return e.emit(c.json, map[string]any{"id": t.GetId(), "key": t.GetKey(), "name": t.GetName()}, fmt.Sprintf("Created team %s (%s)", t.GetKey(), t.GetId()))
	case "delete":
		if _, err := org.DeleteTeam(ctx, connect.NewRequest(&pluxv1.DeleteTeamRequest{Id: rest[0]})); err != nil {
			return e.fail("teams delete", err)
		}
		return e.emit(c.json, map[string]any{"deleted": rest[0]}, "Deleted team "+rest[0])
	case "add":
		res, err := org.AddMember(ctx, connect.NewRequest(&pluxv1.AddMemberRequest{OrganizationId: c.org, TeamId: team, Email: rest[0]}))
		if err != nil {
			return e.fail("teams add", err)
		}
		m := res.Msg.GetMember()
		return e.emit(c.json, map[string]any{"userId": m.GetUserId(), "teamId": m.GetTeamId(), "invitation": res.Msg.GetInvitation()}, addedText(rest[0], res.Msg))
	default: // remove
		if _, err := org.RemoveMember(ctx, connect.NewRequest(&pluxv1.RemoveMemberRequest{OrganizationId: c.org, TeamId: team, UserId: rest[0]})); err != nil {
			return e.fail("teams remove", err)
		}
		return e.emit(c.json, map[string]any{"removed": rest[0], "teamId": team}, fmt.Sprintf("Removed %s from team %s", rest[0], team))
	}
}

// addedText reports a person added to a team. When the account is new, the
// server returns its invitation once, so it is shown here.
func addedText(email string, res *pluxv1.AddMemberResponse) string {
	text := fmt.Sprintf("Added %s to team %s", email, res.GetMember().GetTeamId())
	if inv := res.GetInvitation(); inv != "" {
		text += "\nInvitation (valid seven days, shown once): " + inv
	}
	return text
}
