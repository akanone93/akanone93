// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package archive_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/archive"
	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/cache"
	"github.com/nightCode42/plux3/backend/internal/codegen"
	"github.com/nightCode42/plux3/backend/internal/document"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/schema/uuid7"
	"github.com/nightCode42/plux3/backend/internal/shellupdate"
	"github.com/nightCode42/plux3/backend/internal/signing"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/objects"
	"github.com/nightCode42/plux3/backend/internal/storage/storagetest"
	"github.com/nightCode42/plux3/backend/internal/tenancy"
)

type ids struct{ g *uuid7.Generator }

func (i ids) New() (string, error) {
	u, err := i.g.New()
	return u.String(), err
}

// queue records the generation jobs it is given.
type queue struct{ jobs []shellupdate.Job }

func (q *queue) Enqueue(_ context.Context, _ pgx.Tx, job shellupdate.Job) error {
	q.jobs = append(q.jobs, job)
	return nil
}

type fixture struct {
	db       *storage.DB
	tenancy  *tenancy.Service
	archives *archive.Service
	shells   *shellupdate.Service
	queue    *queue
	objects  *objects.Filesystem
	gen      ids
	log      *audit.Log
	org      string
	owner    auth.Principal
	viewer   auth.Principal
	other    auth.Principal // owner of a second organisation
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
	files, err := objects.NewFilesystem(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{db: db, log: audit.NewLog(gen, nil), queue: &queue{}, objects: files, gen: gen}
	authService, err := auth.NewService(auth.Options{DB: db, Audit: f.log, Cache: cache.NewMemory(nil), Crypter: backend, IDs: gen, VerificationURI: "https://p.example/device"})
	if err != nil {
		t.Fatal(err)
	}
	f.tenancy, err = tenancy.NewService(tenancy.Options{DB: db, Audit: f.log, Auth: authService, Crypter: backend, IDs: gen, SigningKeyPrefix: "targets"})
	if err != nil {
		t.Fatal(err)
	}
	docs, err := document.NewService(document.Options{DB: db, Audit: f.log, Tenancy: f.tenancy, IDs: gen, SnapshotDays: 90})
	if err != nil {
		t.Fatal(err)
	}
	for kind, k := range docs.TrashKinds() {
		f.tenancy.RegisterTrashKind(kind, k)
	}
	// The asset pipeline (scanner, codecs) is not part of this fixture, so the
	// sample has no assets; the embedded samples are checked by
	// TestSamplesAreCurrent_STU_015.
	var kept []document.File
	for _, file := range project(t, "loan-calculator") {
		if !strings.HasPrefix(file.Path, "assets/") {
			kept = append(kept, file)
		}
	}
	data, err := archive.Pack(archive.AppInfo{Key: "loans", Name: "Loans", Description: "A loan calculator.", Tags: []string{"sample"}}, kept)
	if err != nil {
		t.Fatal(err)
	}
	sample := archive.Sample{ID: "loans", Key: "loans", Name: "Loans", Data: data}
	f.archives, err = archive.New(archive.Options{DB: db, Audit: f.log, Tenancy: f.tenancy, Documents: docs, IDs: gen, Samples: []archive.Sample{sample}})
	if err != nil {
		t.Fatal(err)
	}
	keys := func(context.Context, string, string) ([]codegen.RootKey, error) {
		return []codegen.RootKey{{KeyID: "k1", Algorithm: "ed25519", Role: "targets", PublicKey: bytes.Repeat([]byte{7}, 32)}}, nil
	}
	f.shells, err = shellupdate.New(shellupdate.Options{DB: db, Audit: f.log, IDs: gen, Queue: f.queue, Keys: keys, Objects: files, Endpoint: "https://plux.example"})
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
	person := func(id string) auth.Identity {
		return auth.Identity{Kind: auth.KindUser, ID: id, UserID: id, Display: "user", SecondFactor: true, InstallationAdmin: true}.WithStepUp(0)
	}
	resolve := func(id, org string) auth.Principal {
		p, err := authService.Resolve(ctx, person(id), org)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	o, err := f.tenancy.CreateOrganization(ctx, person(admin.ID), "acme", "Acme")
	if err != nil {
		t.Fatal(err)
	}
	o2, err := f.tenancy.CreateOrganization(ctx, person(admin.ID), "other", "Other")
	if err != nil {
		t.Fatal(err)
	}
	f.org, f.owner, f.other = o.ID, resolve(admin.ID, o.ID), resolve(admin.ID, o2.ID)
	m, _, err := f.tenancy.AddMember(ctx, f.owner, "", "viewer@example.com", "viewer")
	if err != nil {
		t.Fatal(err)
	}
	viewer := auth.Identity{Kind: auth.KindUser, ID: m.UserID, UserID: m.UserID, Display: "viewer", SecondFactor: true}.WithStepUp(0)
	if f.viewer, err = authService.Resolve(ctx, viewer, o.ID); err != nil {
		t.Fatal(err)
	}
	return f
}

func defaultLimits() limits.Set { return limits.Defaults() }

func actions(t *testing.T, f *fixture, p auth.Principal) []audit.Action {
	t.Helper()
	entries, err := f.tenancy.AuditEntries(context.Background(), p, 0, 500)
	if err != nil {
		t.Fatal(err)
	}
	var out []audit.Action
	for _, e := range entries {
		out = append(out, e.Action)
	}
	return out
}

// TestImportExportDuplicate_STU_023 imports a sample app, exports it, and
// duplicates it: the copies hold the same documents, new identifiers, and
// every step is audited. A bad archive leaves no app behind.
func TestImportExportDuplicate_STU_023(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	samples := f.archives.ListSamples()
	if len(samples) != 1 {
		t.Fatalf("samples: %d", len(samples))
	}
	imp, err := f.archives.ImportSample(ctx, f.owner, "loans", "my-loans", "")
	if err != nil || imp.Documents == 0 || imp.App.Key != "my-loans" {
		t.Fatalf("ImportSample: %+v %v", imp, err)
	}
	exp, err := f.archives.Export(ctx, f.owner, imp.App.ID)
	if err != nil || exp.Filename != "my-loans.plux" {
		t.Fatalf("Export: %+v %v", exp, err)
	}
	again, err := f.archives.Export(ctx, f.owner, imp.App.ID)
	if err != nil || !bytes.Equal(exp.Data, again.Data) {
		t.Fatalf("an export of an unchanged app is not byte-identical: %v", err)
	}
	dup, err := f.archives.Duplicate(ctx, f.owner, imp.App.ID, "", "")
	if err != nil || dup.App.Key != "my-loans-copy" || dup.App.ID == imp.App.ID || dup.Documents != imp.Documents {
		t.Fatalf("Duplicate: %+v %v", dup, err)
	}
	copyExport, err := f.archives.Export(ctx, f.owner, dup.App.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, a, _ := archive.Unpack(exp.Data, archive.LimitsOf(defaultLimits()))
	_, b, _ := archive.Unpack(copyExport.Data, archive.LimitsOf(defaultLimits()))
	if len(a) != len(b) || len(a) == 0 {
		t.Fatalf("the duplicate holds %d files, the source %d", len(b), len(a))
	}
	for i := range a {
		if a[i].Path != b[i].Path || !bytes.Equal(a[i].Content, b[i].Content) {
			t.Fatalf("%s differs in the duplicate", a[i].Path)
		}
	}
	got := actions(t, f, f.owner)
	for _, want := range []audit.Action{audit.AppImported, audit.AppExported, audit.AppDuplicated} {
		if !slices.Contains(got, want) {
			t.Errorf("no %s entry in the audit log", want)
		}
	}
	// An archive that fails schema validation leaves no app.
	bad, err := archive.Pack(archive.AppInfo{Key: "broken", Name: "Broken"}, []document.File{{Path: "app.json", Content: []byte(`{"nope":true}`)}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.archives.Import(ctx, f.owner, bad, "", ""); err == nil {
		t.Fatal("an invalid document was imported")
	}
	apps, err := f.tenancy.ListApps(ctx, f.owner, tenancy.Page{Size: 50, Filter: tenancy.AppFilter{Status: "all"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range apps {
		if a.Key == "broken" {
			t.Fatal("a failed import left its app behind")
		}
	}
	if _, err := f.archives.Import(ctx, f.owner, []byte("PK not an archive"), "", ""); code(err) != plxerr.InvalidFormat {
		t.Fatalf("a malformed archive: %v", err)
	}
}

// TestArchiveRefusals_STU_023 proves permissions and tenancy: a viewer cannot
// import, and another organisation can neither export nor duplicate an app.
func TestArchiveRefusals_STU_023(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	imp, err := f.archives.ImportSample(ctx, f.owner, "loans", "", "")
	if err != nil {
		t.Fatal(err)
	}
	sample := f.archives.ListSamples()[0].Data
	if _, err := f.archives.Import(ctx, f.viewer, sample, "viewer-app", ""); code(err) != plxerr.PermissionDenied {
		t.Errorf("a viewer imported: %v", err)
	}
	if _, err := f.archives.Export(ctx, f.other, imp.App.ID); err == nil {
		t.Error("another organisation exported an app")
	}
	if _, err := f.archives.Duplicate(ctx, f.other, imp.App.ID, "stolen", ""); err == nil {
		t.Error("another organisation duplicated an app")
	}
	if _, err := f.archives.ImportSample(ctx, f.owner, "no-such-sample", "", ""); code(err) != plxerr.ResourceNotFound {
		t.Errorf("an unknown sample: %v", err)
	}
}

// TestListAppsFiltersAndSummaries_STU_022 covers the status and mine filters
// and the bounded summaries, including their tenancy.
func TestListAppsFiltersAndSummaries_STU_022(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	for _, k := range []string{"alpha", "beta", "gamma"} {
		if _, err := f.tenancy.CreateApp(ctx, f.owner, k, k); err != nil {
			t.Fatal(err)
		}
	}
	apps, _ := f.tenancy.ListApps(ctx, f.owner, tenancy.Page{Size: 10})
	beta := apps[1]
	archived := true
	if _, err := f.tenancy.UpdateAppMetadata(ctx, f.owner, beta.ID, tenancy.Metadata{Archived: &archived}); err != nil {
		t.Fatal(err)
	}
	keys := func(status string, mine bool) []string {
		got, err := f.tenancy.ListApps(ctx, f.owner, tenancy.Page{Size: 10, Filter: tenancy.AppFilter{Status: status, Mine: mine}})
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, a := range got {
			out = append(out, a.Key)
		}
		return out
	}
	if got := keys("", false); !slices.Equal(got, []string{"alpha", "gamma"}) {
		t.Errorf("active: %v", got)
	}
	if got := keys("archived", false); !slices.Equal(got, []string{"beta"}) {
		t.Errorf("archived: %v", got)
	}
	if got := keys("all", false); len(got) != 3 {
		t.Errorf("all: %v", got)
	}
	if got := keys("", true); len(got) != 0 {
		t.Errorf("mine without a grant: %v", got)
	}
	if _, err := f.tenancy.GrantAccess(ctx, f.owner, apps[0].ID, "", f.owner.UserID, "developer"); err != nil {
		t.Fatal(err)
	}
	if got := keys("", true); !slices.Equal(got, []string{"alpha"}) {
		t.Errorf("mine with a grant: %v", got)
	}
	if _, err := f.tenancy.ListApps(ctx, f.owner, tenancy.Page{Size: 10, Filter: tenancy.AppFilter{Status: "bogus"}}); code(err) != plxerr.InvalidEnumValue {
		t.Errorf("a bogus status: %v", err)
	}
	if !slices.Contains(actions(t, f, f.owner), audit.AppArchived) {
		t.Error("archiving is not audited")
	}

	sums, next, err := f.tenancy.ListAppSummaries(ctx, f.owner, "", 1)
	if err != nil || len(sums) != 1 || sums[0].App.Key != "alpha" || next != "alpha" {
		t.Fatalf("summaries page one: %+v %q %v", sums, next, err)
	}
	if len(sums[0].Releases) != 3 || sums[0].CrashFreeRate != 1 || len(sums[0].Limits) != 9 || sums[0].Limits[0].Key != string(limits.AppPlugins) || sums[0].Limits[0].Limit == 0 || sums[0].LastPublisher != nil {
		t.Errorf("summary content: %+v", sums[0])
	}
	sums, next, err = f.tenancy.ListAppSummaries(ctx, f.owner, "alpha", 50)
	if err != nil || len(sums) != 1 || sums[0].App.Key != "gamma" || next != "" {
		t.Fatalf("summaries page two (archived left out): %+v %q %v", sums, next, err)
	}
	if other, _, err := f.tenancy.ListAppSummaries(ctx, f.other, "", 50); err != nil || len(other) != 0 {
		t.Fatalf("another organisation sees %d summaries: %v", len(other), err)
	}
}

// TestShellUpdateAndProjectJobs_GEN_004 covers the settings, the comparison
// and the queued generation job.
func TestShellUpdateAndProjectJobs_GEN_004(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	app, err := f.tenancy.CreateApp(ctx, f.owner, "shop", "Shop")
	if err != nil {
		t.Fatal(err)
	}
	st, err := f.shells.Status(ctx, f.owner, app.ID)
	if err != nil || st.Generated != nil || len(st.Changes) != 0 || st.Current.Package != "shop" {
		t.Fatalf("initial status: %+v %v", st, err)
	}
	if _, err := f.shells.SetSettings(ctx, f.viewer, app.ID, st.Current); code(err) != plxerr.PermissionDenied {
		t.Errorf("a viewer set the settings: %v", err)
	}
	bad := st.Current
	bad.ApplicationID = "nope"
	if _, err := f.shells.SetSettings(ctx, f.owner, app.ID, bad); err == nil {
		t.Error("invalid settings were stored")
	}
	if _, err := f.shells.Status(ctx, f.other, app.ID); err == nil {
		t.Error("another organisation read the shell status")
	}
	job, err := f.shells.Generate(ctx, f.owner, app.ID, "development", "production", "")
	if err != nil || job.Status != shellupdate.StatusQueued || len(f.queue.jobs) != 1 {
		t.Fatalf("Generate: %+v %v", job, err)
	}
	if again, err := f.shells.Generate(ctx, f.owner, app.ID, "development", "production", ""); err != nil || again.ID != job.ID || len(f.queue.jobs) != 1 {
		t.Fatalf("a second request while one is active must return it: %+v %v", again, err)
	}
	if _, err := f.shells.Job(ctx, f.other, app.ID, job.ID); err == nil {
		t.Error("another organisation read a job")
	}
	if _, _, err := f.shells.Download(ctx, f.viewer, app.ID, job.ID); err == nil {
		t.Error("a viewer downloaded a project")
	}
	if err := f.shells.Work(ctx, f.queue.jobs[0]); err != nil {
		t.Fatalf("Work: %v", err)
	}
	done, err := f.shells.Job(ctx, f.owner, app.ID, job.ID)
	if err != nil || done.Status != shellupdate.StatusSucceeded || done.Size == 0 {
		t.Fatalf("job after work: %+v %v", done, err)
	}
	_, zipped, err := f.shells.Download(ctx, f.owner, app.ID, job.ID)
	if err != nil || len(zipped) == 0 {
		t.Fatalf("Download: %d %v", len(zipped), err)
	}
	st, err = f.shells.Status(ctx, f.owner, app.ID)
	if err != nil || st.Generated == nil || len(st.Changes) != 0 {
		t.Fatalf("status after generation: %+v %v", st, err)
	}
	next := st.Current
	next.AppVersion, next.Push = "1.1.0+2", true
	if _, err := f.shells.SetSettings(ctx, f.owner, app.ID, next); err != nil {
		t.Fatal(err)
	}
	if st, err = f.shells.Status(ctx, f.owner, app.ID); err != nil || len(st.Changes) != 2 {
		t.Fatalf("status after a settings change: %+v %v", st, err)
	}
	if _, err := f.shells.Generate(ctx, f.owner, app.ID, "nowhere", "production", ""); code(err) != plxerr.ResourceNotFound {
		t.Errorf("an unknown environment: %v", err)
	}
	if !slices.Contains(actions(t, f, f.owner), audit.ProjectGenerated) {
		t.Error("generation is not audited")
	}
}

// TestAppUsageInSummariesAndLimits_STU_025 measures an imported sample app's
// usage once in the summaries and once in ListAppLimits, and proves the two
// agree, that a batch of apps costs no more queries than one, and that another
// organisation neither sees the usage nor reads the limits (LIM-005).
func TestAppUsageInSummariesAndLimits_STU_025(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	imp, err := f.archives.ImportSample(ctx, f.owner, "loans", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.tenancy.CreateApp(ctx, f.owner, "empty", "Empty"); err != nil {
		t.Fatal(err)
	}
	sums, _, err := f.tenancy.ListAppSummaries(ctx, f.owner, "", 50)
	if err != nil || len(sums) != 2 {
		t.Fatalf("summaries: %d %v", len(sums), err)
	}
	var loans, empty tenancy.AppSummary
	for _, s := range sums {
		if s.App.ID == imp.App.ID {
			loans = s
		} else {
			empty = s
		}
	}
	listed, err := f.tenancy.ListAppLimits(ctx, f.owner, imp.App.ID)
	if err != nil {
		t.Fatal(err)
	}
	byKey := map[string]int64{}
	for _, l := range listed {
		byKey[string(l.Key)] = l.Value
	}
	if len(loans.Limits) == 0 {
		t.Fatal("no usage in the summary")
	}
	for _, u := range loans.Limits {
		if byKey[u.Key] != u.Used {
			t.Errorf("%s: the summary says %d, ListAppLimits %d", u.Key, u.Used, byKey[u.Key])
		}
	}
	if byKey[string(limits.AppPlugins)] < 1 || byKey[string(limits.PluginPages)] < 1 || byKey[string(limits.AppLocales)] < 1 {
		t.Errorf("the sample's usage is not measured: %v", byKey)
	}
	for _, u := range empty.Limits {
		if u.Used != 0 {
			t.Errorf("an empty app uses %d of %s", u.Used, u.Key)
		}
	}
	if _, err := f.tenancy.ListAppLimits(ctx, f.other, imp.App.ID); code(err) != plxerr.ResourceNotFound {
		t.Errorf("another organisation reads the limits: %v", err)
	}
}
