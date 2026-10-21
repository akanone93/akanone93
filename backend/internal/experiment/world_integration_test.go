// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package experiment_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/controldoc"
	"github.com/nightCode42/plux3/backend/internal/document"
	"github.com/nightCode42/plux3/backend/internal/experiment"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/release"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/schema/uuid7"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/storagetest"
)

// ids adapts the UUID generator to the identifier interface of the service.
type ids struct{ g *uuid7.Generator }

func (i ids) New() (string, error) {
	u, err := i.g.New()
	if err != nil {
		return "", err //nolint:wrapcheck // a test
	}
	return u.String(), nil
}

// recordingControl stands in for the release service: it keeps what the
// experiment service last wrote into the control documents.
type recordingControl struct {
	mu     sync.Mutex
	last   release.Experimentation
	writes int
	env    string
}

func (c *recordingControl) ApplyExperimentation(_ context.Context, _ pgx.Tx, _ auth.Principal, environmentID string, ex release.Experimentation, _ string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.last, c.writes, c.env = ex, c.writes+1, environmentID
	return nil
}

// segments resolves the segments a test defines.
type segments map[string]controldoc.Rule

func (s segments) Rule(_ context.Context, _ auth.Principal, _, key string) (controldoc.Rule, error) {
	if r, ok := s[key]; ok {
		return r, nil
	}
	return controldoc.Rule{}, plxerr.New(plxerr.UnresolvedReference, "the app defines no segment %q", key)
}

// refusingGate refuses every start, as the approval engine does for an
// experiment without an approval.
type refusingGate struct{ asked int }

func (g *refusingGate) RequireLaunch(context.Context, experiment.Launch) error {
	g.asked++
	return plxerr.New(plxerr.ApprovalRequired, "this experiment needs an approval")
}

// tenant is an organisation with an app, two environments and a principal.
type tenant struct {
	org, app, prod, dev string
	admin               auth.Principal
}

// world is a database and a service wired to fakes of its neighbours.
type world struct {
	t       *testing.T
	db      *storage.DB
	svc     *experiment.Service
	ids     ids
	control *recordingControl
	lim     limits.Set
	a       tenant
	other   tenant
	now     time.Time
	opts    experiment.Options
}

// perms is what the test administrator holds.
var perms = []auth.Permission{auth.AppRead, auth.FlagsManage, auth.ExperimentsManage, auth.AnalyticsRead}

func newWorld(t *testing.T, mutate func(*experiment.Options)) *world {
	t.Helper()
	g := ids{g: uuid7.NewGenerator(time.Now, rand.Reader)}
	w := &world{t: t, db: storagetest.Open(t), ids: g, control: &recordingControl{}, lim: limits.Defaults(), now: time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)}
	w.opts = experiment.Options{
		DB: w.db, Audit: audit.NewLog(g, nil), IDs: g, Control: w.control, Now: func() time.Time { return w.now },
		Limits: func(context.Context, pgx.Tx, string, string) (limits.Set, error) { return w.lim, nil },
		Segments: segments{
			"beta-testers": {Attribute: "os", Operator: controldoc.OpEq, ValueType: controldoc.ValueString, Values: []string{"ios"}},
			"germans":      {Attribute: "locale", Operator: controldoc.OpIn, ValueType: controldoc.ValueString, Values: []string{"de", "de-AT"}},
		},
	}
	if mutate != nil {
		mutate(&w.opts)
	}
	svc, err := experiment.New(w.opts)
	if err != nil {
		t.Fatal(err)
	}
	w.svc = svc
	w.a, w.other = w.tenant("acme"), w.tenant("globex")
	return w
}

func (w *world) id() string {
	w.t.Helper()
	s, err := w.ids.New()
	if err != nil {
		w.t.Fatal(err)
	}
	return s
}

func (w *world) exec(org, sql string, args ...any) {
	w.t.Helper()
	err := w.db.InTx(context.Background(), storage.Tenant{OrganizationID: org}, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, sql, args...)
		return err //nolint:wrapcheck // a test
	})
	if err != nil {
		w.t.Fatalf("%s: %v", sql, err)
	}
}

// tenant creates an organisation with an app and its production and
// development environments.
func (w *world) tenant(key string) tenant {
	w.t.Helper()
	x := tenant{org: w.id(), app: w.id(), prod: w.id(), dev: w.id()}
	if _, err := w.db.Pool().Exec(context.Background(), `INSERT INTO organizations (id, key, name) VALUES ($1, $2, $2)`, storage.MustUUID(x.org), key); err != nil {
		w.t.Fatal(err)
	}
	w.exec(x.org, `INSERT INTO apps (id, organization_id, key, name) VALUES ($1, $2, 'demo', 'Demo')`, storage.MustUUID(x.app), storage.MustUUID(x.org))
	for id, env := range map[string]string{x.prod: "production", x.dev: "development"} {
		w.exec(x.org, `INSERT INTO environments (id, organization_id, app_id, key, name, production) VALUES ($1, $2, $3, $4, $4, $5)`,
			storage.MustUUID(id), storage.MustUUID(x.org), storage.MustUUID(x.app), env, env == "production")
	}
	x.admin = auth.Principal{
		Identity:       auth.Identity{Kind: auth.KindSystem, ID: "experiment-test", SecondFactor: true},
		OrganizationID: x.org,
		Permissions:    slices.Clone(perms),
	}
	return x
}

// as returns the tenant's principal restricted to the permissions.
func (x tenant) as(p ...auth.Permission) auth.Principal {
	out := x.admin
	out.Permissions = p
	return out
}

// document returns the experiment document of the conformance project, with
// edits applied.
func experimentDoc(t *testing.T, name string, edit func(map[string]any)) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(conformanceDir, "experiments", name+".experiment.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if edit != nil {
		edit(doc)
	}
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// conformanceDir is the project of flags, segments and experiments the
// compiler and the runtime are tested with.
var conformanceDir = filepath.Join("..", "..", "..", "schema", "testdata", "documents", "experiments")

// newDoc makes an experiment document with a fresh key, id and salt over a
// layer.
func newDoc(t *testing.T, key, layer string, edit func(map[string]any)) []byte {
	t.Helper()
	return experimentDoc(t, "heroLayout", func(d map[string]any) {
		d["key"], d["id"] = key, uuid()
		d["salt"] = key + "-salt"
		d["minimumSampleSize"] = 100
		if layer != "" {
			d["layer"] = layer
		}
		if edit != nil {
			edit(d)
		}
	})
}

func uuid() string {
	u, err := uuid7.NewGenerator(time.Now, rand.Reader).New()
	if err != nil {
		panic(err)
	}
	return u.String()
}

// create registers an experiment and fails the test if it is refused.
func (w *world) create(x tenant, env string, doc []byte) experiment.Experiment {
	w.t.Helper()
	e, err := w.svc.CreateExperiment(context.Background(), x.admin, experiment.CreateExperimentRequest{AppID: x.app, EnvironmentID: env, Document: doc})
	if err != nil {
		w.t.Fatalf("CreateExperiment: %v", err)
	}
	return e
}

// wantCode fails the test unless err is a refusal with the code.
func wantCode(t *testing.T, err error, want plxerr.Code) {
	t.Helper()
	got, ok := plxerr.CodeOf(err)
	if !ok || got != want {
		t.Fatalf("error = %v (code %v, %v), want PLX-%d", err, got, ok, want)
	}
}

// fakeDocuments serves a project and keeps what is imported over it.
type fakeDocuments struct {
	mu       sync.Mutex
	files    map[string][]byte
	imported []document.File
}

func projectFiles(t *testing.T) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	err := fs.WalkDir(os.DirFS(conformanceDir), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(filepath.Join(conformanceDir, filepath.FromSlash(p)))
		out[p] = data
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func (*fakeDocuments) GetDocument(context.Context, auth.Principal, string, string, string) (document.Document, error) {
	return document.Document{}, plxerr.New(plxerr.ResourceNotFound, "no such document")
}

func (d *fakeDocuments) Export(context.Context, auth.Principal, string, string) ([]document.File, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []document.File
	for _, name := range slices.Sorted(maps.Keys(d.files)) {
		out = append(out, document.File{Path: name, Content: d.files[name]})
	}
	return out, nil
}

func (d *fakeDocuments) Import(_ context.Context, _ auth.Principal, _, _, _ string, files []document.File) (document.Written, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.imported = files
	return document.Written{}, nil
}

func (*fakeDocuments) ReleaseLock(context.Context, auth.Principal, string, string, string) error {
	return nil
}

// fakePublisher records the publish it was asked for.
type fakePublisher struct {
	asked []release.PublishRequest
	job   string
}

func (p *fakePublisher) Publish(_ context.Context, _ auth.Principal, r release.PublishRequest) (release.PublishJob, error) {
	p.asked = append(p.asked, r)
	return release.PublishJob{ID: p.job, AppID: r.AppID, EnvironmentID: r.EnvironmentID}, nil
}
