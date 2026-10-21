// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package analytics

import (
	"context"
	"fmt"
	"strconv"
	"time"

	ext "github.com/nightCode42/plux3/backend/ext/analytics"
)

// WriteEvents stores a batch in one insert. An event written again has the
// same (organisation, time, id) key, so ClickHouse keeps one copy and reads
// drop the other until the parts merge.
func (s *Store) WriteEvents(ctx context.Context, b ext.Batch) error {
	if len(b.Events) == 0 {
		return nil
	}
	ids, err := batchIDs(b)
	if err != nil {
		return err
	}
	received := s.now().UTC()
	rows := make([][]any, 0, len(b.Events))
	for _, e := range b.Events {
		id, err := canonicalUUID(e.ID)
		if err != nil {
			return fmt.Errorf("analytics: the event identifier: %w", err)
		}
		fields := string(e.Fields)
		if fields == "" {
			fields = "{}"
		}
		rows = append(rows, []any{
			ids[0], ids[1], ids[2], ids[3], id, e.Name, e.Time.UTC(), received,
			e.ReleaseSequence, e.PluginKey, e.Route, fields, b.DeviceClass, b.Locale, e.Category,
		})
	}
	return s.insert(ctx, eventsTable, eventColumns(), rows)
}

// batchIDs checks the identifiers of a batch: organisation, app,
// environment and device, in that order.
func batchIDs(b ext.Batch) ([4]string, error) {
	var out [4]string
	for i, v := range [4]string{b.OrganizationID, b.AppID, b.EnvironmentID, b.DeviceID} {
		id, err := canonicalUUID(v)
		if err != nil {
			return out, fmt.Errorf("analytics: an identifier of the batch: %w", err)
		}
		out[i] = id
	}
	return out, nil
}

// Purge deletes the events and rollups of an organisation past their
// cut-offs and reports how many rows went. It returns when ClickHouse has
// applied the deletion.
func (s *Store) Purge(ctx context.Context, org string, p ext.Purge) (ext.Purged, error) {
	org, err := canonicalUUID(org)
	if err != nil {
		return ext.Purged{}, fmt.Errorf("analytics: purge: the organisation: %w", err)
	}
	var out ext.Purged
	if !p.RawBefore.IsZero() {
		if out.Raw, err = s.purge(ctx, org, p.RawBefore, queryCountRaw, deleteRaw); err != nil {
			return ext.Purged{}, err
		}
	}
	if !p.RollupsBefore.IsZero() {
		if out.Rollups, err = s.purge(ctx, org, p.RollupsBefore, queryCountRollups, deleteRollups); err != nil {
			return ext.Purged{}, err
		}
	}
	return out, nil
}

// purge counts the rows before a time and deletes them.
func (s *Store) purge(ctx context.Context, org string, before time.Time, count, del string) (int64, error) {
	args := []any{param("org", org), timeParam("before", before)}
	var n uint64
	err := s.collect(ctx, count, args, func(r rowScanner) error { return r.Scan(&n) })
	if err != nil {
		return 0, fmt.Errorf("analytics: purge: count: %w", err)
	}
	if err := s.db.Exec(ctx, del, args...); err != nil {
		return 0, fmt.Errorf("analytics: purge: delete: %w", err)
	}
	return int64(n), nil //nolint:gosec // a row count
}

// partitionIDLen is the length of a monthly partition identifier, YYYYMM.
const partitionIDLen = 6

// Compact drops the monthly partitions of raw events that end at or before
// the time, which the core derives from the oldest cut-off of any
// organisation, and returns how many it dropped.
func (s *Store) Compact(ctx context.Context, before time.Time) (int, error) {
	var ids []string
	err := s.collect(ctx, queryPartitions, []any{param("table", eventsTable)}, func(r rowScanner) error {
		var id string
		if err := r.Scan(&id); err != nil {
			return err //nolint:wrapcheck // wrapped by collect
		}
		ids = append(ids, id)
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("analytics: list the partitions: %w", err)
	}
	dropped := 0
	for _, id := range ids {
		end, ok := partitionEnd(id)
		if !ok || end.After(before.UTC()) {
			continue
		}
		// DDL takes no parameters; the identifier was parsed as a year and
		// a month and is written back from the parsed value.
		stmt := "ALTER TABLE " + eventsTable + " DROP PARTITION ID '" + end.AddDate(0, -1, 0).Format("200601") + "'"
		if err := s.db.Exec(ctx, stmt); err != nil {
			return dropped, fmt.Errorf("analytics: drop the partition %s: %w", id, err)
		}
		dropped++
	}
	return dropped, nil
}

// partitionEnd returns the end of the month a YYYYMM partition identifier
// names, or false when the identifier is not one.
func partitionEnd(id string) (time.Time, bool) {
	if len(id) != partitionIDLen {
		return time.Time{}, false
	}
	if _, err := strconv.Atoi(id); err != nil {
		return time.Time{}, false
	}
	m, err := time.Parse("200601", id)
	if err != nil {
		return time.Time{}, false
	}
	return m.AddDate(0, 1, 0), true
}
