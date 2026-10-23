// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api_test

import (
	"testing"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
)

// Verifies: DEV-002, DEV-003, SEC-021.
// A person opens a session in the development environment and a device of
// that environment pairs with its token, authenticated by its access token
// and a DPoP proof; the device is listed, the token is spent, and a call
// without the proof pairs nothing. The production environment is refused.
func TestDevSessionPair_DEV_002(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	ctx := t.Context()
	admin, app, envs := w.appOnly(t)
	_, err := w.devSession.CreateSession(ctx, req(admin, &pluxv1.DevSessionServiceCreateSessionRequest{AppId: app, EnvironmentId: envs["production"]}))
	if codeOf(err) != connect.CodeFailedPrecondition {
		t.Errorf("a production session: %v, want FailedPrecondition (PLX-9203)", err)
	}
	created := must(w.devSession.CreateSession(ctx, req(admin, &pluxv1.DevSessionServiceCreateSessionRequest{AppId: app, EnvironmentId: envs["development"]})))(t)
	token := created.GetPairing().GetToken()
	if token == "" || created.GetSession().GetEnvironmentId() != envs["development"] || created.GetPairing().GetUri() == "" {
		t.Fatalf("created = %v", created)
	}

	dev := w.registerDevelopment(t, app)
	dev.refresh(t)
	unproven := connect.NewRequest(&pluxv1.DevSessionServicePairRequest{PairingToken: token})
	unproven.Header().Set("Authorization", "DPoP "+dev.token)
	if _, err := w.devSession.Pair(ctx, unproven); codeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("a pairing without a proof: %v", err)
	}
	paired, err := w.devSession.Pair(ctx, bound(t, dev, pluxv1connect.DevSessionServicePairProcedure, &pluxv1.DevSessionServicePairRequest{
		PairingToken: token, Name: "linux box", Platform: "android", RuntimeVersion: "1.0.0",
	}))
	if err != nil {
		t.Fatalf("Pair: %v", err)
	}
	if paired.Msg.GetSessionId() != created.GetSession().GetId() || paired.Msg.GetSocketTicket() == "" || paired.Msg.GetSocketTicketExpiresAt() == nil {
		t.Errorf("paired = %v", paired.Msg)
	}
	devices := must(w.devSession.ListDevices(ctx, req(admin, &pluxv1.DevSessionServiceListDevicesRequest{SessionId: created.GetSession().GetId()})))(t).GetDevices()
	if len(devices) != 1 || devices[0].GetDeviceId() != dev.id || devices[0].GetId() != paired.Msg.GetDeviceId() {
		t.Fatalf("devices = %v", devices)
	}
	dev.nonce = paired.Header().Get("DPoP-Nonce")
	_, err = w.devSession.Pair(ctx, bound(t, dev, pluxv1connect.DevSessionServicePairProcedure, &pluxv1.DevSessionServicePairRequest{PairingToken: token}))
	if codeOf(err) != connect.CodeFailedPrecondition {
		t.Errorf("a spent token: %v, want FailedPrecondition (PLX-9202)", err)
	}
	ended := must(w.devSession.EndSession(ctx, req(admin, &pluxv1.DevSessionServiceEndSessionRequest{SessionId: created.GetSession().GetId()})))(t)
	if ended.GetSession().GetEndedAt() == nil {
		t.Errorf("ended = %v", ended)
	}
}
