// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package rollout

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// Window is a new schedule for a rollout. A zero Start keeps the start; a zero
// End keeps the end unless ClearEnd removes it.
type Window struct {
	Start, End time.Time
	ClearEnd   bool
}

// AdvanceNow moves an active rollout to its next stage at once, without
// waiting for the stage's duration (REL-053). It is idempotent in effect: a
// rollout that another caller has already moved on is refused (PLX-1030), and
// one on its last stage is refused (PLX-1006), since finishing it is the
// worker's job once the stage has soaked.
func (s *Service) AdvanceNow(ctx context.Context, p auth.Principal, id string) (Rollout, error) {
	return s.change(ctx, p, id, func(ctx context.Context, tx pgx.Tx, row dbgen.Rollout) (dbgen.Rollout, error) {
		if State(row.State) != StateActive {
			return dbgen.Rollout{}, plxerr.New(plxerr.RevisionConflict, "the rollout is %s and cannot advance", row.State)
		}
		stages, err := decodeStages(row.Stages)
		if err != nil {
			return dbgen.Rollout{}, err
		}
		next := int(row.StageIndex) + 1
		if next >= len(stages) {
			return dbgen.Rollout{}, plxerr.New(plxerr.OutOfRange, "the rollout is on its last stage, stage %d of %d", len(stages), len(stages))
		}
		moved, err := dbgen.New(tx).AdvanceRollout(ctx, dbgen.AdvanceRolloutParams{
			ID: row.ID, StageIndex: int32(next), StageStartedAt: storage.Timestamp(s.now()), //nolint:gosec // an index into the stages
			BasisPoints: int32(stages[next].BasisPoints), UpdatedAt: storage.Timestamp(s.now()), //nolint:gosec // validated to at most 10000
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return dbgen.Rollout{}, plxerr.New(plxerr.RevisionConflict, "the rollout was moved on by another caller")
		}
		if err != nil {
			return dbgen.Rollout{}, fmt.Errorf("rollout: advance: %w", err)
		}
		return moved, s.record(ctx, tx, p.Actor(), p.OrganizationID, audit.RolloutAdvanced, moved,
			fmt.Sprintf("advanced by %s to stage %d of %d: %d basis points", p.Actor().Display, next+1, len(stages), stages[next].BasisPoints))
	})
}

// Reschedule changes when a rollout starts and ends (REL-052). Only a
// scheduled rollout may change its start; one that has begun may change its
// end, which must lie in the future. PLX-3153 refuses a window that meets
// another open rollout of the channel.
func (s *Service) Reschedule(ctx context.Context, p auth.Principal, id string, w Window) (Rollout, error) {
	return s.change(ctx, p, id, func(ctx context.Context, tx pgx.Tx, row dbgen.Rollout) (dbgen.Rollout, error) {
		start, end, err := s.newWindow(row, w)
		if err != nil {
			return dbgen.Rollout{}, err
		}
		q := dbgen.New(tx)
		if err := s.requireOtherWindowFree(ctx, q, row, start, end); err != nil {
			return dbgen.Rollout{}, err
		}
		moved, err := q.RescheduleRollout(ctx, dbgen.RescheduleRolloutParams{
			ID: row.ID, StartAt: storage.Timestamp(start), EndAt: end, At: storage.Timestamp(s.now()),
			FromStates: statesOf([]State{StateScheduled, StateActive, StatePaused}),
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return dbgen.Rollout{}, plxerr.New(plxerr.RevisionConflict, "the rollout is %s and cannot be rescheduled", row.State)
		}
		if err != nil {
			return dbgen.Rollout{}, fmt.Errorf("rollout: reschedule: %w", err)
		}
		return moved, s.record(ctx, tx, p.Actor(), p.OrganizationID, audit.RolloutRescheduled, moved,
			fmt.Sprintf("rescheduled by %s from %s to %s", p.Actor().Display, windowText(storage.Time(row.StartAt), row.EndAt), windowText(start, end)))
	})
}

// newWindow validates a requested window against the rollout's state.
func (s *Service) newWindow(row dbgen.Rollout, w Window) (time.Time, pgtype.Timestamptz, error) {
	start, end := storage.Time(row.StartAt), row.EndAt
	switch State(row.State) {
	case StateScheduled, StateActive, StatePaused:
	default:
		return time.Time{}, end, plxerr.New(plxerr.RevisionConflict, "the rollout is %s and cannot be rescheduled", row.State)
	}
	if !w.Start.IsZero() && !w.Start.Equal(start) {
		if State(row.State) != StateScheduled {
			return time.Time{}, end, plxerr.New(plxerr.OutOfRange, "the rollout has started, so only its end can change")
		}
		start = w.Start
	}
	switch {
	case w.ClearEnd:
		end = pgtype.Timestamptz{}
	case !w.End.IsZero():
		end = storage.Timestamp(w.End)
	}
	if end.Valid && !storage.Time(end).After(start) {
		return time.Time{}, end, plxerr.New(plxerr.OutOfRange, "the end of a rollout must come after its start")
	}
	if end.Valid && State(row.State) != StateScheduled && !storage.Time(end).After(s.now()) {
		return time.Time{}, end, plxerr.New(plxerr.OutOfRange, "the end of a rollout that has started must lie in the future")
	}
	return start, end, nil
}

// requireOtherWindowFree is requireWindowFree that ignores the rollout itself.
func (*Service) requireOtherWindowFree(ctx context.Context, q *dbgen.Queries, row dbgen.Rollout, start time.Time, end pgtype.Timestamptz) error {
	overlaps, err := q.ListOverlappingRollouts(ctx, dbgen.ListOverlappingRolloutsParams{
		ChannelID: row.ChannelID, WindowStart: storage.Timestamp(start), WindowEnd: end,
	})
	if err != nil {
		return fmt.Errorf("rollout: overlaps: %w", err)
	}
	for _, o := range overlaps {
		if o.ID != row.ID {
			return plxerr.New(plxerr.ScheduleOverlapsRollout, "the rollout %s of this channel is %s from %s; schedule this one after its end",
				storage.ID(o.ID), o.State, storage.Time(o.StartAt).UTC().Format(time.RFC3339))
		}
	}
	return nil
}

// windowText renders a window for an audit entry.
func windowText(start time.Time, end pgtype.Timestamptz) string {
	if !end.Valid {
		return start.UTC().Format(time.RFC3339) + " with no end"
	}
	return start.UTC().Format(time.RFC3339) + " to " + storage.Time(end).UTC().Format(time.RFC3339)
}
