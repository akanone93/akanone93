// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package devsession

import (
	"context"
	"encoding/json"
	"fmt"
	"testing/fstest"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/compiler/incremental"
	"github.com/nightCode42/plux3/backend/internal/document"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/jcs"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// The overlay of a CLI session is an ordered log of the patches it pushed,
// in PostgreSQL, so that no replica holds what another needs (DEV-021,
// ADR-0087 Revision 2026-10-24). A replica's draft cache is the stored draft
// with the log replayed over it.

// loggedPatch is one document patch as the log stores it. Values are kept as
// canonical JSON and parsed again on replay, as patches arrive. The patch's
// revision condition is not kept: it held when the patch was accepted.
type loggedPatch struct {
	PluginID string     `json:"pluginId,omitempty"`
	Ops      []loggedOp `json:"ops"`
}

// loggedOp is one operation of a logged patch.
type loggedOp struct {
	Op    string          `json:"op"`
	Path  string          `json:"path"`
	From  string          `json:"from,omitempty"`
	Value json.RawMessage `json:"value,omitempty"`
}

// logRecord is one row of the log to append.
type logRecord struct {
	path  string
	patch []byte
}

// logRecords encodes the patches of a push for the log.
func logRecords(patches []DocumentPatch) ([]logRecord, error) {
	out := make([]logRecord, 0, len(patches))
	for i, dp := range patches {
		lp := loggedPatch{PluginID: dp.PluginID, Ops: make([]loggedOp, 0, len(dp.Ops))}
		for _, op := range dp.Ops {
			lo := loggedOp{Op: op.Op, Path: op.Path, From: op.From}
			if op.Value != nil {
				v, err := jcs.Marshal(op.Value)
				if err != nil {
					return nil, plxerr.Wrap(plxerr.InvalidStructure, err, "patch %d: a value cannot be encoded", i)
				}
				lo.Value = v
			}
			lp.Ops = append(lp.Ops, lo)
		}
		data, err := json.Marshal(lp)
		if err != nil {
			return nil, fmt.Errorf("devsession: encode patch %d: %w", i, err)
		}
		out = append(out, logRecord{path: dp.Path, patch: data})
	}
	return out, nil
}

// decodeRecord reads a logged patch back.
func decodeRecord(path string, data []byte, depth int) (DocumentPatch, error) {
	var lp loggedPatch
	if err := json.Unmarshal(data, &lp); err != nil {
		return DocumentPatch{}, fmt.Errorf("devsession: read the patch log: %w", err)
	}
	dp := DocumentPatch{PluginID: lp.PluginID, Path: path, Ops: make([]document.Op, 0, len(lp.Ops))}
	for _, lo := range lp.Ops {
		op := document.Op{Op: lo.Op, Path: lo.Path, From: lo.From}
		if len(lo.Value) > 0 {
			v, err := jcs.Parse(lo.Value, depth)
			if err != nil {
				return DocumentPatch{}, fmt.Errorf("devsession: read the patch log: %w", err)
			}
			op.Value = v
		}
		dp.Ops = append(dp.Ops, op)
	}
	return dp, nil
}

// catchUp brings a replica's cache of a session up to its patch log: it
// loads the stored draft the first time, then replays the entries it has
// not applied.
func (s *Service) catchUp(ctx context.Context, p auth.Principal, st *draftState, plan pushPlan) error {
	depth := int(plan.set.Get(limits.DocumentJSONDepth))
	return s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		id := storage.MustUUID(plan.session.ID)
		state, err := q.DevSessionPatchLogState(ctx, id)
		if err != nil {
			return fmt.Errorf("devsession: read the patch log: %w", err)
		}
		if st.loaded && st.applied == state.LastSeq {
			return nil
		}
		if !st.loaded || st.applied > state.LastSeq {
			if err := s.load(ctx, tx, st, plan); err != nil {
				return err
			}
		}
		return replay(ctx, q, st, id, depth)
	})
}

// replay applies the entries of the log after the last one the cache holds.
func replay(ctx context.Context, q *dbgen.Queries, st *draftState, id pgtype.UUID, depth int) error {
	rows, err := q.ListDevSessionPatches(ctx, dbgen.ListDevSessionPatchesParams{SessionID: id, After: st.applied})
	if err != nil {
		return fmt.Errorf("devsession: read the patch log: %w", err)
	}
	for _, r := range rows {
		dp, err := decodeRecord(r.Path, r.Patch, depth)
		if err != nil {
			return err
		}
		files, revs, _, err := applyPatches(st, []DocumentPatch{dp}, depth)
		if err != nil {
			return fmt.Errorf("devsession: replay patch %d of the log: %w", r.Seq, err)
		}
		st.files, st.revs, st.applied = files, revs, r.Seq
	}
	return nil
}

// load reads the stored draft into the cache, with an empty log applied.
func (s *Service) load(ctx context.Context, tx pgx.Tx, st *draftState, plan pushPlan) error {
	files, err := s.o.Drafts.DraftFiles(ctx, tx, plan.session.AppID)
	if err != nil {
		return err //nolint:wrapcheck // a domain error
	}
	st.files, st.revs, st.cache, st.sent, st.loaded, st.applied = files, map[string]int64{}, incremental.NewCache(plan.opts), map[string]bool{}, true, 0
	return nil
}

// reload replaces the cache's draft with the stored one, keeping the
// compiler's cache, for a session that follows the stored draft.
func (s *Service) reload(ctx context.Context, tx pgx.Tx, st *draftState, plan pushPlan) (fstest.MapFS, error) {
	if !st.loaded {
		if err := s.load(ctx, tx, st, plan); err != nil {
			return nil, err
		}
		return st.files, nil
	}
	files, err := s.o.Drafts.DraftFiles(ctx, tx, plan.session.AppID)
	if err != nil {
		return nil, err //nolint:wrapcheck // a domain error
	}
	return files, nil
}

// appendLog appends a push's records after the entry after, in the signing
// transaction, and returns the new last entry. The session's row is locked
// so that pushes on any replica go one at a time; a push that another
// replica overtook is refused, and the next push replays it. The log is
// bounded by devSession.patchLogBytes.
func (s *Service) appendLog(ctx context.Context, q *dbgen.Queries, plan pushPlan, after int64, records []logRecord) (int64, error) {
	id := storage.MustUUID(plan.session.ID)
	if _, err := q.GetDevSessionForUpdate(ctx, id); err != nil {
		return 0, fmt.Errorf("devsession: lock the session: %w", err)
	}
	state, err := q.DevSessionPatchLogState(ctx, id)
	if err != nil {
		return 0, fmt.Errorf("devsession: read the patch log: %w", err)
	}
	if state.LastSeq != after {
		return 0, plxerr.New(plxerr.DevPatchRejected, "another push to this dev session came first; push again")
	}
	total := state.TotalBytes
	for _, r := range records {
		total += int64(len(r.patch))
	}
	if max := plan.set.Get(limits.DevSessionPatchLogBytes); total > max {
		return 0, plxerr.New(plxerr.LimitExceeded, "the session's patches would be %d bytes, above the limit devSession.patchLogBytes = %d; restart the session", total, max)
	}
	now := storage.Timestamp(s.now())
	org := storage.MustUUID(plan.session.OrganizationID)
	seq := after
	for _, r := range records {
		seq++
		err := q.InsertDevSessionPatch(ctx, dbgen.InsertDevSessionPatchParams{
			SessionID: id, Seq: seq, OrganizationID: org, Path: r.path, Patch: r.patch,
			Bytes:     int32(len(r.patch)), //nolint:gosec // bounded by devSession.patchBytes
			CreatedAt: now,
		})
		if err != nil {
			return 0, fmt.Errorf("devsession: append to the patch log: %w", err)
		}
	}
	return seq, nil
}
