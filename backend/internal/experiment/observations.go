// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package experiment

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/experiment/stats"
	"github.com/nightCode42/plux3/backend/internal/schema"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// preExposureDays is the length of the pre-exposure window whose behaviour
// is the CUPED covariate (ABT-003, ADR-0079).
const preExposureDays = 14

// MetricQuery asks for the statistics of one metric of one experiment.
type MetricQuery struct {
	OrganizationID, AppID, EnvironmentID string
	// Experiment is the experiment's key, as the exposure events carry it.
	Experiment string
	// Event is the metric's event name; Property the numeric field a mean
	// averages. Kind says how the outcome of a subject is formed: whether it
	// did the event, or the sum of the property over its events.
	Event, Property string
	Kind            schema.MetricKind
	// Since and Until bound the first exposure of a subject; the outcome is
	// counted up to Until.
	Since, Until time.Time
	// PreExposureDays is the length of the covariate window.
	PreExposureDays int
}

// VariantMoments are the statistics of one variant.
type VariantMoments struct {
	Variant string
	Moments stats.Moments
}

// Observations supplies the per-variant statistics that analysis compares
// (ABT-003). A subject is a device: it counts in the variant of its first
// exposure and its outcome is formed from its events from then on. The
// statistics, not the events, cross this boundary.
type Observations interface {
	Moments(ctx context.Context, q MetricQuery) ([]VariantMoments, error)
}

// PostgresObservations reads the statistics from the raw events table. It is
// not built on the rollups: a rollup holds counts and sums per bucket, while
// the variance of an outcome and its relation to the covariate need the
// events of each subject.
type PostgresObservations struct {
	DB *storage.DB
}

// Moments implements Observations.
func (o PostgresObservations) Moments(ctx context.Context, q MetricQuery) ([]VariantMoments, error) {
	if o.DB == nil {
		return nil, errors.New("experiment: no database to read events from")
	}
	app, err := parseID(q.AppID, "app")
	if err != nil {
		return nil, err
	}
	env, err := parseID(q.EnvironmentID, "environment")
	if err != nil {
		return nil, err
	}
	kind := string(q.Kind)
	if kind == "" {
		kind = string(schema.MetricKindProportion)
	}
	var rows []dbgen.SummariseExperimentMetricRow
	err = o.DB.InTx(ctx, storage.Tenant{OrganizationID: q.OrganizationID}, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		rows, err = dbgen.New(tx).SummariseExperimentMetric(ctx, dbgen.SummariseExperimentMetricParams{
			AppID: app, EnvironmentID: env, Experiment: q.Experiment, Since: storage.Timestamp(q.Since), Until: storage.Timestamp(q.Until),
			Property: q.Property, Metric: q.Event, PreDays: int32(q.PreExposureDays), Kind: kind, //nolint:gosec // a few days
		})
		return err //nolint:wrapcheck // one statement
	})
	if err != nil {
		return nil, failure(err, "events")
	}
	out := make([]VariantMoments, len(rows))
	for i, r := range rows {
		out[i] = VariantMoments{Variant: r.Variant, Moments: stats.Moments{
			N: float64(r.Subjects), SumY: r.SumY, SumYY: r.SumYy, SumX: r.SumX, SumXX: r.SumXx, SumXY: r.SumXy,
		}}
	}
	return out, nil
}
