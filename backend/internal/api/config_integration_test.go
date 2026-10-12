// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api_test

import (
	"strconv"
	"testing"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/api"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
)

// Verifies: DAT-090.
// An administrator sets, reads, lists and deletes server-held values over
// the API; a sensitive value's content is never in a response, a stale
// expected version is refused, and an anonymous caller is refused.
func TestConfigAdminService(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	ctx := t.Context()
	admin, app, envs := w.appOnly(t)
	prod := envs["production"]

	fee := &pluxv1.ConfigValue{Key: "feeRate", Tier: pluxv1.ConfigTier_CONFIG_TIER_LIVE, Type: pluxv1.ConfigType_CONFIG_TYPE_DECIMAL, Strict: true, Value: "0.015"}
	set := must(w.secAdmin.SetConfigValue(ctx, req(admin, &pluxv1.SetConfigValueRequest{AppId: app, EnvironmentId: prod, Value: fee})))(t)
	if v := set.GetValue(); v.GetVersion() != 1 || v.GetValue() != "0.015" || !v.GetStrict() || v.GetUpdatedAt() == nil {
		t.Fatalf("SetConfigValue = %+v", v)
	}
	pool := &pluxv1.ConfigValue{Key: "poolAccount", Tier: pluxv1.ConfigTier_CONFIG_TIER_SERVER_BOUND, Type: pluxv1.ConfigType_CONFIG_TYPE_STRING, Sensitive: true, Value: "NL91ABNA0417164300"}
	set = must(w.secAdmin.SetConfigValue(ctx, req(admin, &pluxv1.SetConfigValueRequest{AppId: app, EnvironmentId: prod, Value: pool})))(t)
	if set.GetValue().GetValue() != "" || !set.GetValue().GetSensitive() {
		t.Errorf("a sensitive value in the response of its own set: %+v", set.GetValue())
	}
	got := must(w.secAdmin.GetConfigValue(ctx, req(admin, &pluxv1.GetConfigValueRequest{AppId: app, EnvironmentId: prod, Key: "poolAccount"})))(t)
	if got.GetValue().GetValue() != "" || got.GetValue().GetVersion() != 1 {
		t.Errorf("GetConfigValue = %+v", got.GetValue())
	}
	list := must(w.secAdmin.ListConfigValues(ctx, req(admin, &pluxv1.ListConfigValuesRequest{AppId: app, EnvironmentId: prod})))(t)
	if len(list.GetValues()) != 2 || list.GetEpoch() != 2 || list.GetValues()[0].GetKey() != "feeRate" || list.GetValues()[1].GetValue() != "" {
		t.Errorf("ListConfigValues = %+v", list)
	}
	first := must(w.secAdmin.ListConfigValues(ctx, req(admin, &pluxv1.ListConfigValuesRequest{AppId: app, EnvironmentId: prod, Page: &pluxv1.Page{PageSize: 1}})))(t)
	next := must(w.secAdmin.ListConfigValues(ctx, req(admin, &pluxv1.ListConfigValuesRequest{AppId: app, EnvironmentId: prod, Page: &pluxv1.Page{PageSize: 1, PageToken: first.GetPage().GetNextPageToken()}})))(t)
	if len(first.GetValues()) != 1 || len(next.GetValues()) != 1 || next.GetValues()[0].GetKey() != "poolAccount" {
		t.Errorf("pages: %+v %+v", first, next)
	}

	zero := int64(0)
	_, err := w.secAdmin.SetConfigValue(ctx, req(admin, &pluxv1.SetConfigValueRequest{AppId: app, EnvironmentId: prod, Value: fee, ExpectedVersion: &zero}))
	if codeOf(err) != connect.CodeFailedPrecondition {
		t.Errorf("a stale expected version: %v", err)
	}
	live := &pluxv1.ConfigValue{Key: "x", Tier: pluxv1.ConfigTier_CONFIG_TIER_LIVE, Type: pluxv1.ConfigType_CONFIG_TYPE_INTEGER, Sensitive: true, Value: "1"}
	if _, err := w.secAdmin.SetConfigValue(ctx, req(admin, &pluxv1.SetConfigValueRequest{AppId: app, EnvironmentId: prod, Value: live})); codeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("a sensitive live value: %v", err)
	}
	if _, err := w.secAdmin.SetConfigValue(ctx, connect.NewRequest(&pluxv1.SetConfigValueRequest{AppId: app, EnvironmentId: prod, Value: fee})); codeOf(err) != connect.CodeUnauthenticated {
		t.Errorf("an anonymous set: %v", err)
	}
	must(w.secAdmin.DeleteConfigValue(ctx, req(admin, &pluxv1.DeleteConfigValueRequest{AppId: app, EnvironmentId: prod, Key: "feeRate"})))(t)
	if _, err := w.secAdmin.GetConfigValue(ctx, req(admin, &pluxv1.GetConfigValueRequest{AppId: app, EnvironmentId: prod, Key: "feeRate"})); codeOf(err) != connect.CodeNotFound {
		t.Errorf("a deleted value: %v", err)
	}
}

// Verifies: DAT-094, DAT-095.
// A device reads the live values of its own environment over its
// DPoP-bound channel, with the version and epoch, and never a server-bound
// or a sensitive value or another environment's; a call without a proof is
// refused.
func TestLiveConfigServesDevices(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	ctx := t.Context()
	admin, app, envs := w.appOnly(t)
	dev := w.registerDevelopment(t, app)
	dev.refresh(t)
	set := func(env string, v *pluxv1.ConfigValue) {
		t.Helper()
		must(w.secAdmin.SetConfigValue(ctx, req(admin, &pluxv1.SetConfigValueRequest{AppId: app, EnvironmentId: envs[env], Value: v})))(t)
	}
	set("development", &pluxv1.ConfigValue{Key: "banner", Tier: pluxv1.ConfigTier_CONFIG_TIER_LIVE, Type: pluxv1.ConfigType_CONFIG_TYPE_STRING, Value: "hello"})
	set("development", &pluxv1.ConfigValue{Key: "feeRate", Tier: pluxv1.ConfigTier_CONFIG_TIER_LIVE, Type: pluxv1.ConfigType_CONFIG_TYPE_DECIMAL, Strict: true, Value: "0.015"})
	set("development", &pluxv1.ConfigValue{Key: "pool", Tier: pluxv1.ConfigTier_CONFIG_TIER_SERVER_BOUND, Type: pluxv1.ConfigType_CONFIG_TYPE_STRING, Value: "POOL-1"})
	set("production", &pluxv1.ConfigValue{Key: "other", Tier: pluxv1.ConfigTier_CONFIG_TIER_LIVE, Type: pluxv1.ConfigType_CONFIG_TYPE_STRING, Value: "x"})

	res, err := w.config.GetLiveConfig(ctx, bound(t, dev, pluxv1connect.ConfigServiceGetLiveConfigProcedure, &pluxv1.GetLiveConfigRequest{}))
	if err != nil {
		t.Fatalf("GetLiveConfig: %v", err)
	}
	m := res.Msg
	if m.GetVersion() != 2 || m.GetEpoch() != 3 || len(m.GetValues()) != 2 {
		t.Fatalf("response = %+v", m)
	}
	if v := m.GetValues()[0]; v.GetKey() != "banner" || v.GetValue() != "hello" || v.GetType() != pluxv1.ConfigType_CONFIG_TYPE_STRING || v.GetStrict() {
		t.Errorf("banner = %+v", v)
	}
	if v := m.GetValues()[1]; v.GetKey() != "feeRate" || !v.GetStrict() || v.GetVersion() != 1 {
		t.Errorf("feeRate = %+v", v)
	}
	if got, want := res.Header().Get(api.HeaderConfigEpoch), strconv.Itoa(3); got != want {
		t.Errorf("%s = %q, want %q", api.HeaderConfigEpoch, got, want)
	}
	if _, err := w.config.GetLiveConfig(ctx, connect.NewRequest(&pluxv1.GetLiveConfigRequest{})); codeOf(err) != connect.CodeUnauthenticated {
		t.Errorf("a call with no credentials: %v", err)
	}
	if _, err := w.config.GetLiveConfig(ctx, req(admin, &pluxv1.GetLiveConfigRequest{})); err == nil {
		t.Error("a person's session read the device channel")
	}
}
