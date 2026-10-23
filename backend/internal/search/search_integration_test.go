// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package search_test

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/cache"
	"github.com/nightCode42/plux3/backend/internal/document"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/schema/uuid7"
	"github.com/nightCode42/plux3/backend/internal/search"
	"github.com/nightCode42/plux3/backend/internal/signing"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
	"github.com/nightCode42/plux3/backend/internal/storage/storagetest"
	"github.com/nightCode42/plux3/backend/internal/tenancy"
)

type ids struct{ g *uuid7.Generator }

func (i ids) New() (string, error) {
	u, err := i.g.New()
	return u.String(), err
}

// fixture is the search service against a fresh schema: an organisation
// with two apps and an owner, and a second organisation with one app.
type fixture struct {
	db       *storage.DB
	tenancy  *tenancy.Service
	docs     *document.Service
	search   *search.Service
	gen      ids
	org      string
	appA     string
	appB     string
	owner    auth.Principal
	otherOrg string
	otherApp string
	other    auth.Principal
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	db := storagetest.Open(t)
	backend, err := signing.NewFile(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	gen := ids{g: uuid7.NewGenerator(time.Now, rand.Reader)}
	log := audit.NewLog(gen, nil)
	authService, err := auth.NewService(auth.Options{
		DB: db, Audit: log, Cache: cache.NewMemory(nil), Crypter: backend, IDs: gen, VerificationURI: "https://p.example/device",
	})
	if err != nil {
		t.Fatal(err)
	}
	ten, err := tenancy.NewService(tenancy.Options{
		DB: db, Audit: log, Auth: authService, Crypter: backend, IDs: gen, SigningKeyPrefix: "targets",
	})
	if err != nil {
		t.Fatal(err)
	}
	docs, err := document.NewService(document.Options{DB: db, Audit: log, Tenancy: ten, IDs: gen, SnapshotDays: 90})
	if err != nil {
		t.Fatal(err)
	}
	svc, err := search.NewService(search.Options{DB: db, Tenancy: ten})
	if err != nil {
		t.Fatal(err)
	}
	admin, invitation, err := authService.Bootstrap(ctx, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := authService.AcceptInvitation(ctx, invitation, "Admin", "correct horse battery"); err != nil {
		t.Fatal(err)
	}
	id := auth.Identity{Kind: auth.KindUser, ID: admin.ID, UserID: admin.ID, Display: "Admin", SecondFactor: true, InstallationAdmin: true}.WithStepUp(0)
	f := &fixture{db: db, tenancy: ten, docs: docs, search: svc, gen: gen}
	f.org, f.owner, f.appA = f.organisation(t, authService, id, "acme", "Acme", "demo", "Demo")
	_, err = ten.CreateApp(ctx, f.owner, "vault", "Vault")
	if err != nil {
		t.Fatal(err)
	}
	apps, err := ten.ListApps(ctx, f.owner, tenancy.Page{Size: 10})
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range apps {
		if a.Key == "vault" {
			f.appB = a.ID
		}
	}
	f.otherOrg, f.other, f.otherApp = f.organisation(t, authService, id, "globex", "Globex", "loans-demo", "Loans demo")
	return f
}

// organisation creates an organisation with an app and returns the owner's
// principal in it.
func (f *fixture) organisation(t *testing.T, a *auth.Service, id auth.Identity, key, name, appKey, appName string) (string, auth.Principal, string) {
	t.Helper()
	ctx := context.Background()
	o, err := f.tenancy.CreateOrganization(ctx, id, key, name)
	if err != nil {
		t.Fatal(err)
	}
	p, err := a.Resolve(ctx, id, o.ID)
	if err != nil {
		t.Fatal(err)
	}
	app, err := f.tenancy.CreateApp(ctx, p, appKey, appName)
	if err != nil {
		t.Fatal(err)
	}
	return o.ID, p, app.ID
}

// scoped is a principal that holds the permissions on one app only.
func (f *fixture) scoped(app string, perms ...auth.Permission) auth.Principal {
	return auth.Principal{
		Identity:       auth.Identity{Kind: auth.KindUser, ID: f.owner.UserID, UserID: f.owner.UserID, Display: "Scoped"},
		OrganizationID: f.org,
		AppPermissions: map[string][]auth.Permission{app: perms},
	}
}

func (f *fixture) newID(t *testing.T) string {
	t.Helper()
	id, err := f.gen.New()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// put writes an app-level document as the owner.
func (f *fixture) put(t *testing.T, p auth.Principal, app, path string, content string) {
	t.Helper()
	ctx := context.Background()
	if _, _, err := f.docs.AcquireLock(ctx, p, app, "", "s", false); err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}
	if _, err := f.docs.PutDocument(ctx, p, app, "", "s", path, []byte(content), -1); err != nil {
		t.Fatalf("PutDocument %s: %v", path, err)
	}
}

func (f *fixture) page(t *testing.T, key, title string) string {
	return fmt.Sprintf(`{"schemaVersion":"1.0.0","kind":"page","id":%q,"key":%q,"pageKind":"screen","title":%q,"root":{"id":%q,"type":"Text","props":{"data":"x"}}}`,
		f.newID(t), key, title, f.newID(t))
}

func (f *fixture) names(t *testing.T, p auth.Principal, query string, kinds ...string) []search.Hit {
	t.Helper()
	res, err := f.search.Search(context.Background(), p, search.Query{Text: query, Kinds: kinds})
	if err != nil {
		t.Fatalf("Search(%q): %v", query, err)
	}
	return res.Hits
}

func code(err error) plxerr.Code {
	var e *plxerr.Error
	if errors.As(err, &e) {
		return e.Code
	}
	return 0
}

func kindsOf(hits []search.Hit) []string {
	out := make([]string, len(hits))
	for i, h := range hits {
		out[i] = h.Kind
	}
	return out
}

// Verifies: SRV-061.
// Every kind of resource is indexed: an app, a plugin, a page, a
// component, a template, a translation key catalogue and a function are each
// found by a word of their name, with their kind and path.
func TestSearch_EveryKind_SRV_061(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	plugin, err := f.docs.CreatePlugin(ctx, f.owner, f.appA, "payments", "Payments hub")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.docs.AcquireLock(ctx, f.owner, f.appA, plugin.ID, "p", false); err != nil {
		t.Fatal(err)
	}
	if _, err := f.docs.PutDocument(ctx, f.owner, f.appA, plugin.ID, "p", "plugins/payments/pages/checkout.page.json", []byte(f.page(t, "checkout", "Checkout summary")), -1); err != nil {
		t.Fatal(err)
	}
	component := fmt.Sprintf(`{"schemaVersion":"1.0.0","kind":"component","id":%q,"key":"price-badge","name":"Price badge","version":1,"exported":true,"props":[],"root":{"id":%q,"type":"Text","props":{"data":"x"}}}`, f.newID(t), f.newID(t))
	if _, err := f.docs.PutDocument(ctx, f.owner, f.appA, plugin.ID, "p", "plugins/payments/components/price-badge.component.json", []byte(component), -1); err != nil {
		t.Fatal(err)
	}
	template := fmt.Sprintf(`{"schemaVersion":"1.0.0","kind":"template","id":%q,"key":"invoice-row","name":"Invoice row","category":"display","description":"A row of an invoice.","parameters":[],"visibility":"organization","root":{"id":%q,"type":"Text","props":{"data":"x"}}}`, f.newID(t), f.newID(t))
	f.put(t, f.owner, f.appA, "templates/invoice-row.template.json", template)
	keys := fmt.Sprintf(`{"schemaVersion":"1.0.0","kind":"translationKeys","id":%q,"keys":[{"id":%q,"key":"payments.refund.title","description":"Heading of the refund screen."}]}`, f.newID(t), f.newID(t))
	f.put(t, f.owner, f.appA, "translations/keys.json", keys)
	fn := fmt.Sprintf(`{"schemaVersion":"1.0.0","kind":"function","id":%q,"name":"billing.computeTax","description":"Computes the tax of an invoice.","placement":"server","capabilities":{}}`, f.newID(t))
	f.put(t, f.owner, f.appA, "functions/billing.computeTax/function.json", fn)

	for _, c := range []struct {
		query, kind, path string
	}{
		{"demo", search.KindApp, "demo"},
		{"hub", search.KindPlugin, "plugins/payments"},
		{"check", search.KindPage, "plugins/payments/pages/checkout.page.json"},
		{"badge", search.KindComponent, "plugins/payments/components/price-badge.component.json"},
		{"invoice", search.KindTemplate, "templates/invoice-row.template.json"},
		{"refund", search.KindTranslationKey, "translations/keys.json"},
		{"computetax", search.KindFunction, "functions/billing.computeTax/function.json"},
	} {
		hits := f.names(t, f.owner, c.query)
		var found *search.Hit
		for i := range hits {
			if hits[i].Kind == c.kind {
				found = &hits[i]
			}
		}
		if found == nil {
			t.Errorf("%q: no %s among %v", c.query, c.kind, kindsOf(hits))
			continue
		}
		if found.Path != c.path || found.ID == "" || found.AppID != f.appA || !strings.Contains(found.Snippet, "«") {
			t.Errorf("%q: hit = %+v, want path %q, an ID, app %s and a marked snippet", c.query, *found, c.path, f.appA)
		}
	}
	if hits := f.names(t, f.owner, "refund", search.KindPage); len(hits) != 0 {
		t.Errorf("a kind filter let %v through", kindsOf(hits))
	}
}

// Verifies: SRV-061.
// A query word matches as a prefix, every word must match, and a better
// match ranks first.
func TestSearch_PrefixAndRanking_SRV_061(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.put(t, f.owner, f.appA, "templates/loan-card.template.json", template(f, t, "loan-card", "Loan loan card", "Shows a loan."))
	f.put(t, f.owner, f.appA, "templates/account-card.template.json", template(f, t, "account-card", "Account card", "Mentions a loan once."))

	hits := f.names(t, f.owner, "lo", search.KindTemplate)
	if len(hits) != 2 {
		t.Fatalf("the prefix lo: %d hits, want 2", len(hits))
	}
	if hits[0].Name != "loan-card" || hits[0].Rank <= hits[1].Rank {
		t.Errorf("ranking = %s (%v) before %s (%v), want loan-card first", hits[0].Name, hits[0].Rank, hits[1].Name, hits[1].Rank)
	}
	if got := f.names(t, f.owner, "loan account", search.KindTemplate); len(got) != 1 || got[0].Name != "account-card" {
		t.Errorf("both words must match: %+v", got)
	}
	if got := f.names(t, f.owner, "LOAN-CA", search.KindTemplate); len(got) != 2 {
		t.Errorf("case and hyphens: %d hits, want 2", len(got))
	}
}

func template(f *fixture, t *testing.T, key, name, description string) string {
	return fmt.Sprintf(`{"schemaVersion":"1.0.0","kind":"template","id":%q,"key":%q,"name":%q,"category":"display","description":%q,"parameters":[],"visibility":"organization","root":{"id":%q,"type":"Text","props":{"data":"x"}}}`,
		f.newID(t), key, name, description, f.newID(t))
}

// Verifies: SRV-061.
// The index follows the data: a changed document is found by its new words
// and no longer by the old, a deleted one is gone, and a renamed plugin is
// found by its new name.
func TestSearch_FollowsChanges_SRV_061(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	path := "templates/greeting.template.json"
	f.put(t, f.owner, f.appA, path, template(f, t, "greeting", "Welcome banner", "Says hello."))
	if len(f.names(t, f.owner, "welcome")) != 1 {
		t.Fatal("the new template is not found")
	}
	if _, err := f.docs.PutDocument(ctx, f.owner, f.appA, "", "s", path, []byte(template(f, t, "greeting", "Farewell banner", "Says goodbye.")), -1); err != nil {
		t.Fatal(err)
	}
	if got := f.names(t, f.owner, "welcome"); len(got) != 0 {
		t.Errorf("the old name still matches: %+v", got)
	}
	if got := f.names(t, f.owner, "farewell"); len(got) != 1 || got[0].Name != "greeting" {
		t.Errorf("the new name: %+v", got)
	}
	doc, err := f.docs.GetDocument(ctx, f.owner, f.appA, "", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.docs.DeleteDocument(ctx, f.owner, f.appA, "", "s", path, doc.Revision); err != nil {
		t.Fatal(err)
	}
	if got := f.names(t, f.owner, "farewell"); len(got) != 0 {
		t.Errorf("a deleted document still matches: %+v", got)
	}
	plugin, err := f.docs.CreatePlugin(ctx, f.owner, f.appA, "ledger", "Ledger")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.docs.UpdatePlugin(ctx, f.owner, plugin.ID, "Bookkeeping"); err != nil {
		t.Fatal(err)
	}
	if got := f.names(t, f.owner, "bookkeeping", search.KindPlugin); len(got) != 1 {
		t.Errorf("a renamed plugin: %+v", got)
	}
	if _, err := f.docs.DeletePlugin(ctx, f.owner, plugin.ID); err != nil {
		t.Fatal(err)
	}
	if got := f.names(t, f.owner, "bookkeeping", search.KindPlugin); len(got) != 0 {
		t.Errorf("a deleted plugin: %+v", got)
	}
}

// Verifies: SRV-061, GOV-001, SRV-022.
// A person scoped to one app never has the rows of another app fetched: the
// statement itself returns only the allowed app's rows, and a person with
// the permission on no app is refused.
func TestSearch_ScopedToApps_SRV_061(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	f.put(t, f.owner, f.appA, "templates/ledger-a.template.json", template(f, t, "ledger-a", "Ledger of A", "a"))
	f.put(t, f.owner, f.appB, "templates/ledger-b.template.json", template(f, t, "ledger-b", "Ledger of B", "b"))

	all := f.names(t, f.owner, "ledger", search.KindTemplate)
	if len(all) != 2 {
		t.Fatalf("an organisation-wide caller sees %d, want 2", len(all))
	}
	onlyA := f.names(t, f.scoped(f.appA, auth.SearchRead), "ledger", search.KindTemplate)
	if len(onlyA) != 1 || onlyA[0].AppID != f.appA {
		t.Fatalf("a caller scoped to app A sees %+v", onlyA)
	}
	if got := f.names(t, f.scoped(f.appB, auth.SearchRead), "demo"); len(got) != 0 {
		t.Errorf("a caller scoped to app B found app A: %+v", got)
	}
	if _, err := f.search.Search(ctx, f.scoped(f.appA, auth.AppRead), search.Query{Text: "ledger"}); code(err) != plxerr.PermissionDenied {
		t.Errorf("without search.read: %v", err)
	}

	// The restriction is in the statement: run it as the service does, with
	// the allowed list, and with none, and count the rows the database sends.
	count := func(allowed ...string) int {
		var n int
		err := f.db.InTx(ctx, storage.Tenant{OrganizationID: f.org}, func(ctx context.Context, tx pgx.Tx) error {
			ids := []pgtype.UUID{}
			for _, a := range allowed {
				ids = append(ids, storage.MustUUID(a))
			}
			rows, err := dbgen.New(tx).Search(ctx, dbgen.SearchParams{Query: "ledger:*", Allowed: ids, Excluded: []pgtype.UUID{}, Kinds: []string{}, PageSize: 50})
			n = len(rows)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	if got := count(f.appA); got != 1 {
		t.Errorf("the statement allowed to app A returned %d rows, want 1", got)
	}
	if got := count(f.appA, f.appB); got != 2 {
		t.Errorf("the statement allowed to both apps returned %d rows, want 2", got)
	}
	if got := count(); got != 0 {
		t.Errorf("the statement with an empty allowed list returned %d rows, want 0", got)
	}
}

// Verifies: SRV-061, GOV-006.
// A caller who may search the whole organisation does not find what is on an
// app where a restriction takes search.read away, nor does a restriction on
// another permission, or on one environment, hide it.
func TestSearch_LeavesOutRestrictedApps_GOV_006(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.put(t, f.owner, f.appA, "templates/ledger-a.template.json", template(f, t, "ledger-a", "Ledger of A", "a"))
	f.put(t, f.owner, f.appB, "templates/ledger-b.template.json", template(f, t, "ledger-b", "Ledger of B", "b"))
	applied := func(appID, environment string, removed ...auth.Permission) auth.Principal {
		p := f.owner
		p.Restrictions = []auth.AppliedRestriction{{
			Restriction: auth.Restriction{AppID: appID, Environment: environment}, Removed: removed,
		}}
		return p
	}

	got := f.names(t, applied(f.appA, "", auth.SearchRead), "ledger")
	if len(got) != 1 || got[0].AppID != f.appB {
		t.Errorf("with search.read removed on app A the caller finds %+v, want app B's template only", got)
	}
	if hits := f.names(t, applied(f.appA, "", auth.SearchRead), "demo"); len(hits) != 0 {
		t.Errorf("the restricted app itself is found: %+v", hits)
	}
	for name, p := range map[string]auth.Principal{
		"another permission":  applied(f.appA, "", auth.PluginEdit),
		"one environment":     applied(f.appA, "production", auth.SearchRead),
		"no restriction held": f.owner,
	} {
		if hits := f.names(t, p, "ledger"); len(hits) != 2 {
			t.Errorf("with %s the caller finds %d templates, want 2", name, len(hits))
		}
	}
}

// Verifies: SRV-061, SRV-022.
// Another organisation's rows are never returned, whatever the caller's
// permissions, and the same words find each organisation's own.
func TestSearch_OrganisationIsolation_SRV_061(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.put(t, f.owner, f.appA, "templates/loans.template.json", template(f, t, "loans", "Loans overview", "x"))
	f.put(t, f.other, f.otherApp, "templates/loans.template.json", template(f, t, "loans", "Loans overview", "x"))

	mine := f.names(t, f.owner, "loans")
	theirs := f.names(t, f.other, "loans")
	for _, h := range mine {
		if h.AppID == f.otherApp {
			t.Errorf("the first organisation was shown the second's %+v", h)
		}
	}
	for _, h := range theirs {
		if h.AppID == f.appA || h.AppID == f.appB {
			t.Errorf("the second organisation was shown the first's %+v", h)
		}
	}
	if len(mine) != 1 || len(theirs) != 2 {
		t.Errorf("hits = %d and %d, want 1 (template) and 2 (app and template)", len(mine), len(theirs))
	}
	// The same call with the first organisation's permissions, scoped to the
	// second organisation's app, finds nothing: the database refuses the rows.
	stray := f.scoped(f.otherApp, auth.SearchRead)
	if got := f.names(t, stray, "loans"); len(got) != 0 {
		t.Errorf("a grant naming another organisation's app returned %+v", got)
	}
}

// Verifies: SRV-061, LIM-001.
// The page size comes from search.resultsPerPage, in force for the
// organisation, and a larger request is cut to it; offsets continue.
func TestSearch_PageSize_SRV_061(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	for i := range 5 {
		if _, err := f.tenancy.CreateApp(ctx, f.owner, fmt.Sprintf("shop-%d", i), fmt.Sprintf("Shop %d", i)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.tenancy.SetOrganizationLimit(ctx, f.owner, string(limits.SearchResultsPerPage), 2); err != nil {
		t.Fatal(err)
	}
	res, err := f.search.Search(ctx, f.owner, search.Query{Text: "shop", PageSize: 100})
	if err != nil {
		t.Fatal(err)
	}
	if res.PageSize != 2 || len(res.Hits) != 2 {
		t.Fatalf("page = %d, hits = %d, want the limit 2 for both", res.PageSize, len(res.Hits))
	}
	seen := map[string]bool{}
	for offset := int32(0); offset < 6; offset += 2 {
		page, err := f.search.Search(ctx, f.owner, search.Query{Text: "shop", Offset: offset})
		if err != nil {
			t.Fatal(err)
		}
		for _, h := range page.Hits {
			seen[h.ID] = true
		}
	}
	if len(seen) != 5 {
		t.Errorf("paging reached %d of 5 apps", len(seen))
	}
	small, err := f.search.Search(ctx, f.owner, search.Query{Text: "shop", PageSize: 1})
	if err != nil || len(small.Hits) != 1 || small.PageSize != 1 {
		t.Errorf("a smaller request: %+v, %v", small, err)
	}
}

// Verifies: SRV-061.
// The words of a query never reach the database as tsquery syntax, and a
// malformed request is refused with a code.
func TestSearch_QueryText_SRV_061(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	for _, text := range []string{"demo'; DROP TABLE apps; --", "!demo | (", "demo & & :* <->"} {
		if _, err := f.search.Search(ctx, f.owner, search.Query{Text: text}); err != nil {
			t.Errorf("%q: %v", text, err)
		}
	}
	for _, text := range []string{"", "   ", "!?&|()", "\\"} {
		if _, err := f.search.Search(ctx, f.owner, search.Query{Text: text}); code(err) != plxerr.MissingProperty {
			t.Errorf("%q: %v, want PLX-%d", text, err, plxerr.MissingProperty)
		}
	}
	if _, err := f.search.Search(ctx, f.owner, search.Query{Text: "demo", Kinds: []string{"widget"}}); code(err) != plxerr.InvalidEnumValue {
		t.Errorf("an unknown kind: %v", err)
	}
	if _, err := f.search.Search(ctx, f.owner, search.Query{Text: "demo", Offset: -1}); code(err) != plxerr.OutOfRange {
		t.Errorf("a negative offset: %v", err)
	}
	if _, err := f.search.Search(ctx, auth.Principal{}, search.Query{Text: "demo"}); code(err) != plxerr.AuthenticationRequired {
		t.Errorf("no organisation: %v", err)
	}
}

var errPlanRead = errors.New("plan read; roll the transaction back")

// Verifies: SRV-061.
// The statement uses the GIN indexes: with sequential scans off, the plan of
// each branch reads them.
func TestSearch_UsesGINIndexes_SRV_061(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	for table, c := range map[string]struct{ index, vector string }{
		"apps":      {"apps_search", "plux_search_vector(name, key, '')"},
		"plugins":   {"plugins_search", "plux_search_vector(name, key, '')"},
		"documents": {"documents_search", "plux_search_vector(path, entity_key, search_text)"},
	} {
		var plan strings.Builder
		err := f.db.InTx(ctx, storage.Tenant{OrganizationID: f.org}, func(ctx context.Context, tx pgx.Tx) error {
			// The other indexes would win on empty tables; the change is
			// rolled back by returning errPlanRead.
			for _, stmt := range []string{"SET LOCAL enable_seqscan = off", "DROP INDEX IF EXISTS apps_key, plugins_key, documents_path"} {
				if _, err := tx.Exec(ctx, stmt); err != nil {
					return err
				}
			}
			where := "deleted_at IS NULL"
			if table == "documents" {
				where += " AND kind IN ('page', 'component', 'template', 'translationKeys', 'function')"
			}
			rows, err := tx.Query(ctx, fmt.Sprintf("EXPLAIN SELECT 1 FROM %s WHERE %s AND %s @@ to_tsquery('simple', 'abc:*')", table, where, c.vector))
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var line string
				if err := rows.Scan(&line); err != nil {
					return err
				}
				plan.WriteString(line + "\n")
			}
			if err := rows.Err(); err != nil {
				return err
			}
			return errPlanRead
		})
		if !errors.Is(err, errPlanRead) {
			t.Fatal(err)
		}
		if !strings.Contains(plan.String(), c.index) {
			t.Errorf("%s: the plan does not use %s:\n%s", table, c.index, plan.String())
		}
	}
}
