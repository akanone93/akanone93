// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package prodchange

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// IDs generates the identifiers of the rows the ledger writes.
type IDs interface{ New() (string, error) }

// Change is one production change.
type Change struct {
	OrganizationID string
	// AppID is empty for a change that belongs to the organisation.
	AppID string
	// EnvironmentID is empty for a change that is in no one environment.
	EnvironmentID  string
	EnvironmentKey string
	// Resource, Action and Subject are the approval engine's words for what
	// changed and how; Subject names it stably.
	Resource string
	Action   string
	Subject  string
	// ContentHash is the SHA-256, in hex, of the content changed to.
	ContentHash string
	Summary     string
	// AuthorID is the user who wrote the content, when that is not By.
	AuthorID string
	// By is who made the change and At when.
	By audit.Actor
	At time.Time
	// RequestID is the approval request that authorised the change, and
	// BreakGlassID the invocation that did instead; both may be empty.
	RequestID    string
	BreakGlassID string
}

// Record appends a change to the ledger in the caller's transaction.
func Record(ctx context.Context, tx pgx.Tx, ids IDs, c Change) error {
	id, err := ids.New()
	if err != nil {
		return fmt.Errorf("prodchange: %w", err)
	}
	row := dbgen.InsertProductionChangeParams{
		ID: storage.MustUUID(id), OrganizationID: storage.MustUUID(c.OrganizationID),
		EnvironmentKey: c.EnvironmentKey, Resource: c.Resource, Action: c.Action, Subject: c.Subject,
		ContentHash: c.ContentHash, Summary: c.Summary, AuthorID: c.AuthorID,
		ChangedByKind: c.By.Kind, ChangedByID: c.By.ID, ChangedBy: c.By.Display, ChangedAt: storage.Timestamp(c.At),
	}
	for _, f := range []struct {
		to   *pgtype.UUID
		from string
	}{{&row.AppID, c.AppID}, {&row.EnvironmentID, c.EnvironmentID}, {&row.RequestID, c.RequestID}, {&row.BreakGlassID, c.BreakGlassID}} {
		if *f.to, err = storage.NullUUID(f.from); err != nil {
			return fmt.Errorf("prodchange: %w", err)
		}
	}
	if err := dbgen.New(tx).InsertProductionChange(ctx, row); err != nil {
		return fmt.Errorf("prodchange: record a change: %w", err)
	}
	return nil
}

// Between lists the changes made from from, inclusive, until to, exclusive,
// oldest first, up to max rows; a result of max rows may be incomplete, which
// the caller detects by asking for one more than it will accept.
func Between(ctx context.Context, tx pgx.Tx, from, to time.Time, max int32) ([]dbgen.ProductionChange, error) {
	rows, err := dbgen.New(tx).ListProductionChangesBetween(ctx, dbgen.ListProductionChangesBetweenParams{
		PeriodFrom: storage.Timestamp(from), PeriodTo: storage.Timestamp(to), MaxRows: max,
	})
	if err != nil {
		return nil, fmt.Errorf("prodchange: list the changes: %w", err)
	}
	return rows, nil
}
