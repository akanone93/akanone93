// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package reconstruct

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/release"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
	"github.com/nightCode42/plux3/backend/internal/storage/objects"
	"github.com/nightCode42/plux3/backend/internal/tenancy"
)

// Subject identifies what to reconstruct: a device, or the pseudonymous user
// the host reported (HST-011). Exactly one is set.
type Subject struct {
	DeviceID string
	UserRef  string
}

// kind is KindDevice or KindUser.
func (s Subject) kind() string {
	if s.UserRef != "" {
		return KindUser
	}
	return KindDevice
}

// check reports a subject that names neither or both.
func (s Subject) check() error {
	if (s.DeviceID == "") == (s.UserRef == "") {
		return plxerr.New(plxerr.InvalidFormat, "name either a device or a user, not both and not neither")
	}
	return nil
}

// resolution is a resolved moment: the manifest's content apart from what
// the release adds (its identifier and time of issue), and the rows the
// release is stored with.
type resolution struct {
	device   dbgen.Device
	env      dbgen.Environment
	manifest Manifest
}

// resolver resolves moments inside one transaction.
type resolver struct {
	q       *dbgen.Queries
	objects objects.Store
	// now is the time of the request and cutoff the oldest moment the
	// retention allows, days the retention in days.
	now    time.Time
	cutoff time.Time
	days   int64
}

// expired is PLX-8502 with the reason it applies.
func (r resolver) expired(format string, args ...any) error {
	return plxerr.New(plxerr.ReconstructionDataExpired, format+"; history is kept for %d days", append(args, r.days)...)
}

// resolve finds the state the subject was in at the moment (SEC-142).
func (r resolver) resolve(ctx context.Context, appID pgtype.UUID, sub Subject, at time.Time) (resolution, error) {
	if err := sub.check(); err != nil {
		return resolution{}, err
	}
	if at.After(r.now) {
		return resolution{}, plxerr.New(plxerr.InvalidFormat, "the moment lies in the future")
	}
	if at.Before(r.cutoff) {
		return resolution{}, r.expired("the moment is older than the retention")
	}
	dev, install, err := r.findInstall(ctx, appID, sub, at)
	if err != nil {
		return resolution{}, err
	}
	env, err := r.q.GetEnvironment(ctx, dev.EnvironmentID)
	if err != nil {
		return resolution{}, fmt.Errorf("reconstruct: environment: %w", err)
	}
	m := Manifest{
		Type: ManifestType, SpecVersion: ManifestSpecVersion, App: storage.ID(appID), Environment: env.Key,
		Channel: tenancy.AuditChannelKey, Moment: at.UTC().Format(time.RFC3339),
		Subject:      SubjectRef{Kind: sub.kind(), DeviceID: storage.ID(dev.ID), UserRef: sub.UserRef},
		Release:      ReleaseRef{Sequence: install.ReleaseSequence, Channel: release.ChannelOrDefault(install.ChannelKey)},
		Experiments:  []Experiment{},
		Presentation: presentationOf(install),
	}
	if m.Bundles, err = r.bundles(ctx, dev, install); err != nil {
		return resolution{}, err
	}
	if m.Control, err = r.control(ctx, env, m.Release.Channel, at); err != nil {
		return resolution{}, err
	}
	if m.Route, err = r.route(ctx, dev, install, at); err != nil {
		return resolution{}, err
	}
	return resolution{device: dev, env: env, manifest: m}, nil
}

// findInstall returns the device and the newest history row of it, or of
// the user's, at or before the moment.
func (r resolver) findInstall(ctx context.Context, appID pgtype.UUID, sub Subject, at time.Time) (dbgen.Device, dbgen.DeviceInstall, error) {
	moment := storage.Timestamp(at)
	var install dbgen.DeviceInstall
	var dev dbgen.Device
	var err error
	if sub.UserRef != "" {
		install, err = r.q.UserInstallAt(ctx, dbgen.UserInstallAtParams{AppID: appID, UserRef: sub.UserRef, Moment: moment})
		if errors.Is(err, pgx.ErrNoRows) {
			return dev, install, r.expired("no device reported this user at or before the moment")
		}
		if err != nil {
			return dev, install, fmt.Errorf("reconstruct: history: %w", err)
		}
		dev, err = r.device(ctx, appID, install.DeviceID)
		return dev, install, err
	}
	id, err := storage.UUID(sub.DeviceID)
	if err != nil {
		return dev, install, plxerr.New(plxerr.InvalidFormat, "the device identifier is not valid")
	}
	if dev, err = r.device(ctx, appID, id); err != nil {
		return dev, install, err
	}
	install, err = r.q.DeviceInstallAt(ctx, dbgen.DeviceInstallAtParams{DeviceID: id, Moment: moment})
	if errors.Is(err, pgx.ErrNoRows) {
		return dev, install, r.expired("the device reported nothing at or before the moment")
	}
	if err != nil {
		return dev, install, fmt.Errorf("reconstruct: history: %w", err)
	}
	if install.ReleaseSequence == 0 {
		return dev, install, plxerr.New(plxerr.ResourceNotFound, "the device ran no release at the moment")
	}
	return dev, install, nil
}

// device reads a device of the app.
func (r resolver) device(ctx context.Context, appID, id pgtype.UUID) (dbgen.Device, error) {
	dev, err := r.q.GetDevice(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && dev.AppID != appID) {
		return dbgen.Device{}, plxerr.New(plxerr.ResourceNotFound, "no such device in this app")
	}
	if err != nil {
		return dbgen.Device{}, fmt.Errorf("reconstruct: device: %w", err)
	}
	return dev, nil
}

// presentationOf is what a history row says about the locale, device class,
// theme and text scale.
func presentationOf(install dbgen.DeviceInstall) Presentation {
	p := Presentation{Locale: install.Locale, DeviceClass: install.DeviceClass}
	if install.Theme != nil {
		p.Theme = *install.Theme
	}
	if install.TextScalePercent != nil {
		p.TextScalePercent = *install.TextScalePercent
	}
	return p
}

// bundleRef is a bundle named by key and hash.
type bundleRef struct {
	key  string
	hash []byte
}

// bundles lists the bundles the device held, each checked to be retained.
func (r resolver) bundles(ctx context.Context, dev dbgen.Device, install dbgen.DeviceInstall) ([]Bundle, error) {
	refs, err := r.bundleRefs(ctx, dev, install)
	if err != nil {
		return nil, err
	}
	out := make([]Bundle, 0, len(refs))
	for _, ref := range refs {
		b, err := r.retained(ctx, dev.AppID, ref)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	slices.SortFunc(out, func(a, b Bundle) int { return strings.Compare(a.Key, b.Key) })
	return out, nil
}

// bundleRefs names the bundles of a history row: the ones the device
// reported, or, when it reported none, those of its release.
func (r resolver) bundleRefs(ctx context.Context, dev dbgen.Device, install dbgen.DeviceInstall) ([]bundleRef, error) {
	if install.Bundles != nil {
		var reported []struct {
			Key    string `json:"key"`
			SHA256 string `json:"sha256"`
		}
		if err := json.Unmarshal(install.Bundles, &reported); err != nil {
			return nil, fmt.Errorf("reconstruct: stored bundles: %w", err)
		}
		refs := make([]bundleRef, 0, len(reported))
		for _, b := range reported {
			sum, err := hex.DecodeString(b.SHA256)
			if err != nil {
				return nil, fmt.Errorf("reconstruct: stored bundles: %w", err)
			}
			refs = append(refs, bundleRef{key: b.Key, hash: sum})
		}
		return refs, nil
	}
	rel, err := r.q.GetRelease(ctx, dbgen.GetReleaseParams{AppID: dev.AppID, Sequence: install.ReleaseSequence})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, r.expired("release %d is no longer retained", install.ReleaseSequence)
	}
	if err != nil {
		return nil, fmt.Errorf("reconstruct: release: %w", err)
	}
	app, err := r.q.GetVersionByID(ctx, rel.AppVersionID)
	if err != nil {
		return nil, fmt.Errorf("reconstruct: release: %w", err)
	}
	plugins, err := r.q.ListReleaseVersions(ctx, rel.ID)
	if err != nil {
		return nil, fmt.Errorf("reconstruct: release: %w", err)
	}
	refs := []bundleRef{{hash: app.BundleSha256}}
	for _, v := range plugins {
		if v.PluginKey != "" {
			refs = append(refs, bundleRef{key: v.PluginKey, hash: v.BundleSha256})
		}
	}
	return refs, nil
}

// retained describes a bundle, or fails with PLX-8502 when its version or
// its stored object is gone.
func (r resolver) retained(ctx context.Context, app pgtype.UUID, ref bundleRef) (Bundle, error) {
	label := ref.key
	if label == "" {
		label = "app"
	}
	v, err := r.q.GetVersionByBundle(ctx, dbgen.GetVersionByBundleParams{AppID: app, PluginKey: ref.key, BundleSha256: ref.hash})
	if errors.Is(err, pgx.ErrNoRows) {
		return Bundle{}, r.expired("the %s bundle %s is no longer retained", label, hex.EncodeToString(ref.hash))
	}
	if err != nil {
		return Bundle{}, fmt.Errorf("reconstruct: bundle: %w", err)
	}
	key, err := objects.Key(objects.KindBundle, hex.EncodeToString(ref.hash))
	if err != nil {
		return Bundle{}, fmt.Errorf("reconstruct: %w", err)
	}
	if _, err := r.objects.Stat(ctx, key); errors.Is(err, objects.ErrNotFound) {
		return Bundle{}, r.expired("the %s bundle %s is no longer stored", label, hex.EncodeToString(ref.hash))
	} else if err != nil {
		return Bundle{}, fmt.Errorf("reconstruct: bundle: %w", err)
	}
	return Bundle{Key: ref.key, Version: v.Version, Hash: "sha256:" + hex.EncodeToString(ref.hash), Size: v.BundleSize}, nil
}

// noControl is the control document of a channel that had none yet.
func noControl() Control {
	return Control{KillSwitches: json.RawMessage("[]"), MandatoryUpdate: json.RawMessage("null"), ForcedRollback: json.RawMessage("null")}
}

// control returns the control document in force on the channel at the
// moment: its newest signed version issued by then. A channel that had none
// yet has version zero; one whose earlier versions were pruned is PLX-8502.
func (r resolver) control(ctx context.Context, env dbgen.Environment, channelKey string, at time.Time) (Control, error) {
	ch, err := r.q.GetChannel(ctx, dbgen.GetChannelParams{EnvironmentID: env.ID, Key: channelKey})
	if errors.Is(err, pgx.ErrNoRows) {
		return noControl(), nil
	}
	if err != nil {
		return Control{}, fmt.Errorf("reconstruct: channel: %w", err)
	}
	doc, err := r.q.GetControlDocumentAt(ctx, dbgen.GetControlDocumentAtParams{ChannelID: ch.ID, Moment: storage.Timestamp(at)})
	if err == nil {
		return controlOf(doc), nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Control{}, fmt.Errorf("reconstruct: control document: %w", err)
	}
	oldest, err := r.q.OldestSignedControlDocument(ctx, ch.ID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return noControl(), nil
	case err != nil:
		return Control{}, fmt.Errorf("reconstruct: control document: %w", err)
	case oldest.Version > 1:
		return Control{}, r.expired("the control documents before version %d are no longer kept", oldest.Version)
	}
	return noControl(), nil
}

// controlOf is the manifest's form of a stored control document.
func controlOf(doc dbgen.ControlDocument) Control {
	c := Control{Version: doc.Version, KillSwitches: doc.KillSwitches, MandatoryUpdate: doc.MandatoryUpdate, ForcedRollback: doc.ForcedRollback}
	if len(c.KillSwitches) == 0 {
		c.KillSwitches = json.RawMessage("[]")
	}
	if len(c.MandatoryUpdate) == 0 {
		c.MandatoryUpdate = json.RawMessage("null")
	}
	if len(c.ForcedRollback) == 0 {
		c.ForcedRollback = json.RawMessage("null")
	}
	return c
}

// route is the page the device last reported viewing in the release at or
// before the moment, within the retention; "" when none was recorded.
func (r resolver) route(ctx context.Context, dev dbgen.Device, install dbgen.DeviceInstall, at time.Time) (string, error) {
	route, err := r.q.LastRouteAt(ctx, dbgen.LastRouteAtParams{
		AppID: dev.AppID, EnvironmentID: dev.EnvironmentID, DeviceID: dev.ID, ReleaseSequence: install.ReleaseSequence,
		Moment: storage.Timestamp(at), Since: storage.Timestamp(r.cutoff),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("reconstruct: route: %w", err)
	}
	return route, nil
}
