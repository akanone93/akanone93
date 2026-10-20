// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package dsr

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// minDocumentBytes is a lower bound of the size of one exported document:
// every one carries an identifier or a hash and a timestamp or a name under
// named keys, which is longer than this (the shortest, a device token, is
// about 70 bytes). It turns
// the bytes left of the budget into the most rows worth fetching, so a
// store's query never reads far more than the archive may hold.
const minDocumentBytes = 48

// archive is the exported document.
type archive struct {
	OrganizationID string                       `json:"organizationId"`
	Subject        archiveSubject               `json:"subject"`
	GeneratedAt    time.Time                    `json:"generatedAt"`
	Sections       map[string][]json.RawMessage `json:"sections"`
}

// archiveSubject names the subject of an archive.
type archiveSubject struct {
	Kind Kind   `json:"kind"`
	ID   string `json:"id"`
}

// Export returns a JSON archive of everything held about the subject, one
// section per store, and records the request. The archive is bounded by
// dsr.exportBytes: a larger one is refused (PLX-1320), never truncated. A
// subject with no data anywhere is PLX-8501.
func (s *Service) Export(ctx context.Context, p auth.Principal, subject Subject) (Archive, error) {
	if err := p.Authorize(auth.DSRManage); err != nil {
		return Archive{}, fmt.Errorf("dsr: %w", err)
	}
	sub, err := resolve(p.OrganizationID, subject)
	if err != nil {
		return Archive{}, err
	}
	requestID, err := s.o.IDs.New()
	if err != nil {
		return Archive{}, fmt.Errorf("dsr: %w", err)
	}
	var out Archive
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		set, err := s.o.Limits(ctx, tx, p.OrganizationID)
		if err != nil {
			return fmt.Errorf("dsr: %w", err)
		}
		a := archive{
			OrganizationID: p.OrganizationID, Subject: archiveSubject{Kind: sub.kind, ID: storage.ID(sub.id)},
			GeneratedAt: s.now().UTC(), Sections: map[string][]json.RawMessage{},
		}
		rows, err := collect(ctx, dbgen.New(tx), sub, &a, set.Get(limits.DsrExportBytes))
		if err != nil {
			return err
		}
		doc, err := json.Marshal(a)
		if err != nil {
			return fmt.Errorf("dsr: encode archive: %w", err)
		}
		if err := checkSize(int64(len(doc)), set.Get(limits.DsrExportBytes)); err != nil {
			return err
		}
		out = Archive{RequestID: requestID, JSON: doc, Rows: rows}
		return s.record(ctx, tx, p, sub, requestID, "export", audit.DataSubjectExported, rows, int64(len(doc)))
	})
	return out, err
}

// collect reads every store of the subject into the archive and returns the
// number of rows. It refuses once the documents alone exceed the budget and
// reports PLX-8501 when there are none.
func collect(ctx context.Context, q *dbgen.Queries, sub resolved, a *archive, budget int64) (int64, error) {
	var rows, used int64
	for _, x := range executors(sub.kind) {
		docs, err := x.export(ctx, q, sub, rowLimit(budget-used))
		if err != nil {
			return 0, fmt.Errorf("dsr: export %s: %w", x.store, err)
		}
		section := make([]json.RawMessage, len(docs))
		for i, d := range docs {
			section[i] = d
			used += int64(len(d))
		}
		if err := checkSize(used, budget); err != nil {
			return 0, err
		}
		a.Sections[x.store] = section
		rows += int64(len(docs))
	}
	if rows == 0 {
		return 0, plxerr.New(plxerr.SubjectNotFound, "no data is held about this %s", sub.kind)
	}
	return rows, nil
}

// rowLimit is the most documents worth fetching with remaining bytes of
// budget left: one more than fit, so an overflow is seen and not hidden.
func rowLimit(remaining int64) int32 {
	if remaining < 0 {
		remaining = 0
	}
	return int32(min(remaining/minDocumentBytes+1, math.MaxInt32)) //nolint:gosec // clamped to the int32 range by min
}

// checkSize refuses an archive larger than the budget.
func checkSize(size, budget int64) error {
	if size > budget {
		return plxerr.New(plxerr.LimitExceeded, "the data of this subject exceeds the export limit of %d bytes (dsr.exportBytes)", budget)
	}
	return nil
}
