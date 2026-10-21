// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"fmt"
	"strings"
	"time"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
)

// breakGlassFlags are the flags that make a kill switch, a forced rollback or
// a rollback an emergency action (GOV-024).
type breakGlassFlags struct {
	on            bool
	justification string
}

// register adds the flags to a command.
func (b *breakGlassFlags) register(set *flag.FlagSet) {
	set.BoolVar(&b.on, "break-glass", false, "make this an emergency action: no approval and no change freeze, a post-hoc review follows (needs breakglass.invoke and --justification)")
	set.StringVar(&b.justification, "justification", "", "the written `reason` for a break-glass action")
}

// check refuses the flags that do not make sense together; the server
// enforces the justification (PLX-8005) whatever the client does.
func (b breakGlassFlags) check() error {
	switch {
	case b.on && strings.TrimSpace(b.justification) == "":
		return usageError("--break-glass needs --justification: say why this cannot wait for approval")
	case !b.on && b.justification != "":
		return usageError("--justification belongs to --break-glass")
	}
	return nil
}

const breakGlassUsage = "Usage: plux breakglass list|review [flags]\n"

// breakglass lists the emergency actions of the organisation and completes
// their post-hoc reviews (GOV-024).
func (e env) breakglass(args []string) int {
	var status, comment string
	c, rest, code, ok := e.approvalCmd("breakglass", breakGlassUsage, args, map[string]subUsage{
		"list":   {"Usage: plux breakglass list [--status open|reviewed] [--json]\n\nLists the emergency actions taken without approval, newest first, with the state of their reviews.", 0},
		"review": {"Usage: plux breakglass review [--comment text] [--json] <id>\n\nCompletes the post-hoc review of an emergency action. Whoever took the action cannot review it; this needs a fresh step-up.", 1},
	}, func(set *flag.FlagSet) {
		set.StringVar(&status, "status", "", "list only reviews that are `open` or `reviewed`")
		set.StringVar(&comment, "comment", "", "what the review found")
	})
	if !ok {
		return code
	}
	opts, err := e.authOptions(c)
	if err != nil {
		return e.fail("breakglass", err)
	}
	client := pluxv1connect.NewBreakGlassServiceClient(e.httpClient, c.server, opts)
	if args[0] == "list" {
		return e.breakGlassList(client, c, status)
	}
	res, err := client.ReviewBreakGlass(context.Background(), connect.NewRequest(&pluxv1.ReviewBreakGlassRequest{Id: rest[0], Comment: comment}))
	if err != nil {
		return e.fail("breakglass review", stepUp(err))
	}
	return e.emit(c.json, breakGlassView(res.Msg.GetInvocation()), "Reviewed "+rest[0])
}

// breakGlassList prints the emergency actions.
func (e env) breakGlassList(client pluxv1connect.BreakGlassServiceClient, c common, status string) int {
	res, err := client.ListBreakGlass(context.Background(), connect.NewRequest(&pluxv1.ListBreakGlassRequest{Status: status, Page: &pluxv1.Page{PageSize: 100}}))
	if err != nil {
		return e.fail("breakglass list", err)
	}
	views, text := []map[string]any{}, strings.Builder{}
	for _, b := range res.Msg.GetInvocations() {
		views = append(views, breakGlassView(b))
		fmt.Fprintf(&text, "%s\t%s\t%s\t%s\t%s\t%s\n", b.GetId(), b.GetInvokedAt().AsTime().UTC().Format(time.RFC3339), b.GetAction(), b.GetEnvironment(), reviewState(b), b.GetInvokedBy())
	}
	return e.emit(c.json, map[string]any{"invocations": views}, text.String())
}

// reviewState words the state of a review for a line of output.
func reviewState(b *pluxv1.BreakGlassInvocation) string {
	switch {
	case b.GetOverdue():
		return "OVERDUE (PLX-8006), due " + b.GetReviewDueAt().AsTime().UTC().Format(time.RFC3339)
	case b.GetReviewStatus() == "open":
		return "open, due " + b.GetReviewDueAt().AsTime().UTC().Format(time.RFC3339)
	}
	return "reviewed by " + b.GetReviewedBy()
}

// breakGlassView is an emergency action for --json.
func breakGlassView(b *pluxv1.BreakGlassInvocation) map[string]any {
	return map[string]any{
		"id": b.GetId(), "app": b.GetAppId(), "environment": b.GetEnvironment(), "action": b.GetAction(), "subject": b.GetSubject(),
		"justification": b.GetJustification(), "invokedBy": b.GetInvokedBy(), "invokedAt": b.GetInvokedAt().AsTime().UTC().Format(time.RFC3339),
		"reviewDueAt": b.GetReviewDueAt().AsTime().UTC().Format(time.RFC3339), "reviewStatus": b.GetReviewStatus(), "overdue": b.GetOverdue(),
		"reviewedBy": b.GetReviewedBy(), "reviewComment": b.GetReviewComment(),
	}
}
