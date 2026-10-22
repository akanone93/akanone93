// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package rollout

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// errLost says a conditional update matched nothing: another scheduler, or a
// person, moved the rollout first. The step is then already done.
var errLost = errors.New("rollout: the rollout changed state first")

// closing says how a rollout is to end.
type closing struct {
	// action is "promote" or "revert", state the state the rollout ends in.
	action string
	state  State
	reason string
}

// Tick does the scheduled work of one organisation (REL-052, REL-053): it
// starts the rollouts that are due, moves the active ones on, ends the ones
// past their end, finishes the ones being closed and evaluates health gates.
// It is idempotent: each step is a conditional update, so a second scheduler
// that runs at the same moment does nothing a first has not already done. The
// failures of single rollouts are returned joined; the others still ran.
func (s *Service) Tick(ctx context.Context, org string) error {
	var rows []dbgen.Rollout
	err := s.inOrg(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		rows, err = dbgen.New(tx).ListOpenRollouts(ctx)
		return err //nolint:wrapcheck // one statement
	})
	if err != nil {
		return fmt.Errorf("rollout: list open rollouts: %w", err)
	}
	var errs []error
	for _, row := range rows {
		if err := s.step(ctx, org, row); err != nil && !errors.Is(err, errLost) {
			errs = append(errs, fmt.Errorf("rollout: %s: %w", storage.ID(row.ID), err))
		}
	}
	s.forget()
	return errors.Join(errs...)
}

// Job returns Tick as the scheduler's per-organisation job.
func (s *Service) Job() func(ctx context.Context, org string) error { return s.Tick }

// step does what one open rollout is due.
func (s *Service) step(ctx context.Context, org string, row dbgen.Rollout) error {
	now := s.now()
	switch State(row.State) {
	case StateScheduled:
		if now.Before(storage.Time(row.StartAt)) {
			return nil
		}
		return s.inOrg(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
			_, err := s.start(ctx, tx, row)
			return err
		})
	case StateActive, StatePaused:
		if row.EndAt.Valid && !now.Before(storage.Time(row.EndAt)) {
			return s.closeOut(ctx, org, row.ID, closing{action: "revert", state: StateEnded, reason: "the scheduled end was reached"})
		}
		if State(row.State) == StateActive {
			return s.stepActive(ctx, org, row)
		}
	case StateClosing:
		return s.finishOne(ctx, org, storage.ID(row.ID))
	}
	return nil
}

// stepActive checks the gate, then moves an active rollout through its
// stages, and promotes its candidate when it has reached every device and
// soaked.
func (s *Service) stepActive(ctx context.Context, org string, row dbgen.Rollout) error {
	r, err := rolloutOf(row)
	if err != nil {
		return err
	}
	step := Progress(r.Stages, r.StageIndex, r.StageBegan, s.now())
	due := step.Complete || step.Index > r.StageIndex
	// A rollout about to reach more devices is checked first, whatever the
	// interval: the gate must not be outrun by the schedule.
	if stopped, err := s.checkHealth(ctx, org, r, due); err != nil || stopped {
		return err
	}
	switch {
	case step.Complete:
		return s.closeOut(ctx, org, row.ID, closing{action: "promote", state: StateCompleted, reason: "the rollout reached every device"})
	case due:
		return s.inOrg(ctx, org, func(ctx context.Context, tx pgx.Tx) error { return s.advance(ctx, tx, r, step) })
	}
	return nil
}

// start moves a scheduled rollout to its first stage and has the candidate's
// manifest signed. A channel that moved on to the candidate or beyond before
// the start ends the rollout instead, since the candidate is no longer newer.
func (s *Service) start(ctx context.Context, tx pgx.Tx, row dbgen.Rollout) (dbgen.Rollout, error) {
	q := dbgen.New(tx)
	ch, err := q.GetChannelForUpdate(ctx, dbgen.GetChannelForUpdateParams{EnvironmentID: row.EnvironmentID, Key: row.ChannelKey})
	if err != nil {
		return dbgen.Rollout{}, fmt.Errorf("rollout: channel: %w", err)
	}
	org := storage.ID(row.OrganizationID)
	if ch.ReleaseSequence == 0 || row.CandidateSequence <= ch.ReleaseSequence {
		reason := fmt.Sprintf("the channel serves release %d, so release %d is no longer newer; the rollout did not start", ch.ReleaseSequence, row.CandidateSequence)
		next, err := s.transition(ctx, tx, row, []State{StateScheduled}, StateAborted, reason, nil)
		if err != nil {
			return dbgen.Rollout{}, errLost
		}
		return next, s.record(ctx, tx, systemActor(), org, audit.RolloutReverted, next, reason)
	}
	var stages []Stage
	if stages, err = decodeStages(row.Stages); err != nil {
		return dbgen.Rollout{}, err
	}
	previous := ch.ReleaseSequence
	next, err := q.StartRollout(ctx, dbgen.StartRolloutParams{
		ID: row.ID, PreviousSequence: &previous, StageStartedAt: storage.Timestamp(s.now()), BasisPoints: int32(stages[0].BasisPoints), //nolint:gosec // validated to at most 10000
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return dbgen.Rollout{}, errLost
	}
	if err != nil {
		return dbgen.Rollout{}, fmt.Errorf("rollout: start: %w", err)
	}
	if err := s.o.Releases.EnqueueChannelManifest(ctx, tx, org, storage.ID(row.ChannelID)); err != nil {
		return dbgen.Rollout{}, fmt.Errorf("rollout: %w", err)
	}
	return next, s.record(ctx, tx, systemActor(), org, audit.RolloutStarted, next,
		fmt.Sprintf("release %d at %d basis points, the channel keeps release %d", row.CandidateSequence, stages[0].BasisPoints, previous))
}

// advance moves an active rollout to a later stage.
func (s *Service) advance(ctx context.Context, tx pgx.Tx, r Rollout, step Step) error {
	bp := r.Stages[step.Index].BasisPoints
	next, err := dbgen.New(tx).AdvanceRollout(ctx, dbgen.AdvanceRolloutParams{
		ID: storage.MustUUID(r.ID), StageIndex: int32(step.Index), StageStartedAt: storage.Timestamp(step.Began), //nolint:gosec // an index into the stages
		BasisPoints: int32(bp), UpdatedAt: storage.Timestamp(s.now()), //nolint:gosec // validated to at most 10000
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return errLost
	}
	if err != nil {
		return fmt.Errorf("rollout: advance: %w", err)
	}
	return s.record(ctx, tx, systemActor(), storage.ID(next.OrganizationID), audit.RolloutAdvanced, next,
		fmt.Sprintf("stage %d of %d: %d basis points", step.Index+1, len(r.Stages), bp))
}

// beginClosing marks a rollout as being ended; finishOne then does the work.
func (s *Service) beginClosing(ctx context.Context, tx pgx.Tx, id pgtype.UUID, c closing) (dbgen.Rollout, error) {
	next, err := dbgen.New(tx).BeginClosingRollout(ctx, dbgen.BeginClosingRolloutParams{
		ID: id, CloseAction: &c.action, CloseState: stringPtr(string(c.state)), StateReason: c.reason, UpdatedAt: storage.Timestamp(s.now()),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return dbgen.Rollout{}, plxerr.New(plxerr.RevisionConflict, "the rollout is no longer open and cannot be ended")
	}
	if err != nil {
		return dbgen.Rollout{}, fmt.Errorf("rollout: %w", err)
	}
	return next, nil
}

// closeOut ends a rollout: it marks it closing, then does the closing work.
func (s *Service) closeOut(ctx context.Context, org string, id pgtype.UUID, c closing) error {
	err := s.inOrg(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		_, err := s.beginClosing(ctx, tx, id, c)
		return err
	})
	if err != nil {
		var pe *plxerr.Error
		if errors.As(err, &pe) && pe.Code == plxerr.RevisionConflict {
			return errLost
		}
		return err
	}
	return s.finishOne(ctx, org, storage.ID(id))
}

// finishOne does the closing work of a closing rollout, under a lock that a
// second scheduler waits for and then finds the rollout finished (so the work
// is done once), and records the end state. A crash before the commit leaves the rollout closing, and the next
// pass repeats the work, which finds it already done.
func (s *Service) finishOne(ctx context.Context, org, id string) error {
	var done dbgen.Rollout
	var finished bool
	err := s.inOrg(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		claimed, err := q.ClaimClosingRollout(ctx, storage.MustUUID(id))
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("rollout: claim: %w", err)
		}
		r, err := rolloutOf(claimed)
		if err != nil {
			return err
		}
		reverted, detail, err := s.carryOut(ctx, q, org, r)
		if err != nil {
			return err
		}
		var seq *int64
		if reverted > 0 {
			seq = &reverted
		}
		if done, err = q.FinishClosingRollout(ctx, dbgen.FinishClosingRolloutParams{ID: claimed.ID, RevertedSequence: seq, UpdatedAt: storage.Timestamp(s.now())}); err != nil {
			return fmt.Errorf("rollout: finish: %w", err)
		}
		finished = true
		action := audit.RolloutReverted
		if State(done.State) == StateCompleted {
			action = audit.RolloutCompleted
		}
		return s.record(ctx, tx, systemActor(), org, action, done, string(State(done.State))+": "+r.Reason+detail)
	})
	if err != nil {
		return err
	}
	if finished && State(done.State) == StateRolledBack {
		s.notifyStopped(ctx, done, "rolled_back")
	}
	return nil
}

// carryOut does what closing a rollout asks of its channel: points it at the
// candidate, or at a new release that repeats the previous one. It returns the
// sequence of the release made to revert, zero if none. It touches the channel
// only when the channel is where the rollout left it: a person who promoted
// something else meanwhile is not overruled.
func (s *Service) carryOut(ctx context.Context, q *dbgen.Queries, org string, r Rollout) (int64, string, error) {
	ch, err := q.GetChannelByID(ctx, storage.MustUUID(r.ChannelID))
	if err != nil {
		return 0, "", fmt.Errorf("rollout: channel: %w", err)
	}
	system := auth.System(org)
	if r.closeAction == "promote" {
		if ch.ReleaseSequence >= r.CandidateSequence {
			return 0, "", nil
		}
		if _, err := s.o.Releases.PromoteRelease(ctx, system, r.AppID, r.CandidateSequence, r.EnvironmentID, r.Channel); err != nil {
			return 0, "", fmt.Errorf("rollout: promote the candidate: %w", err)
		}
		return 0, fmt.Sprintf("; the channel now serves release %d", r.CandidateSequence), nil
	}
	return s.revert(ctx, q, system, r, ch)
}

// revert returns a channel to the release it had before the rollout.
func (s *Service) revert(ctx context.Context, q *dbgen.Queries, system auth.Principal, r Rollout, ch dbgen.Channel) (int64, string, error) {
	if r.PreviousSequence == 0 {
		return 0, "", nil
	}
	if ch.ReleaseSequence != r.PreviousSequence {
		cur, err := q.GetRelease(ctx, dbgen.GetReleaseParams{AppID: storage.MustUUID(r.AppID), Sequence: ch.ReleaseSequence})
		if err != nil {
			return 0, "", fmt.Errorf("rollout: release: %w", err)
		}
		if cur.RollbackOf == r.PreviousSequence {
			return cur.Sequence, fmt.Sprintf("; the channel already serves release %d, which repeats release %d", cur.Sequence, r.PreviousSequence), nil
		}
		return 0, fmt.Sprintf("; the channel had moved on to release %d and was left alone", ch.ReleaseSequence), nil
	}
	rel, err := s.o.Releases.RollbackRelease(ctx, system, r.AppID, r.EnvironmentID, r.Channel, r.PreviousSequence,
		fmt.Sprintf("Rollback of rollout %s to release %d", r.ID, r.PreviousSequence))
	if err != nil {
		return 0, "", fmt.Errorf("rollout: revert to release %d: %w", r.PreviousSequence, err)
	}
	return rel.Sequence, fmt.Sprintf("; the channel now serves release %d, which repeats release %d", rel.Sequence, r.PreviousSequence), nil
}

// decodeStages reads a stored schedule, which is never empty.
func decodeStages(raw []byte) ([]Stage, error) {
	var stages []Stage
	if err := decodeOne(raw, &stages); err != nil || len(stages) == 0 {
		return nil, errors.New("rollout: a stored schedule has no stages")
	}
	return stages, nil
}

func stringPtr(s string) *string { return &s }
