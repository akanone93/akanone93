// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package rollout

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/notify"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// Signals counts the events of one release, as the analytics rollups hold
// them (Q13).
type Signals interface {
	// Counts returns the counts of a release of an app in an environment over
	// [from, to), with those of the named custom events.
	Counts(ctx context.Context, org string, q SignalQuery) (Counts, error)
}

// SignalQuery names whose events Signals counts.
type SignalQuery struct {
	AppID, EnvironmentID string
	// Release is the release sequence.
	Release  int64
	From, To time.Time
	// Custom are the names of the custom events to count.
	Custom []string
}

// checkHealth compares the candidate of an active rollout with the previous
// release and acts on a regression: it pauses the rollout (PLX-3150) or
// rolls it back (PLX-3151), and notifies. It reports whether it stopped the
// rollout. Without force it looks only if the last check is older than the
// check interval; a sample that is too small decides nothing. Every check
// stores its figures in the rollout.
func (s *Service) checkHealth(ctx context.Context, org string, r Rollout, force bool) (bool, error) {
	if s.o.Signals == nil || !r.Health.Armed() {
		return false, nil
	}
	now := s.now()
	if !force && !r.LastCheckAt.IsZero() && now.Sub(r.LastCheckAt) < s.o.CheckInterval {
		return false, nil
	}
	gate := s.effectiveGate(r.Health)
	verdict, err := s.judge(ctx, org, r, gate, now)
	if err != nil {
		return false, err
	}
	if err := s.recordCheck(ctx, org, r, verdict, now); err != nil {
		return false, err
	}
	if !verdict.Failed() {
		return false, nil
	}
	return s.stop(ctx, org, r, gate.Action, describe(verdict))
}

// effectiveGate fills a gate's unset sample and window from the service's
// defaults.
func (s *Service) effectiveGate(g Gate) Gate {
	if g.MinSample <= 0 {
		g.MinSample = s.o.MinSample
	}
	if g.WindowSeconds <= 0 {
		g.WindowSeconds = int64(s.o.GateWindow / time.Second)
	}
	if g.Action == "" {
		g.Action = ActionPause
	}
	return g
}

// judge reads the counts of the candidate and the previous release over the
// gate's window, ending now and not reaching back before the rollout began.
func (s *Service) judge(ctx context.Context, org string, r Rollout, g Gate, now time.Time) (Verdict, error) {
	from := now.Add(-g.Window())
	if from.Before(r.StartAt) {
		from = r.StartAt
	}
	custom := make([]string, len(g.Custom))
	for i, c := range g.Custom {
		custom[i] = c.Event
	}
	query := SignalQuery{AppID: r.AppID, EnvironmentID: r.EnvironmentID, From: from, To: now, Custom: custom}
	query.Release = r.CandidateSequence
	candidate, err := s.o.Signals.Counts(ctx, org, query)
	if err != nil {
		return Verdict{}, fmt.Errorf("rollout: count the candidate's events: %w", err)
	}
	query.Release = r.PreviousSequence
	previous, err := s.o.Signals.Counts(ctx, org, query)
	if err != nil {
		return Verdict{}, fmt.Errorf("rollout: count the previous release's events: %w", err)
	}
	return Judge(g, candidate, previous), nil
}

// recordCheck stores a verdict and when it was made.
func (s *Service) recordCheck(ctx context.Context, org string, r Rollout, v Verdict, at time.Time) error {
	raw, err := marshal(v)
	if err != nil {
		return err
	}
	return s.inOrg(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		return dbgen.New(tx).RecordRolloutCheck(ctx, dbgen.RecordRolloutCheckParams{ID: storage.MustUUID(r.ID), LastCheck: raw, LastCheckAt: storage.Timestamp(at)}) //nolint:wrapcheck // one statement
	})
}

// marshal encodes a stored document.
func marshal(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("rollout: %w", err)
	}
	return raw, nil
}

// stop acts on a failed gate. A rollout that a person paused, resumed or ended
// in the meantime is left as it is.
func (s *Service) stop(ctx context.Context, org string, r Rollout, action Action, detail string) (bool, error) {
	if action == ActionRollback {
		reason := fmt.Sprintf("%s: %s", plxerr.RolloutRolledBack, detail)
		err := s.closeOut(ctx, org, storage.MustUUID(r.ID), closing{action: "revert", state: StateRolledBack, reason: reason})
		return err == nil, ignoreLost(err)
	}
	reason := fmt.Sprintf("%s: %s", plxerr.RolloutPausedByHealthGate, detail)
	var paused dbgen.Rollout
	err := s.inOrg(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		row, err := dbgen.New(tx).GetRollout(ctx, storage.MustUUID(r.ID))
		if err != nil {
			return fmt.Errorf("rollout: %w", err)
		}
		paused, err = s.pause(ctx, tx, row, systemActor(), reason)
		return err
	})
	if err != nil {
		return false, ignoreLost(conflictAsLost(err))
	}
	s.notifyStopped(ctx, paused, "paused")
	return true, nil
}

// conflictAsLost turns the refusal of a stale transition into errLost.
func conflictAsLost(err error) error {
	var pe *plxerr.Error
	if errors.As(err, &pe) && pe.Code == plxerr.RevisionConflict {
		return errLost
	}
	return err
}

// ignoreLost drops errLost: the step was done by someone else.
func ignoreLost(err error) error {
	if errors.Is(err, errLost) {
		return nil
	}
	return err
}

// describe names the regressions of a verdict and the figures behind them.
func describe(v Verdict) string {
	parts := make([]string, len(v.Regressions))
	for i, g := range v.Regressions {
		parts[i] = fmt.Sprintf("%s worsened by %.0f basis points (candidate %s, previous %s; the gate allows %d; %d samples)",
			g.Metric, g.Worsening, percent(g.Candidate), percent(g.Previous), g.Threshold, g.SampleSize)
	}
	return strings.Join(parts, "; ")
}

// percent renders a rate as a percentage.
func percent(rate float64) string { return strconv.FormatFloat(rate*100, 'f', 2, 64) + "%" }

// notifyStopped tells the organisation a gate stopped a rollout. A failure is
// logged: it must not undo the decision.
func (s *Service) notifyStopped(ctx context.Context, row dbgen.Rollout, outcome string) {
	if s.o.Notify == nil {
		return
	}
	title := "Rollout paused by its health gate"
	if outcome == "rolled_back" {
		title = "Rollout rolled back by its health gate"
	}
	id := storage.ID(row.ID)
	err := s.o.Notify.Publish(ctx, notify.Event{
		Type: notify.RolloutPaused, OrganizationID: storage.ID(row.OrganizationID), AppID: storage.ID(row.AppID),
		Title: title, Body: row.StateReason, Link: "/apps/" + storage.ID(row.AppID) + "/rollouts/" + id,
		Attributes: map[string]string{
			"rollout": id, "release": strconv.FormatInt(row.CandidateSequence, 10), "channel": row.ChannelKey, "outcome": outcome,
		},
		Params: map[string]string{
			"release": strconv.FormatInt(row.CandidateSequence, 10), "channel": row.ChannelKey, "outcome": outcome, "reason": row.StateReason,
		},
		Subject: id,
	})
	if err != nil {
		s.log.WarnContext(ctx, "rollout: could not notify", "rollout", id, "error", err)
	}
}
