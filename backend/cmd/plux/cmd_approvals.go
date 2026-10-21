// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
)

const (
	policiesUsage  = "Usage: plux policies list|show|put|delete|export [flags]\n"
	approvalsUsage = "Usage: plux approvals list|show|approve|reject [flags]\n"
)

// approvalCmd parses an approval command's flags: the common ones with the
// app, the subcommand's usage and positional count, and extra flags.
func (e env) approvalCmd(cmd, usage string, args []string, usages map[string]subUsage, extra func(*flag.FlagSet)) (common, []string, int, bool) {
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
	c.register(set, true)
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

// approvalClient connects to the approval service.
func (e env) approvalClient(c common) (pluxv1connect.ApprovalServiceClient, error) {
	opts, err := e.authOptions(c)
	if err != nil {
		return nil, err
	}
	return pluxv1connect.NewApprovalServiceClient(e.httpClient, c.server, opts), nil
}

// policyView is a policy version for --json.
func policyView(p *pluxv1.PolicyRecord) map[string]any {
	return map[string]any{
		"appId": p.GetAppId(), "key": p.GetKey(), "version": p.GetVersion(), "documentHash": p.GetDocumentHash(),
		"createdBy": p.GetCreatedBy(), "document": rawJSON(p.GetDocument()),
	}
}

// policyLine is a policy version on one line.
func policyLine(p *pluxv1.PolicyRecord) string {
	return fmt.Sprintf("%s\tv%d\t%s\t%s\n", p.GetKey(), p.GetVersion(), shortHash(p.GetDocumentHash()), p.GetCreatedBy())
}

// policies lists, shows, writes, deletes and exports approval policies
// (GOV-020). An app's policies are the default; --org-wide addresses the
// organisation's, which govern every app.
func (e env) policies(args []string) int {
	var orgWide bool
	var out string
	c, rest, code, ok := e.approvalCmd("policies", policiesUsage, args, map[string]subUsage{
		"list":   {"Usage: plux policies list [--org-wide] [--json]\n\nLists the policies in force. The first call for an app writes the default policies of its security profile.", 0},
		"show":   {"Usage: plux policies show [--org-wide] [--json] <key>\n\nShows a policy document.", 1},
		"put":    {"Usage: plux policies put [--org-wide] [--json] <file>\n\nWrites a policy document as the next version of its key. Changing a policy may itself need approval.", 1},
		"delete": {"Usage: plux policies delete [--org-wide] <key>\n\nWithdraws a policy; a deleted default is not written again.", 1},
		"export": {"Usage: plux policies export [--org-wide] [--out dir]\n\nWrites the policies in force as policies/<key>.policy.json files, the Git layout.", 0},
	}, func(set *flag.FlagSet) {
		set.BoolVar(&orgWide, "org-wide", false, "address the organisation's policies instead of an app's")
		set.StringVar(&out, "out", ".", "the `directory` to export into")
	})
	if !ok {
		return code
	}
	if orgWide {
		c.app = ""
	} else if err := needApp(c); err != nil {
		return e.fail("policies", err)
	}
	client, err := e.approvalClient(c)
	if err != nil {
		return e.fail("policies", err)
	}
	return e.policyAction(context.Background(), client, c, args[0], rest, out)
}

// policyAction runs one policies subcommand.
func (e env) policyAction(ctx context.Context, client pluxv1connect.ApprovalServiceClient, c common, action string, rest []string, out string) int {
	switch action {
	case "list":
		res, err := client.ListPolicies(ctx, connect.NewRequest(&pluxv1.ListPoliciesRequest{AppId: c.app}))
		if err != nil {
			return e.fail("policies list", err)
		}
		views, text := []map[string]any{}, strings.Builder{}
		for _, p := range res.Msg.GetPolicies() {
			views = append(views, policyView(p))
			text.WriteString(policyLine(p))
		}
		return e.emit(c.json, map[string]any{"policies": views}, text.String())
	case "show":
		res, err := client.GetPolicy(ctx, connect.NewRequest(&pluxv1.GetPolicyRequest{AppId: c.app, Key: rest[0]}))
		if err != nil {
			return e.fail("policies show", err)
		}
		return e.emit(c.json, policyView(res.Msg.GetPolicy()), string(res.Msg.GetPolicy().GetDocument()))
	case "put":
		return e.policyPut(ctx, client, c, rest[0])
	case "export":
		return e.policyExport(ctx, client, c, out)
	default: // delete
		if _, err := client.DeletePolicy(ctx, connect.NewRequest(&pluxv1.DeletePolicyRequest{AppId: c.app, Key: rest[0]})); err != nil {
			return e.fail("policies delete", err)
		}
		return e.emit(c.json, map[string]any{"deleted": rest[0]}, "Deleted policy "+rest[0])
	}
}

// policyPut writes a policy file.
func (e env) policyPut(ctx context.Context, client pluxv1connect.ApprovalServiceClient, c common, file string) int {
	doc, err := os.ReadFile(file) //nolint:gosec // G304: the file is the argument the user gave.
	if err != nil {
		return e.fail("policies put", err)
	}
	res, err := client.PutPolicy(ctx, connect.NewRequest(&pluxv1.PutPolicyRequest{AppId: c.app, Document: doc}))
	if err != nil {
		return e.fail("policies put", err)
	}
	return e.emit(c.json, policyView(res.Msg.GetPolicy()), policyLine(res.Msg.GetPolicy()))
}

// policyExport writes the policies in force under a directory.
func (e env) policyExport(ctx context.Context, client pluxv1connect.ApprovalServiceClient, c common, dir string) int {
	res, err := client.ExportPolicies(ctx, connect.NewRequest(&pluxv1.ExportPoliciesRequest{AppId: c.app}))
	if err != nil {
		return e.fail("policies export", err)
	}
	written := []string{}
	for _, f := range res.Msg.GetFiles() {
		target := filepath.Join(dir, filepath.FromSlash(f.GetPath()))
		if err := writeUnder(dir, target, f.GetContent()); err != nil {
			return e.fail("policies export", err)
		}
		written = append(written, f.GetPath())
	}
	return e.emit(c.json, map[string]any{"files": written}, "Wrote "+strings.Join(written, "\n      "))
}

// writeUnder writes a file, creating its directory, only if the target stays
// inside dir: a path from the server never escapes the directory it is
// exported into.
func writeUnder(dir, target string, content []byte) error {
	rel, err := filepath.Rel(dir, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("the path %q is outside %s", target, dir)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(target), err)
	}
	if err := os.WriteFile(target, content, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", target, err)
	}
	return nil
}

// approvalView is a request for --json.
func approvalView(a *pluxv1.ApprovalRequest) map[string]any {
	decisions := []map[string]any{}
	for _, d := range a.GetDecisions() {
		decisions = append(decisions, map[string]any{
			"decider": d.GetDecider(), "decision": d.GetDecision(), "comment": d.GetComment(),
			"decidedAt": d.GetDecidedAt().AsTime(), "signed": len(d.GetSignature()) > 0,
		})
	}
	return map[string]any{
		"id": a.GetId(), "resource": a.GetResource(), "action": a.GetAction(), "subject": a.GetSubject(),
		"environment": a.GetEnvironment(), "status": a.GetStatus(), "contentHash": a.GetContentHash(), "summary": a.GetSummary(),
		"requestedBy": a.GetRequestedBy(), "policies": nonNil(a.GetPolicyKeys()), "approvalsRequired": a.GetApprovalsRequired(),
		"risk": nonNil(a.GetRisk()), "expiresAt": a.GetExpiresAt().AsTime(), "decisions": decisions,
	}
}

// approvalLine is a request on one line.
func approvalLine(a *pluxv1.ApprovalRequest) string {
	approved := 0
	for _, d := range a.GetDecisions() {
		if d.GetDecision() == "approve" {
			approved++
		}
	}
	return fmt.Sprintf("%s\t%s\t%d/%d\t%s\t%s\n", a.GetId(), a.GetStatus(), approved, a.GetApprovalsRequired(), a.GetSubject(), a.GetSummary())
}

// approvals lists requests, shows one with its evidence, and approves or
// rejects it (GOV-021, GOV-023). Deciding needs a step-up with a security
// key, which the command asks for in the browser.
func (e env) approvals(args []string) int {
	var status, comment, out string
	c, rest, code, ok := e.approvalCmd("approvals", approvalsUsage, args, map[string]subUsage{
		"list":    {"Usage: plux approvals list [--status open|approved|rejected|expired|used] [--json]\n\nLists the approval requests of the app, newest first.", 0},
		"show":    {"Usage: plux approvals show [--out dir] [--json] <id>\n\nShows a request with its review bundle, and writes the diff, the reports, the golden images with a pixel diff and review.html into --out.", 1},
		"approve": {"Usage: plux approvals approve [--comment text] [--json] <id>\n\nApproves a request. Needs a step-up. You cannot approve a change you made or wrote.", 1},
		"reject":  {"Usage: plux approvals reject [--comment text] [--json] <id>\n\nRejects a request, which closes it. Needs a step-up.", 1},
	}, func(set *flag.FlagSet) {
		set.StringVar(&status, "status", "", "list only requests with this `status`")
		set.StringVar(&comment, "comment", "", "a `comment` recorded with the decision")
		set.StringVar(&out, "out", "", "the `directory` to write the review into (default approval-<id>)")
	})
	if !ok {
		return code
	}
	if err := needApp(c); err != nil {
		return e.fail("approvals", err)
	}
	client, err := e.approvalClient(c)
	if err != nil {
		return e.fail("approvals", err)
	}
	ctx := context.Background()
	switch args[0] {
	case "list":
		return e.approvalList(ctx, client, c, status)
	case "show":
		return e.approvalShow(ctx, client, c, rest[0], out)
	default:
		return e.approvalDecide(ctx, client, c, args[0], rest[0], comment)
	}
}

// approvalList lists the requests of an app.
func (e env) approvalList(ctx context.Context, client pluxv1connect.ApprovalServiceClient, c common, status string) int {
	views, text := []map[string]any{}, strings.Builder{}
	err := eachPage(func(token string) (string, error) {
		res, err := client.ListApprovals(ctx, connect.NewRequest(&pluxv1.ListApprovalsRequest{
			AppId: c.app, Status: status, Page: &pluxv1.Page{PageSize: 100, PageToken: token},
		}))
		if err != nil {
			return "", err //nolint:wrapcheck // e.fail names the command
		}
		for _, a := range res.Msg.GetApprovals() {
			views = append(views, approvalView(a))
			text.WriteString(approvalLine(a))
		}
		return res.Msg.GetPage().GetNextPageToken(), nil
	})
	if err != nil {
		return e.fail("approvals list", err)
	}
	return e.emit(c.json, map[string]any{"approvals": views}, text.String())
}

// approvalDecide approves or rejects a request.
func (e env) approvalDecide(ctx context.Context, client pluxv1connect.ApprovalServiceClient, c common, verb, id, comment string) int {
	res, err := client.DecideApproval(ctx, connect.NewRequest(&pluxv1.DecideApprovalRequest{AppId: c.app, Id: id, Decision: verb, Comment: comment}))
	if err != nil {
		return e.fail("approvals "+verb, err)
	}
	return e.emit(c.json, approvalView(res.Msg.GetApproval()), approvalLine(res.Msg.GetApproval()))
}
