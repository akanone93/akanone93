// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api_test

import (
	"context"
	"strings"
	"testing"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/pluxv1"
)

// Verifies: GOV-005, SEC-103.
// A session with a second factor that never stepped up is refused, over the
// wire, a permission the catalogue marks stepUp (sessions.manage) with
// PLX-6200, and the same session is served once it has stepped up.
func TestSessionWithoutStepUpIsRefused_GOV_005(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	ctx := context.Background()
	const password = "correct horse battery"

	_, invitation, err := w.auth.Bootstrap(ctx, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}
	must(w.identity.AcceptInvitation(ctx, connect.NewRequest(&pluxv1.AcceptInvitationRequest{
		Invitation: invitation, DisplayName: "Admin", Password: password,
	})))(t)

	admin := w.signIn(t, "admin@example.com")
	org := must(w.org.CreateOrganization(ctx, req(admin, &pluxv1.CreateOrganizationRequest{Key: "acme", Name: "Acme"})))(t).GetOrganization()
	list := &pluxv1.ListSessionsRequest{OrganizationId: org.GetId()}

	plain := w.signInWithoutStepUp(t, "admin@example.com")
	w.withSecondFactor(t, plain)
	_, err = w.identity.ListSessions(ctx, req(plain, list))
	if codeOf(err) != connect.CodePermissionDenied || !strings.Contains(err.Error(), "PLX-6200") {
		t.Fatalf("a session without a step-up: %v, want PLX-6200", err)
	}

	w.recordStepUp(t, plain)
	if _, err := w.identity.ListSessions(ctx, req(plain, list)); err != nil {
		t.Errorf("a session that stepped up: %v", err)
	}
}
