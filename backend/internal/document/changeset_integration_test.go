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

// changeSetFixture is an app with two plugins, loans and cards, each with
// a page, and the owner holding both editing locks.
type changeSetFixture struct {
	*fixture
	loans, cards string
	// offers is the loans page the tests move; its revision as written.
	offers    int64
	homeRev   int64
	loansRev  int64
	cardsRev  int64
	cardsHome string
}

const (
	loansLock = "lock-loans"
	cardsLock = "lock-cards"
	homePath  = "plugins/loans/pages/home.page.json"
	offersAt  = "plugins/loans/pages/offers.page.json"
	movedTo   = "plugins/cards/pages/offers.page.json"
)

// otherPage is pageJSON with its own identifiers.
func otherPage(key, text string) []byte {
	page := strings.ReplaceAll(string(pageJSON(key, text)), "000000022cce", "000000033ddd")
	return []byte(strings.ReplaceAll(page, "00000004977a", "00000005988b"))
}

func newChangeSetFixture(t *testing.T) *changeSetFixture {
	t.Helper()
	ctx := context.Background()
	f := &changeSetFixture{fixture: newFixture(t)}
	f.loans = f.plugin(t)
	cards, err := f.docs.CreatePlugin(ctx, f.owner, f.app, "cards", "Cards")
	if err != nil {
		t.Fatal(err)
	}
	f.cards = cards.ID
	for plugin, lock := range map[string]string{f.loans: loansLock, f.cards: cardsLock} {
		if _, _, err := f.docs.AcquireLock(ctx, f.owner, f.app, plugin, lock, false); err != nil {
			t.Fatal(err)
		}
	}
	put := func(plugin, lock, path string, content []byte) document.Written {
		t.Helper()
		w, err := f.docs.PutDocument(ctx, f.owner, f.app, plugin, lock, path, content, 0)
		if err != nil {
			t.Fatalf("PutDocument %s: %v", path, err)
		}
		return w
	}
	f.cardsRev = put(f.cards, cardsLock, "plugins/cards/plugin.json", pluginJSON("cards", "Cards")).Revision
	f.cardsHome = "plugins/cards/pages/home.page.json"
	f.cardsRev = put(f.cards, cardsLock, f.cardsHome, pageJSON("home", "Cards")).Revision
	home := put(f.loans, loansLock, homePath, pageJSON("home", "Loans"))
	f.homeRev = home.Documents[0].Revision
	offers := put(f.loans, loansLock, offersAt, otherPage("offers", "Offers"))
	f.offers, f.loansRev = offers.Documents[0].Revision, offers.Revision
	return f
}

// move is the change set that moves offers from loans to cards, checking
// with a test operation that the loans home page is as read.
func (f *changeSetFixture) move() []document.ChangeSetDraft {
	return []document.ChangeSetDraft{
		{PluginID: f.cards, Session: cardsLock, IfDraftRevision: f.cardsRev, Edits: []document.ChangeSetEdit{
			{Path: movedTo, Content: otherPage("offers", "Offers")},
		}},
		{PluginID: f.loans, Session: loansLock, IfDraftRevision: f.loansRev, Edits: []document.ChangeSetEdit{
			{Path: offersAt, Delete: true, IfRevision: f.offers},
			{Path: homePath, IfRevision: f.homeRev, Patch: []document.Op{
				{Op: "test", Path: "/root/props/data", Value: "Loans"},
				{Op: "replace", Path: "/root/props/data", Value: "Moved"},
			}},
		}},
	}
}

// unchanged fails unless neither draft was written.
func (f *changeSetFixture) unchanged(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	if _, err := f.docs.GetDocument(ctx, f.viewer, f.app, f.loans, offersAt); err != nil {
		t.Errorf("offers left loans: %v", err)
	}
	if _, err := f.docs.GetDocument(ctx, f.viewer, f.app, f.cards, movedTo); code(err) != plxerr.ResourceNotFound {
		t.Errorf("offers reached cards: %v", err)
	}
	home, err := f.docs.GetDocument(ctx, f.viewer, f.app, f.loans, homePath)
	if err != nil || home.Revision != f.homeRev {
		t.Errorf("home = revision %d, %v; want %d", home.Revision, err, f.homeRev)
	}
}

// Verifies: STU-055, STU-069, STU-070.
// A change set moves a screen to another plugin and updates a link in one
// transaction: each draft gets one snapshot tagged with the same undo
// step, the inverses undo the whole step as a change set, and the draft
// revisions advance together.
func TestApplyChangeSet(t *testing.T) {
	t.Parallel()
	f := newChangeSetFixture(t)
	ctx := context.Background()
	tag := document.Tag{EditorSession: "tab-1", Step: "move-offers"}
	written, err := f.docs.ApplyChangeSet(ctx, f.owner, f.app, tag, f.move())
	if err != nil {
		t.Fatal(err)
	}
	if len(written) != 2 || written[0].PluginID > written[1].PluginID {
		t.Fatalf("written = %+v", written)
	}
	if _, err := f.docs.GetDocument(ctx, f.viewer, f.app, f.cards, movedTo); err != nil {
		t.Errorf("offers in cards: %v", err)
	}
	if _, err := f.docs.GetDocument(ctx, f.viewer, f.app, f.loans, offersAt); code(err) != plxerr.ResourceNotFound {
		t.Errorf("offers still in loans: %v", err)
	}
	home, err := f.docs.GetDocument(ctx, f.viewer, f.app, f.loans, homePath)
	if err != nil || !strings.Contains(string(home.Content), `"data":"Moved"`) {
		t.Errorf("home = %s, %v", home.Content, err)
	}

	// One undo step across both plugins, with inverses for each document.
	inverses := map[string][]document.Op{}
	for _, plugin := range []string{f.loans, f.cards} {
		entries, err := f.docs.ListEditorSessionEntries(ctx, f.viewer, f.app, plugin, "tab-1", 0, 50)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			if e.Step != "move-offers" {
				t.Errorf("%s: step %q", e.Path, e.Step)
			}
			inverses[e.Path] = e.Inverse
		}
	}
	if inv := inverses[movedTo]; len(inv) != 1 || inv[0].Op != "remove" || inv[0].Path != "" {
		t.Errorf("created document's inverse = %+v", inv)
	}
	if inv := inverses[offersAt]; len(inv) != 1 || inv[0].Op != "add" || inv[0].Path != "" || inv[0].Value == nil {
		t.Errorf("deleted document's inverse = %+v", inv)
	}
	if inv := inverses[homePath]; len(inv) != 1 || inv[0].Value != "Loans" {
		t.Errorf("patched document's inverse = %+v", inv)
	}

	// Undoing the step is a change set built from those inverses.
	cardsDoc, loansHome := written[0].Documents, written[1].Documents
	if written[0].PluginID != f.cards {
		cardsDoc, loansHome = loansHome, cardsDoc
	}
	before, err := document.Apply(nil, inverses[offersAt])
	if err != nil {
		t.Fatal(err)
	}
	restored, ok := before.(map[string]any)
	if !ok || restored["key"] != "offers" {
		t.Fatalf("restored offers = %v", before)
	}
	undo := []document.ChangeSetDraft{
		{PluginID: f.cards, Session: cardsLock, Edits: []document.ChangeSetEdit{{Path: movedTo, Delete: true, IfRevision: cardsDoc[0].Revision}}},
		{PluginID: f.loans, Session: loansLock, Edits: []document.ChangeSetEdit{
			{Path: offersAt, Content: otherPage("offers", "Offers")},
			{Path: homePath, IfRevision: loansHome[0].Revision, Patch: inverses[homePath]},
		}},
	}
	if _, err := f.docs.ApplyChangeSet(ctx, f.owner, f.app, document.Tag{EditorSession: "tab-1", Step: "undo-1"}, undo); err != nil {
		t.Fatal(err)
	}
	if _, err := f.docs.GetDocument(ctx, f.viewer, f.app, f.loans, offersAt); err != nil {
		t.Errorf("offers back in loans: %v", err)
	}
	if _, err := f.docs.GetDocument(ctx, f.viewer, f.app, f.cards, movedTo); code(err) != plxerr.ResourceNotFound {
		t.Errorf("offers still in cards: %v", err)
	}
}

// Verifies: STU-055, STU-070.
// A change set is refused whole: a stale draft revision, a stale document
// revision, a failed test operation, a lock the caller does not hold, or an
// invalid document in one draft leaves every draft as it was, including
// those processed before the failure.
func TestApplyChangeSetRefusals(t *testing.T) {
	t.Parallel()
	f := newChangeSetFixture(t)
	ctx := context.Background()
	refused := func(name string, want plxerr.Code, drafts []document.ChangeSetDraft) {
		t.Helper()
		_, err := f.docs.ApplyChangeSet(ctx, f.owner, f.app, document.Tag{EditorSession: "tab-1", Step: "refused"}, drafts)
		if code(err) != want {
			t.Errorf("%s: %v, want %v", name, err, want)
		}
		f.unchanged(t)
	}

	stale := f.move()
	stale[0].IfDraftRevision = f.cardsRev - 1
	refused("stale draft revision", plxerr.RevisionConflict, stale)

	staleDoc := f.move()
	staleDoc[1].Edits[0].IfRevision = f.offers + 7
	refused("stale document revision", plxerr.RevisionConflict, staleDoc)

	failedTest := f.move()
	failedTest[1].Edits[1].Patch[0].Value = "Elsewhere"
	refused("failed test", plxerr.InvalidStructure, failedTest)

	invalid := f.move()
	invalid[0].Edits[0].Content = []byte(`{"kind":"page"}`)
	refused("invalid document", plxerr.MissingProperty, invalid)

	if _, err := f.tenancy.SetOrganizationLimit(ctx, f.owner, string(limits.StudioChangeSetDocuments), 2); err != nil {
		t.Fatal(err)
	}
	refused("too many documents", plxerr.LimitExceeded, f.move())
	if _, err := f.tenancy.SetOrganizationLimit(ctx, f.owner, string(limits.StudioChangeSetDocuments), 3); err != nil {
		t.Fatal(err)
	}

	// The lock of the second draft written is not held: the first draft's
	// writes, already made in the transaction, roll back with it.
	if err := f.docs.ReleaseLock(ctx, f.owner, f.app, f.cards, cardsLock); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.docs.AcquireLock(ctx, f.developer, f.app, f.cards, "other-tab", false); err != nil {
		t.Fatal(err)
	}
	refused("lock not held", plxerr.EditingLockHeld, f.move())

	for name, drafts := range map[string][]document.ChangeSetDraft{
		"no drafts":   nil,
		"no edits":    {{PluginID: f.loans, Session: loansLock}},
		"draft twice": {{PluginID: f.loans, Session: loansLock, Edits: f.move()[1].Edits}, {PluginID: f.loans, Session: loansLock, Edits: f.move()[1].Edits}},
		"path twice":  {{PluginID: f.loans, Session: loansLock, Edits: append(f.move()[1].Edits, f.move()[1].Edits[0])}},
		"two kinds":   {{PluginID: f.loans, Session: loansLock, Edits: []document.ChangeSetEdit{{Path: offersAt, Delete: true, Content: []byte("{}"), IfRevision: 1}}}},
		"no revision": {{PluginID: f.loans, Session: loansLock, Edits: []document.ChangeSetEdit{{Path: offersAt, Delete: true}}}},
		"translations": {{Session: "app-lock", Edits: []document.ChangeSetEdit{
			{Path: "translations/de.json", Content: []byte("{}")},
		}}},
	} {
		if _, err := f.docs.ApplyChangeSet(ctx, f.owner, f.app, document.Tag{}, drafts); code(err) != plxerr.MissingProperty && code(err) != plxerr.InvalidFormat {
			t.Errorf("%s: %v", name, err)
		}
	}
	f.unchanged(t)
}
