// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package gateway_test

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/gateway"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

func (w *world) value(key string, tier gateway.ConfigTier, typ gateway.ConfigType, text string) gateway.ConfigValue {
	return gateway.ConfigValue{AppID: w.app, EnvironmentID: w.envs["production"], Key: key, Tier: tier, Type: typ, Value: text}
}

func epochOf(t *testing.T, w *world) string {
	t.Helper()
	raw, _, err := w.cache.Get(context.Background(), gateway.EpochKey(w.app, w.envs["production"]))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// Verifies: DAT-090, SEC-140.
// Values are created, read, changed, listed and deleted by those who manage
// the app; a viewer can read but not change; each change raises the
// value's version and the environment's epoch, announces the epoch and is
// audited without the value.
func TestConfigLifecycle(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := newWorld(t)

	pool := w.value("poolAccount", gateway.TierServerBound, gateway.TypeString, "POOL-42")
	created, err := w.configs.Set(ctx, w.owner, pool, nil)
	if err != nil || created.Version != 1 || created.Value != "POOL-42" {
		t.Fatalf("Set: %v %+v", err, created)
	}
	if got := epochOf(t, w); got != "1" {
		t.Errorf("epoch announced = %q", got)
	}
	if _, err := w.configs.Set(ctx, w.viewer, pool, nil); code(err) != plxerr.PermissionDenied {
		t.Errorf("a viewer setting: %v", err)
	}
	zero, one := int64(0), int64(1)
	if _, err := w.configs.Set(ctx, w.owner, pool, &zero); code(err) != plxerr.PreconditionFailed {
		t.Errorf("creating over an existing value: %v", err)
	}
	pool.Value = "POOL-43"
	changed, err := w.configs.Set(ctx, w.owner, pool, &one)
	if err != nil || changed.Version != 2 || changed.Value != "POOL-43" {
		t.Fatalf("change: %v %+v", err, changed)
	}
	if _, err := w.configs.Set(ctx, w.owner, pool, &one); code(err) != plxerr.PreconditionFailed {
		t.Errorf("a change from a stale read: %v", err)
	}
	if got := epochOf(t, w); got != "2" {
		t.Errorf("epoch announced = %q after a refused change, want 2", got)
	}

	fee := w.value("feeRate", gateway.TierLive, gateway.TypeDecimal, "0.015")
	fee.Strict = true
	if _, err := w.configs.Set(ctx, w.owner, fee, nil); err != nil {
		t.Fatal(err)
	}
	got, err := w.configs.Get(ctx, w.viewer, w.app, w.envs["production"], "feeRate")
	if err != nil || got.Value != "0.015" || !got.Strict || got.Tier != gateway.TierLive {
		t.Errorf("a viewer reading: %v %+v", err, got)
	}
	list, epoch, err := w.configs.List(ctx, w.viewer, w.app, w.envs["production"], "", 10)
	if err != nil || len(list) != 2 || epoch != 3 || list[0].Key != "feeRate" {
		t.Fatalf("List: %v %d %+v", err, epoch, list)
	}
	if rest, _, _ := w.configs.List(ctx, w.viewer, w.app, w.envs["production"], "feeRate", 10); len(rest) != 1 || rest[0].Key != "poolAccount" {
		t.Errorf("the page after feeRate: %+v", rest)
	}
	if other, _, _ := w.configs.List(ctx, w.viewer, w.app, w.envs["development"], "", 10); len(other) != 0 {
		t.Errorf("another environment sees %d values", len(other))
	}

	if err := w.configs.Delete(ctx, w.viewer, w.app, w.envs["production"], "poolAccount"); code(err) != plxerr.PermissionDenied {
		t.Errorf("a viewer deleting: %v", err)
	}
	if err := w.configs.Delete(ctx, w.owner, w.app, w.envs["production"], "poolAccount"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := w.configs.Delete(ctx, w.owner, w.app, w.envs["production"], "poolAccount"); code(err) != plxerr.ResourceNotFound {
		t.Errorf("deleting twice: %v", err)
	}
	if _, err := w.configs.Get(ctx, w.owner, w.app, w.envs["production"], "poolAccount"); code(err) != plxerr.ResourceNotFound {
		t.Errorf("reading a deleted value: %v", err)
	}
	if got := epochOf(t, w); got != "4" {
		t.Errorf("epoch announced = %q after the delete", got)
	}

	var details []string
	err = w.db.InTx(ctx, storage.Tenant{OrganizationID: w.org}, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT detail FROM audit_log WHERE action = ANY($1) ORDER BY sequence`,
			[]string{string(audit.ConfigValueSet), string(audit.ConfigValueDeleted)})
		if err != nil {
			return err //nolint:wrapcheck // a test
		}
		defer rows.Close()
		for rows.Next() {
			var d string
			if err := rows.Scan(&d); err != nil {
				return err //nolint:wrapcheck // a test
			}
			details = append(details, d)
		}
		return rows.Err() //nolint:wrapcheck // a test
	})
	if err != nil || len(details) != 4 {
		t.Fatalf("audit entries = %d, %v (two sets of one value, one of another, one deletion)", len(details), err)
	}
	for _, d := range details {
		if strings.Contains(d, "POOL-4") || strings.Contains(d, "0.015") {
			t.Errorf("an audit entry carries a value: %q", d)
		}
	}
}

// Verifies: DAT-090, SEC-106, SEC-092.
// A sensitive value is sealed at rest, never returned, and opened only for
// the gateway; the database holds no plaintext of it.
func TestSensitiveValuesAreSealed(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := newWorld(t)
	v := w.value("settlementAccount", gateway.TierServerBound, gateway.TypeString, "NL91ABNA0417164300")
	v.Sensitive = true
	set, err := w.configs.Set(ctx, w.owner, v, nil)
	if err != nil {
		t.Fatal(err)
	}
	if set.Version != 1 {
		t.Errorf("version = %d", set.Version)
	}
	got, err := w.configs.Get(ctx, w.owner, w.app, w.envs["production"], "settlementAccount")
	if err != nil || !got.Sensitive {
		t.Fatalf("Get: %v %+v", err, got)
	}
	if got.Value != "" {
		t.Errorf("Get returned the content of a sensitive value")
	}
	list, _, _ := w.configs.List(ctx, w.owner, w.app, w.envs["production"], "", 10)
	if len(list) != 1 || list[0].Value != "" {
		t.Errorf("List returned %+v", list)
	}
	var plain, sealed bool
	err = w.db.InTx(ctx, storage.Tenant{OrganizationID: w.org}, func(ctx context.Context, tx pgx.Tx) error {
		var value string
		var ciphertext []byte
		if err := tx.QueryRow(ctx, `SELECT value, ciphertext FROM config_values WHERE key = 'settlementAccount'`).Scan(&value, &ciphertext); err != nil {
			return err //nolint:wrapcheck // a test
		}
		plain = value != "" || strings.Contains(string(ciphertext), "NL91")
		sealed = len(ciphertext) > 0
		return nil
	})
	if err != nil || plain || !sealed {
		t.Errorf("stored plaintext = %v, sealed = %v, %v", plain, sealed, err)
	}
	cfg, err := w.configs.Load(ctx, w.org, w.envs["production"])
	if err != nil || len(cfg.Values) != 1 || cfg.Values[0].Text != "NL91ABNA0417164300" || !cfg.Values[0].Sensitive {
		t.Errorf("Load: %v %+v", err, cfg)
	}
	live := w.value("limit", gateway.TierLive, gateway.TypeInteger, "5")
	live.Sensitive = true
	if _, err := w.configs.Set(ctx, w.owner, live, nil); code(err) != plxerr.InvalidFormat {
		t.Errorf("a sensitive live value: %v", err)
	}
}

// Verifies: DAT-090, DAT-096.
// The strict and live epochs follow the values: only a change to a live
// value moves the live epoch, only a strict one the strict epoch, and
// removing a strict value counts as a strict change.
func TestConfigEpochs(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := newWorld(t)
	env := w.envs["production"]
	set := func(v gateway.ConfigValue) {
		t.Helper()
		if _, err := w.configs.Set(ctx, w.owner, v, nil); err != nil {
			t.Fatal(err)
		}
	}
	load := func() (int64, int64, int64) {
		t.Helper()
		c, err := w.configs.Load(ctx, w.org, env)
		if err != nil {
			t.Fatal(err)
		}
		return c.Epoch, c.LiveEpoch, c.StrictEpoch
	}
	if e, l, s := load(); e != 0 || l != 0 || s != 0 {
		t.Fatalf("an empty environment: %d %d %d", e, l, s)
	}
	set(w.value("pool", gateway.TierServerBound, gateway.TypeString, "a"))
	if e, l, s := load(); e != 1 || l != 0 || s != 0 {
		t.Errorf("after a server-bound value: %d %d %d", e, l, s)
	}
	set(w.value("banner", gateway.TierLive, gateway.TypeString, "hi"))
	if e, l, s := load(); e != 2 || l != 2 || s != 0 {
		t.Errorf("after a live value: %d %d %d", e, l, s)
	}
	strict := w.value("fee", gateway.TierLive, gateway.TypeDecimal, "0.01")
	strict.Strict = true
	set(strict)
	set(w.value("pool", gateway.TierServerBound, gateway.TypeString, "b"))
	if e, l, s := load(); e != 4 || l != 3 || s != 3 {
		t.Errorf("after a strict value and a bound one: %d %d %d", e, l, s)
	}
	if err := w.configs.Delete(ctx, w.owner, w.app, env, "fee"); err != nil {
		t.Fatal(err)
	}
	if e, l, s := load(); e != 5 || l != 5 || s != 5 {
		t.Errorf("after removing a strict value: %d %d %d", e, l, s)
	}
	if n, err := w.configs.Epoch(ctx, w.org, env); err != nil || n != 5 {
		t.Errorf("Epoch = %d, %v", n, err)
	}
	if e, err := w.configs.Epoch(ctx, w.org, w.envs["development"]); err != nil || e != 0 {
		t.Errorf("Epoch of another environment = %d, %v", e, err)
	}
}

// Verifies: DAT-090, LIM-001.
// An environment holds at most config.valuesPerEnvironment values, each at
// most config.valueBytes.
func TestConfigLimits(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := newWorld(t)
	big := w.value("big", gateway.TierLive, gateway.TypeString, strings.Repeat("x", 4097))
	if _, err := w.configs.Set(ctx, w.owner, big, nil); code(err) != plxerr.LimitExceeded {
		t.Errorf("a value over config.valueBytes: %v", err)
	}
	for i := range 256 {
		v := w.value("k"+strconv.Itoa(i), gateway.TierLive, gateway.TypeString, "v")
		if _, err := w.configs.Set(ctx, w.owner, v, nil); err != nil {
			t.Fatalf("value %d: %v", i, err)
		}
	}
	over := w.value("over", gateway.TierLive, gateway.TypeString, "v")
	if _, err := w.configs.Set(ctx, w.owner, over, nil); code(err) != plxerr.LimitExceeded {
		t.Errorf("the 257th value: %v", err)
	}
	again := w.value("k0", gateway.TierLive, gateway.TypeString, "w")
	if _, err := w.configs.Set(ctx, w.owner, again, nil); err != nil {
		t.Errorf("changing a value of a full environment: %v", err)
	}
	if epochOf(t, w) != "257" {
		t.Errorf("epoch = %s, want 257 (a refused change announces nothing)", epochOf(t, w))
	}
}

// Verifies: DAT-092.
// A resolver on the database source sees a change at once, through the
// announced epoch.
func TestResolverOnTheDatabase(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := newWorld(t)
	r := &gateway.ConfigResolver{Source: w.configs, Cache: w.cache}
	env := w.envs["production"]
	snap, err := r.Snapshot(ctx, w.org, w.app, env)
	if err != nil || snap.Epoch != 0 || len(snap.Live()) != 0 {
		t.Fatalf("an empty environment: %v %+v", err, snap)
	}
	v := w.value("banner", gateway.TierLive, gateway.TypeString, "one")
	if _, err := w.configs.Set(ctx, w.owner, v, nil); err != nil {
		t.Fatal(err)
	}
	snap, err = r.Snapshot(ctx, w.org, w.app, env)
	if err != nil || snap.Epoch != 1 || len(snap.Live()) != 1 || snap.Live()[0].Text != "one" {
		t.Fatalf("after a change: %v %+v", err, snap)
	}
	v.Value = "two"
	if _, err := w.configs.Set(ctx, w.owner, v, nil); err != nil {
		t.Fatal(err)
	}
	if snap, err = r.Snapshot(ctx, w.org, w.app, env); err != nil || snap.Live()[0].Text != "two" || snap.Live()[0].Version != 2 {
		t.Errorf("a changed value is not seen at once: %v %+v", err, snap.Live())
	}
}
