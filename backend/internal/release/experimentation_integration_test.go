// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package release_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/controldoc"
	"github.com/nightCode42/plux3/backend/internal/experiment"
	"github.com/nightCode42/plux3/backend/internal/release"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/schema/uuid7"
)

type experimentIDs struct{ g *uuid7.Generator }

func (i experimentIDs) New() (string, error) {
	u, err := i.g.New()
	if err != nil {
		return "", err //nolint:wrapcheck // a test
	}
	return u.String(), nil
}

// iosOnly is the one segment the test's source knows.
type iosOnly struct{}

func (iosOnly) Rule(_ context.Context, _ auth.Principal, _, key string) (controldoc.Rule, error) {
	return controldoc.Rule{Attribute: "os", Operator: controldoc.OpEq, ValueType: controldoc.ValueString, Values: []string{"ios"}}, nil
}

func (f *controlFixture) experiments(t *testing.T) *experiment.Service {
	t.Helper()
	g := experimentIDs{g: uuid7.NewGenerator(time.Now, rand.Reader)}
	svc, err := experiment.New(experiment.Options{
		DB: f.db, Audit: audit.NewLog(g, nil), IDs: g, Now: func() time.Time { return f.now },
		Limits: func(ctx context.Context, tx pgx.Tx, org, app string) (limits.Set, error) {
			return f.tenancy.Effective(ctx, tx, org, app, "")
		},
		Control: f.rel, Segments: iosOnly{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

const heroExperiment = `{
  "schemaVersion": "1.0.0", "kind": "experiment", "id": "01f0c450-6c00-7000-8000-000000000706", "key": "heroLayout",
  "scope": "subtree", "salt": "hero-1", "minimumSampleSize": 100, "layer": "home", "segment": "beta-testers",
  "primaryMetric": {"event": "purchase", "kind": "proportion"},
  "variants": [{"name": "control", "control": true, "weight": 50}, {"name": "wide", "weight": 50}]
}`

// Verifies: ABT-006, ABT-001, ABT-004, SYN-060, SEC-052.
// Every change to a flag, an experiment or the holdout makes a new version of
// the channel's control document that a device verifies from its root, the
// kill switches stay as they were, and a channel made later starts with the
// environment's flags.
func TestControlDocumentCarriesFlagsAndExperiments_ABT_006(t *testing.T) {
	t.Parallel()
	f := newControlFixture(t, true)
	ctx := context.Background()
	svc := f.experiments(t)
	f.switchPlugin(t, "loans")
	f.run(t)
	if doc := f.verified(t); doc.Version != 1 || len(doc.Flags) != 0 || len(doc.Experiments) != 0 || doc.Holdout != nil {
		t.Fatalf("before any flag: %+v", doc)
	}

	ref := experiment.FlagRef{AppID: f.app, EnvironmentID: f.prod, Key: "newCheckout"}
	if _, err := svc.CreateFlag(ctx, f.owner, ref, experiment.FlagSpec{
		Type: controldoc.FlagBool, Default: json.RawMessage("false"),
		Segments: []experiment.SegmentValue{{Segment: "beta-testers", Value: json.RawMessage("true")}},
	}); err != nil {
		t.Fatal(err)
	}
	f.run(t)
	doc := f.verified(t)
	if doc.Version != 2 || len(doc.Flags) != 1 || doc.Flags[0].Key != "newCheckout" || doc.Flags[0].Default != "false" ||
		len(doc.Flags[0].Segments) != 1 || doc.Flags[0].Segments[0].Value != "true" || doc.Flags[0].Segments[0].Rule.Attribute != "os" {
		t.Fatalf("after the flag: %+v", doc)
	}
	if len(doc.KillSwitches) != 1 || doc.KillSwitches[0].Target != "loans" {
		t.Fatalf("the kill switch was lost: %+v", doc.KillSwitches)
	}

	e, err := svc.CreateExperiment(ctx, f.owner, experiment.CreateExperimentRequest{AppID: f.app, EnvironmentID: f.prod, Document: []byte(heroExperiment)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.StartExperiment(ctx, f.owner, e.ID, experiment.StartOptions{LayerShare: 25}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetHoldout(ctx, f.owner, experiment.Holdout{AppID: f.app, EnvironmentID: f.prod, Percent: 3}); err != nil {
		t.Fatal(err)
	}
	f.run(t)
	doc = f.verified(t)
	if doc.Version != 4 || len(doc.Experiments) != 1 || doc.Holdout == nil || doc.Holdout.BasisPoints != 300 || len(doc.Flags) != 1 {
		t.Fatalf("after the experiment and the holdout: %+v", doc)
	}
	a := doc.Experiments[0]
	if a.Key != "heroLayout" || a.Salt != "hero-1" || a.Layer != "home" || a.LayerStart != 0 || a.LayerEnd != 2500 || a.Segment == nil ||
		len(a.Variants) != 2 || a.Variants[1].WeightBP != 5000 {
		t.Fatalf("allocation = %+v", a)
	}

	// A kill switch set now keeps the flags and the experiment.
	f.switchPlugin(t, "browse")
	f.run(t)
	if doc = f.verified(t); len(doc.KillSwitches) != 2 || len(doc.Flags) != 1 || len(doc.Experiments) != 1 {
		t.Fatalf("a kill switch changed the experimentation: %+v", doc)
	}

	// A channel made later starts from the environment's flags and experiments.
	if _, err := f.tenancy.CreateChannel(ctx, f.owner, f.prod, "beta"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.rel.RefreshControl(ctx, f.org); err != nil {
		t.Fatal(err)
	}
	beta, err := f.rel.GetControlDocument(ctx, release.ControlRequest{OrganizationID: f.org, EnvironmentID: f.prod, Channel: "beta"})
	if err != nil {
		t.Fatal(err)
	}
	scope := f.scope
	scope.Channel = "beta"
	betaDoc, err := f.verifier(t, 0).Control(beta.Signed, scope)
	if err != nil {
		t.Fatal(err)
	}
	if len(betaDoc.Flags) != 1 || len(betaDoc.Experiments) != 1 || betaDoc.Holdout == nil {
		t.Fatalf("the new channel's document: %+v", betaDoc)
	}

	if _, err := svc.StopExperiment(ctx, f.owner, e.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteFlag(ctx, f.owner, ref); err != nil {
		t.Fatal(err)
	}
	f.run(t)
	if doc = f.verified(t); len(doc.Flags) != 0 || len(doc.Experiments) != 0 || doc.Holdout == nil || len(doc.KillSwitches) != 2 {
		t.Fatalf("after the stop and the delete: %+v", doc)
	}
}

// Verifies: ABT-006, SRV-052.
// A change that says what the document already says writes no new version.
func TestUnchangedExperimentationWritesNothing_ABT_006(t *testing.T) {
	t.Parallel()
	f := newControlFixture(t, true)
	ctx := context.Background()
	svc := f.experiments(t)
	if _, err := svc.SetHoldout(ctx, f.owner, experiment.Holdout{AppID: f.app, EnvironmentID: f.prod, Percent: 0}); err != nil {
		t.Fatal(err)
	}
	state, err := f.rel.ControlDocument(ctx, f.viewer, f.ref)
	if err != nil {
		t.Fatal(err)
	}
	if state.Version > 1 {
		t.Fatalf("an empty holdout made version %d", state.Version)
	}
}
