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

	ext "github.com/nightCode42/plux3/backend/ext/analytics"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

var _ ext.EventReader = (*Postgres)(nil)

// nilUUID sorts before every event identifier, so a cursor that holds only
// a time starts at the first event of that time.
const nilUUID = "00000000-0000-0000-0000-000000000000"

// ReadEvents returns up to limit events of org in [from, to), ordered by
// time and identifier, after the cursor (ANL-011). The cursor is the time
// and identifier of the last event of the previous page; the empty cursor
// starts at the beginning.
func (p *Postgres) ReadEvents(ctx context.Context, org string, from, to time.Time, after string, limit int) ([]ext.StoredEvent, string, error) {
	if limit <= 0 {
		return nil, after, errors.New("analytics: read events: the limit must be positive")
	}
	cursorTime, cursorID, err := parseCursor(after, from)
	if err != nil {
		return nil, after, err
	}
	var rows []dbgen.ListEventsWindowRow
	err = p.db.InTx(ctx, storage.Tenant{OrganizationID: org}, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		rows, err = dbgen.New(tx).ListEventsWindow(ctx, dbgen.ListEventsWindowParams{
			OrganizationID: storage.MustUUID(org), FromTime: storage.Timestamp(from), ToTime: storage.Timestamp(to),
			CursorTime: storage.Timestamp(cursorTime), CursorID: storage.MustUUID(cursorID), RowLimit: int32(min(limit, 1<<30)), //nolint:gosec // bounded above
		})
		return err //nolint:wrapcheck // wrapped below
	})
	if err != nil {
		return nil, after, fmt.Errorf("analytics: read events: %w", err)
	}
	if len(rows) == 0 {
		return nil, after, nil
	}
	events := make([]ext.StoredEvent, len(rows))
	for i, r := range rows {
		events[i] = ext.StoredEvent{
			ID: storage.ID(r.ID), OrganizationID: storage.ID(r.OrganizationID), AppID: storage.ID(r.AppID),
			EnvironmentID: storage.ID(r.EnvironmentID), DeviceID: storage.ID(r.DeviceID), Name: r.Name,
			Time: storage.Time(r.Time).UTC(), ReleaseSequence: r.ReleaseSequence, PluginKey: r.PluginKey, Route: r.Route,
			Category: r.Category, DeviceClass: r.DeviceClass, Locale: r.Locale, Fields: json.RawMessage(r.Fields),
		}
	}
	last := events[len(events)-1]
	return events, last.Time.Format(time.RFC3339Nano) + "|" + last.ID, nil
}

// parseCursor reads a cursor made by ReadEvents. The empty cursor is the
// start of the window.
func parseCursor(cursor string, from time.Time) (time.Time, string, error) {
	if cursor == "" {
		return from, nilUUID, nil
	}
	at, id, ok := strings.Cut(cursor, "|")
	if !ok {
		return time.Time{}, "", errors.New("analytics: read events: the cursor is not one this store made")
	}
	t, err := time.Parse(time.RFC3339Nano, at)
	if err != nil {
		return time.Time{}, "", errors.New("analytics: read events: the cursor is not one this store made")
	}
	if _, err := storage.UUID(id); err != nil {
		return time.Time{}, "", errors.New("analytics: read events: the cursor is not one this store made")
	}
	return t, id, nil
}
