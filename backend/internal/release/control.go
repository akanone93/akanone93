// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package release

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/controldoc"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// maxControlMessage bounds the message shown when a switch is on.
const maxControlMessage = 500

// Control is the set of switches of one channel (REL-030).
type Control struct {
	AppID, EnvironmentID, ChannelKey string
	KillSwitchPlugins                []string
	AppKillSwitch, MandatoryUpdate   bool
	Message                          string
	UpdatedBy                        audit.Actor
	UpdatedAt                        time.Time
}

// GetControl returns a channel's switches; a channel never changed has
// every switch off.
func (s *Service) GetControl(ctx context.Context, p auth.Principal, appID, envID, channelKey string) (Control, error) {
	if err := authorize(p, auth.AppRead, appID); err != nil {
		return Control{}, err
	}
	var out Control
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		_, env, err := s.environment(ctx, q, appID, envID)
		if err != nil {
			return err
		}
		ch, err := q.GetChannel(ctx, dbgen.GetChannelParams{EnvironmentID: env.ID, Key: channelOrDefault(channelKey)})
		if err != nil {
			return failure(err, "channel")
		}
		row, err := s.controlRow(ctx, q, ch)
		if err != nil {
			return err
		}
		out = controlOf(appID, envID, ch.Key, row)
		return nil
	})
	return out, err
}

// SetControl replaces a channel's switches and has a manifest carrying
// them signed; devices obey it at their next manifest check (REL-006).
// The plugin kill switches and the mandatory update flag are also written
// to the channel's signed control document (SYN-060), which supersedes
// this call; the app-wide kill switch and the message exist only here and
// in the manifest, for one release.
func (s *Service) SetControl(ctx context.Context, p auth.Principal, c Control) (Control, error) {
	if err := authorizeSomeControl(p, c.AppID); err != nil {
		return Control{}, err
	}
	kills, err := checkedKillSwitches(c)
	if err != nil {
		return Control{}, err
	}
	var out Control
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		out, err = s.setControlTx(ctx, tx, p, c, kills)
		return err
	})
	return out, err
}

// checkedKillSwitches validates the message and the plugin kill switches of
// a legacy control call and returns the switches sorted and without
// duplicates.
func checkedKillSwitches(c Control) ([]string, error) {
	if len(c.Message) > maxControlMessage {
		return nil, plxerr.New(plxerr.InvalidFormat, "the message is longer than %d characters", maxControlMessage)
	}
	kills := slices.Clone(c.KillSwitchPlugins)
	for _, k := range kills {
		if !schema.ValidKey(k) {
			return nil, plxerr.New(plxerr.InvalidFormat, "%q is not a plugin key", k)
		}
	}
	slices.Sort(kills)
	kills = slices.Compact(kills)
	if kills == nil {
		kills = []string{}
	}
	return kills, nil
}

// setControlTx writes the legacy control row and the control document of
// the channel, queues its manifest and audits the change, in the
// transaction of the request.
func (s *Service) setControlTx(ctx context.Context, tx pgx.Tx, p auth.Principal, c Control, kills []string) (Control, error) {
	q := dbgen.New(tx)
	app, env, err := s.environment(ctx, q, c.AppID, c.EnvironmentID)
	if err != nil {
		return Control{}, err
	}
	if err := authorizeControl(p, c.AppID, env.Key); err != nil {
		return Control{}, err
	}
	if _, err := s.gateChange(ctx, tx, p, c.AppID, env, nil); err != nil {
		return Control{}, err
	}
	ch, err := q.GetChannelForUpdate(ctx, dbgen.GetChannelForUpdateParams{EnvironmentID: env.ID, Key: channelOrDefault(c.ChannelKey)})
	if err != nil {
		return Control{}, failure(err, "channel")
	}
	actor := p.Actor()
	change := controlChange{detail: "set through the legacy control call", apply: legacyApply(s, kills, c.MandatoryUpdate)}
	if _, _, err := s.applyControl(ctx, tx, controlTarget{app: app, env: env, ch: ch}, change, actor); err != nil {
		return Control{}, err
	}
	row, err := q.UpsertChannelControl(ctx, dbgen.UpsertChannelControlParams{
		ChannelID: ch.ID, OrganizationID: ch.OrganizationID, KillSwitchPlugins: kills,
		AppKillSwitch: c.AppKillSwitch, MandatoryUpdate: c.MandatoryUpdate, Message: c.Message,
		UpdatedByKind: actor.Kind, UpdatedByID: actor.ID, UpdatedBy: actor.Display,
	})
	if err != nil {
		return Control{}, failure(err, "control")
	}
	if err := s.enqueueManifest(ctx, tx, ch); err != nil {
		return Control{}, err
	}
	if err := s.record(ctx, tx, p, audit.Entry{
		Action: audit.ControlChanged, TargetKind: "channel", TargetID: storage.ID(ch.ID),
		Detail: "kill switches [" + strings.Join(kills, ", ") + "]" + onOff(" app kill switch", c.AppKillSwitch) + onOff(" mandatory update", c.MandatoryUpdate),
	}); err != nil {
		return Control{}, err
	}
	return controlOf(c.AppID, c.EnvironmentID, ch.Key, row), nil
}

// legacyApply is the change a legacy SetControl makes to the document: the
// plugin kill switches are replaced, and the mandatory flag becomes a
// mandatory update of the channel's current release. A channel without a
// release cannot name one, so its flag stays in the legacy row only.
func legacyApply(s *Service, kills []string, mandatory bool) func(context.Context, *dbgen.Queries, controlTarget, *controlContent) error {
	return func(ctx context.Context, q *dbgen.Queries, t controlTarget, c *controlContent) error {
		c.Switches = slices.DeleteFunc(c.Switches, func(k controldoc.KillSwitch) bool { return k.Scope == controldoc.ScopePlugin })
		for _, k := range kills {
			c.Switches = append(c.Switches, controldoc.KillSwitch{Scope: controldoc.ScopePlugin, Target: k})
		}
		switch {
		case !mandatory:
			c.Mandatory = nil
		case t.ch.ReleaseSequence > 0:
			rel, err := s.channelRelease(ctx, q, t, t.ch.ReleaseSequence, false)
			if err != nil {
				return err
			}
			next := &controldoc.MandatoryUpdate{ReleaseID: storage.ID(rel.ID), ReleaseSequence: uint64(rel.Sequence)} //nolint:gosec // a sequence is positive
			if c.Mandatory != nil && c.Mandatory.ReleaseSequence == next.ReleaseSequence {
				next.MessageKey = c.Mandatory.MessageKey
			}
			c.Mandatory = next
		}
		return nil
	}
}

// controlRow reads a channel's switches, all off when never set.
func (*Service) controlRow(ctx context.Context, q *dbgen.Queries, ch dbgen.Channel) (dbgen.ChannelControl, error) {
	row, err := q.GetChannelControl(ctx, ch.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return dbgen.ChannelControl{ChannelID: ch.ID, KillSwitchPlugins: []string{}}, nil
	}
	if err != nil {
		return dbgen.ChannelControl{}, failure(err, "control")
	}
	return row, nil
}

func controlOf(appID, envID, channel string, r dbgen.ChannelControl) Control {
	return Control{
		AppID: appID, EnvironmentID: envID, ChannelKey: channel, KillSwitchPlugins: r.KillSwitchPlugins,
		AppKillSwitch: r.AppKillSwitch, MandatoryUpdate: r.MandatoryUpdate, Message: r.Message,
		UpdatedBy: audit.Actor{Kind: r.UpdatedByKind, ID: r.UpdatedByID, Display: r.UpdatedBy},
		UpdatedAt: storage.Time(r.UpdatedAt),
	}
}

// ChannelOrDefault is the channel a device without a named one follows.
func ChannelOrDefault(key string) string { return channelOrDefault(key) }

func channelOrDefault(key string) string {
	if key == "" {
		return "production"
	}
	return key
}

func onOff(label string, on bool) string {
	if on {
		return label + " on"
	}
	return ""
}
