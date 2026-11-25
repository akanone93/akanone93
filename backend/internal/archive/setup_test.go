// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package archive

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/function"
	"github.com/nightCode42/plux3/backend/internal/gateway"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/tenancy"
)

// setupFake holds one app's setup with values that must never reach a
// manifest, and refuses the secrets of the production environment.
type setupFake struct{}

const leaked = "VALUE-THAT-MUST-NOT-LEAK"

func (setupFake) ListEnvironments(context.Context, auth.Principal, string, tenancy.Page) ([]tenancy.Environment, error) {
	return []tenancy.Environment{{ID: "e1", Key: "development"}, {ID: "e2", Key: "production", Production: true}}, nil
}

func (setupFake) ListChannels(_ context.Context, _ auth.Principal, env string, _ tenancy.Page) ([]tenancy.Channel, error) {
	return []tenancy.Channel{{Key: "production"}, {Key: "beta-" + env}}, nil
}

func (setupFake) ListVariables(context.Context, auth.Principal, string, tenancy.Page) ([]tenancy.Variable, error) {
	return []tenancy.Variable{{Key: "API_BASE", Value: "https://" + leaked + ".example"}}, nil
}

func (setupFake) ListSecrets(_ context.Context, _ auth.Principal, env string, _ tenancy.Page) ([]tenancy.Secret, error) {
	if env == "e2" {
		return nil, plxerr.New(plxerr.PermissionDenied, "environment.manage is required")
	}
	return []tenancy.Secret{{Key: "PAYMENT_KEY", Hint: leaked}}, nil
}

func (setupFake) List(context.Context, auth.Principal, string, string, string, int32) ([]gateway.ConfigValue, int64, error) {
	return []gateway.ConfigValue{{Key: "feeLimit", Tier: gateway.TierServerBound, Type: gateway.TypeDecimal, Sensitive: true, Value: leaked}}, 3, nil
}

type gatewayFake struct{}

func (gatewayFake) List(_ context.Context, _ auth.Principal, _, env string, _ storage.Cursor, _ int32) ([]gateway.Upstream, error) {
	return []gateway.Upstream{{Name: "core", BaseURL: "https://" + leaked + ".bank", Auth: gateway.Auth{Kind: "oauth", ClientSecretRef: "CORE_SECRET", TokenURL: "https://" + leaked}}}, nil
}

func (gatewayFake) GetProvider(_ context.Context, _ auth.Principal, _, env string) (gateway.IdentityProvider, error) {
	if env == "e1" {
		return gateway.IdentityProvider{}, plxerr.New(plxerr.ResourceNotFound, "no identity provider")
	}
	return gateway.IdentityProvider{Issuer: "https://" + leaked, JWKSURL: "https://" + leaked}, nil
}

type functionsFake struct{}

func (functionsFake) ListFunctions(context.Context, auth.Principal, string, string, string, int32) ([]function.Version, error) {
	return []function.Version{{Name: "fee"}}, nil
}

func (functionsFake) ListTriggers(context.Context, auth.Principal, string) ([]function.Trigger, error) {
	return []function.Trigger{{EnvironmentID: "e2", Function: "fee", Alias: "live", Expression: "0 3 * * *", Timezone: "Europe/Berlin", Input: []byte(leaked)}}, nil
}

func (functionsFake) ListHooks(context.Context, auth.Principal, string) ([]function.Hook, error) {
	return []function.Hook{{EnvironmentID: "e1", Function: "fee", Alias: "draft", Verifier: leaked}}, nil
}

// Verifies: STU-024.
// An app's setup is read by name, kind and the detail recreating it needs,
// never a value, a URL or a secret's hint; a kind the caller may not read
// is named as unread, and an installation without a gateway or functions
// leaves those kinds out (build log row 178).
func TestSetupReader(t *testing.T) {
	t.Parallel()
	r, err := NewSetupReader(SetupDeps{Tenancy: setupFake{}, Configs: setupFake{}, Gateway: gatewayFake{}, Functions: functionsFake{}})
	if err != nil {
		t.Fatal(err)
	}
	items, err := r.Setup(context.Background(), auth.Principal{}, "app")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(items)
	if strings.Contains(string(raw), leaked) || strings.Contains(string(raw), "CORE_SECRET") {
		t.Fatalf("the setup carries a value: %s", raw)
	}
	for _, want := range []SetupItem{
		{Kind: SetupEnvironment, Environment: "production", Name: "production", Detail: "production"},
		{Kind: SetupChannel, Environment: "development", Name: "beta-e1"},
		{Kind: SetupVariable, Environment: "production", Name: "API_BASE"},
		{Kind: SetupSecret, Environment: "development", Name: "PAYMENT_KEY"},
		{Kind: SetupUnread, Environment: "production", Name: SetupSecret},
		{Kind: SetupConfig, Environment: "production", Name: "feeLimit", Detail: "server-bound decimal sensitive"},
		{Kind: SetupUpstream, Environment: "production", Name: "core", Detail: "oauth"},
		{Kind: SetupIdentityProvider, Environment: "production"},
		{Kind: SetupFunction, Environment: "development", Name: "fee"},
		{Kind: SetupSchedule, Environment: "production", Name: "fee@live", Detail: "0 3 * * * Europe/Berlin"},
		{Kind: SetupHook, Environment: "development", Name: "fee@draft"},
	} {
		if !slices.Contains(items, want) {
			t.Errorf("no %+v in\n%s", want, raw)
		}
	}
	if slices.Contains(items, SetupItem{Kind: SetupIdentityProvider, Environment: "development"}) {
		t.Error("an environment without an identity provider names one")
	}
	if !slices.IsSortedFunc(items, func(a, b SetupItem) int { return strings.Compare(a.Environment, b.Environment) }) {
		t.Error("the setup is not in order")
	}

	bare, err := NewSetupReader(SetupDeps{Tenancy: setupFake{}})
	if err != nil {
		t.Fatal(err)
	}
	items, err = bare.Setup(context.Background(), auth.Principal{}, "app")
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if it.Kind == SetupConfig || it.Kind == SetupUpstream || it.Kind == SetupFunction || it.Kind == SetupSchedule {
			t.Errorf("a part the installation does not run is named: %+v", it)
		}
	}
}

// Verifies: STU-024.
// What is still to do is the archive's setup the app lacks, matched by
// kind, environment and name; an unread kind is always to do.
func TestSetupToDo(t *testing.T) {
	t.Parallel()
	want := []SetupItem{
		{Kind: SetupSecret, Environment: "production", Name: "PAYMENT_KEY"},
		{Kind: SetupEnvironment, Environment: "eu", Name: "eu"},
		{Kind: SetupEnvironment, Environment: "production", Name: "production", Detail: "production"},
		{Kind: SetupUnread, Environment: "production", Name: SetupConfig},
	}
	have := []SetupItem{{Kind: SetupEnvironment, Environment: "production", Name: "production"}}
	got := SetupToDo(want, have)
	if len(got) != 3 || got[0].Environment != "eu" || got[1].Kind != SetupSecret || got[2].Kind != SetupUnread {
		t.Errorf("to do: %+v", got)
	}
	if got := SetupToDo(nil, have); got == nil || len(got) != 0 {
		t.Errorf("nothing named: %+v", got)
	}
}

// Verifies: STU-024.
// An archive's setup is untrusted: an unknown kind, an item without its
// environment, a field too long or with a control character, and more
// items than the archive may hold entries are refused.
func TestCheckSetup(t *testing.T) {
	t.Parallel()
	ok := SetupItem{Kind: SetupSecret, Environment: "production", Name: "PAYMENT_KEY"}
	for name, c := range map[string]struct {
		items []SetupItem
		max   int64
		want  plxerr.Code
	}{
		"unknown kind":   {[]SetupItem{{Kind: "password", Environment: "production"}}, 10, plxerr.InvalidFormat},
		"no environment": {[]SetupItem{{Kind: SetupSecret, Name: "x"}}, 10, plxerr.InvalidFormat},
		"too long":       {[]SetupItem{{Kind: SetupSecret, Environment: "production", Name: strings.Repeat("a", maxSetupField+1)}}, 10, plxerr.InvalidFormat},
		"control":        {[]SetupItem{{Kind: SetupSecret, Environment: "production", Name: "a\nb"}}, 10, plxerr.InvalidFormat},
		"too many":       {[]SetupItem{ok, ok}, 1, plxerr.LimitExceeded},
	} {
		if code, _ := plxerr.CodeOf(checkSetup(c.items, c.max)); code != c.want {
			t.Errorf("%s: %v, want %v", name, code, c.want)
		}
	}
	if err := checkSetup([]SetupItem{ok}, 1); err != nil {
		t.Errorf("a good setup: %v", err)
	}
}
