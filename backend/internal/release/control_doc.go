// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package release

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/breakglass"
	"github.com/nightCode42/plux3/backend/internal/controldoc"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// controlHistory is how many signed control documents of a channel are
// kept besides the newest; older ones are pruned when a new one is signed.
const controlHistory = 50

// ChannelRef names a channel of an app's environment.
type ChannelRef struct {
	AppID, EnvironmentID, ChannelKey string
}

// ControlState is a channel's control document as an operator sees it:
// the newest version, which may still wait for its signature, and the
// newest signed one, which is what devices are served.
type ControlState struct {
	ChannelRef
	// Version is the newest version, signed or not; zero when the channel
	// has no control document yet.
	Version         int64
	KillSwitches    []controldoc.KillSwitch
	MandatoryUpdate *controldoc.MandatoryUpdate
	ForcedRollback  *controldoc.ForcedRollback
	// Experimentation is what the document says about flags, experiments
	// and the global holdout (ABT-006, ABT-001, ABT-004).
	Experimentation Experimentation
	UpdatedBy       audit.Actor
	UpdatedAt       time.Time
	// SignedVersion is the newest signed version, zero when none is.
	SignedVersion int64
	// IssuedAt and ExpiresAt are those of the signed version.
	IssuedAt, ExpiresAt time.Time
	// Signed is the newest signed file, as a device receives it.
	Signed []byte
}

// Experimentation is the part of a control document that the feature flags
// and experiments of an environment decide (ABT-006, ABT-001, ABT-004): the
// same in every channel of the environment.
type Experimentation struct {
	Flags       []controldoc.FlagValue
	Experiments []controldoc.Allocation
	Holdout     *controldoc.Holdout
}

// controlContent is what a control document says apart from its envelope.
type controlContent struct {
	Switches  []controldoc.KillSwitch
	Mandatory *controldoc.MandatoryUpdate
	Rollback  *controldoc.ForcedRollback
	Experimentation
}

// The stored forms of the content, in the jsonb columns of
// control_documents.
type (
	switchJSON struct {
		Scope  string `json:"scope"`
		Target string `json:"target"`
	}
	releaseJSON struct {
		ReleaseID       string `json:"releaseId"`
		ReleaseSequence uint64 `json:"releaseSequence"`
		MessageKey      string `json:"messageKey,omitempty"`
	}
)

// contentOf reads the content of a stored row.
func contentOf(row dbgen.ControlDocument) (controlContent, error) {
	var stored []switchJSON
	if err := json.Unmarshal(row.KillSwitches, &stored); err != nil {
		return controlContent{}, fmt.Errorf("release: the stored kill switches: %w", err)
	}
	c := controlContent{Switches: make([]controldoc.KillSwitch, len(stored))}
	for i, k := range stored {
		c.Switches[i] = controldoc.KillSwitch{Scope: controldoc.Scope(k.Scope), Target: k.Target}
	}
	if len(row.MandatoryUpdate) > 0 {
		var m releaseJSON
		if err := json.Unmarshal(row.MandatoryUpdate, &m); err != nil {
			return controlContent{}, fmt.Errorf("release: the stored mandatory update: %w", err)
		}
		c.Mandatory = &controldoc.MandatoryUpdate{ReleaseID: m.ReleaseID, ReleaseSequence: m.ReleaseSequence, MessageKey: m.MessageKey}
	}
	if len(row.ForcedRollback) > 0 {
		var r releaseJSON
		if err := json.Unmarshal(row.ForcedRollback, &r); err != nil {
			return controlContent{}, fmt.Errorf("release: the stored forced rollback: %w", err)
		}
		c.Rollback = &controldoc.ForcedRollback{ReleaseID: r.ReleaseID, ReleaseSequence: r.ReleaseSequence}
	}
	var err error
	if c.Experimentation, err = experimentationOf(row.Flags, row.Experiments, row.Holdout); err != nil {
		return controlContent{}, err
	}
	return c, nil
}

// experimentationOf reads the stored flags, experiments and holdout.
func experimentationOf(flags, experiments, holdout []byte) (Experimentation, error) {
	var out Experimentation
	if err := json.Unmarshal(flags, &out.Flags); err != nil {
		return Experimentation{}, fmt.Errorf("release: the stored flags: %w", err)
	}
	if err := json.Unmarshal(experiments, &out.Experiments); err != nil {
		return Experimentation{}, fmt.Errorf("release: the stored experiments: %w", err)
	}
	if len(holdout) > 0 {
		out.Holdout = new(controldoc.Holdout)
		if err := json.Unmarshal(holdout, out.Holdout); err != nil {
			return Experimentation{}, fmt.Errorf("release: the stored holdout: %w", err)
		}
	}
	return out, nil
}

// stored renders the experimentation as the three jsonb columns; the lists
// are never null, which the columns do not allow.
func (e Experimentation) stored() (flags, experiments, holdout []byte, err error) {
	if flags, err = json.Marshal(nonNil(e.Flags)); err != nil {
		return nil, nil, nil, fmt.Errorf("release: %w", err)
	}
	if experiments, err = json.Marshal(nonNil(e.Experiments)); err != nil {
		return nil, nil, nil, fmt.Errorf("release: %w", err)
	}
	if e.Holdout != nil {
		if holdout, err = json.Marshal(e.Holdout); err != nil {
			return nil, nil, nil, fmt.Errorf("release: %w", err)
		}
	}
	return flags, experiments, holdout, nil
}

// nonNil returns an empty list for nil, so that it marshals as [].
func nonNil[T any](list []T) []T {
	if list == nil {
		return []T{}
	}
	return list
}

// sorted returns the experimentation with flags and experiments by key.
func (e Experimentation) sorted() Experimentation {
	out := e
	out.Flags = slices.Clone(e.Flags)
	slices.SortFunc(out.Flags, func(a, b controldoc.FlagValue) int { return strings.Compare(a.Key, b.Key) })
	out.Experiments = slices.Clone(e.Experiments)
	slices.SortFunc(out.Experiments, func(a, b controldoc.Allocation) int { return strings.Compare(a.Key, b.Key) })
	return out
}

// equal reports whether two experimentations say the same, nil and empty
// lists alike.
func (e Experimentation) equal(o Experimentation) bool {
	af, ae, ah, aerr := e.sorted().stored()
	bf, be, bh, berr := o.sorted().stored()
	return aerr == nil && berr == nil && bytes.Equal(af, bf) && bytes.Equal(ae, be) && bytes.Equal(ah, bh)
}

// parts renders the content as the three jsonb columns.
func (c controlContent) parts() (switches, mandatory, rollback []byte, err error) {
	stored := make([]switchJSON, len(c.Switches))
	for i, k := range c.Switches {
		stored[i] = switchJSON{Scope: string(k.Scope), Target: k.Target}
	}
	if switches, err = json.Marshal(stored); err != nil {
		return nil, nil, nil, fmt.Errorf("release: %w", err)
	}
	if c.Mandatory != nil {
		m := releaseJSON{ReleaseID: c.Mandatory.ReleaseID, ReleaseSequence: c.Mandatory.ReleaseSequence, MessageKey: c.Mandatory.MessageKey}
		if mandatory, err = json.Marshal(m); err != nil {
			return nil, nil, nil, fmt.Errorf("release: %w", err)
		}
	}
	if c.Rollback != nil {
		r := releaseJSON{ReleaseID: c.Rollback.ReleaseID, ReleaseSequence: c.Rollback.ReleaseSequence}
		if rollback, err = json.Marshal(r); err != nil {
			return nil, nil, nil, fmt.Errorf("release: %w", err)
		}
	}
	return switches, mandatory, rollback, nil
}

// sorted returns the content with its kill switches in the document's
// order, without repeats.
func (c controlContent) sorted() controlContent {
	out := c
	out.Switches = slices.Clone(c.Switches)
	order := controldoc.Scopes()
	slices.SortFunc(out.Switches, func(a, b controldoc.KillSwitch) int {
		if d := slices.Index(order, a.Scope) - slices.Index(order, b.Scope); d != 0 {
			return d
		}
		switch {
		case a.Target < b.Target:
			return -1
		case a.Target > b.Target:
			return 1
		}
		return 0
	})
	out.Switches = slices.Compact(out.Switches)
	out.Experimentation = c.Experimentation.sorted()
	return out
}

// equal reports whether two contents say the same.
func (c controlContent) equal(o controlContent) bool {
	a, b := c.sorted(), o.sorted()
	return slices.Equal(a.Switches, b.Switches) && equalPtr(a.Mandatory, b.Mandatory) && equalPtr(a.Rollback, b.Rollback) &&
		a.Experimentation.equal(b.Experimentation)
}

func equalPtr[T comparable](a, b *T) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// pluginSwitches returns the targets of the plugin-scope kill switches.
func (c controlContent) pluginSwitches() []string {
	out := []string{}
	for _, k := range c.Switches {
		if k.Scope == controldoc.ScopePlugin {
			out = append(out, k.Target)
		}
	}
	return out
}

// checkSwitch refuses a kill switch whose target is not of the form its
// scope names.
func checkSwitch(k controldoc.KillSwitch) error {
	ok := false
	switch k.Scope {
	case controldoc.ScopePlugin:
		ok = schema.ValidKey(k.Target)
	case controldoc.ScopePage, controldoc.ScopeActionGraph, controldoc.ScopeDataSource:
		_, err := storage.UUID(k.Target)
		ok = err == nil && k.Target == canonicalUUID(k.Target)
	case controldoc.ScopeFunction:
		ok = schema.ValidFunctionName(k.Target)
	default:
		return plxerr.New(plxerr.InvalidFormat, "%q is not a kill switch scope; use one of plugin, page, actionGraph, dataSource, function", k.Scope)
	}
	if !ok {
		return plxerr.New(plxerr.InvalidFormat, "%q is not a valid %s target", k.Target, k.Scope)
	}
	return nil
}

// canonicalUUID returns the lower-case hyphenated form of a UUID that
// storage.UUID accepted.
func canonicalUUID(s string) string {
	id, err := storage.UUID(s)
	if err != nil {
		return ""
	}
	return storage.ID(id)
}

// controlTarget is a channel with its environment, found for a change.
type controlTarget struct {
	app pgtype.UUID
	env dbgen.Environment
	ch  dbgen.Channel
}

// authorizeControl allows a principal who may promote releases, the
// permission the unsigned control needed, or who holds killswitch.manage,
// on the environment. It is the check of the legacy SetControl call; the
// calls on the signed document need killswitch.manage (REL-060).
func authorizeControl(p auth.Principal, appID, environment string) error {
	errPromote := p.AuthorizeEnvironment(auth.ReleasePromote, appID, environment)
	if errPromote == nil {
		return nil
	}
	errKill := p.AuthorizeEnvironment(auth.KillswitchManage, appID, environment)
	if errKill == nil {
		return nil
	}
	var denied, deniedKill *plxerr.Error
	if errors.As(errPromote, &denied) && errors.As(errKill, &deniedKill) &&
		denied.Code == plxerr.PermissionDenied && deniedKill.Code == plxerr.PermissionDenied {
		return fmt.Errorf("release: %w", plxerr.New(plxerr.PermissionDenied, "release.promote or killswitch.manage is required"))
	}
	if errors.As(errKill, &deniedKill) && deniedKill.Code != plxerr.PermissionDenied {
		return fmt.Errorf("release: %w", errKill)
	}
	return fmt.Errorf("release: %w", errPromote)
}

// authorizeSomeControl is the first check of a change, made before the
// environment is read.
func authorizeSomeControl(p auth.Principal, appID string) error {
	if err := p.AuthorizeSomeEnvironment(auth.ReleasePromote, appID); err == nil {
		return nil
	}
	return authorizeSomeEnvironment(p, auth.KillswitchManage, appID)
}

// ControlDocument returns a channel's control document (REL-060).
func (s *Service) ControlDocument(ctx context.Context, p auth.Principal, ref ChannelRef) (ControlState, error) {
	if err := authorize(p, auth.AppRead, ref.AppID); err != nil {
		return ControlState{}, err
	}
	var out ControlState
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		_, env, err := s.environment(ctx, q, ref.AppID, ref.EnvironmentID)
		if err != nil {
			return err
		}
		ch, err := q.GetChannel(ctx, dbgen.GetChannelParams{EnvironmentID: env.ID, Key: channelOrDefault(ref.ChannelKey)})
		if err != nil {
			return failure(err, "channel")
		}
		out, err = s.controlState(ctx, q, ref, ch)
		return err
	})
	return out, err
}

// controlState reads the newest and the newest signed documents.
func (*Service) controlState(ctx context.Context, q *dbgen.Queries, ref ChannelRef, ch dbgen.Channel) (ControlState, error) {
	out := ControlState{ChannelRef: ChannelRef{AppID: ref.AppID, EnvironmentID: ref.EnvironmentID, ChannelKey: ch.Key}}
	newest, err := q.GetNewestControlDocument(ctx, ch.ID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return out, nil
	case err != nil:
		return ControlState{}, failure(err, "control document")
	}
	content, err := contentOf(newest)
	if err != nil {
		return ControlState{}, err
	}
	out.Version, out.KillSwitches, out.MandatoryUpdate, out.ForcedRollback = newest.Version, content.Switches, content.Mandatory, content.Rollback
	out.Experimentation = content.Experimentation
	out.UpdatedBy = audit.Actor{Kind: newest.CreatedByKind, ID: newest.CreatedByID, Display: newest.CreatedBy}
	out.UpdatedAt = storage.Time(newest.CreatedAt)
	signed, err := q.GetNewestSignedControlDocument(ctx, ch.ID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return out, nil
	case err != nil:
		return ControlState{}, failure(err, "control document")
	}
	out.SignedVersion, out.Signed = signed.Version, signed.Signed
	out.IssuedAt, out.ExpiresAt = storage.Time(signed.IssuedAt), storage.Time(signed.ExpiresAt)
	return out, nil
}

// controlChange is one change of a channel's control content: what to
// say in the audit log and how to alter the content.
type controlChange struct {
	detail string
	apply  func(ctx context.Context, q *dbgen.Queries, t controlTarget, c *controlContent) error
	// emergency is set when the change is a break-glass action (GOV-024).
	emergency *emergency
}

// change applies a change to a channel's control document in one
// transaction, for a principal who holds killswitch.manage on the
// environment (the legacy SetControl still accepts release.promote): it takes the channel's lock, so versions are taken in one
// order whoever changes the channel, writes the next version, keeps the
// manifest's flags in step, queues the signature and audits the change.
// A change that alters nothing writes nothing.
func (s *Service) change(ctx context.Context, p auth.Principal, ref ChannelRef, c controlChange) (ControlState, error) {
	if err := authorizeSomeEnvironment(p, auth.KillswitchManage, ref.AppID); err != nil {
		return ControlState{}, err
	}
	var (
		out   ControlState
		alert *breakglass.Record
	)
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		app, env, err := s.environment(ctx, q, ref.AppID, ref.EnvironmentID)
		if err != nil {
			return err
		}
		if err := authorizeEnvironment(p, auth.KillswitchManage, ref.AppID, env.Key); err != nil {
			return err
		}
		if alert, err = s.gateChange(ctx, tx, p, ref.AppID, env, c.emergency); err != nil {
			return err
		}
		ch, err := q.GetChannelForUpdate(ctx, dbgen.GetChannelForUpdateParams{EnvironmentID: env.ID, Key: channelOrDefault(ref.ChannelKey)})
		if err != nil {
			return failure(err, "channel")
		}
		if err := s.writeControl(ctx, tx, p, controlTarget{app: app, env: env, ch: ch}, c); err != nil {
			return err
		}
		out, err = s.controlState(ctx, q, ref, ch)
		return err
	})
	if err == nil {
		s.alert(ctx, alert)
	}
	return out, err
}

// writeControl applies a change under the channel's lock and, when it
// altered the document, queues the manifest and audits it.
func (s *Service) writeControl(ctx context.Context, tx pgx.Tx, p auth.Principal, t controlTarget, c controlChange) error {
	version, changed, err := s.applyControl(ctx, tx, t, c, p.Actor())
	if err != nil || !changed {
		return err
	}
	if err := s.enqueueManifest(ctx, tx, t.ch); err != nil {
		return err
	}
	return s.record(ctx, tx, p, audit.Entry{
		Action: audit.ControlChanged, TargetKind: "channel", TargetID: storage.ID(t.ch.ID),
		Detail: fmt.Sprintf("control document %d: %s", version, c.detail),
	})
}

// applyControl writes the next version of the document if the change
// alters it, keeps the manifest's legacy flags in step and queues the
// signature. It returns the version written and whether there was one.
func (s *Service) applyControl(ctx context.Context, tx pgx.Tx, t controlTarget, c controlChange, by audit.Actor) (int64, bool, error) {
	q := dbgen.New(tx)
	version, before, err := s.newestContent(ctx, q, t.ch)
	if err != nil {
		return 0, false, err
	}
	after := before
	after.Switches = slices.Clone(before.Switches)
	if err := c.apply(ctx, q, t, &after); err != nil {
		return 0, false, err
	}
	after = after.sorted()
	if err := s.checkContent(after); err != nil {
		return 0, false, err
	}
	if version > 0 && before.equal(after) {
		return version, false, nil
	}
	if err := s.insertControl(ctx, q, t.ch, version+1, after, by); err != nil {
		return 0, false, err
	}
	if err := s.mirrorLegacy(ctx, q, t.ch, after, by); err != nil {
		return 0, false, err
	}
	return version + 1, true, s.enqueueControl(ctx, tx, t.ch)
}

// newestContent returns the version and content of a channel's newest
// document, or zero and nothing when it has none.
func (*Service) newestContent(ctx context.Context, q *dbgen.Queries, ch dbgen.Channel) (int64, controlContent, error) {
	row, err := q.GetNewestControlDocument(ctx, ch.ID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return 0, controlContent{}, nil
	case err != nil:
		return 0, controlContent{}, failure(err, "control document")
	}
	c, err := contentOf(row)
	return row.Version, c, err
}

// checkContent refuses content that is too large or malformed.
func (s *Service) checkContent(c controlContent) error {
	if max := s.o.Limits.Get(limits.ControlKillSwitches); int64(len(c.Switches)) > max {
		return plxerr.New(plxerr.LimitExceeded, "a channel may have at most %d kill switches (control.killSwitches); clear one first", max)
	}
	for _, k := range c.Switches {
		if err := checkSwitch(k); err != nil {
			return err
		}
	}
	if err := controldoc.ValidateExperimentation(c.Flags, c.Experiments, c.Holdout); err != nil {
		return plxerr.New(plxerr.InvalidFormat, "the flags and experiments do not fit the control document: %v", err)
	}
	return nil
}

// insertControl writes a new version of the content, unsigned.
func (*Service) insertControl(ctx context.Context, q *dbgen.Queries, ch dbgen.Channel, version int64, c controlContent, by audit.Actor) error {
	switches, mandatory, rollback, err := c.parts()
	if err != nil {
		return err
	}
	flags, experiments, holdout, err := c.stored()
	if err != nil {
		return err
	}
	if _, err := q.InsertControlDocument(ctx, dbgen.InsertControlDocumentParams{
		ChannelID: ch.ID, OrganizationID: ch.OrganizationID, Version: version, KillSwitches: switches,
		MandatoryUpdate: mandatory, ForcedRollback: rollback, Flags: flags, Experiments: experiments, Holdout: holdout,
		CreatedByKind: by.Kind, CreatedByID: by.ID, CreatedBy: by.Display,
	}); err != nil {
		return failure(err, "control document")
	}
	return nil
}

// mirrorLegacy keeps the unsigned flags the manifest still carries
// (REL-030) in step with the document for one release: the plugin kill
// switches and whether an update is mandatory. The app-wide kill switch
// and the message are the legacy call's alone and are left as they are.
func (s *Service) mirrorLegacy(ctx context.Context, q *dbgen.Queries, ch dbgen.Channel, c controlContent, by audit.Actor) error {
	row, err := s.controlRow(ctx, q, ch)
	if err != nil {
		return err
	}
	_, err = q.UpsertChannelControl(ctx, dbgen.UpsertChannelControlParams{
		ChannelID: ch.ID, OrganizationID: ch.OrganizationID, KillSwitchPlugins: c.pluginSwitches(),
		AppKillSwitch: row.AppKillSwitch, MandatoryUpdate: c.Mandatory != nil, Message: row.Message,
		UpdatedByKind: by.Kind, UpdatedByID: by.ID, UpdatedBy: by.Display,
	})
	if err != nil {
		return failure(err, "control")
	}
	return nil
}
