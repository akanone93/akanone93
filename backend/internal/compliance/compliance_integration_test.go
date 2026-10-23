// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package compliance_test

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/edition"
	"github.com/nightCode42/plux3/backend/ext/governance"
	"github.com/nightCode42/plux3/backend/internal/approval"
	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/breakglass"
	"github.com/nightCode42/plux3/backend/internal/compliance"
	"github.com/nightCode42/plux3/backend/internal/governancetest"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/security/settings"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

const auditKey = "audit"

// renderer is a PDF writer the test controls.
type renderer struct {
	calls   int
	report  governance.Report
	envelop governance.Envelope
}

// PDF implements governance.ReportRenderer.
func (r *renderer) PDF(_ context.Context, rep governance.Report, e governance.Envelope) ([]byte, error) {
	r.calls++
	r.report, r.envelop = rep, e
	return []byte("%PDF-test"), nil
}

// fixture is a world with the approval engine, break-glass and a period of
// seeded production changes.
type fixture struct {
	*governancetest.World
	engine *approval.Service
	bg     *breakglass.Service
	ctx    context.Context
	pub    ed25519.PublicKey
	keyID  string
}

// newFixture builds the world; the licence lists compliance reports.
func newFixture(t *testing.T) *fixture {
	t.Helper()
	w := governancetest.New(t)
	engine, err := approval.New(approval.Options{DB: w.DB, Audit: w.Log, IDs: w.Gen, Now: w.Clock})
	if err != nil {
		t.Fatal(err)
	}
	engine.Use(func(context.Context, string, string) (settings.Profile, error) { return settings.Standard, nil }, nil)
	bg, err := breakglass.New(breakglass.Options{DB: w.DB, Audit: w.Log, IDs: w.Gen, Limits: w.Limits, Now: w.Clock})
	if err != nil {
		t.Fatal(err)
	}
	pub, keyID, err := w.Signer.PublicKey(context.Background(), auditKey)
	if err != nil {
		t.Fatal(err)
	}
	return &fixture{World: w, engine: engine, bg: bg, ctx: governancetest.Licensed(context.Background(), edition.FeatureComplianceReports), pub: pub, keyID: keyID}
}

// service builds a compliance service; sign gives it the audit key, and
// maxChanges tightens governance.reportChanges when it is not zero.
func (f *fixture) service(t *testing.T, sign bool, r governance.ReportRenderer, maxChanges int64) *compliance.Service {
	t.Helper()
	o := compliance.Options{
		DB: f.DB, Audit: f.Log, IDs: f.Gen, Renderer: r, Now: f.Clock,
		Limits: func(ctx context.Context, tx pgx.Tx, org string) (limits.Set, error) {
			set, err := f.Limits(ctx, tx, org)
			if err == nil && maxChanges > 0 {
				set, err = set.Tighten(limits.GovernanceReportChanges, limits.ScopeOrganization, maxChanges)
			}
			return set, err //nolint:wrapcheck // a test
		},
	}
	if sign {
		o.Signer, o.SignerKey = f.Signer, auditKey
	}
	svc, err := compliance.New(o)
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

// promote makes an approved production promotion of release n at the
// fixture's current time, written by the reviewer and approved by the
// release manager, and returns its content hash.
func (f *fixture) promote(t *testing.T, n int) string {
	t.Helper()
	art := approval.Artifact{
		Resource: schema.PolicyResourceRelease, OrganizationID: f.Org, AppID: f.App, Action: approval.ActionPromote,
		Environment: approval.Environment{ID: f.Envs["production"], Key: "production", Production: true},
		Subject:     "release:" + strconv.Itoa(n), Hash: approval.HashOf([]byte(strconv.Itoa(n))), Summary: "release", Requester: f.Owner.Actor(), Author: f.Reviewer.UserID,
	}
	require := func() error {
		return f.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error { return f.engine.Require(ctx, tx, art) })
	}
	var e *plxerr.Error
	if err := require(); !errors.As(err, &e) || e.Code != plxerr.ApprovalRequired {
		t.Fatalf("the promotion was not refused for approval: %v", err)
	}
	if _, err := f.engine.Decide(context.Background(), f.Manager, f.App, e.Details["request"], approval.DecisionApprove, "reviewed"); err != nil {
		t.Fatal(err)
	}
	if err := require(); err != nil {
		t.Fatal(err)
	}
	return art.Hash
}

// emergency takes a break-glass kill switch at the fixture's current time.
func (f *fixture) emergency(t *testing.T) breakglass.Record {
	t.Helper()
	var rec breakglass.Record
	err := f.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		var err error
		rec, err = f.bg.Invoke(ctx, tx, f.Manager, breakglass.Invocation{
			AppID: f.App, EnvironmentID: f.Envs["production"], EnvironmentKey: "production", Production: true,
			Action: breakglass.ActionKillSwitch, Subject: "kill switch plugin loans on production", ContentHash: strings.Repeat("c3", 32), Justification: "loans crashes",
		})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return rec
}

// seed makes a period: a change in September, then in October an approved
// promotion and a break-glass action, then one in November.
func (f *fixture) seed(t *testing.T) (promotion string, emergency breakglass.Record) {
	t.Helper()
	f.Now = time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	f.promote(t, 1)
	f.Now = time.Date(2026, 10, 2, 8, 0, 0, 0, time.UTC)
	promotion = f.promote(t, 2)
	f.Now = time.Date(2026, 10, 9, 9, 30, 0, 0, time.UTC)
	emergency = f.emergency(t)
	f.Now = time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	f.promote(t, 3)
	f.Now = time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	return promotion, emergency
}

// october is the period the tests ask for.
func october() (time.Time, time.Time) {
	return time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
}

// open fetches a report's JSON and verifies it against the fixture's key.
func (f *fixture) open(t *testing.T, svc *compliance.Service, id string) (governance.Report, governance.Envelope) {
	t.Helper()
	info, content, contentType, err := svc.Get(f.ctx, f.Owner, id, compliance.FormatJSON)
	if err != nil || !info.Signed || contentType != compliance.ContentTypeJSON {
		t.Fatalf("Get: %+v %q %v", info, contentType, err)
	}
	env, err := governance.ParseEnvelope(content)
	if err != nil {
		t.Fatal(err)
	}
	report, err := env.Open(map[string]ed25519.PublicKey{f.keyID: f.pub})
	if err != nil {
		t.Fatalf("the report does not verify: %v", err)
	}
	return report, env
}

// wantCode checks the code of a refusal.
func wantCode(t *testing.T, err error, want plxerr.Code, what string) {
	t.Helper()
	if code, ok := plxerr.CodeOf(err); !ok || code != want {
		t.Fatalf("%s: %v, want code %d", what, err, want)
	}
}

// Verifies: GOV-026.
// A report of a period lists every production change in it, oldest first and
// no other, with author, approvers, timestamps, content hash and evidence, and
// a break-glass change with its justification and review.
func TestCreate_ReportContents_GOV_026(t *testing.T) {
	f := newFixture(t)
	promotion, emergency := f.seed(t)
	svc := f.service(t, true, nil, 0)
	from, to := october()
	info, err := svc.Create(f.ctx, f.Owner, from, to)
	if err != nil || !info.Signed || info.Changes != 2 || info.KeyID != f.keyID {
		t.Fatalf("Create = %+v, %v", info, err)
	}
	report, _ := f.open(t, svc, info.ID)
	if report.Version != governance.ReportVersion || report.OrganizationID != f.Org || !report.From.Equal(from) || !report.To.Equal(to) ||
		!report.GeneratedAt.Equal(f.Now) || report.GeneratedBy != f.Owner.Actor().ID || len(report.Changes) != 2 {
		t.Fatalf("report = %+v", report)
	}

	p, b := report.Changes[0], report.Changes[1]
	if p.Resource != "release" || p.Action != "promote" || p.Subject != "release:2" || p.ContentHash != promotion || p.Environment != "production" ||
		p.AppID != f.App || p.Author != f.Reviewer.UserID || p.ChangedBy != f.Owner.Actor().ID || !p.ChangedAt.Equal(time.Date(2026, 10, 2, 8, 0, 0, 0, time.UTC)) {
		t.Errorf("the promotion = %+v", p)
	}
	if len(p.Approvals) != 1 {
		t.Fatalf("approvals = %+v", p.Approvals)
	}
	a := p.Approvals[0]
	if a.DeciderID != f.Manager.UserID || a.Decision != "approve" || a.Comment != "reviewed" || a.AuditSequence == 0 || !a.DecidedAt.Equal(p.ChangedAt) {
		t.Errorf("the approval = %+v", a)
	}
	kinds := map[string]bool{}
	for _, e := range p.Evidence {
		kinds[e.Kind] = true
	}
	if !kinds["approvalRequest"] || !kinds["decisionRecord"] || !kinds["auditEntry"] || !strings.HasPrefix(p.Evidence[0].Link, "/apps/"+f.App+"/approvals/") {
		t.Errorf("evidence = %+v", p.Evidence)
	}

	if b.Resource != "breakglass" || b.Action != "killSwitch" || b.ChangedBy != f.Manager.Actor().ID || b.BreakGlass == nil || len(b.Approvals) != 0 ||
		b.BreakGlass.Justification != "loans crashes" || b.BreakGlass.ReviewStatus != "open" || !b.BreakGlass.ReviewDueAt.Equal(emergency.ReviewDueAt) ||
		b.ContentHash != strings.Repeat("c3", 32) || len(b.Evidence) != 1 || b.Evidence[0].Kind != "breakGlass" || b.Evidence[0].Ref != emergency.ID {
		t.Errorf("the emergency = %+v", b)
	}
}

// Verifies: GOV-026.
// The period is from, inclusive, to, exclusive; an empty period is a valid
// report of no changes; and the audit log records the request.
func TestCreate_PeriodBoundsAndAudit_GOV_026(t *testing.T) {
	f := newFixture(t)
	f.seed(t)
	svc := f.service(t, true, nil, 0)
	at := func(m time.Month, d, h int) time.Time { return time.Date(2026, m, d, h, 0, 0, 0, time.UTC) }
	for name, c := range map[string]struct {
		from, to time.Time
		want     int
	}{
		"exactly the first change":      {at(10, 2, 8), at(10, 2, 9), 1},
		"to is exclusive":               {at(10, 2, 7), at(10, 2, 8), 0},
		"from is inclusive":             {at(11, 1, 0), at(11, 2, 0), 1},
		"everything":                    {at(9, 1, 0), at(12, 1, 0), 4},
		"a period with nothing in it":   {at(8, 1, 0), at(9, 1, 0), 0},
		"a non-UTC instant is the same": {at(10, 1, 0).In(time.FixedZone("x", 3600)), at(11, 1, 0), 2},
	} {
		info, err := svc.Create(f.ctx, f.Owner, c.from, c.to)
		if err != nil || info.Changes != c.want {
			t.Errorf("%s: %+v, %v, want %d changes", name, info, err, c.want)
			continue
		}
		report, _ := f.open(t, svc, info.ID)
		if len(report.Changes) != c.want {
			t.Errorf("%s: the report lists %d changes, want %d", name, len(report.Changes), c.want)
		}
	}
	if rows := f.Audit(t, audit.ComplianceReportCreated); len(rows) != 6 || !strings.Contains(rows[0].Detail, "changes from") {
		t.Errorf("audit entries = %+v", rows)
	}
}

// Verifies: GOV-026.
// A period of more changes than governance.reportChanges is refused rather
// than cut short, and a malformed period is refused.
func TestCreate_RefusesWhatItCannotListWhole_GOV_026(t *testing.T) {
	f := newFixture(t)
	f.seed(t)
	svc := f.service(t, true, nil, 2)
	_, err := svc.Create(f.ctx, f.Owner, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC))
	wantCode(t, err, plxerr.LimitExceeded, "four changes under a limit of two")
	from, to := october()
	if _, err := svc.Create(f.ctx, f.Owner, from, to); err != nil {
		t.Fatalf("two changes under a limit of two: %v", err)
	}
	for name, p := range map[string][2]time.Time{"backwards": {to, from}, "empty": {from, from}, "no start": {{}, to}, "no end": {from, {}}} {
		_, err := svc.Create(f.ctx, f.Owner, p[0], p[1])
		wantCode(t, err, plxerr.InvalidFormat, name)
	}
}

// Verifies: GOV-026.
// The DSSE signature verifies, and a changed byte of the payload, of the
// signature, of the payload type, or a key nobody trusts, fails.
func TestEnvelope_VerifiesAndDetectsChange_GOV_026(t *testing.T) {
	f := newFixture(t)
	f.seed(t)
	svc := f.service(t, true, nil, 0)
	from, to := october()
	info, err := svc.Create(f.ctx, f.Owner, from, to)
	if err != nil {
		t.Fatal(err)
	}
	_, env := f.open(t, svc, info.ID)
	keys := map[string]ed25519.PublicKey{f.keyID: f.pub}

	payload, err := env.Statement()
	if err != nil {
		t.Fatal(err)
	}
	tampered := env
	changed := append([]byte(nil), payload...)
	changed[len(changed)/2] ^= 0x01
	tampered.Payload = base64.StdEncoding.EncodeToString(changed)
	if _, err := tampered.Open(keys); !errors.Is(err, governance.ErrSignature) {
		t.Errorf("a changed payload: %v, want ErrSignature", err)
	}
	sig, _ := base64.StdEncoding.DecodeString(env.Signatures[0].Sig)
	sig[0] ^= 0x01
	badSig := env
	badSig.Signatures = []governance.Signature{{KeyID: env.Signatures[0].KeyID, Sig: base64.StdEncoding.EncodeToString(sig)}}
	if _, err := badSig.Open(keys); !errors.Is(err, governance.ErrSignature) {
		t.Errorf("a changed signature: %v, want ErrSignature", err)
	}
	retyped := env
	retyped.PayloadType = "application/json"
	if _, err := retyped.Open(keys); !errors.Is(err, governance.ErrMalformed) {
		t.Errorf("another payload type: %v, want ErrMalformed", err)
	}
	if _, err := env.Open(map[string]ed25519.PublicKey{"someone-else": f.pub}); !errors.Is(err, governance.ErrSignature) {
		t.Errorf("an untrusted key id: %v, want ErrSignature", err)
	}
	other, _, _ := ed25519.GenerateKey(nil)
	if _, err := env.Open(map[string]ed25519.PublicKey{f.keyID: other}); !errors.Is(err, governance.ErrSignature) {
		t.Errorf("another key: %v, want ErrSignature", err)
	}
	var statement map[string]any
	if err := json.Unmarshal(payload, &statement); err != nil || statement["organizationId"] != f.Org {
		t.Errorf("the statement is not the report: %v", err)
	}
}

// Verifies: GOV-026.
// The API role holds no key: a report it takes is stored unsigned with no
// content, and the worker, which has the audit key, signs it later.
func TestSign_WorkerSignsWhatTheAPIStored_GOV_026(t *testing.T) {
	f := newFixture(t)
	f.seed(t)
	api, worker := f.service(t, false, nil, 0), f.service(t, true, nil, 0)
	from, to := october()
	info, err := api.Create(f.ctx, f.Owner, from, to)
	if err != nil || info.Signed {
		t.Fatalf("Create = %+v, %v", info, err)
	}
	got, content, _, err := api.Get(f.ctx, f.Owner, info.ID, compliance.FormatJSON)
	if err != nil || got.Signed || len(content) != 0 {
		t.Fatalf("an unsigned report served content: %+v %q %v", got, content, err)
	}
	if n, err := api.Sign(f.ctx, f.Org); err != nil || n != 0 {
		t.Fatalf("the api role signed %d: %v", n, err)
	}
	if n, err := worker.Sign(f.ctx, f.Org); err != nil || n != 1 {
		t.Fatalf("the worker signed %d: %v", n, err)
	}
	if n, _ := worker.Sign(f.ctx, f.Org); n != 0 {
		t.Errorf("a second run signed %d again", n)
	}
	report, _ := f.open(t, api, info.ID)
	if len(report.Changes) != 2 {
		t.Errorf("the signed report lists %d changes", len(report.Changes))
	}
}

// Verifies: GOV-026.
// The PDF is rendered from the signed report and its envelope; without a
// renderer, or before signing, there is none.
func TestGet_PDF_GOV_026(t *testing.T) {
	f := newFixture(t)
	f.seed(t)
	from, to := october()
	r := &renderer{}
	svc := f.service(t, true, r, 0)
	info, err := svc.Create(f.ctx, f.Owner, from, to)
	if err != nil {
		t.Fatal(err)
	}
	_, content, contentType, err := svc.Get(f.ctx, f.Owner, info.ID, compliance.FormatPDF)
	if err != nil || string(content) != "%PDF-test" || contentType != compliance.ContentTypePDF || r.calls != 1 {
		t.Fatalf("PDF = %q %q %v after %d calls", content, contentType, err, r.calls)
	}
	if len(r.report.Changes) != 2 || len(r.envelop.Signatures) != 1 || r.envelop.Signatures[0].KeyID != f.keyID {
		t.Errorf("the renderer was given %+v and %+v", r.report, r.envelop)
	}
	_, _, _, err = f.service(t, true, nil, 0).Get(f.ctx, f.Owner, info.ID, compliance.FormatPDF)
	wantCode(t, err, plxerr.PreconditionFailed, "no renderer")
	_, _, _, err = svc.Get(f.ctx, f.Owner, info.ID, "docx")
	wantCode(t, err, plxerr.InvalidEnumValue, "an unknown format")
}

// Verifies: GOV-026, GOV-030.
// Reports need compliance.report and the licence: a community installation, a
// lapsed licence for writing, a viewer and an unknown report are refused.
func TestLicenceAndPermission_GOV_030(t *testing.T) {
	f := newFixture(t)
	f.seed(t)
	svc := f.service(t, true, nil, 0)
	from, to := october()
	info, err := svc.Create(f.ctx, f.Owner, from, to)
	if err != nil {
		t.Fatal(err)
	}
	community := context.Background()
	_, err = svc.Create(community, f.Owner, from, to)
	wantCode(t, err, plxerr.EnterpriseFeatureNotLicensed, "create without a licence")
	_, _, _, err = svc.Get(community, f.Owner, info.ID, compliance.FormatJSON)
	wantCode(t, err, plxerr.EnterpriseFeatureNotLicensed, "get without a licence")
	_, err = svc.List(community, f.Owner, storage.Cursor{}, 10)
	wantCode(t, err, plxerr.EnterpriseFeatureNotLicensed, "list without a licence")

	expired := edition.WithProvider(context.Background(), expiredLicence{})
	_, err = svc.Create(expired, f.Owner, from, to)
	wantCode(t, err, plxerr.EnterpriseFeatureNotLicensed, "create with an expired licence")
	if _, _, _, err := svc.Get(expired, f.Owner, info.ID, compliance.FormatJSON); err != nil {
		t.Errorf("an expired licence reads: %v", err)
	}

	for name, p := range map[string]func() error{
		"create": func() error { _, err := svc.Create(f.ctx, f.Viewer, from, to); return err },
		"get":    func() error { _, _, _, err := svc.Get(f.ctx, f.Viewer, info.ID, compliance.FormatJSON); return err },
		"list":   func() error { _, err := svc.List(f.ctx, f.Viewer, storage.Cursor{}, 10); return err },
	} {
		wantCode(t, p(), plxerr.PermissionDenied, "a viewer: "+name)
	}
	_, _, _, err = svc.Get(f.ctx, f.Owner, f.NewID(t), compliance.FormatJSON)
	wantCode(t, err, plxerr.ResourceNotFound, "an unknown report")
	_, _, _, err = svc.Get(f.ctx, f.Owner, "nope", compliance.FormatJSON)
	wantCode(t, err, plxerr.InvalidFormat, "a malformed identifier")
}

// expiredLicence lists the feature but is past its grace period.
type expiredLicence struct{}

// Status returns an expired licence.
func (expiredLicence) Status() edition.Status {
	return edition.Status{State: edition.StateExpired, Features: []edition.Feature{edition.FeatureComplianceReports}}
}

// Verifies: GOV-026.
// Reports are listed newest first with their signature state.
func TestList_NewestFirst_GOV_026(t *testing.T) {
	f := newFixture(t)
	f.seed(t)
	api, worker := f.service(t, false, nil, 0), f.service(t, true, nil, 0)
	from, to := october()
	first, err := worker.Create(f.ctx, f.Owner, from, to)
	if err != nil {
		t.Fatal(err)
	}
	f.Now = f.Now.Add(time.Minute)
	second, err := api.Create(f.ctx, f.Owner, from, to)
	if err != nil {
		t.Fatal(err)
	}
	list, err := api.List(f.ctx, f.Owner, storage.Cursor{}, 10)
	if err != nil || len(list) != 2 || list[0].ID != second.ID || list[1].ID != first.ID || list[0].Signed || !list[1].Signed || list[1].KeyID != f.keyID {
		t.Fatalf("list = %+v, %v", list, err)
	}
	page, err := api.List(f.ctx, f.Owner, storage.Cursor{Time: list[0].RequestedAt, ID: list[0].ID}, 10)
	if err != nil || len(page) != 1 || page[0].ID != first.ID {
		t.Fatalf("the page after the newest = %+v, %v", page, err)
	}
}
