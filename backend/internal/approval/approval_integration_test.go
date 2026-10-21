// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package approval_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/edition"
	"github.com/nightCode42/plux3/backend/internal/approval"
	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/cache"
	"github.com/nightCode42/plux3/backend/internal/notify"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema"
	"github.com/nightCode42/plux3/backend/internal/schema/uuid7"
	"github.com/nightCode42/plux3/backend/internal/security/settings"
	"github.com/nightCode42/plux3/backend/internal/signing"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/storagetest"
	"github.com/nightCode42/plux3/backend/internal/tenancy"
)

const auditKey = "audit"

type ids struct{ g *uuid7.Generator }

func (i ids) New() (string, error) {
	u, err := i.g.New()
	return u.String(), err
}

// fixture is an organisation with an owner who writes changes, two reviewers
// who may decide them and a viewer who may not, and one app.
type fixture struct {
	db       *storage.DB
	log      *audit.Log
	svc      *approval.Service
	signer   *signing.File
	now      time.Time
	profile  settings.Profile
	org      string
	owner    auth.Principal
	reviewer auth.Principal
	second   auth.Principal
	viewer   auth.Principal
	app      string
	envs     map[string]string
	gen      ids
	notified []string
}

// profileOf answers every environment with the fixture's profile.
func (f *fixture) profileOf(context.Context, string, string) (settings.Profile, error) {
	return f.profile, nil
}

// notices records the notifications the engine publishes.
type notices struct{ f *fixture }

func (n notices) Publish(_ context.Context, e notify.Event) error {
	n.f.notified = append(n.f.notified, string(e.Type)+" "+e.Attributes["subject"])
	return nil
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	db := storagetest.Open(t)
	signer, err := signing.NewFile(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	gen := ids{g: uuid7.NewGenerator(time.Now, rand.Reader)}
	log := audit.NewLog(gen, nil)
	f := &fixture{db: db, log: log, signer: signer, now: time.Now(), profile: settings.Standard, envs: map[string]string{}, gen: gen}
	clock := func() time.Time { return f.now }
	authService, err := auth.NewService(auth.Options{DB: db, Audit: log, Cache: cache.NewMemory(nil), Crypter: signer, IDs: gen, VerificationURI: "https://p.example/device"})
	if err != nil {
		t.Fatal(err)
	}
	ten, err := tenancy.NewService(tenancy.Options{DB: db, Audit: log, Auth: authService, Crypter: signer, IDs: gen, SigningKeyPrefix: "targets", Now: clock})
	if err != nil {
		t.Fatal(err)
	}
	f.svc, err = approval.New(approval.Options{DB: db, Audit: log, IDs: gen, Now: clock})
	if err != nil {
		t.Fatal(err)
	}
	f.svc.Use(f.profileOf, notices{f})
	admin, invitation, err := authService.Bootstrap(ctx, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := authService.AcceptInvitation(ctx, invitation, "Admin", "correct horse battery"); err != nil {
		t.Fatal(err)
	}
	adminID := auth.Identity{Kind: auth.KindUser, ID: admin.ID, UserID: admin.ID, Display: "Admin", SecondFactor: true, InstallationAdmin: true}.WithStepUp(0)
	o, err := ten.CreateOrganization(ctx, adminID, "acme", "Acme")
	if err != nil {
		t.Fatal(err)
	}
	f.org = o.ID
	if f.owner, err = authService.Resolve(ctx, adminID, f.org); err != nil {
		t.Fatal(err)
	}
	a, err := ten.CreateApp(ctx, f.owner, "demo", "Demo")
	if err != nil {
		t.Fatal(err)
	}
	f.app = a.ID
	envs, err := ten.ListEnvironments(ctx, f.owner, f.app, tenancy.Page{Size: 10})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range envs {
		f.envs[e.Key] = e.ID
	}
	member := func(email, role string) auth.Principal {
		m, _, err := ten.AddMember(ctx, f.owner, "", email, role)
		if err != nil {
			t.Fatal(err)
		}
		id := auth.Identity{Kind: auth.KindUser, ID: m.UserID, UserID: m.UserID, Display: email, SecondFactor: true}.WithStepUp(0)
		p, err := authService.Resolve(ctx, id, f.org)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	f.reviewer, f.second, f.viewer = member("r1@example.com", "reviewer"), member("r2@example.com", "reviewer"), member("v@example.com", "viewer")
	return f
}

// release is a promotion of release n to the environment, hashed by content.
func (f *fixture) release(env string, n int, content string) approval.Artifact {
	return approval.Artifact{
		Resource: schema.PolicyResourceRelease, OrganizationID: f.org, AppID: f.app, Action: approval.ActionPromote,
		Environment: approval.Environment{ID: f.envs[env], Key: env, Production: env == "production"},
		Subject:     fmt.Sprintf("release:%d", n), Hash: approval.HashOf([]byte(content)), Summary: fmt.Sprintf("release %d", n),
		Requester: f.owner.Actor(),
	}
}

// require asks the engine in a transaction of the organisation.
func (f *fixture) require(t *testing.T, a approval.Artifact) error {
	t.Helper()
	return f.db.InTx(context.Background(), storage.Tenant{OrganizationID: f.org}, func(ctx context.Context, tx pgx.Tx) error {
		return f.svc.Require(ctx, tx, a)
	})
}

func wantCode(t *testing.T, err error, want plxerr.Code, what string) {
	t.Helper()
	if code, ok := plxerr.CodeOf(err); !ok || code != want {
		t.Fatalf("%s: %v, want code %d", what, err, want)
	}
}

// requestOf returns the request a refusal names.
func requestOf(t *testing.T, err error) string {
	t.Helper()
	var e *plxerr.Error
	if !errors.As(err, &e) || e.Details["request"] == "" {
		t.Fatalf("the refusal names no request: %v", err)
	}
	return e.Details["request"]
}

// decide has p decide the request.
func (f *fixture) decide(p auth.Principal, id, decision string) (approval.Request, error) {
	return f.svc.Decide(context.Background(), p, f.app, id, decision, "looks right")
}

// policyJSON is a policy document.
func (f *fixture) policyJSON(t *testing.T, key, match, require string) []byte {
	t.Helper()
	id, err := f.gen.New()
	if err != nil {
		t.Fatal(err)
	}
	return []byte(fmt.Sprintf(`{"schemaVersion":"1.0.0","kind":"policy","id":%q,"key":%q,"name":%q,"match":%s,"require":%s}`, id, key, key, match, require))
}

// Verifies: GOV-020, GOV-022.
// Nothing is required of a development environment under any default, and a
// production promotion under the standard profile opens one request, however
// often it is tried.
func TestRequire_OpensOneRequest_GOV_022(t *testing.T) {
	f := newFixture(t)
	if err := f.require(t, f.release("development", 1, "a")); err != nil {
		t.Fatalf("development: %v", err)
	}
	art := f.release("production", 1, "a")
	first := requestOf(t, f.require(t, art))
	wantCode(t, f.require(t, art), plxerr.ApprovalRequired, "second attempt")
	if again := requestOf(t, f.require(t, art)); again != first {
		t.Fatalf("a second request %s was opened beside %s", again, first)
	}
	list, err := f.svc.ListApprovals(context.Background(), f.owner, f.app, "", storage.Cursor{}, 10)
	if err != nil || len(list) != 1 || list[0].Status != approval.StatusOpen || list[0].ApprovalsNeeded != 1 {
		t.Fatalf("list = %+v, %v", list, err)
	}
	if len(f.notified) != 1 || !strings.Contains(f.notified[0], "approval.requested") {
		t.Fatalf("notifications = %v", f.notified)
	}
}

// Verifies: GOV-021, GOV-022.
// The requester can never approve; an eligible approver can, and the approval
// is spent by the change it approves.
func TestDecide_AuthorRefusedApproverAccepted_GOV_021(t *testing.T) {
	f := newFixture(t)
	art := f.release("production", 2, "b")
	id := requestOf(t, f.require(t, art))

	_, err := f.decide(f.owner, id, approval.DecisionApprove)
	wantCode(t, err, plxerr.ApproverNotEligible, "the requester")
	_, err = f.decide(f.viewer, id, approval.DecisionApprove)
	wantCode(t, err, plxerr.PermissionDenied, "a viewer")

	got, err := f.decide(f.reviewer, id, approval.DecisionApprove)
	if err != nil || got.Status != approval.StatusApproved || len(got.Decisions) != 1 {
		t.Fatalf("decide = %+v, %v", got, err)
	}
	_, err = f.decide(f.reviewer, id, approval.DecisionApprove)
	wantCode(t, err, plxerr.PreconditionFailed, "a decision on an approved request")

	if err := f.require(t, art); err != nil {
		t.Fatalf("the approved change was refused: %v", err)
	}
	wantCode(t, f.require(t, art), plxerr.ApprovalRequired, "the approval was spent")
}

// Verifies: GOV-021.
// The author of the change, named on the artifact, is refused even when
// someone else asked for it.
func TestDecide_ArtifactAuthorRefused_GOV_021(t *testing.T) {
	f := newFixture(t)
	art := f.release("production", 3, "c")
	art.Author = f.reviewer.UserID
	id := requestOf(t, f.require(t, art))
	_, err := f.decide(f.reviewer, id, approval.DecisionApprove)
	wantCode(t, err, plxerr.ApproverNotEligible, "the author")
	if _, err := f.decide(f.second, id, approval.DecisionApprove); err != nil {
		t.Fatal(err)
	}
}

// Verifies: GOV-021.
// The maximum profile asks four-eyes for a production promotion.
func TestRequire_FourEyes_GOV_021(t *testing.T) {
	f := newFixture(t)
	f.profile = settings.Maximum
	art := f.release("production", 4, "d")
	id := requestOf(t, f.require(t, art))
	if _, err := f.decide(f.reviewer, id, approval.DecisionApprove); err != nil {
		t.Fatal(err)
	}
	wantCode(t, f.require(t, art), plxerr.ApprovalRequired, "one of two approvals")
	if _, err := f.decide(f.reviewer, id, approval.DecisionApprove); err == nil {
		t.Fatal("the same person approved twice")
	}
	got, err := f.decide(f.second, id, approval.DecisionApprove)
	if err != nil || got.Status != approval.StatusApproved {
		t.Fatalf("second approval = %+v, %v", got, err)
	}
	if err := f.require(t, art); err != nil {
		t.Fatalf("two approvals were not enough: %v", err)
	}
}

// Verifies: GOV-021.
// One rejection closes the request, and the change is asked about afresh.
func TestDecide_Reject_GOV_021(t *testing.T) {
	f := newFixture(t)
	art := f.release("production", 5, "e")
	id := requestOf(t, f.require(t, art))
	got, err := f.decide(f.reviewer, id, approval.DecisionReject)
	if err != nil || got.Status != approval.StatusRejected {
		t.Fatalf("reject = %+v, %v", got, err)
	}
	_, err = f.decide(f.second, id, approval.DecisionApprove)
	wantCode(t, err, plxerr.PreconditionFailed, "a decision on a rejected request")
	if next := requestOf(t, f.require(t, art)); next == id {
		t.Fatal("the rejected request was reused")
	}
}

// Verifies: GOV-022.
// An approval does not carry over to changed content: the change is refused
// with PLX-8002 and a new request replaces the approved one.
func TestRequire_ContentChangeInvalidates_GOV_022(t *testing.T) {
	f := newFixture(t)
	original := f.release("production", 6, "content-v1")
	id := requestOf(t, f.require(t, original))
	if _, err := f.decide(f.reviewer, id, approval.DecisionApprove); err != nil {
		t.Fatal(err)
	}
	changed := f.release("production", 6, "content-v2")
	err := f.require(t, changed)
	wantCode(t, err, plxerr.ApprovalInvalidated, "changed content")
	if requestOf(t, err) == id {
		t.Fatal("the new request is the old one")
	}
	old, err := f.svc.GetApproval(context.Background(), f.owner, f.app, id)
	if err != nil || old.Status != approval.StatusSuperseded {
		t.Fatalf("the old request is %+v, %v", old, err)
	}
	wantCode(t, f.require(t, original), plxerr.ApprovalRequired, "the original content no longer has an approval")
}

// Verifies: GOV-021.
// A request expires after its policy's hours: it can no longer be decided,
// the change is refused with PLX-8004 and a new request is opened.
func TestRequire_Expiry_GOV_021(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	_, err := f.svc.PutPolicy(ctx, f.owner, f.app, f.policyJSON(t, "short", `{"resources":["release"],"production":true}`, `{"approvers":1,"expiryHours":2}`))
	if err != nil {
		t.Fatal(err)
	}
	art := f.release("production", 7, "f")
	id := requestOf(t, f.require(t, art))
	f.now = f.now.Add(3 * time.Hour)
	_, err = f.decide(f.reviewer, id, approval.DecisionApprove)
	wantCode(t, err, plxerr.ApprovalExpired, "deciding an expired request")
	next := f.require(t, art)
	wantCode(t, next, plxerr.ApprovalExpired, "the expired request")
	if requestOf(t, next) == id {
		t.Fatal("a new request was not opened")
	}
	listed, err := f.svc.ListApprovals(ctx, f.owner, f.app, approval.StatusExpired, storage.Cursor{}, 10)
	if err != nil || len(listed) != 0 {
		t.Fatalf("the expired request is retired, not listed as expired: %+v, %v", listed, err)
	}
}

// Verifies: GOV-023, SEC-103.
// Deciding needs a fresh step-up.
func TestDecide_NeedsStepUp_GOV_023(t *testing.T) {
	f := newFixture(t)
	id := requestOf(t, f.require(t, f.release("production", 8, "g")))
	stale := f.reviewer
	stale.Identity = auth.Identity{Kind: auth.KindUser, ID: f.reviewer.ID, UserID: f.reviewer.UserID, Display: "r1", SecondFactor: true}
	_, err := f.decide(stale, id, approval.DecisionApprove)
	wantCode(t, err, plxerr.StepUpRequired, "no step-up")
	old := f.reviewer
	old.Identity = old.Identity.WithStepUp(time.Hour)
	_, err = f.decide(old, id, approval.DecisionApprove)
	wantCode(t, err, plxerr.StepUpExpired, "an old step-up")
}

// Verifies: GOV-023.
// A decision is a record the audit chain covers and the audit key signs; it
// verifies offline, and a changed payload or a foreign key does not.
func TestDecide_SignedRecord_GOV_023(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	signing, err := approval.New(approval.Options{
		DB: f.db, Audit: f.log, IDs: f.gen, Now: func() time.Time { return f.now }, Signer: f.signer, SignerKey: auditKey,
	})
	if err != nil {
		t.Fatal(err)
	}
	id := requestOf(t, f.require(t, f.release("production", 9, "h")))
	if got, err := f.decide(f.reviewer, id, approval.DecisionApprove); err != nil || got.Decisions[0].Signature != nil {
		t.Fatalf("the api role signed a record: %+v, %v", got.Decisions, err)
	}
	n, err := signing.SignDecisions(ctx, f.org)
	if err != nil || n != 1 {
		t.Fatalf("SignDecisions = %d, %v", n, err)
	}
	if n, _ := signing.SignDecisions(ctx, f.org); n != 0 {
		t.Fatalf("a signed record was signed again: %d", n)
	}
	req, err := f.svc.GetApproval(ctx, f.owner, f.app, id)
	if err != nil {
		t.Fatal(err)
	}
	d := req.Decisions[0]
	keys, err := signing.PublicKeys(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := approval.VerifyRecord(d.Payload, d.Signature, d.KeyID, keys); err != nil {
		t.Fatalf("the record does not verify: %v", err)
	}
	var rec approval.Record
	if err := json.Unmarshal(d.Payload, &rec); err != nil || rec.ContentHash != req.ContentHash || rec.DeciderID != f.reviewer.UserID || rec.Decision != "approve" {
		t.Fatalf("record = %+v, %v", rec, err)
	}
	if approval.VerifyRecord([]byte(strings.Replace(string(d.Payload), "approve", "reject", 1)), d.Signature, d.KeyID, keys) == nil {
		t.Fatal("a changed payload verified")
	}
	if approval.VerifyRecord(d.Payload, d.Signature, "other-key", keys) == nil {
		t.Fatal("an unknown key verified")
	}
	f.verifyChain(t)
}

// verifyChain checks the organisation's audit chain, which holds every
// request and decision.
func (f *fixture) verifyChain(t *testing.T) {
	t.Helper()
	err := f.db.InTx(context.Background(), storage.Tenant{OrganizationID: f.org}, func(ctx context.Context, tx pgx.Tx) error {
		n, err := f.log.VerifyChain(ctx, tx, f.org)
		if err == nil && n == 0 {
			err = errors.New("the chain is empty")
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Verifies: GOV-021.
// A policy's roles and named approvers decide who may approve.
func TestDecide_RolesAndNamedApprovers_GOV_021(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.svc.PutPolicy(ctx, f.owner, f.app, f.policyJSON(t, "managers", `{"resources":["release"],"production":true}`, `{"approvers":1,"roles":["release-manager"]}`)); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.DeletePolicy(ctx, f.owner, f.app, "default-standard-production-release"); err != nil {
		t.Fatal(err)
	}
	id := requestOf(t, f.require(t, f.release("production", 10, "i")))
	_, err := f.decide(f.reviewer, id, approval.DecisionApprove)
	wantCode(t, err, plxerr.ApproverNotEligible, "a reviewer where a release manager is named")

	named := f.policyJSON(t, "managers", `{"resources":["release"],"production":true}`, fmt.Sprintf(`{"approvers":1,"users":[%q]}`, f.second.UserID))
	if _, err := f.svc.PutPolicy(ctx, f.owner, f.app, named); err != nil {
		t.Fatal(err)
	}
	_, err = f.decide(f.reviewer, id, approval.DecisionApprove)
	wantCode(t, err, plxerr.ApproverNotEligible, "a reviewer who is not the named approver")
	if _, err := f.decide(f.second, id, approval.DecisionApprove); err != nil {
		t.Fatalf("the named approver was refused: %v", err)
	}
}

// licensed is a provider with the multi-stage feature.
type licensed struct{}

func (licensed) Status() edition.Status {
	return edition.Status{State: edition.StateValid, Features: []edition.Feature{edition.FeatureMultiStageApprovals}}
}

// Verifies: GOV-021, GOV-030.
// Stages and distinct-team rules are enterprise features: without a licence
// they are refused with PLX-8600, with one they are stored.
func TestPutPolicy_EnterpriseRulesNeedALicence_GOV_021(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	staged := f.policyJSON(t, "staged", `{"resources":["release"]}`, `{"approvers":2,"roles":["security","compliance"],"stages":"sequential"}`)
	_, err := f.svc.PutPolicy(ctx, f.owner, f.app, staged)
	wantCode(t, err, plxerr.EnterpriseFeatureNotLicensed, "stages without a licence")
	teams := f.policyJSON(t, "teams", `{"resources":["release"]}`, `{"approvers":2,"distinctTeams":true}`)
	_, err = f.svc.PutPolicy(ctx, f.owner, f.app, teams)
	wantCode(t, err, plxerr.EnterpriseFeatureNotLicensed, "distinct teams without a licence")
	core := f.policyJSON(t, "core", `{"resources":["release"]}`, `{"approvers":2,"roles":["owner"],"distinctFromAuthor":true,"expiryHours":24}`)
	if _, err := f.svc.PutPolicy(ctx, f.owner, f.app, core); err != nil {
		t.Fatalf("a core policy was refused: %v", err)
	}
	ee := edition.WithProvider(ctx, licensed{})
	for _, doc := range [][]byte{staged, teams} {
		if _, err := f.svc.PutPolicy(ee, f.owner, f.app, doc); err != nil {
			t.Fatalf("with a licence: %v", err)
		}
	}
}

// Verifies: GOV-020.
// Policies are versioned, audited, exported in the Git layout, and a default
// that was withdrawn is not written again.
func TestPolicies_VersionExportAndDefaults_GOV_020(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.profile = settings.Maximum
	list, err := f.svc.ListPolicies(ctx, f.owner, f.app)
	if err != nil || len(list) != 6 {
		t.Fatalf("the maximum defaults = %d policies, %v", len(list), err)
	}
	doc := f.policyJSON(t, "mine", `{"resources":["waiver"]}`, `{"approvers":1}`)
	v1, err := f.svc.PutPolicy(ctx, f.owner, f.app, doc)
	if err != nil || v1.Version != 1 {
		t.Fatalf("first version = %+v, %v", v1, err)
	}
	v2, err := f.svc.PutPolicy(ctx, f.owner, f.app, f.policyJSON(t, "mine", `{"resources":["waiver"]}`, `{"approvers":2}`))
	if err != nil || v2.Version != 2 || v2.Hash == v1.Hash {
		t.Fatalf("second version = %+v, %v", v2, err)
	}
	if _, err := f.svc.PutPolicy(ctx, f.viewer, f.app, doc); err == nil {
		t.Fatal("a viewer wrote a policy")
	}
	if _, err := f.svc.PutPolicy(ctx, f.owner, f.app, []byte(`{"kind":"policy"}`)); err == nil {
		t.Fatal("an invalid policy was stored")
	} else {
		wantCode(t, err, plxerr.PolicyDocumentInvalid, "invalid policy")
	}
	files, err := f.svc.ExportPolicies(ctx, f.owner, f.app)
	if err != nil || len(files) != 7 || !strings.Contains(string(files["policies/mine.policy.json"]), `"approvers": 2`) {
		t.Fatalf("export = %d files, %v", len(files), err)
	}
	if err := f.svc.DeletePolicy(ctx, f.owner, f.app, "mine"); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.DeletePolicy(ctx, f.owner, f.app, "default-maximum-golden-baseline"); err != nil {
		t.Fatal(err)
	}
	if after, _ := f.svc.ListPolicies(ctx, f.owner, f.app); len(after) != 5 {
		t.Fatalf("after two deletions %d policies remain", len(after))
	}
	if _, err := f.svc.GetPolicy(ctx, f.owner, f.app, "mine"); err == nil {
		t.Fatal("a deleted policy is still served")
	}
	f.verifyChain(t)
}

// Verifies: GOV-020, GOV-022.
// A change to a policy is itself guarded by a policy that matches resource
// "policy", and is bound to the new document's hash.
func TestPutPolicy_IsGuarded_GOV_022(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	guard := f.policyJSON(t, "guard", `{"resources":["policy"]}`, `{"approvers":1}`)
	if _, err := f.svc.PutPolicy(ctx, f.owner, f.app, guard); err != nil {
		t.Fatal(err)
	}
	next := f.policyJSON(t, "other", `{"resources":["waiver"]}`, `{"approvers":1}`)
	_, err := f.svc.PutPolicy(ctx, f.owner, f.app, next)
	wantCode(t, err, plxerr.ApprovalRequired, "an unapproved policy change")
	if _, err := f.decide(f.reviewer, requestOf(t, err), approval.DecisionApprove); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.PutPolicy(ctx, f.owner, f.app, next); err != nil {
		t.Fatalf("the approved policy change was refused: %v", err)
	}
	other := f.policyJSON(t, "other", `{"resources":["waiver"]}`, `{"approvers":3}`)
	_, err = f.svc.PutPolicy(ctx, f.owner, f.app, other)
	wantCode(t, err, plxerr.ApprovalRequired, "a different document")
}

// Verifies: GOV-023.
// The review bundle carries the request, the proposed content, the install
// base and a computed risk summary.
func TestReview_GOV_023(t *testing.T) {
	f := newFixture(t)
	art := f.release("production", 11, "j")
	art.Risk = []schema.PolicyRisk{schema.PolicyRiskTouchesMoneyFlow}
	art.Payload = json.RawMessage(`{"b":2,"a":1}`)
	id := requestOf(t, f.require(t, art))
	r, err := f.svc.Review(context.Background(), f.viewer, f.app, id)
	if err != nil {
		t.Fatal(err)
	}
	if string(r.Evidence.Diff) != `{"a":1,"b":2}` || r.Request.ID != id {
		t.Fatalf("review = %+v", r)
	}
	if r.Risk.Level != "medium" && r.Risk.Level != "high" {
		t.Fatalf("risk = %+v", r.Risk)
	}
}
