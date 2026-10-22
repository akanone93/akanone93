// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package analytics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Record is a stored raw event with the dimensions of its device, as the
// exports carry it. Its JSON form is the body of the webhook export.
type Record struct {
	ID              string          `json:"id"`
	OrganizationID  string          `json:"organization_id"`
	AppID           string          `json:"app_id"`
	EnvironmentID   string          `json:"environment_id"`
	DeviceID        string          `json:"device_id"`
	Name            string          `json:"name"`
	Time            time.Time       `json:"time"`
	ReleaseSequence int64           `json:"release_sequence"`
	PluginKey       string          `json:"plugin_key"`
	Route           string          `json:"route"`
	Category        string          `json:"category"`
	DeviceClass     string          `json:"device_class"`
	Locale          string          `json:"locale"`
	Fields          json.RawMessage `json:"fields"`
}

// zeroUUID is the identifier that sorts before every event identifier.
const zeroUUID = "00000000-0000-0000-0000-000000000000"

// Cursor marks the last event of a page; the next page starts after it. The
// zero Cursor starts at the beginning of the window.
type Cursor struct {
	Time time.Time
	ID   string
}

// Events returns up to limit events of an organisation in [from, to), ordered
// by time and identifier, starting after the cursor. Each event is returned
// once, however often it was written. The next cursor is that of the last
// event; a page shorter than limit is the last.
func (s *Store) Events(ctx context.Context, org string, from, to time.Time, after Cursor, limit int) ([]Record, Cursor, error) {
	if limit <= 0 {
		return nil, after, errors.New("analytics: read events: the limit must be positive")
	}
	org, err := canonicalUUID(org)
	if err != nil {
		return nil, after, fmt.Errorf("analytics: read events: the organisation: %w", err)
	}
	cursorID, cursorTime := zeroUUID, from
	if after.ID != "" {
		if cursorID, err = canonicalUUID(after.ID); err != nil {
			return nil, after, fmt.Errorf("analytics: read events: the cursor: %w", err)
		}
		cursorTime = after.Time
	}
	args := []any{
		param("org", org), timeParam("from", from), timeParam("to", to),
		timeParam("cursor_time", cursorTime), param("cursor_id", cursorID), param("row_limit", uint64(limit)),
	}
	var out []Record
	err = s.collect(ctx, queryEvents, args, func(r rowScanner) error {
		e, err := scanEvent(r)
		if err != nil {
			return err
		}
		out = append(out, e)
		return nil
	})
	if err != nil {
		return nil, after, fmt.Errorf("analytics: read events: %w", err)
	}
	if len(out) == 0 {
		return nil, after, nil
	}
	last := out[len(out)-1]
	return out, Cursor{Time: last.Time, ID: last.ID}, nil
}

// scanEvent reads a row selected with eventSelect.
func scanEvent(r rowScanner) (Record, error) {
	var (
		e      Record
		fields string
	)
	err := r.Scan(&e.ID, &e.OrganizationID, &e.AppID, &e.EnvironmentID, &e.DeviceID, &e.Name, &e.Time, &e.ReleaseSequence,
		&e.PluginKey, &e.Route, &e.Category, &e.DeviceClass, &e.Locale, &fields)
	e.Time = e.Time.UTC()
	e.Fields = json.RawMessage(fields)
	if err != nil {
		return e, fmt.Errorf("analytics.scanEvent: %w", err)
	}
	return e, nil
}
