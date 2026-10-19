// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package localisation_test

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/document"
	"github.com/nightCode42/plux3/backend/internal/localisation"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

const (
	app     = "app-1"
	idTitle = "01d0c450-6c00-7000-8000-000000000280"
	idUser  = "01d0c450-6c00-7000-8000-000000000281"
)

// fakeDocs is an in-memory document service that records the order of its
// calls.
type fakeDocs struct {
	files map[string]string
	rev   map[string]int64
	calls []string
	// putErr, when set, fails every save.
	putErr error
}

func newDocs() *fakeDocs {
	return &fakeDocs{
		files: map[string]string{
			"app.json":               `{"defaultLocale":"en","supportedLocales":["en","de","fr"]}`,
			"translations/keys.json": `{"keys":[{"id":"` + idTitle + `","key":"login.title","description":"Heading","maxLength":40},{"id":"` + idUser + `","key":"login.username"}]}`,
			"translations/en.json":   `{"id":"e","kind":"translations","locale":"en","messages":{"` + idTitle + `":"Welcome","` + idUser + `":"Username"}}`,
			"translations/de.json":   `{"id":"d","kind":"translations","locale":"de","messages":{"` + idTitle + `":"Willkommen"},"states":{"` + idTitle + `":"reviewed"},"x-note":"kept"}`,
		},
		rev: map[string]int64{"translations/de.json": 7},
	}
}

func (f *fakeDocs) GetDocument(_ context.Context, _ auth.Principal, _, _, path string) (document.Document, error) {
	c, ok := f.files[path]
	if !ok {
		return document.Document{}, plxerr.New(plxerr.ResourceNotFound, "no such document")
	}
	return document.Document{Path: path, Content: []byte(c), Revision: f.rev[path]}, nil
}

func (f *fakeDocs) ListDocuments(_ context.Context, _ auth.Principal, _, _, after string, _ int32, _ bool) ([]document.Document, error) {
	var paths []string
	for p := range f.files {
		if p > after {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	var out []document.Document
	for _, p := range paths {
		out = append(out, document.Document{Path: p, Content: []byte(f.files[p])})
	}
	return out, nil
}

func (f *fakeDocs) PutDocument(_ context.Context, _ auth.Principal, _, _, session, path string, content []byte, rev int64) (document.Written, error) {
	f.calls = append(f.calls, fmt.Sprintf("put %s rev=%d session=%s", path, rev, strings.SplitN(session, "-", 2)[0]))
	if f.putErr != nil {
		return document.Written{}, f.putErr
	}
	f.files[path] = string(content)
	return document.Written{}, nil
}

func (f *fakeDocs) AcquireScopedLock(_ context.Context, _ auth.Principal, _, _, _, scope string, _ bool) (document.Lock, string, error) {
	f.calls = append(f.calls, "lock "+scope)
	return document.Lock{}, "", nil
}

func (f *fakeDocs) ReleaseScopedLock(_ context.Context, _ auth.Principal, _, _, _, scope string) error {
	f.calls = append(f.calls, "release "+scope)
	return nil
}

type ids struct{ n int }

func (i *ids) New() (string, error) {
	i.n++
	return fmt.Sprintf("00000000-0000-7000-8000-%012d", i.n), nil
}

func principal(perms ...auth.Permission) auth.Principal {
	return auth.Principal{OrganizationID: "org", AppPermissions: map[string][]auth.Permission{app: perms}}
}

var (
	reader     = principal(auth.AppRead)
	translator = principal(auth.AppRead, auth.TranslationsEdit)
	approver   = principal(auth.AppRead, auth.TranslationsEdit, auth.TranslationsApprove)
)

func newService(f *fakeDocs) *localisation.Service {
	return localisation.New(f, &ids{}, limits.Defaults())
}

func code(t *testing.T, err error) plxerr.Code {
	t.Helper()
	var pe *plxerr.Error
	if !errors.As(err, &pe) {
		t.Fatalf("error %v has no code", err)
	}
	return pe.Code
}

// Verifies: I18N-009. Export needs app.read only, uses the default locale
// as the source, and lets a locale without a document be exported.
func TestExport(t *testing.T) {
	t.Parallel()
	s := newService(newDocs())
	got, err := s.Export(context.Background(), reader, app, "fr", "xliff")
	if err != nil {
		t.Fatal(err)
	}
	if got.SourceLocale != "en" || !strings.Contains(string(got.Content), `srcLang="en" trgLang="fr"`) ||
		!strings.Contains(string(got.Content), "<source>Welcome</source>") {
		t.Errorf("export = %s", got.Content)
	}
	if _, err := s.Export(context.Background(), principal(), app, "fr", "xliff"); code(t, err) != plxerr.PermissionDenied {
		t.Errorf("without app.read: %v", err)
	}
	if _, err := s.Export(context.Background(), reader, app, "fr", "docx"); code(t, err) != plxerr.InvalidEnumValue {
		t.Errorf("unknown format: %v", err)
	}
	if _, err := s.Export(context.Background(), reader, app, "../x", "csv"); code(t, err) != plxerr.InvalidFormat {
		t.Errorf("bad locale: %v", err)
	}
}

// Verifies: I18N-009, I18N-008. An import saves through the document service
// under the locale's lock, keeps the document's other properties, and
// releases the lock.
func TestImportSaves(t *testing.T) {
	t.Parallel()
	f := newDocs()
	s := newService(f)
	csv := "keyId,target\n" + idUser + ",Benutzer\n"
	got, err := s.Import(context.Background(), translator, app, "de", "csv", []byte(csv), false)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Saved || len(got.ChangedKeys) != 1 || got.ChangedKeys[0] != idUser {
		t.Fatalf("result = %+v", got)
	}
	want := []string{"lock translations/de", "put translations/de.json rev=7 session=l10n", "release translations/de"}
	if strings.Join(f.calls, "|") != strings.Join(want, "|") {
		t.Errorf("calls = %q, want %q", f.calls, want)
	}
	saved := f.files["translations/de.json"]
	for _, w := range []string{`"x-note":"kept"`, `"` + idUser + `":"Benutzer"`, `"` + idUser + `":"translated"`, `"` + idTitle + `":"reviewed"`} {
		if !strings.Contains(saved, w) {
			t.Errorf("saved document lacks %s:\n%s", w, saved)
		}
	}
}

// Verifies: I18N-009. A dry run saves nothing and takes no lock; an import
// that changes nothing saves nothing.
func TestImportDryRunAndNoChange(t *testing.T) {
	t.Parallel()
	f := newDocs()
	s := newService(f)
	csv := "keyId,target\n" + idUser + ",Benutzer\n"
	got, err := s.Import(context.Background(), translator, app, "de", "csv", []byte(csv), true)
	if err != nil || got.Saved || len(got.ChangedKeys) != 1 {
		t.Fatalf("dry run = %+v, %v", got, err)
	}
	same := "keyId,target\n" + idTitle + ",Willkommen\n"
	got, err = s.Import(context.Background(), translator, app, "de", "csv", []byte(same), false)
	if err != nil || got.Saved || len(got.ChangedKeys) != 0 {
		t.Fatalf("no-change import = %+v, %v", got, err)
	}
	if len(f.calls) != 0 {
		t.Errorf("calls = %q, want none", f.calls)
	}
}

// Verifies: I18N-009. A new locale gets a document with a fresh identifier,
// created, not updated.
func TestImportCreatesLocale(t *testing.T) {
	t.Parallel()
	f := newDocs()
	s := newService(f)
	arb := `{"@@locale":"fr","login.title":"Bienvenue"}`
	got, err := s.Import(context.Background(), approver, app, "fr", "arb", []byte(arb), false)
	if err != nil || !got.Saved {
		t.Fatalf("import = %+v, %v", got, err)
	}
	if f.calls[1] != "put translations/fr.json rev=0 session=l10n" {
		t.Errorf("calls = %q", f.calls)
	}
	for _, w := range []string{`"kind":"translations"`, `"locale":"fr"`, `"id":"00000000-0000-7000-8000-000000000001"`, `"` + idTitle + `":"Bienvenue"`} {
		if !strings.Contains(f.files["translations/fr.json"], w) {
			t.Errorf("document lacks %s:\n%s", w, f.files["translations/fr.json"])
		}
	}
}

// Verifies: I18N-008, I18N-009. Importing needs translations.edit; the
// approved state in a file needs translations.approve; unknown keys and
// changed sources are reported.
func TestImportAuthorisationAndDiagnostics(t *testing.T) {
	t.Parallel()
	s := newService(newDocs())
	csv := "keyId,source,target,state\n" + idUser + ",Username,Benutzer,approved\n" + idTitle + ",Hello,Willkommen!,\nnope,,x,\n"
	if _, err := s.Import(context.Background(), reader, app, "de", "csv", []byte(csv), true); code(t, err) != plxerr.PermissionDenied {
		t.Errorf("reader: %v", err)
	}
	got, err := s.Import(context.Background(), translator, app, "de", "csv", []byte(csv), true)
	if err != nil {
		t.Fatal(err)
	}
	var unknown, changed int
	for _, d := range got.Diagnostics {
		switch d.Code {
		case plxerr.TranslationImportKeyUnknown:
			unknown++
		case plxerr.TranslationImportSourceChanged:
			changed++
		}
	}
	if unknown != 1 || changed != 1 {
		t.Errorf("diagnostics = %v", got.Diagnostics)
	}
}

// Verifies: I18N-009. A file larger than translation.importSize is refused.
func TestImportSizeLimit(t *testing.T) {
	t.Parallel()
	s := newService(newDocs())
	big := []byte("keyId,target\n" + strings.Repeat("x", int(limits.Defaults().Get(limits.TranslationImportSize))))
	_, err := s.Import(context.Background(), translator, app, "de", "csv", big, true)
	if code(t, err) != plxerr.LimitExceeded {
		t.Errorf("err = %v", err)
	}
}

// Verifies: I18N-009. The lock is released when the save fails.
func TestImportReleasesLockOnFailure(t *testing.T) {
	t.Parallel()
	f := newDocs()
	f.putErr = plxerr.New(plxerr.TranslationDocumentForbidden, "no")
	s := newService(f)
	_, err := s.Import(context.Background(), translator, app, "de", "csv", []byte("keyId,target\n"+idUser+",Benutzer\n"), false)
	if code(t, err) != plxerr.TranslationDocumentForbidden {
		t.Fatalf("err = %v", err)
	}
	if last := f.calls[len(f.calls)-1]; last != "release translations/de" {
		t.Errorf("calls = %q", f.calls)
	}
}

// Verifies: I18N-008. SetStates sets states, refuses approved without
// translations.approve, unknown keys and keys without a translation.
func TestSetStates(t *testing.T) {
	t.Parallel()
	f := newDocs()
	s := newService(f)
	ctx := context.Background()
	changed, err := s.SetStates(ctx, translator, app, "de", map[string]string{idTitle: "translated"})
	if err != nil || len(changed) != 1 {
		t.Fatalf("changed = %v, %v", changed, err)
	}
	if !strings.Contains(f.files["translations/de.json"], `"`+idTitle+`":"translated"`) {
		t.Errorf("saved:\n%s", f.files["translations/de.json"])
	}
	_, err = s.SetStates(ctx, translator, app, "de", map[string]string{idTitle: "approved"})
	if code(t, err) != plxerr.TranslationStateForbidden {
		t.Errorf("approve as translator: %v", err)
	}
	if _, err = s.SetStates(ctx, approver, app, "de", map[string]string{idTitle: "approved"}); err != nil {
		t.Errorf("approve as approver: %v", err)
	}
	if _, err = s.SetStates(ctx, translator, app, "de", map[string]string{"zzz": "reviewed"}); code(t, err) != plxerr.TranslationImportKeyUnknown {
		t.Errorf("unknown key: %v", err)
	}
	if _, err = s.SetStates(ctx, translator, app, "de", map[string]string{idUser: "reviewed"}); code(t, err) != plxerr.InvalidFormat {
		t.Errorf("untranslated key: %v", err)
	}
	if _, err = s.SetStates(ctx, translator, app, "de", map[string]string{idTitle: "done"}); code(t, err) != plxerr.InvalidEnumValue {
		t.Errorf("bad state: %v", err)
	}
	if _, err = s.SetStates(ctx, reader, app, "de", map[string]string{idTitle: "new"}); code(t, err) != plxerr.PermissionDenied {
		t.Errorf("reader: %v", err)
	}
}

// Verifies: I18N-008. Status counts the declared keys by state for every
// supported locale and every locale with a document.
func TestStatus(t *testing.T) {
	t.Parallel()
	s := newService(newDocs())
	got, err := s.Status(context.Background(), reader, app)
	if err != nil {
		t.Fatal(err)
	}
	if got.SourceLocale != "en" || len(got.Locales) != 3 {
		t.Fatalf("status = %+v", got)
	}
	want := map[string]localisation.LocaleStatus{
		"de": {Locale: "de", Total: 2, Reviewed: 1, Missing: 1},
		"en": {Locale: "en", Total: 2, New: 2},
		"fr": {Locale: "fr", Total: 2, Missing: 2},
	}
	for _, l := range got.Locales {
		if l != want[l.Locale] {
			t.Errorf("%s = %+v, want %+v", l.Locale, l, want[l.Locale])
		}
	}
	if _, err := s.Status(context.Background(), principal(), app); code(t, err) != plxerr.PermissionDenied {
		t.Errorf("without app.read: %v", err)
	}
}
