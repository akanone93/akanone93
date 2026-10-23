// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package release

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/breakglass"
	"github.com/nightCode42/plux3/backend/internal/controldoc"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// SetKillSwitch switches one thing off for a channel's devices (REL-060).
// Setting a switch that is already on changes nothing. With WithBreakGlass
// it is an emergency action (GOV-024); a function is disabled with a switch
// of scope function.
func (s *Service) SetKillSwitch(ctx context.Context, p auth.Principal, ref ChannelRef, k controldoc.KillSwitch, opts ...ChangeOption) (ControlState, error) {
	if err := checkSwitch(k); err != nil {
		return ControlState{}, err
	}
	return s.change(ctx, p, ref, controlChange{
		emergency: emergencyOf(opts, breakglass.ActionKillSwitch, switchSubject(ref, k, "on"), emergencyHash(string(k.Scope), k.Target, "on")),
		detail:    fmt.Sprintf("kill switch %s %s on", k.Scope, k.Target),
		apply: func(_ context.Context, _ *dbgen.Queries, _ controlTarget, c *controlContent) error {
			if !slices.Contains(c.Switches, k) {
				c.Switches = append(c.Switches, k)
			}
			return nil
		},
	})
}

// ClearKillSwitch switches one thing back on. Clearing a switch that is not
// set changes nothing. With WithBreakGlass it is an emergency action
// (GOV-024), for putting back what a mistaken switch took away.
func (s *Service) ClearKillSwitch(ctx context.Context, p auth.Principal, ref ChannelRef, k controldoc.KillSwitch, opts ...ChangeOption) (ControlState, error) {
	if err := checkSwitch(k); err != nil {
		return ControlState{}, err
	}
	return s.change(ctx, p, ref, controlChange{
		emergency: emergencyOf(opts, breakglass.ActionKillSwitch, switchSubject(ref, k, "off"), emergencyHash(string(k.Scope), k.Target, "off")),
		detail:    fmt.Sprintf("kill switch %s %s off", k.Scope, k.Target),
		apply: func(_ context.Context, _ *dbgen.Queries, _ controlTarget, c *controlContent) error {
			c.Switches = slices.DeleteFunc(c.Switches, func(o controldoc.KillSwitch) bool { return o == k })
			return nil
		},
	})
}

// SetMandatoryUpdate makes a release mandatory: devices activate it before
// they show another Plux page (REL-070). The release must exist, must not
// be newer than the channel's current one, which is what devices are
// served, and in a production environment must have been promoted to one. messageKey is the localisation
// key of the update screen's message and may be empty.
func (s *Service) SetMandatoryUpdate(ctx context.Context, p auth.Principal, ref ChannelRef, sequence int64, messageKey string) (ControlState, error) {
	if messageKey != "" && !controldoc.ValidIdentifier(messageKey) {
		return ControlState{}, plxerr.New(plxerr.InvalidFormat, "%q is not a message key", messageKey)
	}
	return s.change(ctx, p, ref, controlChange{
		detail: fmt.Sprintf("mandatory update to release %d", sequence),
		apply: func(ctx context.Context, q *dbgen.Queries, t controlTarget, c *controlContent) error {
			rel, err := s.channelRelease(ctx, q, t, sequence, false)
			if err != nil {
				return err
			}
			c.Mandatory = &controldoc.MandatoryUpdate{ReleaseID: storage.ID(rel.ID), ReleaseSequence: uint64(rel.Sequence), MessageKey: messageKey} //nolint:gosec // a sequence is positive
			return nil
		},
	})
}

// ClearMandatoryUpdate withdraws the mandatory update.
func (s *Service) ClearMandatoryUpdate(ctx context.Context, p auth.Principal, ref ChannelRef) (ControlState, error) {
	return s.change(ctx, p, ref, controlChange{
		detail: "mandatory update withdrawn",
		apply: func(_ context.Context, _ *dbgen.Queries, _ controlTarget, c *controlContent) error {
			c.Mandatory = nil
			return nil
		},
	})
}

// SetForcedRollback directs devices to an earlier release of the channel
// that they retain (REL-006, ADR-0004 Revision). The release must exist,
// be earlier than the channel's current one and, in a production
// environment, have been promoted to one, which keeps it for ever (REL-007);
// a release that has been purged is gone and refused. With WithBreakGlass it
// is an emergency action (GOV-024).
func (s *Service) SetForcedRollback(ctx context.Context, p auth.Principal, ref ChannelRef, sequence int64, opts ...ChangeOption) (ControlState, error) {
	return s.change(ctx, p, ref, controlChange{
		emergency: emergencyOf(opts, breakglass.ActionForcedRollback, fmt.Sprintf("forced rollback to release %d on %s", sequence, channelName(ref)), emergencyHash("forcedRollback", fmt.Sprint(sequence))),
		detail:    fmt.Sprintf("forced rollback to release %d", sequence),
		apply: func(ctx context.Context, q *dbgen.Queries, t controlTarget, c *controlContent) error {
			rel, err := s.channelRelease(ctx, q, t, sequence, true)
			if err != nil {
				return err
			}
			c.Rollback = &controldoc.ForcedRollback{ReleaseID: storage.ID(rel.ID), ReleaseSequence: uint64(rel.Sequence)} //nolint:gosec // a sequence is positive
			return nil
		},
	})
}

// ClearForcedRollback withdraws the forced rollback. With WithBreakGlass it
// is an emergency action (GOV-024).
func (s *Service) ClearForcedRollback(ctx context.Context, p auth.Principal, ref ChannelRef, opts ...ChangeOption) (ControlState, error) {
	return s.change(ctx, p, ref, controlChange{
		emergency: emergencyOf(opts, breakglass.ActionForcedRollback, "forced rollback withdrawn on "+channelName(ref), emergencyHash("forcedRollback", "withdrawn")),
		detail:    "forced rollback withdrawn",
		apply: func(_ context.Context, _ *dbgen.Queries, _ controlTarget, c *controlContent) error {
			c.Rollback = nil
			return nil
		},
	})
}

// channelRelease finds the release a mandatory update or a forced rollback
// names and checks that a device of the channel can act on it.
func (*Service) channelRelease(ctx context.Context, q *dbgen.Queries, t controlTarget, sequence int64, rollback bool) (dbgen.Release, error) {
	if sequence < 1 {
		return dbgen.Release{}, plxerr.New(plxerr.InvalidFormat, "a release sequence is a positive number")
	}
	rel, err := q.GetRelease(ctx, dbgen.GetReleaseParams{AppID: t.app, Sequence: sequence})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return dbgen.Release{}, plxerr.New(plxerr.PreconditionFailed,
				"release %d does not exist or is no longer retained; list the app's releases and name one that is", sequence)
		}
		return dbgen.Release{}, failure(err, "release")
	}
	switch {
	case t.env.Production && !rel.Production:
		return dbgen.Release{}, plxerr.New(plxerr.PreconditionFailed,
			"release %d has never been promoted to a production environment, so no device of %s can hold it", sequence, t.env.Key)
	case t.ch.ReleaseSequence == 0:
		return dbgen.Release{}, plxerr.New(plxerr.PreconditionFailed, "channel %s has no release yet", t.ch.Key)
	case rollback && rel.Sequence >= t.ch.ReleaseSequence:
		return dbgen.Release{}, plxerr.New(plxerr.PreconditionFailed,
			"release %d is not earlier than the current release %d of channel %s; a rollback names an earlier one", sequence, t.ch.ReleaseSequence, t.ch.Key)
	case !rollback && rel.Sequence > t.ch.ReleaseSequence:
		return dbgen.Release{}, plxerr.New(plxerr.PreconditionFailed,
			"release %d is newer than the current release %d of channel %s; promote it first", sequence, t.ch.ReleaseSequence, t.ch.Key)
	}
	return rel, nil
}

// switchSubject names a kill-switch change for an emergency record.
func switchSubject(ref ChannelRef, k controldoc.KillSwitch, state string) string {
	return fmt.Sprintf("kill switch %s %s %s on %s", k.Scope, k.Target, state, channelName(ref))
}

// channelName is the channel a change is made to, as an emergency record
// names it.
func channelName(ref ChannelRef) string { return channelOrDefault(ref.ChannelKey) }
