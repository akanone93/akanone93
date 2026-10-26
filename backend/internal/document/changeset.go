// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package document

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// Change sets write documents of the app and of several of its plugins in
// one transaction (contracts #15): moving a screen to another plugin with
// its links (STU-055), redirecting the links into a screen before deleting
// it (STU-070), and undoing either as one step (STU-069, F3). Every draft
// is written as an ordinary write would be — its revision checked, its
// editing lock held, every document validated — and any refusal leaves
// every draft as it was.

// ChangeSetEdit is one document a change set writes: exactly one of a
// JSON Patch, whole content, or a deletion.
type ChangeSetEdit struct {
	Path    string
	Patch   []Op
	Content []byte
	Delete  bool
	// IfRevision is the document's revision as read; 0 creates it.
	IfRevision int64
}

// ChangeSetDraft is the part of a change set that falls on one draft.
type ChangeSetDraft struct {
	// PluginID is empty for the app-level draft.
	PluginID string
	// Session is the editing session that holds the draft's lock.
	Session string
	// IfDraftRevision, when positive, must be the draft's revision.
	IfDraftRevision int64
	Edits           []ChangeSetEdit
}

// DraftWritten is what a change set wrote to one draft.
type DraftWritten struct {
	PluginID string
	Written
}

// ApplyChangeSet applies a change set atomically. tag, when set, tags every
// write with one editor session and undo step; a document the change set
// creates gets the inverse that deletes it, [{"op":"remove","path":""}], and
// one it deletes the inverse that creates it again,
// [{"op":"add","path":"","value":<document>}], so that undoing the step is
// itself a change set. A session displaced from one of the locks is refused
// whole: a change set is never preserved in part (SRV-041).
func (s *Service) ApplyChangeSet(ctx context.Context, p auth.Principal, appID string, tag Tag, drafts []ChangeSetDraft) ([]DraftWritten, error) {
	if err := tag.check(); err != nil {
		return nil, err
	}
	parts, edits, err := checkChangeSet(p, appID, drafts)
	if err != nil {
		return nil, err
	}
	out := make([]DraftWritten, 0, len(parts))
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		for i, part := range parts {
			w, err := s.applyChangeSetDraft(ctx, tx, q, p, appID, tag, part, edits, i == 0)
			if err != nil {
				return withDetail(err, "pluginId", part.PluginID)
			}
			out = append(out, DraftWritten{PluginID: part.PluginID, Written: w})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, w := range out {
		s.documentsChanged(ctx, p, w.Documents)
		s.draftWritten(ctx, p, appID, w.PluginID, w.Documents)
	}
	return out, nil
}

// checkChangeSet checks a change set's shape before anything is read: at
// least one edit per draft, each draft and path once, each edit one kind,
// and no translations, which are written under their locale's lock. The
// drafts come back in plugin order, the app's first, so concurrent change
// sets lock drafts in the same order.
func checkChangeSet(p auth.Principal, appID string, drafts []ChangeSetDraft) ([]ChangeSetDraft, int, error) {
	if len(drafts) == 0 {
		return nil, 0, plxerr.New(plxerr.MissingProperty, "a change set writes at least one draft")
	}
	parts := slices.Clone(drafts)
	slices.SortFunc(parts, func(a, b ChangeSetDraft) int { return strings.Compare(a.PluginID, b.PluginID) })
	edits := 0
	for i, part := range parts {
		if i > 0 && parts[i-1].PluginID == part.PluginID {
			return nil, 0, plxerr.New(plxerr.InvalidFormat, "a change set names each draft once")
		}
		if err := checkSession(part.Session); err != nil {
			return nil, 0, err
		}
		if len(part.Edits) == 0 {
			return nil, 0, plxerr.New(plxerr.MissingProperty, "a change set writes at least one document of each draft it names")
		}
		paths := map[string]bool{}
		for _, e := range part.Edits {
			if err := checkEdit(p, appID, part.PluginID, e); err != nil {
				return nil, 0, err
			}
			if paths[e.Path] {
				return nil, 0, plxerr.New(plxerr.InvalidFormat, "a change set writes %s once", e.Path)
			}
			paths[e.Path] = true
		}
		edits += len(part.Edits)
	}
	return parts, edits, nil
}

// checkEdit checks one edit of a change set.
func checkEdit(p auth.Principal, appID, pluginID string, e ChangeSetEdit) error {
	kinds := 0
	for _, set := range []bool{len(e.Patch) > 0, e.Content != nil, e.Delete} {
		if set {
			kinds++
		}
	}
	switch {
	case kinds != 1:
		return plxerr.New(plxerr.InvalidFormat, "%s: an edit is exactly one of a patch, content or a deletion", e.Path)
	case e.IfRevision < 0:
		return plxerr.New(plxerr.OutOfRange, "if_revision must not be negative")
	case e.Content == nil && e.IfRevision == 0:
		return plxerr.New(plxerr.MissingProperty, "%s: a patch or a deletion names the revision it read", e.Path)
	case translationsScope(pluginID, e.Path) != "":
		return plxerr.New(plxerr.InvalidFormat, "%s: translations are saved one document at a time, under their locale's lock", e.Path)
	}
	return guardTranslator(p, appID, pluginID, e.Path, !e.Delete)
}

// applyChangeSetDraft writes one draft's part of a change set in the
// change set's transaction: the draft row locked, its revision checked,
// its lock held, its documents prepared and applied as one snapshot.
func (s *Service) applyChangeSetDraft(ctx context.Context, tx pgx.Tx, q *dbgen.Queries, p auth.Principal, appID string,
	tag Tag, part ChangeSetDraft, edits int, first bool,
) (Written, error) {
	d, err := s.resolve(ctx, q, p, appID, part.PluginID)
	if err != nil {
		return Written{}, err
	}
	if err := authorize(p, d.editPermission(), appID); err != nil {
		return Written{}, err
	}
	row, err := q.GetDraftForUpdate(ctx, d.row.ID)
	if err != nil {
		return Written{}, failure(err, "draft")
	}
	if part.IfDraftRevision > 0 && row.Revision != part.IfDraftRevision {
		return Written{}, withDetail(plxerr.New(plxerr.RevisionConflict, "the draft is at revision %d, not %d; read it again", row.Revision, part.IfDraftRevision),
			"revision", fmt.Sprint(row.Revision))
	}
	d.row, d.tx = row, tx
	lim, err := s.limitsFor(ctx, tx, d)
	if err != nil {
		return Written{}, err
	}
	if max := lim.Get(limits.StudioChangeSetDocuments); first && int64(edits) > max {
		return Written{}, plxerr.New(plxerr.LimitExceeded, "the change set writes %d documents, above the limit studio.changeSetDocuments = %d", edits, max)
	}
	if err := s.holds(ctx, q, d, p, part.Session, ""); err != nil {
		return Written{}, err
	}
	changes := make([]change, 0, len(part.Edits))
	var (
		diags   plxerr.Diagnostics
		waivers [][]waiverChange
	)
	for _, e := range part.Edits {
		c, found, err := s.changeSetChange(ctx, q, d, lim, tag, e)
		diags = append(diags, found...)
		if err != nil {
			return Written{}, err
		}
		w, err := s.reviewWaivers(ctx, q, d, p, lim, c)
		if err != nil {
			return Written{}, err
		}
		changes, waivers = append(changes, c), append(waivers, w)
	}
	if err := touchLock(ctx, q, d, "", part.Session, storage.Timestamp(s.now())); err != nil {
		return Written{}, fmt.Errorf("document: record the last edit: %w", err)
	}
	out, err := s.apply(ctx, q, tx, d, p, changes, reasonWrite)
	if err != nil {
		return Written{}, err
	}
	for i, c := range changes {
		if err := s.auditWaivers(ctx, tx, p, c.path, waivers[i]); err != nil {
			return Written{}, err
		}
	}
	out.Diagnostics = diags
	return out, nil
}

// changeSetChange prepares one edit of a change set as a change, tagged
// with its inverse when the change set is.
func (s *Service) changeSetChange(ctx context.Context, q *dbgen.Queries, d draft, lim limits.Set, tag Tag, e ChangeSetEdit) (change, plxerr.Diagnostics, error) {
	switch {
	case e.Delete:
		if !strings.HasPrefix(e.Path, d.prefix()) {
			return change{}, nil, plxerr.New(plxerr.InvalidProjectLayout, "%q does not belong to this draft", e.Path)
		}
		c := change{path: e.Path, delete: true, ifRevision: e.IfRevision}
		if tag.EditorSession == "" {
			return c, nil, nil
		}
		before, found, err := s.storedTree(ctx, q, d, e.Path, int(lim.Get(limits.DocumentJSONDepth)))
		if err != nil {
			return change{}, nil, err
		}
		if !found {
			return change{}, nil, plxerr.New(plxerr.RevisionConflict, "%s does not exist", e.Path)
		}
		c, err = s.tag(ctx, q, d, lim, tag, c, []Op{{Op: "add", Path: "", Value: before}}, false)
		return c, nil, err
	case e.Content != nil:
		c, diags, err := s.prepare(d, lim, e.Path, e.Content, e.IfRevision)
		if err != nil || tag.EditorSession == "" {
			return c, diags, err
		}
		if e.IfRevision == 0 {
			c, err = s.tag(ctx, q, d, lim, tag, c, []Op{{Op: "remove", Path: ""}}, false)
		} else {
			c, err = s.tagPut(ctx, q, d, lim, tag, c)
		}
		return c, diags, err
	default:
		tree, patched, data, err := s.patchStored(ctx, q, d, lim, e.Path, e.Patch)
		if err != nil {
			return change{}, nil, err
		}
		c, diags, err := s.prepare(d, lim, e.Path, data, e.IfRevision)
		if err == nil && tag.EditorSession != "" {
			c, err = s.tag(ctx, q, d, lim, tag, c, Diff(patched, tree), false)
		}
		return c, diags, err
	}
}
