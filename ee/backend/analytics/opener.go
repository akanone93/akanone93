// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package analytics

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"

	"github.com/nightCode42/plux3/backend/edition"
	ext "github.com/nightCode42/plux3/backend/ext/analytics"
)

// URLVariable is the environment variable that names the ClickHouse server
// (ANL-010): a DSN such as clickhouse://user:password@host:9000/plux. Unset,
// the installation keeps PostgreSQL.
const URLVariable = "PLUX_ANALYTICS_CLICKHOUSE_URL"

// errBadURL says the configured URL is not a ClickHouse DSN. It does not
// quote the URL, which holds the password.
var errBadURL = errors.New("analytics: " + URLVariable + " is not a valid ClickHouse URL")

// Opener opens the ClickHouse store. It implements ext.Opener.
type Opener struct {
	// URL is the ClickHouse DSN; empty leaves the core on PostgreSQL.
	URL string
	// Now is the clock; nil uses time.Now.
	Now func() time.Time
}

var _ ext.Opener = Opener{}

// OpenerFromEnv returns the opener configured by URLVariable.
func OpenerFromEnv(getenv func(string) string) Opener {
	return Opener{URL: getenv(URLVariable)}
}

// Open connects to ClickHouse and creates the tables. It returns no store,
// and no error, when no URL is configured or the licence in ctx does not
// allow edition.FeatureClickHouse, so that the core keeps its PostgreSQL
// store; the second case is logged as a warning.
func (o Opener) Open(ctx context.Context, s ext.Settings) (ext.Store, error) {
	if o.URL == "" {
		return nil, nil //nolint:nilnil // no store: the core keeps PostgreSQL
	}
	if !edition.EnabledForWrite(ctx, edition.FeatureClickHouse) {
		s.Log.Warn("the licence does not allow ClickHouse analytics storage; analytics stay in PostgreSQL",
			"variable", URLVariable, "feature", string(edition.FeatureClickHouse))
		return nil, nil //nolint:nilnil // no store: the core keeps PostgreSQL
	}
	opts, err := clickhouse.ParseDSN(o.URL)
	if err != nil {
		return nil, errBadURL
	}
	db, err := clickhouse.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("analytics: open ClickHouse: %w", err)
	}
	store := newStore(db, o.Now)
	if err := db.Ping(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("analytics: reach ClickHouse: %w", err)
	}
	if err := store.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}
