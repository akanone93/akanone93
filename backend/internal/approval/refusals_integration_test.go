// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package approval_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/approval"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema"
	"github.com/nightCode42/plux3/backend/internal/security/settings"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

// Verifies: GOV-020.
// The engine refuses to be built without what it needs, and a nil gate
// requires nothing.
func TestNew_RefusesIncompleteOptions_GOV_020(t *testing.T) {
	if _, err := approval.New(approval.Options{}); err == nil {
		t.Error("an engine without a database was built")
	}
	f := newFixture(t)
	if _, err := approval.New(approval.Options{DB: f.db, Audit: f.log, IDs: f.gen, Signer: f.signer}); err == nil {
		t.Error("a signer without a key reference was accepted")
	}
	if err := approval.Require(context.Background(), nil, nil); err != nil {
		t.Errorf("a nil gate required %v", err)
	}
}

// Verifies: GOV-020, GOV-022.
// A policy that names an environment the app lacks is refused, and a policy
// that is missing, withdrawn or guarded stays so.
func TestPolicies_Refusals_GOV_020(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	bad := f.policyJSON(t, "ghost", `{"resources":["release"],"environments":["no-such-env"]}`, `{"approvers":1}`)
	_, err := f.svc.PutPolicy(ctx, f.owner, f.app, bad)
	wantCode(t, err, plxerr.PolicyDocumentInvalid, "an unknown environment")

	_, err = f.svc.GetPolicy(ctx, f.owner, f.app, "ghost")
	wantCode(t, err, plxerr.ResourceNotFound, "a policy never written")
	wantCode(t, f.svc.DeletePolicy(ctx, f.owner, f.app, "ghost"), plxerr.ResourceNotFound, "deleting a policy never written")
	_, err = f.svc.GetPolicy(ctx, f.owner, "not-a-uuid", "ghost")
	wantCode(t, err, plxerr.InvalidFormat, "a malformed app")

	doc := f.policyJSON(t, "mine", `{"resources":["waiver"]}`, `{"approvers":1}`)
	if _, err := f.svc.PutPolicy(ctx, f.owner, f.app, doc); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.GetPolicy(ctx, f.viewer, f.app, "mine"); err != nil {
		t.Fatalf("a viewer cannot read a policy: %v", err)
	}
	wantCode(t, f.svc.DeletePolicy(ctx, f.viewer, f.app, "mine"), plxerr.PermissionDenied, "a viewer deleting a policy")
	if err := f.svc.DeletePolicy(ctx, f.owner, f.app, "mine"); err != nil {
		t.Fatal(err)
	}
	_, err = f.svc.GetPolicy(ctx, f.owner, f.app, "mine")
	wantCode(t, err, plxerr.ResourceNotFound, "a withdrawn policy")
	wantCode(t, f.svc.DeletePolicy(ctx, f.owner, f.app, "mine"), plxerr.ResourceNotFound, "withdrawing twice")
}

// Verifies: GOV-021.
// A decision on a request that does not exist, that is malformed, that
// belongs to another scope, or that is neither approve nor reject, is refused.
func TestDecide_Refusals_GOV_021(t *testing.T) {
	f := newFixture(t)
	id := requestOf(t, f.require(t, f.release("production", 21, "x")))

	_, err := f.decide(f.reviewer, id, "abstain")
	wantCode(t, err, plxerr.InvalidEnumValue, "an unknown decision")
	_, err = f.decide(f.reviewer, "not-a-uuid", approval.DecisionApprove)
	wantCode(t, err, plxerr.InvalidFormat, "a malformed identifier")
	_, err = f.decide(f.reviewer, "01b0c450-6c00-7000-8000-0000000000aa", approval.DecisionApprove)
	wantCode(t, err, plxerr.ResourceNotFound, "an unknown request")
	_, err = f.svc.Decide(context.Background(), f.reviewer, "", id, approval.DecisionApprove, "")
	if err == nil {
		t.Error("an app's request was decided in the organisation scope")
	}
	_, err = f.svc.Decide(context.Background(), f.viewer, f.app, id, approval.DecisionReject, "")
	wantCode(t, err, plxerr.PermissionDenied, "a viewer rejecting")
}

// reviewerFunc adapts a function to a Reviewer.
type reviewerFunc func(context.Context, pgx.Tx, approval.Request) (approval.Evidence, error)

// Evidence implements approval.Reviewer.
func (r reviewerFunc) Evidence(ctx context.Context, tx pgx.Tx, req approval.Request) (approval.Evidence, error) {
	return r(ctx, tx, req)
}

// Verifies: GOV-023.
// A registered reviewer supplies the evidence, an unknown request has no
// review, and the risk grows with what the change guards.
func TestReview_Refusals_AndReviewer_GOV_023(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.svc.Register(schema.PolicyResourcePolicy, reviewerFunc(func(context.Context, pgx.Tx, approval.Request) (approval.Evidence, error) {
		return approval.Evidence{Diff: json.RawMessage(`{"from":"reviewer"}`), Notes: "from the reviewer"}, nil
	}))

	_, err := f.svc.Review(ctx, f.viewer, f.app, "01b0c450-6c00-7000-8000-0000000000aa")
	wantCode(t, err, plxerr.ResourceNotFound, "an unknown request")
	_, err = f.svc.Review(ctx, f.viewer, f.app, "not-a-uuid")
	wantCode(t, err, plxerr.InvalidFormat, "a malformed identifier")

	guard := f.policyJSON(t, "guard", `{"resources":["policy"]}`, `{"approvers":1}`)
	if _, err := f.svc.PutPolicy(ctx, f.owner, f.app, guard); err != nil {
		t.Fatal(err)
	}
	next := f.policyJSON(t, "other", `{"resources":["waiver"]}`, `{"approvers":1}`)
	_, err = f.svc.PutPolicy(ctx, f.owner, f.app, next)
	r, err := f.svc.Review(ctx, f.viewer, f.app, requestOf(t, err))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(r.Evidence.Diff), "reviewer") || r.Risk.Level != "high" {
		t.Fatalf("review = %+v", r)
	}
}

// Verifies: GOV-021.
// The list of requests is paged newest first, and an unknown request is not
// found.
func TestListApprovals_PagesNewestFirst_GOV_021(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	first := requestOf(t, f.require(t, f.release("production", 31, "a")))
	f.now = f.now.Add(time.Minute)
	second := requestOf(t, f.require(t, f.release("production", 32, "b")))

	page, err := f.svc.ListApprovals(ctx, f.owner, f.app, "", storage.Cursor{}, 1)
	if err != nil || len(page) != 1 || page[0].ID != second {
		t.Fatalf("first page = %+v, %v", page, err)
	}
	next, err := f.svc.ListApprovals(ctx, f.owner, f.app, "", storage.Cursor{Time: page[0].CreatedAt, ID: page[0].ID}, 1)
	if err != nil || len(next) != 1 || next[0].ID != first {
		t.Fatalf("second page = %+v, %v", next, err)
	}
	_, err = f.svc.GetApproval(ctx, f.owner, f.app, "01b0c450-6c00-7000-8000-0000000000aa")
	wantCode(t, err, plxerr.ResourceNotFound, "an unknown request")
}

// Verifies: GOV-023.
// Without a signer there are no public keys to verify records with.
func TestPublicKeys_WithoutSigner_GOV_023(t *testing.T) {
	f := newFixture(t)
	keys, err := f.svc.PublicKeys(context.Background())
	if err != nil || len(keys) != 0 {
		t.Fatalf("keys = %v, %v", keys, err)
	}
}

// Verifies: GOV-022.
// When the security profile cannot be resolved the change is refused rather
// than waved through, and an artifact that names its profile needs no lookup.
func TestRequire_ProfileFailure_GOV_022(t *testing.T) {
	f := newFixture(t)
	boom := errors.New("profile store down")
	f.svc.Use(func(context.Context, string, string) (settings.Profile, error) { return "", boom }, nil)
	if err := f.require(t, f.release("production", 41, "a")); !errors.Is(err, boom) {
		t.Fatalf("a profile failure was %v", err)
	}
	art := f.release("production", 42, "b")
	art.Profile = string(settings.Standard)
	wantCode(t, f.require(t, art), plxerr.ApprovalRequired, "an artifact that names its profile")
}

// Verifies: GOV-020, GOV-021.
// An organisation-wide policy is written, read, exported and withdrawn like an
// app's, only by those who may manage the organisation's policies.
func TestPolicies_OrganisationScope_GOV_020(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	doc := f.policyJSON(t, "org-wide", `{"resources":["waiver"]}`, `{"approvers":1}`)
	_, err := f.svc.PutPolicy(ctx, f.viewer, "", doc)
	wantCode(t, err, plxerr.PermissionDenied, "a viewer writing an organisation policy")
	if _, err := f.svc.PutPolicy(ctx, f.owner, "", doc); err != nil {
		t.Fatal(err)
	}
	got, err := f.svc.GetPolicy(ctx, f.owner, "", "org-wide")
	if err != nil || got.Key != "org-wide" || got.AppID != "" {
		t.Fatalf("get = %+v, %v", got, err)
	}
	files, err := f.svc.ExportPolicies(ctx, f.owner, "")
	if err != nil || files["policies/org-wide.policy.json"] == nil {
		t.Fatalf("export = %v, %v", files, err)
	}
	if err := f.svc.DeletePolicy(ctx, f.owner, "", "org-wide"); err != nil {
		t.Fatal(err)
	}
	_, err = f.svc.GetPolicy(ctx, f.owner, "", "org-wide")
	wantCode(t, err, plxerr.ResourceNotFound, "a withdrawn organisation policy")
}

// Verifies: GOV-020.
// A change that belongs to an app but to no environment takes the strictest
// profile among the app's environments, and is only ever held for approval.
func TestRequire_StrictestProfileOfApp_GOV_020(t *testing.T) {
	f := newFixture(t)
	for _, profile := range []settings.Profile{settings.Standard, settings.Strict, settings.Maximum} {
		f.profile = profile
		art := f.release("production", 51, string(profile))
		art.Environment = approval.Environment{}
		art.Resource = schema.PolicyResourcePolicy
		if err := f.require(t, art); err != nil {
			wantCode(t, err, plxerr.ApprovalRequired, "an app-wide change under "+string(profile))
		}
	}
}
