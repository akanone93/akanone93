// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package document_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/document"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// Message keys of the tests, as the document names them (SCH-002).
const (
	keyHello = "01a0c450-6c00-7011-8000-0000000a0001"
	keyBye   = "01a0c450-6c00-7011-8000-0000000a0002"
)

// translationsDoc is the translations document of a locale, as its file
// holds it; extra are extension properties.
func translationsDoc(t *testing.T, locale string, messages, states map[string]string, extra map[string]any) []byte {
	t.Helper()
	doc := map[string]any{
		"schemaVersion": "1.0.0", "kind": "translations", "locale": locale,
		"id": "01a0c450-6c00-7011-8000-0000000b0001", "messages": messages,
	}
	if len(states) > 0 {
		doc["states"] = states
	}
	for k, v := range extra {
		doc[k] = v
	}
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// translationsWorld is a fixture with the app's German and Arabic
// translations, written by the owner, and a member in each role that
// matters to translating.
type translationsWorld struct {
	*fixture
	admin, translator, second auth.Principal
}

func newTranslationsWorld(t *testing.T) *translationsWorld {
	t.Helper()
	ctx := context.Background()
	w := &translationsWorld{fixture: newFixture(t)}
	w.admin = w.member(t, "admin2@example.com", "admin")
	w.translator = w.member(t, "tr1@example.com", "translator")
	w.second = w.member(t, "tr2@example.com", "translator")
	if _, _, err := w.docs.AcquireLock(ctx, w.owner, w.app, "", "own", false); err != nil {
		t.Fatal(err)
	}
	for locale, text := range map[string]string{"de": "Hallo", "ar": "مرحبا"} {
		doc := translationsDoc(t, locale, map[string]string{keyHello: text}, map[string]string{keyHello: "translated"}, nil)
		if _, err := w.docs.PutDocument(ctx, w.owner, w.app, "", "own", "translations/"+locale+".json", doc, 0); err != nil {
			t.Fatalf("PutDocument %s: %v", locale, err)
		}
	}
	return w
}

// germanTranslations is the path of the German translations document.
const germanTranslations = "translations/de.json"

// revision returns the stored revision of a document.
func (w *translationsWorld) revision(ctx context.Context, t *testing.T, path string) int64 {
	t.Helper()
	d, err := w.docs.GetDocument(ctx, w.owner, w.app, "", path)
	if err != nil {
		t.Fatal(err)
	}
	return d.Revision
}

// stored returns the messages and states of the stored German translations
// document.
func (w *translationsWorld) stored(t *testing.T) (messages, states map[string]string) {
	t.Helper()
	d, err := w.docs.GetDocument(context.Background(), w.owner, w.app, "", germanTranslations)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Messages map[string]string `json:"messages"`
		States   map[string]string `json:"states"`
	}
	if err := json.Unmarshal(d.Content, &doc); err != nil {
		t.Fatal(err)
	}
	return doc.Messages, doc.States
}

// Verifies: I18N-008.
// A translator saves the messages and states of a locale under that
// locale's lock, and nothing else: not the draft lock, not another
// document, not another property, not a new locale, not a deletion.
func TestTranslatorSavesUnderLocaleLock_I18N_008(t *testing.T) {
	w := newTranslationsWorld(t)
	ctx := context.Background()
	const path = "translations/de.json"
	scope := document.TranslationsScope("de")

	if _, _, err := w.docs.AcquireLock(ctx, w.translator, w.app, "", "tr", false); code(err) != plxerr.PermissionDenied {
		t.Fatalf("a translator took the draft lock: %v", err)
	}
	edit := translationsDoc(t, "de", map[string]string{keyHello: "Hallo Welt", keyBye: "Tschüss"},
		map[string]string{keyHello: "translated", keyBye: "translated"}, nil)
	if _, err := w.docs.PutDocument(ctx, w.translator, w.app, "", "tr", path, edit, w.revision(ctx, t, path)); code(err) != plxerr.EditingLockHeld {
		t.Fatalf("a save without the locale lock: %v", err)
	}
	l, _, err := w.docs.AcquireScopedLock(ctx, w.translator, w.app, "", "tr", scope, false)
	if err != nil || l.Scope != scope || l.Session != "tr" {
		t.Fatalf("AcquireScopedLock = %+v, %v", l, err)
	}
	written, err := w.docs.PutDocument(ctx, w.translator, w.app, "", "tr", path, edit, w.revision(ctx, t, path))
	if err != nil || len(written.Documents) != 1 {
		t.Fatalf("a translator's save under the locale lock: %v", err)
	}
	if m, s := w.stored(t); m[keyBye] != "Tschüss" || s[keyBye] != "translated" {
		t.Errorf("stored %v %v", m, s)
	}
	patched, err := w.docs.PatchDocument(ctx, w.translator, w.app, "", "tr", path,
		[]document.Op{{Op: "replace", Path: "/messages/" + keyBye, Value: "Auf Wiedersehen"}}, w.revision(ctx, t, path))
	if err != nil || len(patched.Documents) != 1 {
		t.Fatalf("a translator's patch: %v", err)
	}

	rev := w.revision(ctx, t, path)
	for name, c := range map[string]struct {
		path string
		do   func() error
	}{
		"app.json": {"app.json", func() error {
			_, err := w.docs.PutDocument(ctx, w.translator, w.app, "", "tr", "app.json", []byte(`{}`), 0)
			return err
		}},
		"keys.json": {"translations/keys.json", func() error {
			_, err := w.docs.PutDocument(ctx, w.translator, w.app, "", "tr", "translations/keys.json", []byte(`{}`), 0)
			return err
		}},
		"another property": {path, func() error {
			other := translationsDoc(t, "de", map[string]string{keyHello: "Hallo"}, nil, map[string]any{"x-note": "mine"})
			_, err := w.docs.PutDocument(ctx, w.translator, w.app, "", "tr", path, other, rev)
			return err
		}},
		"a new locale": {"translations/fr.json", func() error {
			fresh := translationsDoc(t, "fr", map[string]string{keyHello: "Bonjour"}, nil, nil)
			_, err := w.docs.PutDocument(ctx, w.translator, w.app, "", "tr", "translations/fr.json", fresh, 0)
			return err
		}},
		"a deletion": {path, func() error {
			_, err := w.docs.DeleteDocument(ctx, w.translator, w.app, "", "tr", path, rev)
			return err
		}},
	} {
		if err := c.do(); code(err) != plxerr.TranslationDocumentForbidden {
			t.Errorf("%s: %v, want PLX-8093", name, err)
		}
	}
	if m, _ := w.stored(t); len(m) != 2 {
		t.Errorf("a refused save changed the document: %v", m)
	}
	if _, err := w.docs.PutDocument(ctx, w.translator, w.app, "", "tr", "translations/ar.json",
		translationsDoc(t, "ar", map[string]string{keyHello: "أهلا"}, nil, nil), w.revision(ctx, t, "translations/ar.json")); code(err) != plxerr.EditingLockHeld {
		t.Errorf("a save of another locale under this locale's lock: %v", err)
	}
	// A viewer holds neither permission.
	if _, _, err := w.docs.AcquireScopedLock(ctx, w.viewer, w.app, "", "v", scope, false); code(err) != plxerr.PermissionDenied {
		t.Errorf("a viewer took a locale lock: %v", err)
	}
	if err := w.docs.ReleaseScopedLock(ctx, w.translator, w.app, "", "tr", scope); err != nil {
		t.Errorf("ReleaseScopedLock: %v", err)
	}
	if got, err := w.docs.GetScopedLock(ctx, w.owner, w.app, "", scope); err != nil || got.Session != "" {
		t.Errorf("the locale lock after its release: %+v %v", got, err)
	}
}

// Verifies: I18N-008.
// Approving needs translations.approve and a second factor; a changed
// text without it is saved as translated; the owner approves.
func TestTranslationStatuses_I18N_008(t *testing.T) {
	w := newTranslationsWorld(t)
	ctx := context.Background()
	const path = "translations/de.json"
	scope := document.TranslationsScope("de")
	save := func(p auth.Principal, session string, messages, states map[string]string) error {
		_, err := w.docs.PutDocument(ctx, p, w.app, "", session, path, translationsDoc(t, "de", messages, states, nil), w.revision(ctx, t, path))
		return err
	}
	if _, _, err := w.docs.AcquireScopedLock(ctx, w.developer, w.app, "", "dev", scope, false); err != nil {
		t.Fatalf("a developer takes a locale lock: %v", err)
	}

	// Setting a state to approved needs translations.approve (PLX-8092).
	if err := save(w.developer, "dev", map[string]string{keyHello: "Hallo"}, map[string]string{keyHello: "approved"}); code(err) != plxerr.TranslationStateForbidden {
		t.Errorf("a developer approving: %v", err)
	}
	if err := save(w.translator, "tr", map[string]string{keyHello: "Hallo"}, map[string]string{keyHello: "approved"}); code(err) == 0 {
		t.Errorf("a translator without the locale lock approving: %v", err)
	}

	// A text change resets the key to translated, whatever state is sent,
	// and one without a state is saved as translated.
	if err := save(w.developer, "dev", map[string]string{keyHello: "Hallo!"}, map[string]string{keyHello: "reviewed"}); err != nil {
		t.Fatalf("a developer's text change: %v", err)
	}
	if m, s := w.stored(t); m[keyHello] != "Hallo!" || s[keyHello] != "translated" {
		t.Errorf("after a text change with state reviewed: %v %v", m, s)
	}
	if err := save(w.developer, "dev", map[string]string{keyHello: "Hallo!", keyBye: "Tschüss"}, nil); err != nil {
		t.Fatalf("a developer's new message: %v", err)
	}
	if m, s := w.stored(t); m[keyBye] != "Tschüss" || s[keyBye] != "translated" {
		t.Errorf("after a new message without a state: %v %v", m, s)
	}
	if _, s := w.stored(t); s[keyHello] != "translated" {
		t.Errorf("a state left out of a save is not kept: %v", s)
	}
	// Without a text change the state is the caller's to set.
	if err := save(w.developer, "dev", map[string]string{keyHello: "Hallo!", keyBye: "Tschüss"},
		map[string]string{keyHello: "reviewed", keyBye: "translated"}); err != nil {
		t.Fatalf("a developer's review: %v", err)
	}
	if _, s := w.stored(t); s[keyHello] != "reviewed" {
		t.Errorf("a review without a text change: %v", s)
	}

	// The owner approves, without a second factor not.
	unverified := w.owner
	unverified.SecondFactor = false
	if _, _, err := w.docs.AcquireScopedLock(ctx, w.owner, w.app, "", "own", scope, false); code(err) != plxerr.EditingLockHeld {
		t.Fatalf("the owner taking the developer's locale lock: %v", err)
	}
	approved := map[string]string{keyHello: "approved", keyBye: "translated"}
	if err := save(unverified, "own", map[string]string{keyHello: "Hallo!", keyBye: "Tschüss"}, approved); code(err) != plxerr.MultiFactorRequired {
		t.Errorf("an approval without a second factor: %v", err)
	}
	if err := save(w.owner, "own", map[string]string{keyHello: "Hallo!", keyBye: "Tschüss"}, approved); err != nil {
		t.Fatalf("the owner approving under the draft lock: %v", err)
	}
	if _, s := w.stored(t); s[keyHello] != "approved" {
		t.Fatalf("approved state: %v", s)
	}

	// A state left out of a save is kept, so a developer cannot drop an
	// approval by omission; an explicit state, or a changed text, alters it.
	texts := map[string]string{keyHello: "Hallo!", keyBye: "Tschüss"}
	if err := save(w.developer, "dev", texts, nil); err != nil {
		t.Fatalf("a developer's save without states: %v", err)
	}
	if _, s := w.stored(t); s[keyHello] != "approved" || s[keyBye] != "translated" {
		t.Errorf("states after a save that left them out: %v", s)
	}
	if err := save(w.developer, "dev", map[string]string{keyHello: "Hallo!"}, nil); err != nil {
		t.Fatalf("a developer removing a message: %v", err)
	}
	if m, s := w.stored(t); len(m) != 1 || s[keyBye] != "" || s[keyHello] != "approved" {
		t.Errorf("after removing a message: %v %v", m, s)
	}
	if err := save(w.developer, "dev", texts, map[string]string{keyHello: "approved"}); err != nil {
		t.Fatalf("a developer re-adding a message: %v", err)
	}

	// An approved message whose text changes stays approved only for
	// someone who may approve.
	if err := save(w.developer, "dev", map[string]string{keyHello: "Hallo?", keyBye: "Tschüss"}, approved); code(err) != plxerr.TranslationStateForbidden {
		t.Errorf("a developer changing an approved text: %v", err)
	}
	if err := save(w.developer, "dev", map[string]string{keyHello: "Hallo?", keyBye: "Tschüss"},
		map[string]string{keyHello: "translated", keyBye: "translated"}); err != nil {
		t.Errorf("a developer demoting a changed text: %v", err)
	}
	if err := save(w.owner, "own", map[string]string{keyHello: "Hallo?", keyBye: "Tschüss"}, approved); err != nil {
		t.Errorf("the owner re-approving: %v", err)
	}
	if err := save(w.admin, "own", map[string]string{keyHello: "Hallo.", keyBye: "Tschüss"}, approved); code(err) != plxerr.EditingLockHeld {
		t.Errorf("the admin saving under the owner's lock: %v", err)
	}
	// The owner's lease lapses after two missed heartbeats (L-1).
	w.now = w.now.Add(document.LockLeaseLapse)
	if _, _, err := w.docs.AcquireLock(ctx, w.admin, w.app, "", "adm", true); err != nil {
		t.Fatalf("the admin taking the draft lock over: %v", err)
	}
	if err := save(w.admin, "adm", map[string]string{keyHello: "Hallo.", keyBye: "Tschüss"}, approved); err != nil {
		t.Errorf("the admin changing an approved text: %v", err)
	}
	if m, s := w.stored(t); m[keyHello] != "Hallo." || s[keyHello] != "approved" {
		t.Errorf("after the admin: %v %v", m, s)
	}
}

// Verifies: I18N-008.
// Two translators hold the locks of two locales while an administrator
// holds the draft lock; a locale lock is held by one session at a time,
// and a locale lock covers its own document only.
func TestLocaleLocksAreIndependent_I18N_008(t *testing.T) {
	w := newTranslationsWorld(t)
	ctx := context.Background()
	de, ar := document.TranslationsScope("de"), document.TranslationsScope("ar")

	w.now = w.now.Add(document.LockLeaseLapse)
	if _, _, err := w.docs.AcquireLock(ctx, w.admin, w.app, "", "adm", true); err != nil {
		t.Fatalf("the admin takes the draft lock: %v", err)
	}
	if _, _, err := w.docs.AcquireScopedLock(ctx, w.translator, w.app, "", "tr1", de, false); err != nil {
		t.Fatalf("translator one takes de: %v", err)
	}
	if _, _, err := w.docs.AcquireScopedLock(ctx, w.second, w.app, "", "tr2", ar, false); err != nil {
		t.Fatalf("translator two takes ar: %v", err)
	}
	if _, _, err := w.docs.AcquireScopedLock(ctx, w.second, w.app, "", "tr2", de, false); code(err) != plxerr.EditingLockHeld {
		t.Errorf("translator two takes de: %v", err)
	}
	if _, _, err := w.docs.AcquireScopedLock(ctx, w.second, w.app, "", "tr2", de, true); code(err) != plxerr.PermissionDenied {
		t.Errorf("translator two takes de over: %v", err)
	}
	if got, err := w.docs.GetLock(ctx, w.owner, w.app, ""); err != nil || got.Session != "adm" || got.Scope != "" {
		t.Errorf("the draft lock = %+v, %v", got, err)
	}
	if _, err := w.docs.RenewScopedLock(ctx, w.translator, w.app, "", "tr1", de); err != nil {
		t.Errorf("RenewScopedLock: %v", err)
	}
	if _, err := w.docs.RenewScopedLock(ctx, w.second, w.app, "", "tr2", de); code(err) != plxerr.EditingLockHeld {
		t.Errorf("renewing a lock it does not hold: %v", err)
	}
	if got, err := w.docs.RequestScopedLock(ctx, w.second, w.app, "", de); err != nil || len(got.RequestedBy) != 1 {
		t.Errorf("RequestScopedLock = %+v, %v", got, err)
	}

	saves := map[string]struct {
		who     auth.Principal
		session string
		locale  string
		want    plxerr.Code
	}{
		"one saves de":        {w.translator, "tr1", "de", 0},
		"two saves ar":        {w.second, "tr2", "ar", 0},
		"one saves ar":        {w.translator, "tr1", "ar", plxerr.EditingLockHeld},
		"two saves de":        {w.second, "tr2", "de", plxerr.EditingLockHeld},
		"the admin saves de":  {w.admin, "adm", "de", 0},
		"the admin, wrongly":  {w.admin, "tr1", "de", plxerr.EditingLockHeld},
		"the owner, no lock":  {w.owner, "own2", "de", plxerr.EditingLockHeld},
		"a viewer saves de":   {w.viewer, "v", "de", plxerr.PermissionDenied},
		"a developer, no one": {w.developer, "dev", "de", plxerr.EditingLockHeld},
	}
	for name, c := range saves {
		path := "translations/" + c.locale + ".json"
		doc := translationsDoc(t, c.locale, map[string]string{keyHello: name}, map[string]string{keyHello: "translated"}, nil)
		_, err := w.docs.PutDocument(ctx, c.who, w.app, "", c.session, path, doc, w.revision(ctx, t, path))
		if code(err) != c.want {
			t.Errorf("%s: %v, want code %d", name, err, c.want)
		}
	}

	for name, scope := range map[string]string{"another document": "app", "a path": "translations/../x", "no locale": "translations/", "an upper-case locale": "translations/DE", "a region only": "translations/de-de"} {
		if _, _, err := w.docs.AcquireScopedLock(ctx, w.owner, w.app, "", "own", scope, false); code(err) != plxerr.InvalidFormat {
			t.Errorf("a scope of %s: %v", name, err)
		}
	}
	p := w.plugin(t)
	if _, _, err := w.docs.AcquireScopedLock(ctx, w.owner, w.app, p, "own", de, false); code(err) != plxerr.InvalidFormat {
		t.Errorf("a locale lock of a plugin: %v", err)
	}
	if err := w.docs.ReleaseScopedLock(ctx, w.translator, w.app, "", "tr1", de); err != nil {
		t.Errorf("release de: %v", err)
	}
	if _, _, err := w.docs.AcquireScopedLock(ctx, w.second, w.app, "", "tr2", de, false); err != nil {
		t.Errorf("translator two takes de after the release: %v", err)
	}
}
