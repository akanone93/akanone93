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
	restrictionsUsage = "Usage: plux restrictions create|list|delete [flags]\n"
	accessUsage       = "Usage: plux access explain [flags]\n"
)

// restrictionView is a restriction as JSON.
func restrictionView(r *pluxv1.Restriction) map[string]any {
	return map[string]any{
		"id": r.GetId(), "subjectKind": r.GetSubjectKind(), "subjectId": r.GetSubjectId(),
		"patterns": nonNil(r.GetPatterns()), "appId": r.GetAppId(), "environment": r.GetEnvironment(),
		"reason": r.GetReason(), "createdBy": r.GetCreatedBy(),
	}
}

// restrictionLine is a restriction as one line of text: its identifier,
// whom it applies to, what it removes and where.
func restrictionLine(r *pluxv1.Restriction) string {
	subject := r.GetSubjectKind()
	if r.GetSubjectId() != "" {
		subject += " " + r.GetSubjectId()
	}
	where := "organization"
	switch {
	case r.GetEnvironment() != "":
		where = r.GetAppId() + "/" + r.GetEnvironment()
	case r.GetAppId() != "":
		where = r.GetAppId()
	}
	return fmt.Sprintf("%s\t%s\t%s\t%s\n", r.GetId(), subject, strings.Join(r.GetPatterns(), ","), where)
}

// restrictions creates, lists and deletes restrictions: ceilings that remove
// permissions from an organisation, a team or a person, whatever their
// grants say (GOV-006).
func (e env) restrictions(args []string) int {
	var kind, subject, patterns, app, environment, reason string
	c, rest, code, ok := e.governanceCmd("restrictions", restrictionsUsage, args, map[string]subUsage{
		"create": {"Usage: plux restrictions create --patterns a,b [--subject-kind organization|team|user] [--subject id] [--app id] [--environment key] [--reason text] [--json]\n\nRemoves permissions (names, or the wildcards \"*\", \"area.*\" and \"area.feature.*\") from the subject in the organisation, an app or an environment.", 0},
		"list":   {"Usage: plux restrictions list [--subject id] [--app id] [--json]\n\nLists the organisation's restrictions.", 0},
		"delete": {"Usage: plux restrictions delete <id>\n\nDeletes a restriction.", 1},
	}, func(set *flag.FlagSet) {
		set.StringVar(&kind, "subject-kind", "user", "whom it applies to: organization, team or user")
		set.StringVar(&subject, "subject", "", "the team or user `id`; empty for the organization (for list: only its restrictions)")
		set.StringVar(&patterns, "patterns", "", "the permissions it removes, comma-separated (`list`)")
		set.StringVar(&app, "app", "", "limit it to this app `id`")
		set.StringVar(&environment, "environment", "", "limit it to this environment `key` of the app")
		set.StringVar(&reason, "reason", "", "why, for the audit log")
	})
	if !ok {
		return code
	}
	gov, _, err := e.governanceClients(c)
	if err != nil {
		return e.fail("restrictions", err)
	}
	ctx := context.Background()
	switch args[0] {
	case "list":
		return e.listRestrictions(ctx, gov, c.json, subject, app)
	case "create":
		res, err := gov.CreateRestriction(ctx, connect.NewRequest(&pluxv1.CreateRestrictionRequest{
			SubjectKind: kind, SubjectId: subject, Patterns: splitList(patterns), AppId: app, Environment: environment, Reason: reason,
		}))
		if err != nil {
			return e.fail("restrictions create", err)
		}
		return e.emit(c.json, restrictionView(res.Msg.GetRestriction()), restrictionLine(res.Msg.GetRestriction()))
	default: // delete
		if _, err := gov.DeleteRestriction(ctx, connect.NewRequest(&pluxv1.DeleteRestrictionRequest{Id: rest[0]})); err != nil {
			return e.fail("restrictions delete", err)
		}
		return e.emit(c.json, map[string]any{"deleted": rest[0]}, "Deleted restriction "+rest[0])
	}
}

// listRestrictions prints the restrictions of the organisation, following
// the pages.
func (e env) listRestrictions(ctx context.Context, gov pluxv1connect.GovernanceServiceClient, asJSON bool, subject, app string) int {
	views := []map[string]any{}
	var text strings.Builder
	if err := eachPage(func(token string) (string, error) {
		res, err := gov.ListRestrictions(ctx, connect.NewRequest(&pluxv1.ListRestrictionsRequest{
			SubjectId: subject, AppId: app, Page: &pluxv1.Page{PageSize: 100, PageToken: token},
		}))
		if err != nil {
			return "", err //nolint:wrapcheck // e.fail names the command
		}
		for _, r := range res.Msg.GetRestrictions() {
			views = append(views, restrictionView(r))
			text.WriteString(restrictionLine(r))
		}
		return res.Msg.GetPage().GetNextPageToken(), nil
	}); err != nil {
		return e.fail("restrictions list", err)
	}
	return e.emit(asJSON, map[string]any{"restrictions": views}, text.String())
}

// explainFlags are the flags of plux access explain.
type explainFlags struct {
	permission, user, app, environment string
	// grantRole and grantTeam propose a grant; restrict, restrictKind and
	// restrictSubject propose a restriction.
	grantRole, grantTeam                string
	restrict, restrictKind, restrictFor string
}

// register adds the flags to a flag set.
func (f *explainFlags) register(set *flag.FlagSet) {
	set.StringVar(&f.permission, "permission", "", "the `permission` asked about")
	set.StringVar(&f.user, "user", "", "the person's user `id`; empty explains your own access")
	set.StringVar(&f.app, "app", "", "the app `id` of the scope; empty for the organization")
	set.StringVar(&f.environment, "environment", "", "the environment `key` of the scope")
	set.StringVar(&f.grantRole, "grant-role", "", "evaluate granting this role `key` in the scope, saving nothing")
	set.StringVar(&f.grantTeam, "grant-team", "", "grant to this team `id` instead of the person")
	set.StringVar(&f.restrict, "restrict", "", "evaluate restricting these permissions, comma-separated (`list`), saving nothing")
	set.StringVar(&f.restrictKind, "restrict-kind", "user", "whom the proposed restriction applies to: organization, team or user")
	set.StringVar(&f.restrictFor, "restrict-subject", "", "the team or user `id` of the proposed restriction; empty means the person")
}

// request builds the explain request, with the proposal the flags name.
func (f explainFlags) request(org string) (*pluxv1.ExplainAccessRequest, error) {
	m := &pluxv1.ExplainAccessRequest{
		OrganizationId: org, UserId: f.user, Permission: f.permission, AppId: f.app, Environment: f.environment,
	}
	if f.grantRole != "" && f.restrict != "" {
		return nil, usageError("propose a grant (--grant-role) or a restriction (--restrict), not both")
	}
	switch {
	case f.grantRole != "":
		m.Proposal = &pluxv1.ExplainAccessRequest_ProposedGrant{ProposedGrant: f.proposedGrant()}
	case f.restrict != "":
		subject := f.restrictFor
		if subject == "" && f.restrictKind == "user" {
			subject = f.user
		}
		m.Proposal = &pluxv1.ExplainAccessRequest_ProposedRestriction{ProposedRestriction: &pluxv1.CreateRestrictionRequest{
			SubjectKind: f.restrictKind, SubjectId: subject, Patterns: splitList(f.restrict), AppId: f.app, Environment: f.environment,
		}}
	}
	return m, nil
}

// proposedGrant is the grant --grant-role names: the role, held by the team
// or the person, at the scope the question asks about.
func (f explainFlags) proposedGrant() *pluxv1.ProposedGrant {
	g := &pluxv1.ProposedGrant{Role: f.grantRole, Scope: "organization", AppId: f.app, Environment: f.environment, TeamId: f.grantTeam}
	if f.grantTeam == "" {
		g.UserId = f.user
	}
	switch {
	case f.environment != "":
		g.Scope = "environment"
	case f.app != "":
		g.Scope = "app"
	}
	return g
}

// explainView is an explanation as JSON.
func explainView(r *pluxv1.ExplainAccessResponse) map[string]any {
	return map[string]any{
		"decision": r.GetDecision(), "allowingGrantIds": nonNil(r.GetAllowingGrantIds()),
		"removingRestrictionIds": nonNil(r.GetRemovingRestrictionIds()), "ownerFloor": r.GetOwnerFloor(),
	}
}

// explainText is an explanation as text: the decision, and the grants and
// restrictions behind it.
func explainText(r *pluxv1.ExplainAccessResponse) string {
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "%s\n", r.GetDecision())
	if len(r.GetAllowingGrantIds()) == 0 {
		b.WriteString("no grant carries the permission here\n")
	}
	for _, id := range r.GetAllowingGrantIds() {
		_, _ = fmt.Fprintf(&b, "allowed by grant %s\n", id)
	}
	for _, id := range r.GetRemovingRestrictionIds() {
		_, _ = fmt.Fprintf(&b, "removed by restriction %s\n", id)
	}
	if r.GetOwnerFloor() {
		b.WriteString("kept by the owner floor despite a restriction\n")
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// access explains why a person can or cannot use a permission, naming the
// grants and restrictions responsible, and evaluates a proposed grant or
// restriction without saving it (GOV-007).
func (e env) access(args []string) int {
	var f explainFlags
	c, _, code, ok := e.governanceCmd("access", accessUsage, args, map[string]subUsage{
		"explain": {"Usage: plux access explain --permission name [--user id] [--app id] [--environment key] [--grant-role key [--grant-team id] | --restrict a,b [--restrict-kind kind] [--restrict-subject id]] [--json]\n\nSays why a person can or cannot use a permission in a scope, naming the grants that allow it and the restrictions that remove it. With a proposal it evaluates the change and saves nothing. Explaining another person's access needs members.manage.", 0},
	}, f.register)
	if !ok {
		return code
	}
	if f.permission == "" {
		return e.fail("access explain", usageError("--permission names the permission to explain"))
	}
	m, err := f.request(c.org)
	if err != nil {
		return e.fail("access explain", err)
	}
	gov, _, err := e.governanceClients(c)
	if err != nil {
		return e.fail("access", err)
	}
	res, err := gov.ExplainAccess(context.Background(), connect.NewRequest(m))
	if err != nil {
		return e.fail("access explain", err)
	}
	return e.emit(c.json, explainView(res.Msg), explainText(res.Msg))
}
