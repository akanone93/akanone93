// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package document_test

import (
	"context"
	"strings"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/document"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

// TestEditorSessionEntries: a write tagged with an editor session stores
// the inverse patch that undoes it and tags its snapshot; the session's
// entries list newest first for anyone who can read the draft; sending an
// inverse back undoes the write; only studio.undoDepth entries keep their
// inverse (STU-069, ADR-0095 contracts #4 and #25).
func TestEditorSessionEntries(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	plugin := f.plugin(t)
	const lock, tab = "lock-1", "tab-1"
	if _, _, err := f.docs.AcquireLock(ctx, f.owner, f.app, plugin, lock, false); err != nil {
		t.Fatal(err)
	}
	home := "plugins/loans/pages/home.page.json"
	if _, err := f.docs.PutDocumentTagged(ctx, f.owner, f.app, plugin, lock, document.Tag{EditorSession: tab, Step: "s1"}, home, pageJSON("home", "One"), 0); err != nil {
		t.Fatal(err)
	}
	text := func(v string) []document.Op {
		return []document.Op{{Op: "replace", Path: "/root/props/data", Value: v}}
	}
	second, err := f.docs.PatchDocumentTagged(ctx, f.owner, f.app, plugin, lock, document.Tag{EditorSession: tab, Step: "s2"}, home, text("Two"), 1)
	if err != nil {
		t.Fatal(err)
	}
	// An untagged write and another session's write are not this session's.
	if _, err := f.docs.PatchDocument(ctx, f.owner, f.app, plugin, lock, home, text("Three"), 2); err != nil {
		t.Fatal(err)
	}
	if _, err := f.docs.PatchDocumentTagged(ctx, f.owner, f.app, plugin, lock, document.Tag{EditorSession: "tab-2"}, home, text("Four"), 3); err != nil {
		t.Fatal(err)
	}

	entries, err := f.docs.ListEditorSessionEntries(ctx, f.viewer, f.app, plugin, tab, 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	// The created document has no inverse; the patch's inverse restores "One".
	if len(entries) != 1 || entries[0].SnapshotID != second.SnapshotID || entries[0].Path != home ||
		len(entries[0].Inverse) != 1 || entries[0].Inverse[0].Value != "One" || entries[0].Step != "s2" {
		t.Fatalf("entries = %+v", entries)
	}
	snaps, err := f.docs.ListSnapshots(ctx, f.viewer, f.app, plugin, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	tags := map[string]string{}
	for _, s := range snaps {
		tags[s.ID] = s.EditorSession
	}
	if tags[second.SnapshotID] != tab {
		t.Errorf("snapshot %s tagged %q, want %q", second.SnapshotID, tags[second.SnapshotID], tab)
	}

	// Undo is the inverse sent as a new forward write.
	_, err = f.docs.PatchDocumentTagged(ctx, f.owner, f.app, plugin, lock, document.Tag{EditorSession: "tab-2"}, home, entries[0].Inverse, 4)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := f.docs.GetDocument(ctx, f.viewer, f.app, plugin, home)
	if err != nil || !strings.Contains(string(doc.Content), `"data":"One"`) {
		t.Errorf("after undo: %s %v", doc.Content, err)
	}

	// Retention: with studio.undoDepth 2, only the newest two keep theirs.
	if _, err := f.tenancy.SetOrganizationLimit(ctx, f.owner, string(limits.StudioUndoDepth), 2); err != nil {
		t.Fatal(err)
	}
	rev := int64(5)
	for _, v := range []string{"a", "b", "c"} {
		if _, err := f.docs.PatchDocumentTagged(ctx, f.owner, f.app, plugin, lock, document.Tag{EditorSession: tab, Step: v}, home, text(v), rev); err != nil {
			t.Fatal(err)
		}
		rev++
	}
	entries, err = f.docs.ListEditorSessionEntries(ctx, f.viewer, f.app, plugin, tab, 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Inverse[0].Value != "b" || entries[1].Inverse[0].Value != "a" {
		t.Errorf("after trimming: %+v", entries)
	}

	if _, err := f.docs.ListEditorSessionEntries(ctx, f.viewer, f.app, plugin, "", 0, 50); code(err) != plxerr.MissingProperty {
		t.Errorf("no session: %v", err)
	}
	if _, err := f.docs.PatchDocumentTagged(ctx, f.owner, f.app, plugin, lock, document.Tag{EditorSession: "bad tab"}, home, text("x"), rev); code(err) != plxerr.InvalidFormat {
		t.Errorf("malformed session: %v", err)
	}
	if _, err := f.docs.PatchDocumentTagged(ctx, f.owner, f.app, plugin, lock, document.Tag{Step: "s9"}, home, text("x"), rev); code(err) != plxerr.InvalidFormat {
		t.Errorf("step without a session: %v", err)
	}
}
