// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package rollout

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/release"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// saltBytes is the length of a rollout's salt.
const saltBytes = 32

// CreateInput describes a rollout to schedule.
type CreateInput struct {
	AppID, EnvironmentID string
	// Channel defaults to production.
	Channel string
	// Candidate is the sequence of the release to roll out.
	Candidate int64
	Stages    []Stage
	// StartAt defaults to now; EndAt, when set, reverts the rollout then.
	StartAt, EndAt time.Time
	// Rule and Segments are the targeting (ParseTargeting).
	Rule     []byte
	Segments [][]byte
	// AllowDevices and DenyDevices are the testers' lists.
	AllowDevices, DenyDevices []string
	Health                    Gate
}

// Create schedules a rollout of a release on a channel (REL-050 to REL-053).
// It starts at once when its start time is not in the future. PLX-3153
// refuses a rollout whose window overlaps another rollout of the channel,
// PLX-3152 an invalid targeting rule, and PLX-1320 the rollout that would
// exceed rollouts.activePerApp.
func (s *Service) Create(ctx context.Context, p auth.Principal, in CreateInput) (Rollout, error) {
	spec, err := s.specify(in)
	if err != nil {
		return Rollout{}, err
	}
	channel, err := s.o.Releases.PrepareRollout(ctx, p, in.AppID, in.EnvironmentID, spec.channel, in.Candidate)
	if err != nil {
		return Rollout{}, fmt.Errorf("rollout: %w", err)
	}
	var out Rollout
	err = s.inOrg(ctx, p.OrganizationID, func(ctx context.Context, tx pgx.Tx) error {
		row, err := s.insert(ctx, tx, p, in, spec, channel)
		if err != nil {
			return err
		}
		if !spec.start.After(s.now()) {
			if row, err = s.start(ctx, tx, row); err != nil {
				return err
			}
		}
		out, err = rolloutOf(row)
		return err
	})
	if err != nil {
		return Rollout{}, fmt.Errorf("rollout: create: %w", err)
	}
	s.forget()
	return out, nil
}

// spec is a validated CreateInput.
type spec struct {
	channel      string
	start        time.Time
	end          pgtype.Timestamptz
	targeting    Targeting
	allow, deny  []string
	salt         []byte
	health       Gate
	stagesJSON   []byte
	targetJSON   []byte
	healthJSON   []byte
	allowed, den []pgtype.UUID
}

// specify validates what a caller asked for, before anything is read.
func (s *Service) specify(in CreateInput) (spec, error) {
	var sp spec
	if err := ValidateStages(in.Stages); err != nil {
		return spec{}, err
	}
	if in.Health.Action == "" {
		in.Health.Action = ActionPause
	}
	if err := in.Health.Validate(); err != nil {
		return spec{}, err
	}
	targeting, err := ParseTargeting(in.Rule, in.Segments)
	if err != nil {
		return spec{}, err
	}
	sp.targeting, sp.health = targeting, in.Health
	if sp.allow, sp.deny, err = parseDevices(in.AllowDevices, in.DenyDevices); err != nil {
		return spec{}, err
	}
	sp.start = in.StartAt
	if sp.start.IsZero() {
		sp.start = s.now()
	}
	if !in.EndAt.IsZero() {
		if !in.EndAt.After(sp.start) {
			return spec{}, plxerr.New(plxerr.OutOfRange, "the end of a rollout must come after its start")
		}
		sp.end = storage.Timestamp(in.EndAt)
	}
	sp.channel = channelOrDefault(in.Channel)
	sp.salt = make([]byte, saltBytes)
	if _, err := io.ReadFull(s.o.Random, sp.salt); err != nil {
		return spec{}, fmt.Errorf("rollout: draw a salt: %w", err)
	}
	return sp, s.encode(&sp, in.Stages)
}

// encode renders the JSON and UUID columns of a spec.
func (*Service) encode(sp *spec, stages []Stage) error {
	var err error
	if sp.stagesJSON, err = marshal(stages); err != nil {
		return err
	}
	if sp.targetJSON, err = marshal(sp.targeting); err != nil {
		return err
	}
	if sp.healthJSON, err = marshal(sp.health); err != nil {
		return err
	}
	sp.allowed, sp.den = uuids(sp.allow), uuids(sp.deny)
	return nil
}

// uuids converts validated identifiers.
func uuids(in []string) []pgtype.UUID {
	out := make([]pgtype.UUID, len(in))
	for i, id := range in {
		out[i] = storage.MustUUID(id)
	}
	return out
}

// channelOrDefault names the production channel when none is given.
func channelOrDefault(key string) string {
	if key == "" {
		return "production"
	}
	return key
}

// insert records a scheduled rollout after the checks that need the channel
// held: that it still has a release older than the candidate, that no rollout
// overlaps this one's window and that the app has room for another.
func (s *Service) insert(ctx context.Context, tx pgx.Tx, p auth.Principal, in CreateInput, sp spec, channel release.RolloutChannel) (dbgen.Rollout, error) {
	q := dbgen.New(tx)
	env := storage.MustUUID(channel.EnvironmentID)
	ch, err := q.GetChannelForUpdate(ctx, dbgen.GetChannelForUpdateParams{EnvironmentID: env, Key: sp.channel})
	if err != nil {
		return dbgen.Rollout{}, fmt.Errorf("rollout: channel: %w", err)
	}
	if ch.ReleaseSequence == 0 || in.Candidate <= ch.ReleaseSequence {
		return dbgen.Rollout{}, plxerr.New(plxerr.OutOfRange, "release %d is not newer than release %d, which the channel serves", in.Candidate, ch.ReleaseSequence)
	}
	if err := s.requireWindowFree(ctx, q, ch.ID, sp.start, sp.end); err != nil {
		return dbgen.Rollout{}, err
	}
	if err := s.requireRoom(ctx, tx, p.OrganizationID, in.AppID); err != nil {
		return dbgen.Rollout{}, err
	}
	id, err := s.newID()
	if err != nil {
		return dbgen.Rollout{}, err
	}
	row, err := q.InsertRollout(ctx, dbgen.InsertRolloutParams{
		ID: storage.MustUUID(id), OrganizationID: ch.OrganizationID, AppID: storage.MustUUID(in.AppID), EnvironmentID: env, ChannelID: ch.ID,
		EnvironmentKey: channel.EnvironmentKey, ChannelKey: sp.channel, CandidateSequence: in.Candidate, Salt: sp.salt,
		Stages: sp.stagesJSON, StartAt: storage.Timestamp(sp.start), EndAt: sp.end, Targeting: sp.targetJSON,
		AllowDevices: sp.allowed, DenyDevices: sp.den, Health: sp.healthJSON,
		CreatedBy: p.Actor().ID, CreatedAt: storage.Timestamp(s.now()),
	})
	if err != nil {
		return dbgen.Rollout{}, fmt.Errorf("rollout: insert: %w", err)
	}
	return row, s.record(ctx, tx, p.Actor(), p.OrganizationID, audit.RolloutCreated, row,
		fmt.Sprintf("release %d on %s/%s from %s, %d stage(s)", in.Candidate, channel.EnvironmentKey, sp.channel, sp.start.UTC().Format(time.RFC3339), len(in.Stages)))
}

// requireWindowFree refuses (PLX-3153) a window that meets another open
// rollout of the channel: two rollouts cannot both decide which release a
// device gets.
func (*Service) requireWindowFree(ctx context.Context, q *dbgen.Queries, channel pgtype.UUID, start time.Time, end pgtype.Timestamptz) error {
	overlaps, err := q.ListOverlappingRollouts(ctx, dbgen.ListOverlappingRolloutsParams{
		ChannelID: channel, WindowStart: storage.Timestamp(start), WindowEnd: end,
	})
	if err != nil {
		return fmt.Errorf("rollout: overlaps: %w", err)
	}
	if len(overlaps) > 0 {
		return plxerr.New(plxerr.ScheduleOverlapsRollout, "the rollout %s of this channel is %s from %s; schedule this one after its end",
			storage.ID(overlaps[0].ID), overlaps[0].State, storage.Time(overlaps[0].StartAt).UTC().Format(time.RFC3339))
	}
	return nil
}

// requireRoom refuses (PLX-1320) a rollout beyond rollouts.activePerApp.
func (s *Service) requireRoom(ctx context.Context, tx pgx.Tx, org, app string) error {
	set, err := s.o.Limits(ctx, tx, org, app)
	if err != nil {
		return fmt.Errorf("rollout: limits: %w", err)
	}
	open, err := dbgen.New(tx).CountOpenRollouts(ctx, storage.MustUUID(app))
	if err != nil {
		return fmt.Errorf("rollout: count: %w", err)
	}
	if max := set.Get(limits.RolloutsActivePerApp); open >= max {
		return plxerr.New(plxerr.LimitExceeded, "the app has %d rollouts in progress, the most rollouts.activePerApp allows (%d)", open, max)
	}
	return nil
}

// record appends an audit entry about a rollout.
func (s *Service) record(ctx context.Context, tx pgx.Tx, actor audit.Actor, org string, action audit.Action, row dbgen.Rollout, detail string) error {
	_, err := s.o.Audit.Append(ctx, tx, audit.Entry{
		OrganizationID: org, Actor: actor, Action: action, TargetKind: "rollout", TargetID: storage.ID(row.ID), Detail: detail,
	})
	if err != nil {
		return fmt.Errorf("rollout: %w", err)
	}
	return nil
}

// Get returns a rollout.
func (s *Service) Get(ctx context.Context, p auth.Principal, id string) (Rollout, error) {
	var out Rollout
	err := s.inOrg(ctx, p.OrganizationID, func(ctx context.Context, tx pgx.Tx) error {
		row, err := s.load(ctx, dbgen.New(tx), p, id)
		if err != nil {
			return err
		}
		out, err = rolloutOf(row)
		return err
	})
	if err != nil {
		return Rollout{}, fmt.Errorf("rollout: get: %w", err)
	}
	return out, nil
}

// load reads a rollout and checks the principal may manage it. A rollout of
// another organisation is invisible by row-level security.
func (*Service) load(ctx context.Context, q *dbgen.Queries, p auth.Principal, id string) (dbgen.Rollout, error) {
	if !holdsAnywhere(p) {
		return dbgen.Rollout{}, fmt.Errorf("rollout: %w", p.Authorize(auth.RolloutManage))
	}
	uid, err := storage.UUID(id)
	if err != nil {
		return dbgen.Rollout{}, plxerr.New(plxerr.InvalidFormat, "the rollout identifier is not valid")
	}
	row, err := q.GetRollout(ctx, uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return dbgen.Rollout{}, plxerr.New(plxerr.ResourceNotFound, "no such rollout")
	}
	if err != nil {
		return dbgen.Rollout{}, fmt.Errorf("rollout: %w", err)
	}
	if err := p.AuthorizeEnvironment(auth.RolloutManage, storage.ID(row.AppID), row.EnvironmentKey); err != nil {
		return dbgen.Rollout{}, fmt.Errorf("rollout: %w", err)
	}
	return row, nil
}

// List returns an app's rollouts, newest first, after the one named by before
// (empty for the first page).
func (s *Service) List(ctx context.Context, p auth.Principal, appID string, onlyOpen bool, before string, size int32) ([]Rollout, error) {
	if err := p.AuthorizeApp(auth.RolloutManage, appID); err != nil {
		return nil, fmt.Errorf("rollout: %w", err)
	}
	app, err := storage.UUID(appID)
	if err != nil {
		return nil, plxerr.New(plxerr.InvalidFormat, "the app identifier is not valid")
	}
	args := dbgen.ListRolloutsParams{AppID: app, OnlyOpen: onlyOpen, PageSize: size}
	if before != "" {
		if args.Before, err = storage.UUID(before); err != nil {
			return nil, plxerr.New(plxerr.InvalidPageToken, "the page token is not valid")
		}
	}
	var rows []dbgen.Rollout
	err = s.inOrg(ctx, p.OrganizationID, func(ctx context.Context, tx pgx.Tx) error {
		rows, err = dbgen.New(tx).ListRollouts(ctx, args)
		return err //nolint:wrapcheck // one statement
	})
	if err != nil {
		return nil, fmt.Errorf("rollout: list: %w", err)
	}
	return rolloutsOf(rows)
}

// Pause stops an active rollout from advancing. The devices it has reached
// keep the candidate (REL-053).
func (s *Service) Pause(ctx context.Context, p auth.Principal, id, reason string) (Rollout, error) {
	reason = "paused by " + p.Actor().Display + manualReason(reason)
	return s.change(ctx, p, id, func(ctx context.Context, tx pgx.Tx, row dbgen.Rollout) (dbgen.Rollout, error) {
		return s.pause(ctx, tx, row, p.Actor(), reason)
	})
}

// Resume continues a paused rollout with its current stage afresh.
func (s *Service) Resume(ctx context.Context, p auth.Principal, id string) (Rollout, error) {
	return s.change(ctx, p, id, func(ctx context.Context, tx pgx.Tx, row dbgen.Rollout) (dbgen.Rollout, error) {
		now := s.now()
		next, err := s.transition(ctx, tx, row, []State{StatePaused}, StateActive, "", &now)
		if err != nil {
			return dbgen.Rollout{}, err
		}
		return next, s.record(ctx, tx, p.Actor(), p.OrganizationID, audit.RolloutResumed, next, "resumed by "+p.Actor().Display)
	})
}

// Abort ends a rollout and returns its channel to the release it had, as a
// new release (REL-006). A rollout that has not started just ends.
func (s *Service) Abort(ctx context.Context, p auth.Principal, id, reason string) (Rollout, error) {
	var scheduled bool
	out, err := s.change(ctx, p, id, func(ctx context.Context, tx pgx.Tx, row dbgen.Rollout) (dbgen.Rollout, error) {
		reason = "aborted by " + p.Actor().Display + manualReason(reason)
		if State(row.State) == StateScheduled {
			scheduled = true
			next, err := s.transition(ctx, tx, row, []State{StateScheduled}, StateAborted, reason, nil)
			if err != nil {
				return dbgen.Rollout{}, err
			}
			return next, s.record(ctx, tx, p.Actor(), p.OrganizationID, audit.RolloutReverted, next, reason)
		}
		return s.beginClosing(ctx, tx, row.ID, closing{action: "revert", state: StateAborted, reason: reason})
	})
	if err != nil || scheduled {
		return out, err
	}
	if err := s.finishOne(ctx, p.OrganizationID, out.ID); err != nil {
		return Rollout{}, err
	}
	return s.Get(ctx, p, id)
}

// manualReason renders a person's reason as a suffix.
func manualReason(reason string) string {
	if reason = strings.TrimSpace(reason); reason == "" {
		return ""
	}
	return ": " + reason
}

// change runs a transition of one rollout in a transaction.
func (s *Service) change(ctx context.Context, p auth.Principal, id string, f func(context.Context, pgx.Tx, dbgen.Rollout) (dbgen.Rollout, error)) (Rollout, error) {
	var out Rollout
	err := s.inOrg(ctx, p.OrganizationID, func(ctx context.Context, tx pgx.Tx) error {
		row, err := s.load(ctx, dbgen.New(tx), p, id)
		if err != nil {
			return err
		}
		if row, err = f(ctx, tx, row); err != nil {
			return err
		}
		out, err = rolloutOf(row)
		return err
	})
	if err != nil {
		return Rollout{}, fmt.Errorf("rollout: %w", err)
	}
	s.forget()
	return out, nil
}

// transition moves a rollout from one of the states to another, and refuses
// (PLX-1030) when it is in none of them. The stage restarts at stageStart when
// one is given.
func (s *Service) transition(ctx context.Context, tx pgx.Tx, row dbgen.Rollout, from []State, to State, reason string, stageStart *time.Time) (dbgen.Rollout, error) {
	args := dbgen.SetRolloutStateParams{
		ID: row.ID, State: string(to), Reason: reason, At: storage.Timestamp(s.now()),
		FromStates: statesOf(from),
	}
	if stageStart != nil {
		args.StageStartedAt = storage.Timestamp(*stageStart)
	}
	next, err := dbgen.New(tx).SetRolloutState(ctx, args)
	if errors.Is(err, pgx.ErrNoRows) {
		return dbgen.Rollout{}, plxerr.New(plxerr.RevisionConflict, "the rollout is %s and cannot become %s", row.State, to)
	}
	if err != nil {
		return dbgen.Rollout{}, fmt.Errorf("rollout: %w", err)
	}
	return next, nil
}

// statesOf renders states for a query.
func statesOf(in []State) []string {
	out := make([]string, len(in))
	for i, st := range in {
		out[i] = string(st)
	}
	return slices.Clone(out)
}

// pause moves an active rollout to paused and records who or what did it.
func (s *Service) pause(ctx context.Context, tx pgx.Tx, row dbgen.Rollout, actor audit.Actor, reason string) (dbgen.Rollout, error) {
	next, err := s.transition(ctx, tx, row, []State{StateActive}, StatePaused, reason, nil)
	if err != nil {
		return dbgen.Rollout{}, err
	}
	return next, s.record(ctx, tx, actor, storage.ID(row.OrganizationID), audit.RolloutPaused, next, reason)
}

// holdsAnywhere reports whether the principal holds rollout.manage across the
// organisation, on some app or in some environment. It is the first check of
// a call that names only a rollout, made before the rollout is read so that a
// caller without the permission learns nothing about which rollouts exist.
func holdsAnywhere(p auth.Principal) bool {
	if p.Holds(auth.RolloutManage) {
		return true
	}
	for _, perms := range p.AppPermissions {
		if slices.Contains(perms, auth.RolloutManage) {
			return true
		}
	}
	for _, perms := range p.EnvPermissions {
		if slices.Contains(perms, auth.RolloutManage) {
			return true
		}
	}
	return false
}
