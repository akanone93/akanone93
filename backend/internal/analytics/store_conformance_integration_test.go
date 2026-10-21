// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package analytics_test

import (
	"context"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/ext/analytics/storetest"
	"github.com/nightCode42/plux3/backend/internal/analytics/analyticstest"
)

// subject is the fixture's PostgreSQL store as a conformance subject.
func subject(f *analyticstest.Fixture) storetest.Subject {
	return storetest.Subject{
		Store: f.Store, Organization: f.Org, App: f.App, Environment: f.Env, Device: f.Device,
		Rollup: func(ctx context.Context) error {
			// Events carry the wall clock as their receipt time.
			return f.Store.Rollup(ctx, f.Org, time.Now().Add(time.Hour))
		},
	}
}

// Verifies: ANL-010.
// The PostgreSQL store answers the conformance queries of the public
// analytics package with the committed series, which the enterprise
// ClickHouse store must reproduce.
func TestStoreConformance_ANL_010(t *testing.T) {
	f := analyticstest.New(t, analyticstest.Options{})
	storetest.Conformance(t, subject(f))
}
