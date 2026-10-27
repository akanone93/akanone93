// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package library_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/cache"
	"github.com/nightCode42/plux3/backend/internal/library"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/schema/uuid7"
	"github.com/nightCode42/plux3/backend/internal/signing"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/storagetest"
	"github.com/nightCode42/plux3/backend/internal/tenancy"
)

type ids struct{ g *uuid7.Generator }

func (i ids) New() (string, error) {
	u, err := i.g.New()
	return u.String(), err
}

// instances answers the uses of a component from a table.
type instances map[string][]library.Instance

func (i instances) ComponentInstances(_ context.Context, _ auth.Principal, _, componentID string) ([]library.Instance, error) {
	return i[componentID], nil
}

// fixture is two organisations, each with an owner and an app, and in the
// first a developer, who may author but not manage the library, and a viewer.
type fixture struct {
	db        *storage.DB
	svc       *library.Service
	found     instances
	ownerA    auth.Principal
	ownerB    auth.Principal
	developer auth.Principal
	viewer    auth.Principal
	appA      string
	appB      string
	counter   atomic.Int64
}

func newFixture(t *testing.T, tighten func(limits.Set) limits.Set) *fixture {
	t.Helper()
	ctx := context.Background()
	db := storagetest.Open(t)
	backend, err := signing.NewFile(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	gen := ids{g: uuid7.NewGenerator(time.Now, rand.Reader)}
	log := audit.NewLog(gen, nil)
	authService, err := auth.NewService(auth.Options{DB: db, Audit: log, Cache: cache.NewMemory(nil), Crypter: backend, IDs: gen, VerificationURI: "https://p.example/device"})
	if err != nil {
		t.Fatal(err)
	}
	ten, err := tenancy.NewService(tenancy.Options{DB: db, Audit: log, Auth: authService, Crypter: backend, IDs: gen, SigningKeyPrefix: "targets"})
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{db: db, found: instances{}}
	opts := library.Options{DB: db, Audit: log, IDs: gen, Instances: f.found}
	if tighten != nil {
		set := tighten(limits.Defaults())
		opts.Limits = func(context.Context, pgx.Tx, string) (limits.Set, error) { return set, nil }
	}
	if f.svc, err = library.NewService(opts); err != nil {
		t.Fatal(err)
	}
	admin, invitation, err := authService.Bootstrap(ctx, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := authService.AcceptInvitation(ctx, invitation, "Admin", "correct horse battery"); err != nil {
		t.Fatal(err)
	}
	adminID := auth.Identity{Kind: auth.KindUser, ID: admin.ID, UserID: admin.ID, Display: "Admin", SecondFactor: true, InstallationAdmin: true}.WithStepUp(0)
	resolve := func(id auth.Identity, org string) auth.Principal {
		p, err := authService.Resolve(ctx, id, org)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	orgA, err := ten.CreateOrganization(ctx, adminID, "acme", "Acme")
	if err != nil {
		t.Fatal(err)
	}
	orgB, err := ten.CreateOrganization(ctx, adminID, "beta", "Beta")
	if err != nil {
		t.Fatal(err)
	}
	f.ownerA, f.ownerB = resolve(adminID, orgA.ID), resolve(adminID, orgB.ID)
	member := func(email, role string) auth.Principal {
		m, _, err := ten.AddMember(ctx, f.ownerA, "", email, role)
		if err != nil {
			t.Fatal(err)
		}
		return resolve(auth.Identity{Kind: auth.KindUser, ID: m.UserID, UserID: m.UserID, Display: role, SecondFactor: true}.WithStepUp(0), orgA.ID)
	}
	f.developer, f.viewer = member("dev@example.com", "developer"), member("viewer@example.com", "viewer")
	appA, err := ten.CreateApp(ctx, f.ownerA, "demo", "Demo")
	if err != nil {
		t.Fatal(err)
	}
	appB, err := ten.CreateApp(ctx, f.ownerB, "other", "Other")
	if err != nil {
		t.Fatal(err)
	}
	f.appA, f.appB = appA.ID, appB.ID
	return f
}

// uuid makes a UUIDv7-shaped document identifier no other call made.
func (f *fixture) uuid() string {
	n := f.counter.Add(1)
	return fmt.Sprintf("01a0c450-6c00-7%03x-8000-%012x", n, n)
}

// doc is a template or component document with a text node holding data.
func (f *fixture) doc(kind, key, data string, props ...string) []byte {
	root := fmt.Sprintf(`{"id":%q,"type":"Text","props":{"data":%q}}`, f.uuid(), data)
	if kind == "template" {
		return []byte(fmt.Sprintf(`{"schemaVersion":"1.0.0","kind":"template","id":%q,"key":%q,"name":"Item","category":"display","visibility":"private","root":%s}`, f.uuid(), key, root))
	}
	var list []string
	for _, p := range props {
		list = append(list, fmt.Sprintf(`{"name":%q,"type":"string"}`, p))
	}
	return []byte(fmt.Sprintf(`{"schemaVersion":"1.0.0","kind":"component","id":%q,"key":%q,"name":"Item","version":1,"props":[%s],"root":%s}`, f.uuid(), key, strings.Join(list, ","), root))
}

// sameDoc is doc with the identifier and key of an earlier document, as a
// revision has them.
func sameDoc(t *testing.T, original []byte, data string) []byte {
	t.Helper()
	i := bytes.Index(original, []byte(`"data":"`))
	j := bytes.Index(original[i+8:], []byte(`"`))
	return append(append(append([]byte{}, original[:i+8]...), data...), original[i+8+j:]...)
}

func codeIs(err error, code plxerr.Code) bool {
	return err != nil && errors.Is(err, plxerr.New(code, ""))
}

const (
	tpl  = library.KindTemplate
	comp = library.KindComponent
)

// Verifies: STU-080, STU-081.
// A template is made private, revised into immutable versions with a guard
// against lost updates, listed newest first, and only its author sees it
// until it is shared.
func TestAuthoring_VersionsAndPrivacy_STU_080(t *testing.T) {
	t.Parallel()
	f := newFixture(t, nil)
	ctx := context.Background()
	content := f.doc("template", "card", "one")

	item, v1, err := f.svc.Create(ctx, f.developer, tpl, library.Draft{Content: content, Note: "first"})
	if err != nil || item.Visibility != library.Private || item.HeadVersion != 1 || v1.Number != 1 || item.TemplateKind != "page" {
		t.Fatalf("Create = %+v, %+v, %v", item, v1, err)
	}
	if _, _, err := f.svc.Create(ctx, f.viewer, tpl, library.Draft{Content: f.doc("template", "viewer", "x")}); !codeIs(err, plxerr.PermissionDenied) {
		t.Errorf("a viewer created a template: %v", err)
	}
	if _, _, err := f.svc.Create(ctx, f.developer, tpl, library.Draft{Content: content}); !codeIs(err, plxerr.ResourceExists) {
		t.Errorf("a second template with the key = %v, want ResourceExists", err)
	}
	if _, _, err := f.svc.Create(ctx, f.developer, tpl, library.Draft{Content: []byte(`{"kind":"template"}`)}); !codeIs(err, plxerr.InvalidFormat) {
		t.Errorf("an invalid document = %v, want InvalidFormat", err)
	}
	if _, _, err := f.svc.Create(ctx, f.developer, comp, library.Draft{Content: content}); !codeIs(err, plxerr.InvalidFormat) {
		t.Errorf("a template as a component = %v, want InvalidFormat", err)
	}

	// Private: not even the owner of the organisation sees it.
	if _, _, err := f.svc.Versions(ctx, f.ownerA, tpl, item.ID); !codeIs(err, plxerr.ResourceNotFound) {
		t.Errorf("the owner read a private template: %v", err)
	}
	if got, err := f.svc.List(ctx, f.ownerA, tpl, library.ListFilter{}, 10); err != nil || len(got) != 0 {
		t.Errorf("owner List = %v, %v; want none", got, err)
	}
	if got, err := f.svc.List(ctx, f.developer, tpl, library.ListFilter{}, 10); err != nil || len(got) != 1 {
		t.Errorf("author List = %v, %v; want one", got, err)
	}

	// Revise: new immutable versions, guarded by the version revised from.
	revised, v2, err := f.svc.Revise(ctx, f.developer, tpl, item.ID, 1, library.Draft{Content: sameDoc(t, content, "two"), Name: "Card", Note: "second"})
	if err != nil || revised.HeadVersion != 2 || v2.Number != 2 || revised.Name != "Card" {
		t.Fatalf("Revise = %+v, %+v, %v", revised, v2, err)
	}
	if _, _, err := f.svc.Revise(ctx, f.developer, tpl, item.ID, 1, library.Draft{Content: sameDoc(t, content, "three")}); !codeIs(err, plxerr.RevisionConflict) {
		t.Errorf("a stale revision = %v, want RevisionConflict", err)
	}
	if _, _, err := f.svc.Revise(ctx, f.developer, tpl, item.ID, 2, library.Draft{Content: f.doc("template", "card", "other id")}); !codeIs(err, plxerr.InvalidFormat) {
		t.Errorf("a revision with another id = %v, want InvalidFormat", err)
	}
	_, versions, err := f.svc.Versions(ctx, f.developer, tpl, item.ID)
	if err != nil || len(versions) != 2 || versions[0].Number != 2 || versions[1].Number != 1 || !bytes.Contains(versions[1].Content, []byte(`"one"`)) {
		t.Fatalf("Versions = %+v, %v", versions, err)
	}

	// A stored version cannot be updated, even by the database role the
	// server runs as.
	err = f.db.InTx(ctx, storage.Tenant{OrganizationID: f.ownerA.OrganizationID, UserID: f.developer.UserID}, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE library_versions SET content = '\x7b7d' WHERE version = 1`)
		return err
	})
	if err == nil {
		t.Error("a library version was updated")
	}
}

// Verifies: STU-081, SEC-102.
// Who may change what: the author their own, library.manage others', and
// library.publish for anything public.
func TestAuthoring_Permissions_STU_081(t *testing.T) {
	t.Parallel()
	f := newFixture(t, nil)
	ctx := context.Background()
	mine, _, err := f.svc.Create(ctx, f.developer, comp, library.Draft{Content: f.doc("component", "button", "a", "label")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.SetVisibility(ctx, f.developer, comp, mine.ID, library.Organization); err != nil {
		t.Fatalf("the author shared their component: %v", err)
	}
	// The owner holds library.manage and revises another person's component.
	if _, _, err := f.svc.Revise(ctx, f.ownerA, comp, mine.ID, 1, library.Draft{Content: sameDoc(t, mustContent(t, f, mine.ID), "b")}); err != nil {
		t.Errorf("the owner revised a shared component: %v", err)
	}
	theirs, _, err := f.svc.Create(ctx, f.ownerA, comp, library.Draft{Content: f.doc("component", "badge", "a", "text")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.SetVisibility(ctx, f.ownerA, comp, theirs.ID, library.Organization); err != nil {
		t.Fatal(err)
	}
	// Visible, not the developer's, and the developer lacks library.manage.
	if _, _, err := f.svc.Revise(ctx, f.developer, comp, theirs.ID, 1, library.Draft{Content: sameDoc(t, mustContent(t, f, theirs.ID), "x")}); !codeIs(err, plxerr.PermissionDenied) {
		t.Errorf("a developer revised another person's component: %v", err)
	}
	if _, err := f.svc.SetVisibility(ctx, f.developer, comp, theirs.ID, library.Private); !codeIs(err, plxerr.PermissionDenied) {
		t.Errorf("a developer unshared another person's component: %v", err)
	}
	// Public needs library.publish, which the developer lacks even for their own.
	if _, err := f.svc.SetVisibility(ctx, f.developer, comp, mine.ID, library.Public); !codeIs(err, plxerr.PermissionDenied) {
		t.Errorf("a developer made a component public: %v", err)
	}
	if _, err := f.svc.SetVisibility(ctx, f.ownerA, comp, mine.ID, "everyone"); !codeIs(err, plxerr.InvalidEnumValue) {
		t.Errorf("an unknown visibility = %v, want InvalidEnumValue", err)
	}
}

func mustContent(t *testing.T, f *fixture, id string) []byte {
	t.Helper()
	_, vs, err := f.svc.Versions(context.Background(), f.ownerA, comp, id)
	if err != nil {
		t.Fatal(err)
	}
	return vs[0].Content
}

// Verifies: STU-081, SEC-092.
// Content with a secret or a private URL cannot be made public, nor can a
// public item be revised into it; the refusal never repeats the secret. A
// clean item becomes public and is then seen, never changed, from another
// organisation.
func TestPublicScan_STU_081(t *testing.T) {
	t.Parallel()
	f := newFixture(t, nil)
	ctx := context.Background()
	const secret = "AK" + "IAABCDEFGHIJKLMNOP"
	dirty, _, err := f.svc.Create(ctx, f.ownerA, tpl, library.Draft{Content: f.doc("template", "dirty", "key "+secret)})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.svc.SetVisibility(ctx, f.ownerA, tpl, dirty.ID, library.Public)
	if !codeIs(err, plxerr.LibraryPublicScanFailed) || strings.Contains(err.Error(), secret) || !strings.Contains(err.Error(), "(secret)") {
		t.Fatalf("a secret went public or was echoed: %v", err)
	}
	private, _, err := f.svc.Create(ctx, f.ownerA, tpl, library.Draft{Content: f.doc("template", "intranet", "see https://wiki.internal/start")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.svc.SetVisibility(ctx, f.ownerA, tpl, private.ID, library.Public); !codeIs(err, plxerr.LibraryPublicScanFailed) || !strings.Contains(err.Error(), "(url)") {
		t.Errorf("a private URL went public: %v", err)
	}

	clean, _, err := f.svc.Create(ctx, f.ownerA, tpl, library.Draft{Content: f.doc("template", "welcome", "Hello", "")})
	if err != nil {
		t.Fatal(err)
	}
	if it, err := f.svc.SetVisibility(ctx, f.ownerA, tpl, clean.ID, library.Public); err != nil || it.Visibility != library.Public {
		t.Fatalf("a clean template did not go public: %+v, %v", it, err)
	}
	if _, _, err := f.svc.Revise(ctx, f.ownerA, tpl, clean.ID, 1, library.Draft{Content: sameDoc(t, f.contentOf(t, f.ownerA, tpl, clean.ID), "token "+secret)}); !codeIs(err, plxerr.LibraryPublicScanFailed) {
		t.Errorf("a public template was revised into a secret: %v", err)
	}

	// Another organisation sees the public template and cannot touch it.
	got, err := f.svc.List(ctx, f.ownerB, tpl, library.ListFilter{}, 10)
	if err != nil || len(got) != 1 || got[0].ID != clean.ID {
		t.Fatalf("organisation B List = %+v, %v; want the public template only", got, err)
	}
	if _, _, err := f.svc.Revise(ctx, f.ownerB, tpl, clean.ID, 1, library.Draft{Content: f.contentOf(t, f.ownerB, tpl, clean.ID)}); !codeIs(err, plxerr.ResourceNotFound) {
		t.Errorf("another organisation revised a public template: %v", err)
	}
	if _, err := f.svc.SetVisibility(ctx, f.ownerB, tpl, clean.ID, library.Private); !codeIs(err, plxerr.ResourceNotFound) {
		t.Errorf("another organisation hid a public template: %v", err)
	}
	// Taking it back from public is possible for its own organisation.
	if it, err := f.svc.SetVisibility(ctx, f.ownerA, tpl, clean.ID, library.Organization); err != nil || it.Visibility != library.Organization {
		t.Errorf("unpublish = %+v, %v", it, err)
	}
	if got, _ := f.svc.List(ctx, f.ownerB, tpl, library.ListFilter{}, 10); len(got) != 0 {
		t.Errorf("organisation B still sees %d templates", len(got))
	}
}

func (f *fixture) contentOf(t *testing.T, p auth.Principal, kind library.Kind, id string) []byte {
	t.Helper()
	_, vs, err := f.svc.Versions(context.Background(), p, kind, id)
	if err != nil {
		t.Fatal(err)
	}
	return vs[0].Content
}

// Verifies: STU-080, SRV-022.
// Nothing crosses organisations: not reading, revising, exporting or adopting
// an item that is not public, nor adopting into another organisation's app.
func TestTenancy_STU_080(t *testing.T) {
	t.Parallel()
	f := newFixture(t, nil)
	ctx := context.Background()
	item, _, err := f.svc.Create(ctx, f.ownerA, comp, library.Draft{Content: f.doc("component", "card", "x", "title")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.SetVisibility(ctx, f.ownerA, comp, item.ID, library.Organization); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.svc.Versions(ctx, f.ownerB, comp, item.ID); !codeIs(err, plxerr.ResourceNotFound) {
		t.Errorf("Versions across organisations = %v", err)
	}
	if _, _, err := f.svc.Revise(ctx, f.ownerB, comp, item.ID, 1, library.Draft{Content: f.contentOf(t, f.ownerA, comp, item.ID)}); !codeIs(err, plxerr.ResourceNotFound) {
		t.Errorf("Revise across organisations = %v", err)
	}
	if _, _, err := f.svc.Export(ctx, f.ownerB, comp, item.ID, 0); !codeIs(err, plxerr.ResourceNotFound) {
		t.Errorf("Export across organisations = %v", err)
	}
	if _, err := f.svc.SetVisibility(ctx, f.ownerB, comp, item.ID, library.Organization); !codeIs(err, plxerr.ResourceNotFound) {
		t.Errorf("SetVisibility across organisations = %v", err)
	}
	if _, _, err := f.svc.Adopt(ctx, f.ownerB, f.appB, item.ID, 1); !codeIs(err, plxerr.ResourceNotFound) {
		t.Errorf("Adopt across organisations = %v", err)
	}
	if _, _, err := f.svc.Adopt(ctx, f.ownerA, f.appB, item.ID, 1); !codeIs(err, plxerr.ResourceNotFound) {
		t.Errorf("Adopt into another organisation's app = %v", err)
	}
	// The kind is part of the identity: a component is not a template.
	if _, _, err := f.svc.Versions(ctx, f.ownerA, tpl, item.ID); !codeIs(err, plxerr.ResourceNotFound) {
		t.Errorf("a component read as a template = %v", err)
	}
	if got, err := f.svc.Adoptions(ctx, f.ownerA, f.appB); err != nil || len(got) != 0 {
		t.Errorf("Adoptions of another organisation's app = %v, %v", got, err)
	}
}

// Verifies: STU-080.
// A library file exports to the same bytes wherever it is exported from: a
// copy imported into another organisation exports byte for byte the same; a
// clash needs another key; and a file that is not one is refused.
func TestExportImport_RoundTrip_STU_080(t *testing.T) {
	t.Parallel()
	f := newFixture(t, nil)
	ctx := context.Background()
	for _, kind := range []library.Kind{tpl, comp} {
		item, _, err := f.svc.Create(ctx, f.ownerA, kind, library.Draft{Content: f.doc(string(kind), "round-"+string(kind), "Hello", "title"), Description: "A round trip"})
		if err != nil {
			t.Fatal(err)
		}
		file, name, err := f.svc.Export(ctx, f.ownerA, kind, item.ID, 0)
		if err != nil || name != "round-"+string(kind)+".plux-"+string(kind)+".json" {
			t.Fatalf("Export = %q, %v", name, err)
		}
		again, _, err := f.svc.Export(ctx, f.ownerA, kind, item.ID, 1)
		if err != nil || !bytes.Equal(file, again) {
			t.Errorf("the same version exported twice differs: %v", err)
		}
		imported, v, err := f.svc.Import(ctx, f.ownerB, kind, file, "")
		if err != nil || imported.Visibility != library.Private || imported.Key != item.Key || imported.DocumentID != item.DocumentID || v.Number != 1 {
			t.Fatalf("Import = %+v, %+v, %v", imported, v, err)
		}
		back, _, err := f.svc.Export(ctx, f.ownerB, kind, imported.ID, 0)
		if err != nil || !bytes.Equal(file, back) {
			t.Fatalf("%s: the round trip changed the file (%v):\n%s\n%s", kind, err, file, back)
		}
		if _, _, err := f.svc.Import(ctx, f.ownerA, kind, file, ""); !codeIs(err, plxerr.ResourceExists) {
			t.Errorf("importing over the original = %v, want ResourceExists", err)
		}
		copied, _, err := f.svc.Import(ctx, f.ownerA, kind, file, "round-copy-"+string(kind))
		if err != nil || copied.DocumentID == item.DocumentID || copied.Key != "round-copy-"+string(kind) {
			t.Errorf("Import with another key = %+v, %v; want a new key and document identifier", copied, err)
		}
		if _, _, err := f.svc.Import(ctx, f.viewer, kind, file, "viewer-"+string(kind)); !codeIs(err, plxerr.PermissionDenied) {
			t.Errorf("a viewer imported a file: %v", err)
		}
		other := comp
		if kind == comp {
			other = tpl
		}
		if _, _, err := f.svc.Import(ctx, f.ownerA, other, file, "wrong"); !codeIs(err, plxerr.InvalidFormat) {
			t.Errorf("a %s file imported as a %s = %v", kind, other, err)
		}
		tampered := bytes.Replace(file, []byte("Hello"), []byte("Hellp"), 1)
		if _, _, err := f.svc.Import(ctx, f.ownerA, kind, tampered, "tampered"); !codeIs(err, plxerr.InvalidFormat) {
			t.Errorf("a file whose content does not match its hash = %v", err)
		}
	}
	for _, junk := range []string{``, `[]`, `{"format":"other"}`, `{"format":"plux-library","formatVersion":9}`, "not json"} {
		if _, _, err := f.svc.Import(ctx, f.ownerA, tpl, []byte(junk), "junk"); !codeIs(err, plxerr.InvalidFormat) {
			t.Errorf("Import(%q) = %v, want InvalidFormat", junk, err)
		}
	}
}

// Verifies: STU-085.
// An app adopts a version explicitly; adopting reports the instances whose
// overrides the version has no prop for, and changes no document. Adopting
// again replaces the version.
func TestAdopt_STU_085(t *testing.T) {
	t.Parallel()
	f := newFixture(t, nil)
	ctx := context.Background()
	v1doc := f.doc("component", "card", "x", "title", "tone")
	item, _, err := f.svc.Create(ctx, f.ownerA, comp, library.Draft{Content: v1doc})
	if err != nil {
		t.Fatal(err)
	}
	v2doc := bytes.Replace(sameDoc(t, v1doc, "y"), []byte(`,{"name":"tone","type":"string"}`), nil, 1)
	if _, _, err := f.svc.Revise(ctx, f.ownerA, comp, item.ID, 1, library.Draft{Content: v2doc}); err != nil {
		t.Fatal(err)
	}
	f.found[item.DocumentID] = []library.Instance{
		{File: "plugins/loans/pages/home.page.json", Path: "/root/children/0", Props: []string{"title", "tone"}},
		{File: "plugins/loans/pages/home.page.json", Path: "/root/children/1", Props: []string{"title"}},
	}
	if _, _, err := f.svc.Adopt(ctx, f.viewer, f.appA, item.ID, 1); !codeIs(err, plxerr.PermissionDenied) {
		t.Errorf("a viewer adopted a version: %v", err)
	}
	if _, _, err := f.svc.Adopt(ctx, f.ownerA, f.appA, item.ID, 3); !codeIs(err, plxerr.ResourceNotFound) {
		t.Errorf("adopting a version that does not exist = %v", err)
	}
	a, conflicts, err := f.svc.Adopt(ctx, f.ownerA, f.appA, item.ID, 1)
	if err != nil || a.Version != 1 || len(conflicts) != 0 {
		t.Fatalf("Adopt v1 = %+v, %v, %v; want no conflicts", a, conflicts, err)
	}
	a, conflicts, err = f.svc.Adopt(ctx, f.ownerA, f.appA, item.ID, 2)
	if err != nil || a.Version != 2 || len(conflicts) != 1 || conflicts[0].Path != "/root/children/0" || len(conflicts[0].RemovedProps) != 1 || conflicts[0].RemovedProps[0] != "tone" {
		t.Fatalf("Adopt v2 = %+v, %+v, %v", a, conflicts, err)
	}
	list, err := f.svc.Adoptions(ctx, f.ownerA, f.appA)
	if err != nil || len(list) != 1 || list[0].Version != 2 || list[0].ItemID != item.ID {
		t.Errorf("Adoptions = %+v, %v", list, err)
	}
}

// Verifies: STU-080, LIM-001.
// The registry's limits cap the library.
func TestLimits_STU_080(t *testing.T) {
	t.Parallel()
	f := newFixture(t, func(s limits.Set) limits.Set {
		s, _ = s.Tighten(limits.LibraryVersionsPerItem, limits.ScopeInstallation, 2)
		s, _ = s.Tighten(limits.LibraryItemsPerOrganization, limits.ScopeInstallation, 2)
		return s
	})
	ctx := context.Background()
	content := f.doc("template", "one", "a")
	item, _, err := f.svc.Create(ctx, f.ownerA, tpl, library.Draft{Content: content})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.svc.Revise(ctx, f.ownerA, tpl, item.ID, 1, library.Draft{Content: sameDoc(t, content, "b")}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.svc.Revise(ctx, f.ownerA, tpl, item.ID, 2, library.Draft{Content: sameDoc(t, content, "c")}); !codeIs(err, plxerr.LimitExceeded) {
		t.Errorf("a third version = %v, want LimitExceeded", err)
	}
	if _, _, err := f.svc.Create(ctx, f.ownerA, tpl, library.Draft{Content: f.doc("template", "two", "a")}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.svc.Create(ctx, f.ownerA, tpl, library.Draft{Content: f.doc("template", "three", "a")}); !codeIs(err, plxerr.LimitExceeded) {
		t.Errorf("a third item = %v, want LimitExceeded", err)
	}
}

// Verifies: STU-080, SEC-140.
// Creating, revising, sharing and adopting are audited in the organisation's
// chain, which stays intact.
func TestAudit_STU_080(t *testing.T) {
	t.Parallel()
	f := newFixture(t, nil)
	ctx := context.Background()
	content := f.doc("component", "audited", "a", "title")
	item, _, err := f.svc.Create(ctx, f.ownerA, comp, library.Draft{Content: content})
	if err != nil {
		t.Fatal(err)
	}
	steps := []func() error{
		func() error {
			_, _, err := f.svc.Revise(ctx, f.ownerA, comp, item.ID, 1, library.Draft{Content: sameDoc(t, content, "b")})
			return err
		},
		func() error {
			_, err := f.svc.SetVisibility(ctx, f.ownerA, comp, item.ID, library.Organization)
			return err
		},
		func() error { _, _, err := f.svc.Adopt(ctx, f.ownerA, f.appA, item.ID, 2); return err },
	}
	for _, step := range steps {
		if err := step(); err != nil {
			t.Fatal(err)
		}
	}
	var actions []string
	err = f.db.InTx(ctx, storage.Tenant{OrganizationID: f.ownerA.OrganizationID, UserID: f.ownerA.UserID}, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT action FROM audit_log WHERE action LIKE 'library.%' ORDER BY sequence`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var a string
			if err := rows.Scan(&a); err != nil {
				return err
			}
			actions = append(actions, a)
		}
		return rows.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"library.item_created", "library.version_added", "library.visibility_set", "library.version_adopted"}
	if strings.Join(actions, ",") != strings.Join(want, ",") {
		t.Errorf("audit actions = %v, want %v", actions, want)
	}
}
