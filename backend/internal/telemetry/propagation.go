// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package telemetry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// controlApplied is the event a device reports once it has applied a
// control document; its "version" field names the document (ADR-0077).
const controlApplied = "control_applied"

// ControlChanges tells when a control document came into being.
type ControlChanges interface {
	// ChangedAt returns when the document of the given version was changed
	// on the source's environment. It reports false when no such document
	// exists or a newer one has replaced it, so that a device catching up
	// late on an old version does not skew the measurement.
	ChangedAt(ctx context.Context, src Source, version int64) (time.Time, bool, error)
}

// Propagation records how long a control change took to reach a device.
// observability.Metrics implements it.
type Propagation interface {
	ControlPropagated(app string, d time.Duration)
}

// PostgresControlChanges reads change times from the control documents.
type PostgresControlChanges struct{ DB *storage.DB }

// ChangedAt implements ControlChanges.
func (c PostgresControlChanges) ChangedAt(ctx context.Context, src Source, version int64) (time.Time, bool, error) {
	var at time.Time
	err := c.DB.InTx(ctx, storage.Tenant{OrganizationID: src.OrganizationID}, func(ctx context.Context, tx pgx.Tx) error {
		ts, err := dbgen.New(tx).GetControlChangeTime(ctx, dbgen.GetControlChangeTimeParams{
			EnvironmentID: storage.MustUUID(src.EnvironmentID), Version: version,
		})
		if err != nil {
			return err //nolint:wrapcheck // wrapped below
		}
		at = ts.Time
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("telemetry.ChangedAt: %w", err)
	}
	return at, true, nil
}

// measurePropagation records, for every stored control_applied event, the
// time from the change of the version it names to the moment the device
// reported it (NFR-024, OBS-005). It is best effort: a failed lookup costs
// a sample, never the batch. The device's own clock is trusted only when it
// falls between the change and now; otherwise the arrival is the moment.
func (s *Service) measurePropagation(ctx context.Context, m message) {
	if s.o.Propagation == nil || s.o.Changes == nil {
		return
	}
	for _, q := range m.Events {
		version, ok := appliedVersion(q)
		if !ok {
			continue
		}
		changed, found, err := s.o.Changes.ChangedAt(ctx, m.Source, version)
		if err != nil || !found {
			continue
		}
		reported := q.Time
		if now := s.now(); reported.Before(changed) || reported.After(now) {
			reported = now
		}
		s.o.Propagation.ControlPropagated(m.Source.AppID, reported.Sub(changed))
	}
}

// appliedVersion returns the control document version of a control_applied
// event.
func appliedVersion(q queued) (int64, bool) {
	if q.Name != controlApplied {
		return 0, false
	}
	var f struct {
		Version *int64 `json:"version"`
	}
	if json.Unmarshal(q.Fields, &f) != nil || f.Version == nil {
		return 0, false
	}
	return *f.Version, true
}
