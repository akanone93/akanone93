// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package experiment_test

import (
	"context"
	"errors"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/approval"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/experiment"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/security/settings"
)

const (
	secureCheckout = `{"key":"checkout","security":{"secure":true},"root":{"type":"Variant","experiment":"hero","control":{"type":"Text"}}}`
	secureOther    = `{"key":"pay","security":{"secure":true},"root":{"type":"Variant","experiment":"hero2","control":{"type":"Text"}}}`
	plainHome      = `{"key":"home","root":{"type":"Variant","experiment":"plain","control":{"type":"Text"}}}`
)

// approver is a person holding the permission to decide approvals.
func approver(t *testing.T, w *world, x tenant) auth.Principal {
	t.Helper()
	id := w.id()
	return auth.Principal{
		Identity:       auth.Identity{Kind: auth.KindUser, ID: id, UserID: id, Display: id, SecondFactor: true}.WithStepUp(0),
		OrganizationID: x.org,
		Permissions:    []auth.Permission{auth.ApprovalsDecide},
	}
}

// Verifies: ABT-007, GOV-020, GOV-021, GOV-022.
// Under the maximum profile an experiment shown on a page marked secure needs
// the approval of two people before it starts, and another experiment cannot
// borrow that approval; an experiment on a plain page starts without one.
func TestEngineGateGuardsSecureExperiment_ABT_007(t *testing.T) {
	t.Parallel()
	var engine *approval.Service
	w := newWorld(t, func(o *experiment.Options) {
		var err error
		if engine, err = approval.New(approval.Options{DB: o.DB, Audit: o.Audit, IDs: o.IDs}); err != nil {
			t.Fatal(err)
		}
		engine.Use(func(context.Context, string, string) (settings.Profile, error) { return settings.Maximum, nil }, nil)
		o.Approval = experiment.EngineGate{Engine: engine}
		o.Documents = &fakeDocuments{files: map[string][]byte{
			"plugins/shop/pages/checkout.page.json": []byte(secureCheckout),
			"plugins/shop/pages/home.page.json":     []byte(plainHome),
			"plugins/shop/pages/pay.page.json":      []byte(secureOther),
		}}
	})
	ctx := context.Background()
	first, second := approver(t, w, w.a), approver(t, w, w.a)
	secure := w.create(w.a, w.a.dev, newDoc(t, "hero", "", nil))
	plain := w.create(w.a, w.a.dev, newDoc(t, "plain", "", nil))

	_, err := w.start(secure.ID, 0)
	var refusal *plxerr.Error
	if !errors.As(err, &refusal) || refusal.Code != plxerr.ApprovalRequired {
		t.Fatalf("a secure experiment without an approval: %v", err)
	}
	request := refusal.Details["request"]
	if w.control.writes != 0 {
		t.Errorf("the refused start wrote %d control documents", w.control.writes)
	}
	if _, err := engine.Decide(ctx, first, w.a.app, request, approval.DecisionApprove, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := w.start(secure.ID, 0); !errors.As(err, &refusal) || refusal.Code != plxerr.ApprovalRequired {
		t.Fatalf("one approval of two: %v", err)
	}
	if _, err := engine.Decide(ctx, second, w.a.app, request, approval.DecisionApprove, ""); err != nil {
		t.Fatal(err)
	}
	started, err := w.start(secure.ID, 0)
	if err != nil || started.State != experiment.StateRunning {
		t.Fatalf("the approved start: %+v, %v", started, err)
	}
	other := w.create(w.a, w.a.dev, newDoc(t, "hero2", "", nil))
	if _, err := w.start(other.ID, 0); !errors.As(err, &refusal) || refusal.Code != plxerr.ApprovalRequired || refusal.Details["request"] == request {
		t.Fatalf("another experiment borrowed the approval: %v", err)
	}

	if _, err := w.start(plain.ID, 0); err != nil {
		t.Errorf("an experiment on a plain page asks for no approval: %v", err)
	}
}

// Verifies: ABT-007.
// The default policies of the standard profile ask nothing of an experiment,
// even one shown on a secure page.
func TestEngineGateStandardAsksNothing_ABT_007(t *testing.T) {
	t.Parallel()
	w := newWorld(t, func(o *experiment.Options) {
		engine, err := approval.New(approval.Options{DB: o.DB, Audit: o.Audit, IDs: o.IDs})
		if err != nil {
			t.Fatal(err)
		}
		engine.Use(func(context.Context, string, string) (settings.Profile, error) { return settings.Standard, nil }, nil)
		o.Approval = experiment.EngineGate{Engine: engine}
		o.Documents = &fakeDocuments{files: map[string][]byte{"plugins/shop/pages/checkout.page.json": []byte(secureCheckout)}}
	})
	e := w.create(w.a, w.a.prod, newDoc(t, "hero", "", nil))
	if _, err := w.start(e.ID, 0); err != nil {
		t.Errorf("the standard profile asks nothing of an experiment: %v", err)
	}
}
