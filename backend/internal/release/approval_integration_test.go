// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package release_test

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/approval"
	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/golden"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/release"
	"github.com/nightCode42/plux3/backend/internal/schema"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/schema/uuid7"
	"github.com/nightCode42/plux3/backend/internal/seccfg"
	"github.com/nightCode42/plux3/backend/internal/security/settings"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

// guarded is a release fixture whose promotions and capability changes ask
// an approval engine, and the engine, whose profile the test sets.
type guarded struct {
	*fixture
	engine  *approval.Service
	profile settings.Profile
}

// newGuarded builds the fixture with the engine bound to the release service
// as its reviewer.
func newGuarded(t *testing.T, profile settings.Profile) *guarded {
	t.Helper()
	g := &guarded{profile: profile}
	g.fixture = newFixtureWith(t, "loan-calculator", limits.Set{}, func(o *release.Options) {
		engine, err := approval.New(approval.Options{DB: o.DB, Audit: o.Audit, IDs: o.IDs})
		if err != nil {
			t.Fatal(err)
		}
		engine.Use(func(context.Context, string, string) (settings.Profile, error) { return g.profile, nil }, nil)
		o.Approvals, g.engine = engine, engine
	})
	g.engine.Register(schema.PolicyResourceRelease, g.rel)
	return g
}

// reviewer adds a member who may decide approvals.
func (f *fixture) reviewer(t *testing.T, email string) auth.Principal {
	t.Helper()
	ctx := context.Background()
	m, _, err := f.tenancy.AddMember(ctx, f.owner, "", email, "reviewer")
	if err != nil {
		t.Fatal(err)
	}
	p, err := f.auth.Resolve(ctx, auth.Identity{Kind: auth.KindUser, ID: m.UserID, UserID: m.UserID, Display: email, SecondFactor: true}.WithStepUp(0), f.org)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// requestNamed returns the request a refusal names.
func requestNamed(t *testing.T, err error, want plxerr.Code) string {
	t.Helper()
	var e *plxerr.Error
	if !errors.As(err, &e) || e.Code != want || e.Details["request"] == "" {
		t.Fatalf("want code %d with a request, got %v", want, err)
	}
	return e.Details["request"]
}

// releaseOfDevelopment publishes the app and its plugin and creates a
// release of the development environment.
func (f *fixture) releaseOfDevelopment(t *testing.T) release.Release {
	t.Helper()
	f.publish(t, "", false)
	if job := f.publish(t, f.loans, false); job.State != release.StateSucceeded {
		t.Fatalf("publish: %+v", job.Diagnostics)
	}
	rel, diags, err := f.rel.CreateRelease(context.Background(), f.owner, f.app, f.envs["development"].ID, nil, "")
	if err != nil || diags.HasErrors() {
		t.Fatalf("CreateRelease: %v %v", diags, err)
	}
	return rel
}

// Verifies: GOV-020, GOV-021, GOV-022, GOV-023, REL-004.
// Promoting a release to a production environment asks the approval engine:
// under the standard profile one person other than the release's author must
// approve, the approval is spent by the promotion, a promotion to another
// environment asks nothing, and the review bundle shows the changes, the
// reports and the risk of what is approved.
func TestPromotionNeedsApproval_GOV_020(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	g := newGuarded(t, settings.Standard)
	rel := g.releaseOfDevelopment(t)
	reviewer := g.reviewer(t, "r@example.com")
	prod, staging := g.envs["production"].ID, g.envs["staging"].ID

	if _, err := g.rel.PromoteRelease(ctx, g.owner, g.app, rel.Sequence, staging, ""); err != nil {
		t.Fatalf("a promotion to a non-production environment asks nothing: %v", err)
	}
	_, err := g.rel.PromoteRelease(ctx, g.owner, g.app, rel.Sequence, prod, "")
	id := requestNamed(t, err, plxerr.ApprovalRequired)
	if _, err := g.engine.Decide(ctx, g.owner, g.app, id, approval.DecisionApprove, ""); code(err) != plxerr.ApproverNotEligible {
		t.Fatalf("the author approved their own release: %v", err)
	}

	review, err := g.engine.Review(ctx, g.viewer, g.app, id)
	if err != nil {
		t.Fatal(err)
	}
	if review.Request.Resource != schema.PolicyResourceRelease || len(review.Evidence.Changelog) == 0 || review.Evidence.Notes != rel.Notes {
		t.Errorf("review = %+v", review.Evidence)
	}
	kinds := map[string]bool{}
	for _, r := range review.Evidence.Reports {
		kinds[r.Kind] = true
	}
	if !kinds["validation"] || !kinds["accessibility"] {
		t.Errorf("reports = %v", kinds)
	}
	var diff []map[string]string
	if err := json.Unmarshal(review.Evidence.Diff, &diff); err != nil || len(diff) == 0 {
		t.Errorf("the diff is not the changes: %s %v", review.Evidence.Diff, err)
	}

	if _, err := g.engine.Decide(ctx, reviewer, g.app, id, approval.DecisionApprove, "reviewed"); err != nil {
		t.Fatal(err)
	}
	if _, err := g.rel.PromoteRelease(ctx, g.owner, g.app, rel.Sequence, prod, ""); err != nil {
		t.Fatalf("the approved promotion: %v", err)
	}
	_, err = g.rel.PromoteRelease(ctx, g.owner, g.app, rel.Sequence, prod, "")
	requestNamed(t, err, plxerr.ApprovalRequired)
}

// editResult changes the loans plugin's result page and publishes the plugin.
func (f *fixture) editResult(t *testing.T, note string) {
	t.Helper()
	ctx := context.Background()
	if _, _, err := f.docs.AcquireLock(ctx, f.owner, f.app, f.loans, "edit", false); err != nil {
		t.Fatal(err)
	}
	page, err := f.docs.GetDocument(ctx, f.owner, f.app, f.loans, "plugins/loans/pages/result.page.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(page.Content, &doc); err != nil {
		t.Fatal(err)
	}
	doc["x-note"] = note
	changed, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.docs.PutDocument(ctx, f.owner, f.app, f.loans, "edit", page.Path, changed, page.Revision); err != nil {
		t.Fatal(err)
	}
	if job := f.publish(t, f.loans, false); job.State != release.StateSucceeded {
		t.Fatalf("publish: %+v", job.Diagnostics)
	}
}

// promotionDiff asks to promote a release to production and returns the
// changes its approval request shows.
func (g *guarded) promotionDiff(t *testing.T, sequence int64) (string, []release.ChangelogEntry) {
	t.Helper()
	ctx := context.Background()
	_, err := g.rel.PromoteRelease(ctx, g.owner, g.app, sequence, g.envs["production"].ID, "")
	var refusal *plxerr.Error
	if !errors.As(err, &refusal) || refusal.Code != plxerr.ApprovalRequired {
		t.Fatalf("the promotion asked for no approval: %v", err)
	}
	id := refusal.Details["request"]
	review, err := g.engine.Review(ctx, g.viewer, g.app, id)
	if err != nil {
		t.Fatal(err)
	}
	var diff []release.ChangelogEntry
	if err := json.Unmarshal(review.Evidence.Diff, &diff); err != nil {
		t.Fatal(err)
	}
	return id, diff
}

// Verifies: GOV-020, GOV-022.
// A promotion's risk and the diff its approvers see are read against the
// release the channel holds: against an empty channel every document is new,
// however many releases came before, and against a channel several releases
// behind the changes of all of them count, not only the last one's.
func TestPromotionChangesAgainstTheChannel_GOV_020(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	g := newGuarded(t, settings.Standard)
	dev := g.envs["development"].ID
	g.releaseOfDevelopment(t)
	g.editResult(t, "second")
	second, _, err := g.rel.CreateRelease(ctx, g.owner, g.app, dev, map[string]int64{"loans": 2}, "")
	if err != nil {
		t.Fatal(err)
	}
	id, diff := g.promotionDiff(t, second.Sequence)
	if len(diff) < 2 || slices.ContainsFunc(diff, func(e release.ChangelogEntry) bool { return e.Change != "added" }) {
		t.Fatalf("against an empty channel every document is new, got %+v", diff)
	}
	if _, err := g.engine.Decide(ctx, g.reviewer(t, "r@example.com"), g.app, id, approval.DecisionApprove, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := g.rel.PromoteRelease(ctx, g.owner, g.app, second.Sequence, g.envs["production"].ID, ""); err != nil {
		t.Fatal(err)
	}

	g.editResult(t, "third")
	if _, _, err := g.rel.CreateRelease(ctx, g.owner, g.app, dev, map[string]int64{"loans": 3}, ""); err != nil {
		t.Fatal(err)
	}
	fourth, _, err := g.rel.CreateRelease(ctx, g.owner, g.app, dev, map[string]int64{"loans": 3}, "")
	if err != nil {
		t.Fatal(err)
	}
	_, diff = g.promotionDiff(t, fourth.Sequence)
	if len(diff) != 1 || diff[0].Change != "changed" || diff[0].Kind != "page" || diff[0].Name != "result" {
		t.Fatalf("against a channel two releases behind, the page change counts, got %+v", diff)
	}
}

// Verifies: GOV-020, TST-003.
// Approving golden images asks the approval engine under the maximum
// profile: two people besides the one who approves; under the standard
// profile it asks nothing.
func TestGoldenApprovalNeedsApproval_GOV_020(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	g := newGuarded(t, settings.Maximum)
	gen := ids{g: uuid7.NewGenerator(time.Now, rand.Reader)}
	svc := golden.New(golden.Options{DB: g.db, Audit: audit.NewLog(gen, nil), Objects: g.store, IDs: gen, Approvals: g.engine})
	first, second := g.reviewer(t, "r1@example.com"), g.reviewer(t, "r2@example.com")
	hash := sha256.Sum256([]byte("bundle"))
	image := sha256.Sum256([]byte("A"))
	run := golden.Run{AppID: g.app, PluginKey: "loans", Bundle: hash[:], App: hash[:], Canonical: true, Cases: []golden.Case{
		{ID: "home/a", Frame: "small-360x640", Locale: "en", Theme: "light", TextScale: 1, SHA256: image[:], Image: []byte("A")},
	}}
	runID, _, err := svc.Upload(ctx, g.owner, run)
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.Approve(ctx, g.owner, g.app, runID, nil)
	id := requestNamed(t, err, plxerr.ApprovalRequired)
	for _, p := range []auth.Principal{first, second} {
		if _, err := g.engine.Decide(ctx, p, g.app, id, approval.DecisionApprove, ""); err != nil {
			t.Fatal(err)
		}
	}
	if approved, err := svc.Approve(ctx, g.owner, g.app, runID, nil); err != nil || len(approved) != 1 {
		t.Fatalf("the approved baselines: %v %v", approved, err)
	}

	g.profile = settings.Standard
	other := golden.Run{AppID: g.app, PluginKey: "other", Bundle: hash[:], App: hash[:], Canonical: true, Cases: []golden.Case{
		{ID: "home/b", Frame: "small-360x640", Locale: "en", Theme: "light", TextScale: 1, SHA256: image[:], Image: []byte("A")},
	}}
	next, _, err := svc.Upload(ctx, g.owner, other)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Approve(ctx, g.owner, g.app, next, nil); code(err) != plxerr.ApprovalRequired {
		t.Fatalf("the maximum defaults stay in force for the app once written: %v", err)
	}
}

// publishTo publishes the app document to an environment and runs the job.
func (f *fixture) publishTo(t *testing.T, env string) release.PublishJob {
	t.Helper()
	job, err := f.rel.Publish(context.Background(), f.owner, release.PublishRequest{AppID: f.app, EnvironmentID: f.envs[env].ID, Label: "l"})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	f.run(t)
	got, err := f.rel.GetPublishJob(context.Background(), f.viewer, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// Verifies: TST-005.
// With the security setting requiredTests a publish to a production
// environment needs a canonical test and golden run for the exact bundles,
// as goldens.blockPublish does; a development environment and an
// environment without the setting are not asked.
func TestRequiredTestsGatePublish_TST_005(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newFixture(t)
	if job := f.publishTo(t, "production"); job.State != release.StateSucceeded {
		t.Fatalf("a production publish without the setting: %+v", job.Diagnostics)
	}
	if _, err := f.sec.Set(ctx, f.owner, seccfg.Change{
		AppID: f.app, EnvironmentID: f.envs["production"].ID, Profile: "strict", Overrides: []byte(`{"requiredTests":true}`),
	}); err != nil {
		t.Fatal(err)
	}
	job := f.publishTo(t, "production")
	if job.State != release.StateFailed || !hasCode(job.Diagnostics, plxerr.GoldenRunMissing) {
		t.Fatalf("a production publish with requiredTests and no run: %+v", job)
	}
	found := false
	for _, d := range job.Diagnostics {
		found = found || strings.Contains(d.Message, "requiredTests")
	}
	if !found {
		t.Errorf("the refusal does not name the setting: %+v", job.Diagnostics)
	}
	if job := f.publishTo(t, "development"); job.State != release.StateSucceeded {
		t.Errorf("a development publish is not asked for tests: %+v", job.Diagnostics)
	}
	st, ok := settings.Lookup(settings.RequiredTests)
	if !ok {
		t.Fatal("requiredTests is not in the registry")
	}
	if d, _ := st.Defaults.For(settings.Maximum); !d.Bool() {
		t.Error("requiredTests is not on under the maximum profile")
	}
	if d, _ := st.Defaults.For(settings.Standard); d.Bool() {
		t.Error("requiredTests is on under the standard profile")
	}
}

// Verifies: SEC-080, GOV-020.
// Changing the app's approved capability set at a publish asks the approval
// engine when a policy of resource capabilitySet says so; the first publish
// and an unchanged set ask nothing.
func TestCapabilitySetChangeNeedsApproval_SEC_080(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	g := newGuarded(t, settings.Standard)
	reviewer := g.reviewer(t, "r@example.com")
	policy := `{"schemaVersion":"1.0.0","kind":"policy","id":"01b0c450-6c00-7000-8000-0000000000b1","key":"capabilities","name":"Capabilities","match":{"resources":["capabilitySet"]},"require":{"approvers":1}}`
	if _, err := g.engine.PutPolicy(ctx, g.owner, g.app, []byte(policy)); err != nil {
		t.Fatal(err)
	}
	if job := g.publish(t, "", false); job.State != release.StateSucceeded {
		t.Fatalf("the first app publish: %+v", job.Diagnostics)
	}
	if job := g.publish(t, "", false); job.State != release.StateSucceeded {
		t.Fatalf("an unchanged set: %+v", job.Diagnostics)
	}
	app, err := g.docs.GetDocument(ctx, g.owner, g.app, "", "app.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(app.Content, &doc); err != nil {
		t.Fatal(err)
	}
	doc["capabilities"] = map[string]any{"networkDomains": []string{"api.example.com"}}
	content, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.docs.PutDocument(ctx, g.owner, g.app, "", "s", "app.json", content, app.Revision); err != nil {
		t.Fatal(err)
	}

	job := g.publish(t, "", false)
	if job.State != release.StateFailed || !hasCode(job.Diagnostics, plxerr.ApprovalRequired) {
		t.Fatalf("a changed capability set: %+v", job)
	}
	pending, err := g.engine.ListApprovals(ctx, g.owner, g.app, approval.StatusOpen, storage.Cursor{}, 10)
	if err != nil || len(pending) != 1 || pending[0].Resource != schema.PolicyResourceCapabilitySet {
		t.Fatalf("pending = %+v, %v", pending, err)
	}
	if _, err := g.engine.Decide(ctx, reviewer, g.app, pending[0].ID, approval.DecisionApprove, ""); err != nil {
		t.Fatal(err)
	}
	if job := g.publish(t, "", false); job.State != release.StateSucceeded {
		t.Fatalf("the approved capability set: %+v", job.Diagnostics)
	}
}
