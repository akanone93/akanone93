// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package analytics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// The events that report errors (Appendix G.2).
const (
	// EventError is a problem the runtime reported and recovered from.
	EventError = "error"
	// EventFatalError is an uncaught error.
	EventFatalError = "fatal_error"
)

// DefaultErrorDays is how far back error groups are listed, and how long
// their trend is, when a caller does not say.
const DefaultErrorDays = 14

// maxErrorDays bounds a request for a trend to what a year of daily rows is.
const maxErrorDays = 366

// ErrorDay is the events of a group on one UTC day.
type ErrorDay struct {
	Day    time.Time
	Events int64
}

// ErrorGroup is the errors with one fingerprint in one release of an app
// (ANL-040).
type ErrorGroup struct {
	ID              string
	AppID           string
	ReleaseSequence int64
	Fingerprint     string
	Code            string
	PluginKey       string
	Route           string
	// NodePath is where the runtime reported the error. Studio opens the
	// node by it (P11).
	NodePath string
	// JSONPath and SourceFile locate the node in the source documents
	// (CMP-041); empty when the release has no retained source map or the
	// node is not in it.
	JSONPath, SourceFile string
	// Frames are the normalised top frames the fingerprint covers.
	Frames              []string
	Fatal               bool
	FirstSeen, LastSeen time.Time
	Events, Devices     int64
	// Trend is the events per UTC day over the window asked for, oldest
	// first; days without events are absent.
	Trend []ErrorDay
}

// ErrorSample is one event of a group.
type ErrorSample struct {
	EventID    string
	OccurredAt time.Time
	// Fields is the event's flat JSON object of scalars.
	Fields json.RawMessage
}

// RecordErrors groups the error and fatal_error events of a batch
// (ANL-040). It runs at ingestion, after the events are stored: the
// fingerprint needs only the event, so grouping adds one small transaction
// per batch and no read of a bundle or source map. An event delivered twice
// is counted once, because the sample it leaves is keyed by its identifier.
// A batch with no error events costs nothing.
func (s *Service) RecordErrors(ctx context.Context, b Batch) error {
	var errs []Event
	for _, e := range b.Events {
		if e.Name == EventError || e.Name == EventFatalError {
			errs = append(errs, e)
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return s.inOrg(ctx, b.OrganizationID, "", func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		for _, e := range errs {
			if err := s.recordError(ctx, q, b, e); err != nil {
				return err
			}
		}
		return nil
	})
}

// reported is the part of an error event that groups it.
type reported struct {
	Code     string `json:"code"`
	NodePath string `json:"node_path"`
	Frames   string `json:"frames"`
}

// recordError adds one event to its group. An event without a code cannot
// be grouped and is skipped: it stays in the raw events and the rollups.
func (s *Service) recordError(ctx context.Context, q *dbgen.Queries, b Batch, e Event) error {
	var r reported
	if err := json.Unmarshal(e.Fields, &r); err != nil || r.Code == "" {
		return nil //nolint:nilerr // an event that does not describe an error is not grouped
	}
	frames := NormaliseFrames(r.Frames)
	org := storage.MustUUID(b.OrganizationID)
	eventID, err := storage.UUID(e.ID)
	if err != nil {
		return nil //nolint:nilerr // ingestion assigns identifiers; anything else is not ours to count
	}
	groupID, err := s.o.IDs.New()
	if err != nil {
		return fmt.Errorf("analytics: %w", err)
	}
	group, err := q.UpsertErrorGroup(ctx, dbgen.UpsertErrorGroupParams{
		ID: storage.MustUUID(groupID), OrganizationID: org, AppID: storage.MustUUID(b.AppID), ReleaseSequence: e.ReleaseSequence,
		Fingerprint: Fingerprint(r.Code, r.NodePath, frames), Code: r.Code, PluginKey: e.PluginKey, Route: e.Route, NodePath: r.NodePath,
		Frames: strings.Join(frames, "\n"), Fatal: e.Name == EventFatalError, FirstSeen: storage.Timestamp(e.Time),
	})
	if err != nil {
		return fmt.Errorf("analytics: group the error: %w", err)
	}
	fresh, err := q.InsertErrorSample(ctx, dbgen.InsertErrorSampleParams{
		GroupID: group, OrganizationID: org, EventID: eventID, OccurredAt: storage.Timestamp(e.Time), Fields: e.Fields,
	})
	if err != nil {
		return fmt.Errorf("analytics: keep the sample: %w", err)
	}
	if fresh == 0 {
		return nil // delivered before
	}
	return s.countError(ctx, q, org, group, b.DeviceID, e.Time)
}

// countError counts a new event in its group: the total, the device if it is
// new to the group, and the day. It then trims the samples to the newest.
func (s *Service) countError(ctx context.Context, q *dbgen.Queries, org, group pgtype.UUID, device string, at time.Time) error {
	var newDevices int64
	if dev, err := storage.UUID(device); err == nil {
		if newDevices, err = q.AddErrorGroupDevice(ctx, dbgen.AddErrorGroupDeviceParams{GroupID: group, OrganizationID: org, DeviceID: dev}); err != nil {
			return fmt.Errorf("analytics: count the device: %w", err)
		}
	}
	if err := q.CountErrorGroupEvent(ctx, dbgen.CountErrorGroupEventParams{ID: group, NewDevices: newDevices}); err != nil {
		return fmt.Errorf("analytics: count the event: %w", err)
	}
	day := pgtype.Date{Time: at.UTC().Truncate(24 * time.Hour), Valid: true}
	if err := q.AddErrorGroupDay(ctx, dbgen.AddErrorGroupDayParams{GroupID: group, OrganizationID: org, Day: day}); err != nil {
		return fmt.Errorf("analytics: count the day: %w", err)
	}
	keep := int32(s.o.Limits.Get(limits.APIPageSize)) //nolint:gosec // G115: a page size.
	if err := q.PruneErrorSamples(ctx, dbgen.PruneErrorSamplesParams{GroupID: group, Keep: keep}); err != nil {
		return fmt.Errorf("analytics: trim the samples: %w", err)
	}
	return nil
}

// ListErrorGroups lists an app's error groups last seen within the last
// days, newest activity first, with their trend by day (ANL-040). A release
// sequence of zero lists every release. Groups are listed as recorded; the
// document path is resolved when a single group is read.
func (s *Service) ListErrorGroups(ctx context.Context, p auth.Principal, appID string, release int64, days, limit int) ([]ErrorGroup, error) {
	if err := p.AuthorizeApp(auth.AnalyticsRead, appID); err != nil {
		return nil, fmt.Errorf("analytics: %w", err)
	}
	app, err := storage.UUID(appID)
	if err != nil {
		return nil, plxerr.New(plxerr.InvalidFormat, "the app identifier is not valid")
	}
	days, err = errorDays(days)
	if err != nil {
		return nil, err
	}
	since := s.now().UTC().AddDate(0, 0, -days)
	if maxRows := int(s.o.Limits.Get(limits.APIPageSize)); limit <= 0 || limit > maxRows {
		limit = maxRows
	}
	var out []ErrorGroup
	err = s.inOrg(ctx, p.OrganizationID, p.UserID, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		rows, err := q.ListErrorGroups(ctx, dbgen.ListErrorGroupsParams{
			OrganizationID: storage.MustUUID(p.OrganizationID), AppID: app, ReleaseSequence: release,
			Since: storage.Timestamp(since), RowLimit: int32(limit), //nolint:gosec // G115: bounded by the page size.
		})
		if err != nil {
			return fmt.Errorf("analytics: list the error groups: %w", err)
		}
		out, err = s.withTrends(ctx, q, rows, since)
		return err
	})
	return out, err
}

// errorDays defaults and bounds a number of days.
func errorDays(days int) (int, error) {
	switch {
	case days == 0:
		return DefaultErrorDays, nil
	case days < 0 || days > maxErrorDays:
		return 0, plxerr.New(plxerr.OutOfRange, "days is between 1 and %d", maxErrorDays)
	}
	return days, nil
}

// withTrends converts rows and attaches each group's trend.
func (*Service) withTrends(ctx context.Context, q *dbgen.Queries, rows []dbgen.ErrorGroup, since time.Time) ([]ErrorGroup, error) {
	ids := make([]pgtype.UUID, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	trend, err := q.ListErrorGroupDaysFor(ctx, dbgen.ListErrorGroupDaysForParams{
		GroupIds: ids, Since: pgtype.Date{Time: since.UTC().Truncate(24 * time.Hour), Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("analytics: the trend: %w", err)
	}
	byGroup := map[pgtype.UUID][]ErrorDay{}
	for _, d := range trend {
		byGroup[d.GroupID] = append(byGroup[d.GroupID], ErrorDay{Day: d.Day.Time.UTC(), Events: d.Events})
	}
	out := make([]ErrorGroup, len(rows))
	for i, r := range rows {
		out[i] = errorGroupOf(r)
		out[i].Trend = byGroup[r.ID]
	}
	return out, nil
}

// GetErrorGroup returns one group with its newest sample events. The first
// time a group is read it is symbolicated: the node path is resolved through
// the release's retained source map to the document and JSON pointer it was
// compiled from (CMP-041), and the result is stored with the group.
//
// Symbolication is lazy, at query time, because ingestion is the hot path
// and must not read objects; a group is read far less often than its events
// arrive, and the answer for a release never changes. A release without a
// source map, or a node not in it, leaves the path empty and is not tried
// again.
func (s *Service) GetErrorGroup(ctx context.Context, p auth.Principal, id string, days int) (ErrorGroup, []ErrorSample, error) {
	gid, err := storage.UUID(id)
	if err != nil {
		return ErrorGroup{}, nil, plxerr.New(plxerr.InvalidFormat, "the error group identifier is not valid")
	}
	if days, err = errorDays(days); err != nil {
		return ErrorGroup{}, nil, err
	}
	row, art, ok, err := s.loadGroup(ctx, p, gid)
	if err != nil {
		return ErrorGroup{}, nil, err
	}
	var outcome symbolOutcome
	if ok && s.o.Objects != nil {
		if outcome, err = s.symbolicateGroup(ctx, row, art); err != nil {
			return ErrorGroup{}, nil, err
		}
	}
	return s.readGroup(ctx, p, row, outcome, days)
}

// loadGroup reads a group, authorises the caller for its app and finds the
// artifacts it can be symbolicated from, in one short transaction so that no
// object is read while a connection is held.
func (s *Service) loadGroup(ctx context.Context, p auth.Principal, id pgtype.UUID) (row dbgen.ErrorGroup, art dbgen.ReleasePluginArtifactsRow, ok bool, err error) {
	err = s.inOrg(ctx, p.OrganizationID, p.UserID, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		var err error
		row, err = q.GetErrorGroup(ctx, dbgen.GetErrorGroupParams{ID: id, OrganizationID: storage.MustUUID(p.OrganizationID)})
		if errors.Is(err, pgx.ErrNoRows) {
			return plxerr.New(plxerr.ResourceNotFound, "there is no such error group")
		}
		if err != nil {
			return fmt.Errorf("analytics: load the error group: %w", err)
		}
		if err := p.AuthorizeApp(auth.AnalyticsRead, storage.ID(row.AppID)); err != nil {
			return fmt.Errorf("analytics: %w", err)
		}
		art, ok, err = symbolArtifacts(ctx, q, row)
		return err
	})
	return row, art, ok, err
}

// readGroup records the symbolication outcome and reads the group's trend and
// samples.
func (s *Service) readGroup(ctx context.Context, p auth.Principal, row dbgen.ErrorGroup, outcome symbolOutcome, days int) (ErrorGroup, []ErrorSample, error) {
	var (
		group   ErrorGroup
		samples []ErrorSample
	)
	err := s.inOrg(ctx, p.OrganizationID, p.UserID, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		if err := s.recordSymbol(ctx, q, &row, outcome); err != nil {
			return err
		}
		groups, err := s.withTrends(ctx, q, []dbgen.ErrorGroup{row}, s.now().UTC().AddDate(0, 0, -days))
		if err != nil {
			return err
		}
		group = groups[0]
		samples, err = s.samples(ctx, q, row.ID)
		return err
	})
	return group, samples, err
}

// samples reads a group's newest events.
func (s *Service) samples(ctx context.Context, q *dbgen.Queries, group pgtype.UUID) ([]ErrorSample, error) {
	rows, err := q.ListErrorSamples(ctx, dbgen.ListErrorSamplesParams{GroupID: group, RowLimit: int32(s.o.Limits.Get(limits.APIPageSize))}) //nolint:gosec // G115: a page size.
	if err != nil {
		return nil, fmt.Errorf("analytics: the samples: %w", err)
	}
	out := make([]ErrorSample, len(rows))
	for i, r := range rows {
		out[i] = ErrorSample{EventID: storage.ID(r.EventID), OccurredAt: storage.Time(r.OccurredAt), Fields: r.Fields}
	}
	return out, nil
}

// errorGroupOf converts a row.
func errorGroupOf(r dbgen.ErrorGroup) ErrorGroup {
	g := ErrorGroup{
		ID: storage.ID(r.ID), AppID: storage.ID(r.AppID), ReleaseSequence: r.ReleaseSequence, Fingerprint: r.Fingerprint, Code: r.Code,
		PluginKey: r.PluginKey, Route: r.Route, NodePath: r.NodePath, JSONPath: r.JsonPath, SourceFile: r.SourceFile, Fatal: r.Fatal,
		FirstSeen: storage.Time(r.FirstSeen), LastSeen: storage.Time(r.LastSeen), Events: r.Events, Devices: r.Devices,
	}
	if r.Frames != "" {
		g.Frames = strings.Split(r.Frames, "\n")
	}
	return g
}
