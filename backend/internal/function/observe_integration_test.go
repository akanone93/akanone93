// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package function_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/function"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

func (w *world) meta(version int32) function.LogMeta {
	return function.LogMeta{
		OrganizationID: w.org, AppID: w.app, EnvironmentID: w.envs["development"], Function: "bank.fee", Version: version,
		Traceparent: "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
	}
}

func (w *world) query(version int32) function.LogQuery {
	return function.LogQuery{AppID: w.app, EnvironmentID: w.envs["development"], Name: "bank.fee", Version: version}
}

// Verifies: FN-032.
func TestFunctionLogs_FN_032(t *testing.T) {
	w := newWorld(t, nil, nil)
	ctx := context.Background()
	base := time.Now().UTC().Add(-time.Minute)
	for v := int32(1); v <= 2; v++ {
		var lines []function.LogLine
		for n := 0; n < 3; n++ {
			lines = append(lines, function.LogLine{Level: uint32(n), Message: fmt.Sprintf("v%d line %d", v, n), Time: base.Add(time.Duration(int(v)*10+n) * time.Second)})
		}
		w.svc.Record(ctx, w.meta(v), lines)
	}

	var got []string
	var after storage.Cursor
	for {
		page, last, err := w.svc.ListLogs(ctx, w.viewer, w.query(0), after, 4)
		if err != nil {
			t.Fatalf("ListLogs: %v", err)
		}
		for _, l := range page {
			got = append(got, l.Message)
		}
		if len(page) < 4 {
			break
		}
		after = last
	}
	want := []string{"v2 line 2", "v2 line 1", "v2 line 0", "v1 line 2", "v1 line 1", "v1 line 0"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("newest first across pages: got %v, want %v", got, want)
	}

	one, _, err := w.svc.ListLogs(ctx, w.viewer, w.query(1), storage.Cursor{}, 10)
	if err != nil || len(one) != 3 || one[0].Version != 1 || one[0].Level != "warn" || one[2].Level != "debug" {
		t.Fatalf("version filter: %v %+v", err, one)
	}
	if one[0].TraceID != "0af7651916cd43dd8448eb211c80319c" {
		t.Errorf("trace id %q", one[0].TraceID)
	}

	q := w.query(0)
	q.Since = base.Add(20 * time.Second)
	recent, _, err := w.svc.ListLogs(ctx, w.viewer, q, storage.Cursor{}, 10)
	if err != nil || len(recent) != 3 {
		t.Fatalf("since filter: %v %d lines", err, len(recent))
	}
}

// Verifies: FN-032, SRV-022.
func TestFunctionLogsAreTenantAndAppScoped_FN_032(t *testing.T) {
	w := newWorld(t, nil, nil)
	ctx := context.Background()
	w.svc.Record(ctx, w.meta(1), []function.LogLine{{Level: 1, Message: "hello", Time: time.Now()}})

	q := w.query(0)
	q.AppID, q.EnvironmentID = w.otherApp, w.otherEnvs["development"]
	other, _, err := w.svc.ListLogs(ctx, w.owner, q, storage.Cursor{}, 10)
	if err != nil || len(other) != 0 {
		t.Fatalf("another app sees %d lines (%v)", len(other), err)
	}
	q = w.query(0)
	q.EnvironmentID = w.otherEnvs["development"]
	if _, _, err := w.svc.ListLogs(ctx, w.owner, q, storage.Cursor{}, 10); err == nil {
		t.Error("an environment of another app was accepted")
	}
	if _, _, err := w.svc.ListLogs(ctx, auth.Principal{OrganizationID: w.org}, w.query(0), storage.Cursor{}, 10); err == nil {
		t.Error("a principal without function.read listed logs")
	} else if c, _ := plxerr.CodeOf(err); c == 0 {
		t.Errorf("not a domain error: %v", err)
	}
}

// Verifies: FN-032.
func TestFunctionMetrics_FN_032(t *testing.T) {
	w := newWorld(t, nil, nil)
	ctx := context.Background()
	obs := func(result string, micros int64, replayed bool) {
		w.svc.Observe(ctx, function.Observation{
			OrganizationID: w.org, AppID: w.app, EnvironmentID: w.envs["development"], Function: "bank.fee", Version: 1,
			Result: result, Duration: time.Duration(micros) * time.Microsecond, MemoryPeakBytes: uint64(micros) * 1024, Replayed: replayed,
		})
	}
	for n := int64(1); n <= 100; n++ {
		obs("ok", n*1000, false)
	}
	obs("business_error", 500, false)
	obs("function_timeout", 9000000, false)
	obs("ok", 0, true)
	obs("ok", 0, true)

	m, err := w.svc.Metrics(ctx, w.viewer, w.app, w.envs["development"], "bank.fee", time.Hour)
	if err != nil {
		t.Fatalf("Metrics: %v", err)
	}
	if m.Invocations != 104 || m.CacheHits != 2 || m.ErrorsByKind["function_timeout"] != 1 || len(m.ErrorsByKind) != 1 {
		t.Errorf("counts: %+v", m)
	}
	if m.LatencyP50 < 49*time.Millisecond || m.LatencyP50 > 52*time.Millisecond || m.LatencyP99 < 98*time.Millisecond {
		t.Errorf("percentiles %v %v %v", m.LatencyP50, m.LatencyP95, m.LatencyP99)
	}
	if m.MemoryPeakBytes != 9000000*1024 {
		t.Errorf("memory peak %d", m.MemoryPeakBytes)
	}
	none, err := w.svc.Metrics(ctx, w.viewer, w.app, w.envs["development"], "other.fn", time.Hour)
	if err != nil || none.Invocations != 0 {
		t.Errorf("another function: %v %+v", err, none)
	}
}

// Verifies: FN-032.
func TestFunctionObservationsArePurged_FN_032(t *testing.T) {
	w := newWorld(t, nil, nil)
	ctx := context.Background()
	w.svc.Record(ctx, w.meta(1), []function.LogLine{{Message: "old", Time: time.Now()}})
	w.svc.Observe(ctx, function.Observation{OrganizationID: w.org, AppID: w.app, EnvironmentID: w.envs["development"], Function: "bank.fee", Version: 1, Result: "ok"})
	if n, err := w.svc.PurgeObservations(ctx, w.org); err != nil || n != 0 {
		t.Fatalf("a fresh row was purged: %d %v", n, err)
	}
	err := w.db.InTx(ctx, storage.Tenant{OrganizationID: w.org}, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE function_logs SET logged_at = now() - interval '90 days'`); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE function_invocations SET finished_at = now() - interval '90 days'`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if n, err := w.svc.PurgeObservations(ctx, w.org); err != nil || n != 2 {
		t.Fatalf("purged %d rows (%v), want 2", n, err)
	}
}
