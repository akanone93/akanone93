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

const (
	sessionsUsage = "Usage: plux sessions list|revoke [flags]\n"
	tokensUsage   = "Usage: plux tokens revoke [flags]\n"
)

// identityClient connects to the identity service with the caller's token.
func (e env) identityClient(c common) (pluxv1connect.IdentityServiceClient, error) {
	token, err := e.creds.load(c.server)
	if err != nil {
		return nil, err
	}
	if token == "" {
		return nil, authError("not signed in to " + c.server + ": run plux login or set " + tokenEnv)
	}
	return pluxv1connect.NewIdentityServiceClient(e.httpClient, c.server, connect.WithInterceptors(e.headers(c.server, token, c.org))), nil
}

// sessions lists the sessions and tokens of the organisation's members and
// ends one (GOV-005). Both need a step-up, which the CLI obtains itself.
func (e env) sessions(args []string) int {
	c, rest, code, ok := e.governanceCmd("sessions", sessionsUsage, args, map[string]subUsage{
		"list":   {"Usage: plux sessions list [--json]\n\nLists the browser sessions and tokens of the organisation's members.", 0},
		"revoke": {"Usage: plux sessions revoke <id>\n\nEnds a browser session, or revokes a token, by the identifier `plux sessions list` shows.", 1},
	}, func(*flag.FlagSet) {})
	if !ok {
		return code
	}
	client, err := e.identityClient(c)
	if err != nil {
		return e.fail("sessions", err)
	}
	ctx := context.Background()
	if args[0] == "list" {
		return e.listSessions(ctx, client, c)
	}
	_, err = client.RevokeSession(ctx, connect.NewRequest(&pluxv1.RevokeSessionRequest{Id: rest[0]}))
	return e.revoked("sessions revoke", rest[0], err, c)
}

// tokens revokes a personal access token of the caller's, or of anyone's
// with tokens.manage (SRV-064, GOV-005).
func (e env) tokens(args []string) int {
	c, rest, code, ok := e.governanceCmd("tokens", tokensUsage, args, map[string]subUsage{
		"revoke": {"Usage: plux tokens revoke <id>\n\nRevokes an access token at once; every server instance refuses it from then on.", 1},
	}, func(*flag.FlagSet) {})
	if !ok {
		return code
	}
	client, err := e.identityClient(c)
	if err != nil {
		return e.fail("tokens", err)
	}
	_, err = client.RevokeAccessToken(context.Background(), connect.NewRequest(&pluxv1.RevokeAccessTokenRequest{Id: rest[0]}))
	return e.revoked("tokens revoke", rest[0], err, c)
}

// revoked reports the outcome of a revocation.
func (e env) revoked(cmd, id string, err error, c common) int {
	if err != nil {
		return e.fail(cmd, err)
	}
	return e.emit(c.json, map[string]any{"revoked": id}, "Revoked "+id)
}

// listSessions prints every page of the organisation's sessions.
func (e env) listSessions(ctx context.Context, client pluxv1connect.IdentityServiceClient, c common) int {
	views := []map[string]any{}
	var text strings.Builder
	err := eachPage(func(token string) (string, error) {
		res, err := client.ListSessions(ctx, connect.NewRequest(&pluxv1.ListSessionsRequest{OrganizationId: c.org, Page: &pluxv1.Page{PageSize: 100, PageToken: token}}))
		if err != nil {
			return "", err //nolint:wrapcheck // e.fail names the command
		}
		for _, s := range res.Msg.GetSessions() {
			views = append(views, sessionView(s))
			_, _ = fmt.Fprintf(&text, "%s\t%s\t%s\t%s\t%s\n", s.GetId(), s.GetClientKind(), s.GetUserId(), firstOf(s.GetName(), "-"), lastSeen(s))
		}
		return res.Msg.GetPage().GetNextPageToken(), nil
	})
	if err != nil {
		return e.fail("sessions list", err)
	}
	return e.emit(c.json, map[string]any{"sessions": views}, text.String())
}

// lastSeen renders when a session was last used.
func lastSeen(s *pluxv1.SessionInfo) string {
	if s.GetLastSeenAt() == nil {
		return "never"
	}
	return s.GetLastSeenAt().AsTime().UTC().Format(time.RFC3339)
}

// sessionView is a session as --json prints it.
func sessionView(s *pluxv1.SessionInfo) map[string]any {
	return map[string]any{
		"id": s.GetId(), "userId": s.GetUserId(), "clientKind": s.GetClientKind(), "name": s.GetName(),
		"ipPrefix": s.GetIpPrefix(), "userAgent": s.GetUserAgent(), "lastSeenAt": lastSeen(s),
	}
}
