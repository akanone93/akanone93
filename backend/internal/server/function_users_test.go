// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/device"
	"github.com/nightCode42/plux3/backend/internal/gateway"
	"github.com/nightCode42/plux3/backend/internal/signing"
)

// Verifies: SEC-026, HST-010, FN-002.
// A function receives the claims of a user token exchanged for the audience
// function:<name>; it refuses the token of an upstream, of another function
// and of another device.
func TestFunctionUsersVerifyTheFunctionAudience(t *testing.T) {
	t.Parallel()
	file, err := signing.NewFile(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	signer := signing.TokenOnly(file)
	tokens := &gateway.UserTokens{Signer: signer, Keys: signer.TokenKeys, Issuer: "https://plux.example"}
	dev := device.Identity{DeviceID: "dev-1", AppID: "app-1", EnvironmentID: "env-1", OrganizationID: "org-1"}
	users := functionUsers{tokens: tokens, production: func(context.Context, string) (bool, error) { return false, nil }}

	issue := func(d device.Identity, audience string) string {
		t.Helper()
		token, err := tokens.Issue(t.Context(), gateway.BoundTo{DeviceID: d.DeviceID, AppID: d.AppID, EnvironmentID: d.EnvironmentID, Upstream: audience},
			gateway.User{Subject: "user-7", Issuer: "https://idp.example", Claims: map[string]any{"tier": "gold"}}, time.Now().Add(time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		return token
	}

	raw, err := users.VerifyUser(t.Context(), issue(dev, gateway.FunctionAudience("bank.transferFee")), dev, "bank.transferFee")
	if err != nil {
		t.Fatalf("VerifyUser: %v", err)
	}
	var got gateway.User
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.Subject != "user-7" || got.Claims["tier"] != "gold" {
		t.Errorf("claims = %s", raw)
	}

	other := dev
	other.DeviceID = "dev-2"
	for name, token := range map[string]string{
		"a token for an upstream":          issue(dev, "bank"),
		"a token for the bare name":        issue(dev, "bank.transferFee"),
		"a token for another function":     issue(dev, gateway.FunctionAudience("bank.other")),
		"a token for another device":       issue(other, gateway.FunctionAudience("bank.transferFee")),
		"a token that is not a user token": "a.b.c",
	} {
		if _, err := users.VerifyUser(t.Context(), token, dev, "bank.transferFee"); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}
