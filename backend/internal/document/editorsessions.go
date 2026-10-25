// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package document

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/jcs"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// Editor sessions back Studio's undo across a reload (STU-069, ADR-0095,
// contracts #4 and #25). A write tagged with one stores the inverse patch
// that undoes it; Studio lists a session's entries to rebuild its stacks
// and undoes by sending an inverse patch as a new forward write.

// EditorSessionEntry is one document a tagged write changed.
type EditorSessionEntry struct {
	SnapshotID string
	Sequence   int64
	Path       string
	Revision   int64
	// Inverse turns the document the write produced back into the one it
	// replaced.
	Inverse   []Op
	CreatedAt time.Time
}

// checkEditorSession accepts an empty editor session, which tags nothing,
// or one shaped like an editing session.
func checkEditorSession(id string) error {
	if id == "" {
		return nil
	}
	if len(id) > 128 {
		return plxerr.New(plxerr.InvalidFormat, "an editor session is at most 128 characters")
	}
	for _, r := range id {
		if r < 0x21 || r > 0x7e {
			return plxerr.New(plxerr.InvalidFormat, "an editor session is printable ASCII without spaces")
		}
	}
	return nil
}

// tagPut tags a whole-document write: the inverse is the patch from the
// new content back to the stored document; a created document has none.
func (s *Service) tagPut(ctx context.Context, q *dbgen.Queries, d draft, lim limits.Set, editorSession string, c change) (change, error) {
	row, err := q.GetDocument(ctx, dbgen.GetDocumentParams{DraftID: d.row.ID, Path: c.path})
	if errors.Is(err, pgx.ErrNoRows) {
		return s.tag(ctx, q, d, lim, editorSession, c, nil, true)
	}
	if err != nil {
		return change{}, fmt.Errorf("document: read %s: %w", c.path, err)
	}
	current, err := s.blob(ctx, q, d.row.OrganizationID, row.Sha256)
	if err != nil {
		return change{}, err
	}
	depth := int(lim.Get(limits.DocumentJSONDepth))
	before, err := jcs.Parse(current, depth)
	if err != nil {
		return change{}, fmt.Errorf("document: read %s: %w", c.path, err)
	}
	after, err := jcs.Parse(c.content, depth)
	if err != nil {
		return change{}, fmt.Errorf("document: read %s: %w", c.path, err)
	}
	return s.tag(ctx, q, d, lim, editorSession, c, Diff(after, before), false)
}

// tag marks a change as made by an editor session with its inverse patch
// (none for a created document), and clears the inverse patches of the
// session's older writes to the draft so that, with this one, at most
// studio.undoDepth remain.
func (*Service) tag(ctx context.Context, q *dbgen.Queries, d draft, lim limits.Set, editorSession string, c change, inverse []Op, created bool) (change, error) {
	c.editorSession = editorSession
	if !created {
		data, err := encodeOps(inverse)
		if err != nil {
			return change{}, err
		}
		c.inverse = data
	}
	keep := max(lim.Get(limits.StudioUndoDepth)-1, 0)
	if err := q.PruneEditorSessionEntries(ctx, dbgen.PruneEditorSessionEntriesParams{
		DraftID: d.row.ID, EditorSessionID: editorSession, Keep: keep,
	}); err != nil {
		return change{}, fmt.Errorf("document: trim the editor session: %w", err)
	}
	return c, nil
}

// encodeOps writes a patch as RFC 6902 JSON, in canonical form.
func encodeOps(ops []Op) ([]byte, error) {
	list := make([]any, 0, len(ops))
	for _, op := range ops {
		m := map[string]any{"op": op.Op, "path": op.Path}
		if op.From != "" {
			m["from"] = op.From
		}
		if op.Op == "add" || op.Op == "replace" || op.Op == "test" {
			m["value"] = op.Value
		}
		list = append(list, m)
	}
	data, err := jcs.Marshal(list)
	if err != nil {
		return nil, plxerr.Wrap(plxerr.InvalidStructure, err, "an inverse patch cannot be encoded")
	}
	return data, nil
}

// decodeOps reads a patch encodeOps wrote.
func decodeOps(data []byte, depth int) ([]Op, error) {
	tree, err := jcs.Parse(data, depth+2)
	if err != nil {
		return nil, fmt.Errorf("document: read an inverse patch: %w", err)
	}
	list, ok := tree.([]any)
	if !ok {
		return nil, errors.New("document: an inverse patch is not a list")
	}
	out := make([]Op, 0, len(list))
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, errors.New("document: an inverse patch operation is not an object")
		}
		op := Op{Value: m["value"]}
		op.Op, _ = m["op"].(string)
		op.Path, _ = m["path"].(string)
		op.From, _ = m["from"].(string)
		out = append(out, op)
	}
	return out, nil
}

// ListEditorSessionEntries lists an editor session's entries on a draft,
// newest first, before a sequence (0 for the newest), at most
// studio.undoDepth in all. Reading them takes what reading the draft's
// snapshots takes.
func (s *Service) ListEditorSessionEntries(ctx context.Context, p auth.Principal, appID, pluginID, editorSession string, before int64, size int32) ([]EditorSessionEntry, error) {
	if editorSession == "" {
		return nil, plxerr.New(plxerr.MissingProperty, "name the editor session to list")
	}
	if err := checkEditorSession(editorSession); err != nil {
		return nil, err
	}
	var out []EditorSessionEntry
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		d, err := s.resolve(ctx, q, p, appID, pluginID)
		if err != nil {
			return err
		}
		if err := authorize(p, auth.PluginRead, appID); err != nil {
			return err
		}
		lim, err := s.limitsFor(ctx, tx, d)
		if err != nil {
			return err
		}
		if before <= 0 {
			before = d.row.Snapshots + 1
		}
		size = int32(min(int64(size), lim.Get(limits.StudioUndoDepth))) //nolint:gosec // bounded by size
		rows, err := q.ListEditorSessionEntries(ctx, dbgen.ListEditorSessionEntriesParams{
			DraftID: d.row.ID, EditorSessionID: editorSession, BeforeSequence: before, PageSize: size,
		})
		if err != nil {
			return failure(err, "snapshot")
		}
		depth := int(lim.Get(limits.DocumentJSONDepth))
		for _, row := range rows {
			ops, err := decodeOps(row.InversePatch, depth)
			if err != nil {
				return err
			}
			out = append(out, EditorSessionEntry{
				SnapshotID: storage.ID(row.ID), Sequence: row.Sequence, Path: row.Path,
				Revision: row.Revision, Inverse: ops, CreatedAt: storage.Time(row.CreatedAt),
			})
		}
		return nil
	})
	return out, err
}
