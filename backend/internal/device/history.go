// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package device

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// Bounds on the free-text fields of a report; they keep a hostile device
// from filling the history with long strings.
const (
	maxThemeLength   = 64
	maxUserRefLength = 256
	maxTextScale     = 1000
)

// Report is what a device says about itself when it syncs or reports a
// release (REL-080, SEC-142). Only the sequence is required.
type Report struct {
	// Sequence is the release the device runs, zero for none.
	Sequence int64
	// Bundles are the bundles the device holds; nil when it names none.
	Bundles []Installed
	// Channel is the channel the device follows; empty when it names none.
	Channel string
	// Theme is the theme in use, such as "dark"; empty when not reported.
	Theme string
	// TextScalePercent is the text scale in percent, 100 for the default;
	// zero when not reported.
	TextScalePercent int32
	// UserRef is the host's pseudonymous user identifier (HST-011); empty
	// when the host set none.
	UserRef string
}

// check reports a malformed report as the invalid-format failure the
// device sees.
func (r Report) check() error {
	switch {
	case r.Sequence < 0:
		return plxerr.New(plxerr.InvalidFormat, "the release sequence is negative")
	case len(r.Bundles) > maxBundles:
		return plxerr.New(plxerr.InvalidFormat, "more than %d bundles", maxBundles)
	case len(r.Theme) > maxThemeLength:
		return plxerr.New(plxerr.InvalidFormat, "the theme is longer than %d bytes", maxThemeLength)
	case len(r.UserRef) > maxUserRefLength:
		return plxerr.New(plxerr.InvalidFormat, "the user identifier is longer than %d bytes", maxUserRefLength)
	case r.TextScalePercent < 0 || r.TextScalePercent > maxTextScale:
		return plxerr.New(plxerr.InvalidFormat, "the text scale is outside 1 to %d percent", maxTextScale)
	}
	for _, b := range r.Bundles {
		if len(b.SHA256) != 32 {
			return plxerr.New(plxerr.InvalidFormat, "a bundle hash is not a SHA-256")
		}
	}
	return nil
}

// historyBundle is one bundle of a stored report.
type historyBundle struct {
	Key    string `json:"key"`
	SHA256 string `json:"sha256"`
}

// bundlesJSON renders the bundles of a report sorted by key, nil for a
// report that names none.
func bundlesJSON(bundles []Installed) ([]byte, error) {
	if bundles == nil {
		return nil, nil
	}
	out := make([]historyBundle, 0, len(bundles))
	for _, b := range bundles {
		out = append(out, historyBundle{Key: b.Key, SHA256: hex.EncodeToString(b.SHA256)})
	}
	slices.SortFunc(out, func(a, b historyBundle) int { return strings.Compare(a.Key, b.Key) })
	raw, err := json.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("device: %w", err)
	}
	return raw, nil
}

// recordInstall appends the report to the device's history unless it says
// what the device's newest row already says (SEC-142). Fields the report
// leaves out are carried over from that row, so a sync without a theme does
// not forget the theme the host reported before.
func (s *Service) recordInstall(ctx context.Context, q *dbgen.Queries, d Identity, r Report) error {
	id := storage.MustUUID(d.DeviceID)
	dims, err := q.GetDeviceDimensions(ctx, id)
	if err != nil {
		return fmt.Errorf("device: %w", err)
	}
	bundles, err := bundlesJSON(r.Bundles)
	if err != nil {
		return err
	}
	row := dbgen.InsertDeviceInstallParams{
		OrganizationID: storage.MustUUID(d.OrganizationID), AppID: storage.MustUUID(d.AppID),
		EnvironmentID: storage.MustUUID(d.EnvironmentID), DeviceID: id, ChannelKey: r.Channel,
		ReleaseSequence: r.Sequence, Bundles: bundles, DeviceClass: dims.DeviceClass, Locale: dims.Locale,
		Theme: optionalText(r.Theme), TextScalePercent: optionalInt(r.TextScalePercent), UserRef: optionalText(r.UserRef),
		ReportedAt: storage.Timestamp(s.now()),
	}
	last, err := q.LatestDeviceInstall(ctx, id)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return fmt.Errorf("device: %w", err)
	default:
		carryOver(&row, last)
		if sameInstall(row, last) {
			return nil
		}
	}
	newID, err := s.ids.New()
	if err != nil {
		return fmt.Errorf("device: %w", err)
	}
	row.ID = storage.MustUUID(newID)
	if err := q.InsertDeviceInstall(ctx, row); err != nil {
		return fmt.Errorf("device: %w", err)
	}
	return nil
}

// carryOver fills the fields a report left out from the device's newest
// row: its presentation, user and, for a sync that names no channel, its
// channel.
func carryOver(row *dbgen.InsertDeviceInstallParams, last dbgen.DeviceInstall) {
	if row.Theme == nil {
		row.Theme = last.Theme
	}
	if row.TextScalePercent == nil {
		row.TextScalePercent = last.TextScalePercent
	}
	if row.UserRef == nil {
		row.UserRef = last.UserRef
	}
	if row.ChannelKey == "" {
		row.ChannelKey = last.ChannelKey
	}
}

// sameInstall reports whether a row to insert repeats the stored one in
// everything but its time.
func sameInstall(row dbgen.InsertDeviceInstallParams, last dbgen.DeviceInstall) bool {
	return row.ReleaseSequence == last.ReleaseSequence && row.ChannelKey == last.ChannelKey &&
		bytes.Equal(row.Bundles, last.Bundles) && row.DeviceClass == last.DeviceClass && row.Locale == last.Locale &&
		equalPtr(row.Theme, last.Theme) && equalPtr(row.TextScalePercent, last.TextScalePercent) && equalPtr(row.UserRef, last.UserRef)
}

// equalPtr compares two optional values, NULL equal only to NULL.
func equalPtr[T comparable](a, b *T) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// optionalText is NULL for an empty string.
func optionalText(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// optionalInt is NULL for zero.
func optionalInt(n int32) *int32 {
	if n == 0 {
		return nil
	}
	return &n
}
