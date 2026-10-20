// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package gitsync_test

import (
	"context"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/http/cgi"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/cache"
	"github.com/nightCode42/plux3/backend/internal/document"
	"github.com/nightCode42/plux3/backend/internal/gitsync"
	"github.com/nightCode42/plux3/backend/internal/httpx"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/uuid7"
	"github.com/nightCode42/plux3/backend/internal/signing"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
	"github.com/nightCode42/plux3/backend/internal/storage/storagetest"
	"github.com/nightCode42/plux3/backend/internal/tenancy"
)

const token = "s3cret-token-value"

type ids struct{ g *uuid7.Generator }

func (i ids) New() (string, error) {
	u, err := i.g.New()
	return u.String(), err
}

// recorder is the job queue: it keeps the jobs a publish enqueued.
type recorder struct {
	mu   sync.Mutex
	jobs []gitsync.ExportJob
}

func (r *recorder) Enqueue(_ context.Context, _ pgx.Tx, job gitsync.ExportJob) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.jobs = append(r.jobs, job)
	return nil
}

func (r *recorder) take() []gitsync.ExportJob {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := r.jobs
	r.jobs = nil
	return out
}

// remote is a Git server over HTTPS: git http-backend behind a token check.
type remote struct {
	*httptest.Server
	dir    string
	reject bool
	mu     sync.Mutex
	pushes int
}

func newRemote(t *testing.T) *remote {
	t.Helper()
	gitBin, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is not installed; the export tests serve a repository with git http-backend")
	}
	out, err := exec.Command(gitBin, "--exec-path").Output() //nolint:gosec // the git binary found on PATH
	if err != nil {
		t.Skipf("git --exec-path: %v", err)
	}
	r := &remote{dir: t.TempDir()}
	bare := filepath.Join(r.dir, "app.git")
	for _, args := range [][]string{{"init", "--bare", "-b", "main", bare}, {"-C", bare, "config", "http.receivepack", "true"}} {
		if b, err := exec.Command(gitBin, args...).CombinedOutput(); err != nil { //nolint:gosec // fixed arguments
			t.Fatalf("git %v: %v\n%s", args, err, b)
		}
	}
	backend := &cgi.Handler{
		Path: strings.TrimSpace(string(out)) + "/git-http-backend",
		Env:  []string{"GIT_PROJECT_ROOT=" + r.dir, "GIT_HTTP_EXPORT_ALL=1"},
	}
	r.Server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if _, pass, ok := req.BasicAuth(); !ok || pass != token {
			w.Header().Set("WWW-Authenticate", `Basic realm="git"`)
			http.Error(w, "credentials required", http.StatusUnauthorized)
			return
		}
		if strings.Contains(req.URL.Path, "git-receive-pack") || req.URL.Query().Get("service") == "git-receive-pack" {
			r.mu.Lock()
			reject := r.reject
			r.pushes++
			r.mu.Unlock()
			if reject {
				http.Error(w, "protected branch", http.StatusForbidden)
				return
			}
		}
		backend.ServeHTTP(w, req)
	}))
	t.Cleanup(r.Close)
	return r
}

func (r *remote) url() string { return r.Server.URL + "/app.git" }

func (r *remote) caBundle() []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: r.Certificate().Raw})
}

func (r *remote) setReject(v bool) {
	r.mu.Lock()
	r.reject = v
	r.mu.Unlock()
}

// commits returns the commits of the branch, newest first, and the files of
// the newest one.
func (r *remote) commits(t *testing.T) ([]*object.Commit, map[string]string) {
	t.Helper()
	repo, err := git.PlainOpen(filepath.Join(r.dir, "app.git"))
	if err != nil {
		t.Fatal(err)
	}
	head, err := repo.Reference(plumbing.NewBranchReferenceName("main"), true)
	if err != nil {
		return nil, nil
	}
	iter, err := repo.Log(&git.LogOptions{From: head.Hash()})
	if err != nil {
		t.Fatal(err)
	}
	var commits []*object.Commit
	if err := iter.ForEach(func(c *object.Commit) error { commits = append(commits, c); return nil }); err != nil {
		t.Fatal(err)
	}
	tree, err := commits[0].Tree()
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	if err := tree.Files().ForEach(func(f *object.File) error {
		s, err := f.Contents()
		files[f.Name] = s
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return commits, files
}

type fixture struct {
	db      *storage.DB
	tenancy *tenancy.Service
	docs    *document.Service
	audit   *audit.Log
	gen     ids
	queue   *recorder
	remote  *remote
	org     string
	app     string
	env     string
	owner   auth.Principal
	git     *gitsync.Service
}

func newFixture(t *testing.T, policy httpx.Policy) *fixture {
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
	admin, invitation, err := authService.Bootstrap(ctx, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := authService.AcceptInvitation(ctx, invitation, "Admin", "correct horse battery"); err != nil {
		t.Fatal(err)
	}
	id := auth.Identity{Kind: auth.KindUser, ID: admin.ID, UserID: admin.ID, Display: "Ada Publisher", SecondFactor: true, InstallationAdmin: true}
	f := &fixture{db: db, tenancy: ten, docs: docs, audit: log, gen: gen, queue: &recorder{}, remote: newRemote(t)}
	org, err := ten.CreateOrganization(ctx, id, "acme", "Acme")
	if err != nil {
		t.Fatal(err)
	}
	f.org = org.ID
	if f.owner, err = authService.Resolve(ctx, id, org.ID); err != nil {
		t.Fatal(err)
	}
	app, err := ten.CreateApp(ctx, f.owner, "demo", "Demo")
	if err != nil {
		t.Fatal(err)
	}
	f.app = app.ID
	env, err := ten.GetEnvironmentByKey(ctx, f.owner, f.app, "development")
	if err != nil {
		t.Fatal(err)
	}
	f.env = env.ID
	if _, err := ten.SetSecret(ctx, f.owner, f.env, "GIT_TOKEN", token); err != nil {
		t.Fatal(err)
	}
	f.git, err = gitsync.New(gitsync.Options{
		DB: db, Audit: log, Documents: docs, Secrets: ten, Jobs: f.queue, IDs: gen, Policy: policy, CABundle: f.remote.caBundle(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *fixture) settings() gitsync.Settings {
	return gitsync.Settings{
		AppID: f.app, Enabled: true, RemoteURL: f.remote.url(), Branch: "main",
		CredentialEnvironmentID: f.env, CredentialKey: "GIT_TOKEN",
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

func (f *fixture) putTemplate(t *testing.T, key, name string) {
	t.Helper()
	ctx := context.Background()
	if _, _, err := f.docs.AcquireLock(ctx, f.owner, f.app, "", "s", false); err != nil {
		t.Fatal(err)
	}
	doc := fmt.Sprintf(`{"schemaVersion":"1.0.0","kind":"template","id":%q,"key":%q,"name":%q,"category":"display","description":"d","parameters":[],"visibility":"organization","root":{"id":%q,"type":"Text","props":{"data":"x"}}}`,
		f.newID(t), key, name, f.newID(t))
	if _, err := f.docs.PutDocument(ctx, f.owner, f.app, "", "s", "templates/"+key+".template.json", []byte(doc), -1); err != nil {
		t.Fatal(err)
	}
}

// publish simulates what the publish records: the hook, in a transaction,
// for a version; it returns the jobs enqueued.
func (f *fixture) publish(t *testing.T, version string) []gitsync.ExportJob {
	t.Helper()
	err := f.db.InTx(context.Background(), storage.Tenant{OrganizationID: f.org}, func(ctx context.Context, tx pgx.Tx) error {
		return f.git.Published(ctx, tx, f.org, f.app, version, "app bundle version 1", f.owner.ID, f.owner.Display)
	})
	if err != nil {
		t.Fatal(err)
	}
	return f.queue.take()
}

func code(err error) plxerr.Code {
	var e *plxerr.Error
	if errors.As(err, &e) {
		return e.Code
	}
	return 0
}

func (f *fixture) exportRow(t *testing.T, version string) dbgen.GitExport {
	t.Helper()
	var row dbgen.GitExport
	err := f.db.InTx(context.Background(), storage.Tenant{OrganizationID: f.org}, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		row, err = dbgen.New(tx).GetGitExportForUpdate(ctx, dbgen.GetGitExportForUpdateParams{AppID: storage.MustUUID(f.app), VersionID: storage.MustUUID(version)})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return row
}

// Verifies: GOV-011, SCH-006.
// A publish pushes one commit holding exactly the SCH-006 layout the export
// renders, authored by the publisher under a no-reply address, with the
// release identifier in the message; a second version of the same tree adds
// no commit, a changed tree adds one on top, and a version is exported once.
func TestExport_PushesTheLayout_GOV_011(t *testing.T) {
	t.Parallel()
	f := newFixture(t, httpx.Policy{AllowLoopback: true})
	ctx := context.Background()
	if _, err := f.git.Configure(ctx, f.owner, f.settings()); err != nil {
		t.Fatal(err)
	}
	f.putTemplate(t, "greeting", "Greeting")
	v1, v2, v3 := f.newID(t), f.newID(t), f.newID(t)

	jobs := f.publish(t, v1)
	if len(jobs) != 1 {
		t.Fatalf("a publish enqueued %d jobs, want 1", len(jobs))
	}
	if err := f.git.Work(ctx, jobs[0]); err != nil {
		t.Fatalf("Work: %v", err)
	}
	commits, files := f.remote.commits(t)
	if len(commits) != 1 {
		t.Fatalf("%d commits, want 1", len(commits))
	}
	want, err := f.docs.Export(ctx, f.owner, f.app, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != len(want) || len(want) == 0 {
		t.Fatalf("the commit holds %d files, the export %d", len(files), len(want))
	}
	for _, w := range want {
		if files[w.Path] != string(w.Content) {
			t.Errorf("%s differs from the export", w.Path)
		}
	}
	c := commits[0]
	if c.Author.Name != "Ada Publisher" || !strings.HasSuffix(c.Author.Email, "@users.noreply.plux.invalid") || !strings.Contains(c.Message, "Release-ID: "+v1) {
		t.Errorf("commit = %q <%s> %q", c.Author.Name, c.Author.Email, c.Message)
	}
	if row := f.exportRow(t, v1); row.State != "pushed" || row.CommitSha != c.Hash.String() || row.Attempts != 1 {
		t.Errorf("export row = %+v", row)
	}

	// The same version again: nothing is enqueued, and a repeated job pushes nothing.
	if again := f.publish(t, v1); len(again) != 0 {
		t.Errorf("the same version enqueued %d more jobs", len(again))
	}
	if err := f.git.Work(ctx, jobs[0]); err != nil {
		t.Fatal(err)
	}
	if got, _ := f.remote.commits(t); len(got) != 1 {
		t.Errorf("a repeated job made %d commits", len(got))
	}
	// Another version, same tree: no new commit, but it is recorded as pushed.
	next := f.publish(t, v2)
	if err := f.git.Work(ctx, next[0]); err != nil {
		t.Fatal(err)
	}
	if got, _ := f.remote.commits(t); len(got) != 1 {
		t.Errorf("an unchanged tree made %d commits", len(got))
	}
	if row := f.exportRow(t, v2); row.State != "pushed" || row.CommitSha != c.Hash.String() {
		t.Errorf("unchanged export row = %+v", row)
	}
	// A changed tree is a commit on top of the first.
	f.putTemplate(t, "farewell", "Farewell")
	third := f.publish(t, v3)
	if err := f.git.Work(ctx, third[0]); err != nil {
		t.Fatal(err)
	}
	got, files := f.remote.commits(t)
	if len(got) != 2 || got[0].ParentHashes[0] != c.Hash || files["templates/farewell.template.json"] == "" {
		t.Errorf("commits = %d, files = %d", len(got), len(files))
	}
	assertAudited(t, f, audit.GitExported)
}

func assertAudited(t *testing.T, f *fixture, action audit.Action) {
	t.Helper()
	var n int
	err := f.db.InTx(context.Background(), storage.Tenant{OrganizationID: f.org}, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SELECT count(*) FROM audit_log WHERE action = $1", string(action)).Scan(&n)
	})
	if err != nil || n == 0 {
		t.Errorf("no %s audit entry (%d, %v)", action, n, err)
	}
}

// Verifies: GOV-011, SEC-092.
// A remote that refuses the push gives PLX-8503, the failure is recorded as
// a code and counted, the credential appears in no error, and a later
// attempt that the remote accepts succeeds.
func TestExport_RefusedPush_GOV_011(t *testing.T) {
	t.Parallel()
	f := newFixture(t, httpx.Policy{AllowLoopback: true})
	ctx := context.Background()
	if _, err := f.git.Configure(ctx, f.owner, f.settings()); err != nil {
		t.Fatal(err)
	}
	f.putTemplate(t, "greeting", "Greeting")
	version := f.newID(t)
	jobs := f.publish(t, version)

	f.remote.setReject(true)
	err := f.git.Work(ctx, jobs[0])
	if code(err) != plxerr.GitRemoteRefused {
		t.Fatalf("Work = %v, want PLX-%d", err, plxerr.GitRemoteRefused)
	}
	if strings.Contains(err.Error(), token) || strings.Contains(fmt.Sprintf("%+v", err), token) {
		t.Errorf("the error carries the credential: %v", err)
	}
	row := f.exportRow(t, version)
	if row.State != "failed" || row.ErrorCode != int32(plxerr.GitRemoteRefused) || row.Attempts != 1 {
		t.Errorf("export row after a refusal = %+v", row)
	}
	if got, _ := f.remote.commits(t); len(got) != 0 {
		t.Errorf("a refused push left %d commits", len(got))
	}

	f.remote.setReject(false)
	if err := f.git.Work(ctx, jobs[0]); err != nil {
		t.Fatalf("the retry: %v", err)
	}
	if row := f.exportRow(t, version); row.State != "pushed" || row.Attempts != 2 || row.ErrorCode != 0 {
		t.Errorf("export row after the retry = %+v", row)
	}
}

// Verifies: GOV-011, SEC-092.
// A wrong credential is refused as PLX-8503 without saying what it was.
func TestExport_WrongCredential_GOV_011(t *testing.T) {
	t.Parallel()
	f := newFixture(t, httpx.Policy{AllowLoopback: true})
	ctx := context.Background()
	if _, err := f.tenancy.SetSecret(ctx, f.owner, f.env, "GIT_TOKEN", "not-the-token"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.git.Configure(ctx, f.owner, f.settings()); err != nil {
		t.Fatal(err)
	}
	f.putTemplate(t, "greeting", "Greeting")
	jobs := f.publish(t, f.newID(t))
	err := f.git.Work(ctx, jobs[0])
	if code(err) != plxerr.GitRemoteRefused || strings.Contains(err.Error(), "not-the-token") {
		t.Errorf("Work = %v", err)
	}
}

// Verifies: GOV-011, SEC-163.
// A remote at a reserved address is refused when it is configured and when
// the export runs, and no connection is made.
func TestExport_PrivateAddressRefused_GOV_011(t *testing.T) {
	t.Parallel()
	f := newFixture(t, httpx.Policy{})
	ctx := context.Background()
	for _, remote := range []string{
		f.remote.url(), "https://169.254.169.254/latest.git", "https://10.1.2.3/x.git", "https://[::1]/x.git", "https://192.168.0.9:8443/x.git",
	} {
		s := f.settings()
		s.RemoteURL = remote
		if _, err := f.git.Configure(ctx, f.owner, s); code(err) != plxerr.GitRemoteRefused {
			t.Errorf("Configure(%s) = %v, want PLX-%d", remote, err, plxerr.GitRemoteRefused)
		}
	}

	// A setting stored while loopback was allowed is refused by the job.
	permissive := newFixtureServices(t, f)
	if _, err := permissive.Configure(ctx, f.owner, f.settings()); err != nil {
		t.Fatal(err)
	}
	f.putTemplate(t, "greeting", "Greeting")
	version := f.newID(t)
	job := gitsync.ExportJob{OrganizationID: f.org, AppID: f.app, VersionID: version, Summary: "x", ActorID: f.owner.ID, Actor: "A"}
	err := f.db.InTx(ctx, storage.Tenant{OrganizationID: f.org}, func(ctx context.Context, tx pgx.Tx) error {
		return permissive.Published(ctx, tx, f.org, f.app, version, "x", f.owner.ID, "A")
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.git.Work(ctx, job); code(err) != plxerr.GitRemoteRefused {
		t.Errorf("Work against a loopback remote = %v, want PLX-%d", err, plxerr.GitRemoteRefused)
	}
	f.remote.mu.Lock()
	defer f.remote.mu.Unlock()
	if f.remote.pushes != 0 {
		t.Errorf("the server connected %d times", f.remote.pushes)
	}
}

// newFixtureServices returns a service on the fixture's database that allows
// loopback addresses.
func newFixtureServices(t *testing.T, f *fixture) *gitsync.Service {
	t.Helper()
	svc, err := gitsync.New(gitsync.Options{
		DB: f.db, Audit: f.audit, Documents: f.docs, Secrets: f.tenancy, Jobs: f.queue, IDs: f.gen,
		Policy: httpx.Policy{AllowLoopback: true}, CABundle: f.remote.caBundle(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

// Verifies: GOV-011, SEC-102.
// The export is off by default, a disabled one enqueues nothing, and the
// configuration is validated, permissioned and audited.
func TestExport_Configure_GOV_011(t *testing.T) {
	t.Parallel()
	f := newFixture(t, httpx.Policy{AllowLoopback: true})
	ctx := context.Background()
	if _, err := f.git.Get(ctx, f.owner, f.app); code(err) != plxerr.ResourceNotFound {
		t.Errorf("Get before configuring = %v", err)
	}
	if jobs := f.publish(t, f.newID(t)); len(jobs) != 0 {
		t.Fatalf("an unconfigured app enqueued %d jobs", len(jobs))
	}
	off := f.settings()
	off.Enabled = false
	if _, err := f.git.Configure(ctx, f.owner, off); err != nil {
		t.Fatal(err)
	}
	if jobs := f.publish(t, f.newID(t)); len(jobs) != 0 {
		t.Errorf("a disabled export enqueued %d jobs", len(jobs))
	}
	for name, mutate := range map[string]func(*gitsync.Settings){
		"http":           func(s *gitsync.Settings) { s.RemoteURL = "http://example.com/a.git" },
		"ssh":            func(s *gitsync.Settings) { s.RemoteURL = "git@example.com:a/b.git" },
		"credentials":    func(s *gitsync.Settings) { s.RemoteURL = "https://user:pw@example.com/a.git" },
		"query":          func(s *gitsync.Settings) { s.RemoteURL = "https://example.com/a.git?x=1" },
		"branch":         func(s *gitsync.Settings) { s.Branch = "a..b" },
		"no credential":  func(s *gitsync.Settings) { s.CredentialKey = "" },
		"username colon": func(s *gitsync.Settings) { s.Username = "a:b" },
	} {
		s := f.settings()
		mutate(&s)
		if _, err := f.git.Configure(ctx, f.owner, s); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	viewer := auth.Principal{Identity: f.owner.Identity, OrganizationID: f.org, AppPermissions: map[string][]auth.Permission{f.app: {auth.AppRead}}}
	if _, err := f.git.Configure(ctx, viewer, f.settings()); code(err) != plxerr.PermissionDenied {
		t.Errorf("without git.export: %v", err)
	}
	if _, err := f.git.List(ctx, viewer, f.app); code(err) != plxerr.PermissionDenied {
		t.Errorf("List without git.export: %v", err)
	}
	got, err := f.git.Configure(ctx, f.owner, f.settings())
	if err != nil || !got.Enabled || got.Branch != "main" {
		t.Fatalf("Configure = %+v, %v", got, err)
	}
	if jobs := f.publish(t, f.newID(t)); len(jobs) != 1 {
		t.Errorf("an enabled export enqueued %d jobs", len(jobs))
	}
	assertAudited(t, f, audit.GitExportConfigured)
	if exports, err := f.git.List(ctx, f.owner, f.app); err != nil || len(exports) != 1 || exports[0].State != "pending" {
		t.Errorf("List = %+v, %v", exports, err)
	}
	// The credential must belong to the app.
	s := f.settings()
	s.CredentialEnvironmentID = f.newID(t)
	if _, err := f.git.Configure(ctx, f.owner, s); code(err) != plxerr.ResourceNotFound {
		t.Errorf("a foreign environment = %v", err)
	}
}
