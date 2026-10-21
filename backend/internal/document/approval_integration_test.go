// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package document_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/approval"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/document"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

// Verifies: A11Y-008, A11Y-002, GOV-020, GOV-021.
// Under the maximum profile a waiver the saver may add still needs the
// approval of two other people, bound to the waiver and its justification;
// a changed justification needs a new approval, and under the strict
// profile nothing is asked.
func TestWaiverNeedsApprovalUnderMaximum_A11Y_008(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	var engine *approval.Service
	f := newFixture(t, func(o *document.Options) {
		var err error
		if engine, err = approval.New(approval.Options{DB: o.DB, Audit: o.Audit, IDs: o.IDs}); err != nil {
			t.Fatal(err)
		}
		o.Approvals = engine
	})
	if _, err := f.docs.Import(ctx, f.owner, f.app, "", "s", project(t, "loan-calculator")); err != nil {
		t.Fatal(err)
	}
	plugins, err := f.docs.ListPlugins(ctx, f.owner, f.app, storage.Cursor{}, 10)
	if err != nil || len(plugins) != 1 {
		t.Fatalf("plugins: %v %v", plugins, err)
	}
	loans := plugins[0].ID
	reviewer := func(email string) auth.Principal {
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
	first, second := reviewer("r1@example.com"), reviewer("r2@example.com")
	setProfile := func(profile string) {
		t.Helper()
		app, err := f.docs.GetDocument(ctx, f.owner, f.app, "", "app.json")
		if err != nil {
			t.Fatal(err)
		}
		var doc map[string]any
		if err := json.Unmarshal(app.Content, &doc); err != nil {
			t.Fatal(err)
		}
		doc["securityProfile"] = profile
		content, err := json.Marshal(doc)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.docs.PutDocument(ctx, f.owner, f.app, "", "s", "app.json", content, app.Revision); err != nil {
			t.Fatalf("set the profile %s: %v", profile, err)
		}
	}
	save := func(waiver string) error {
		content, rev := pageWithWaiver(t, f, loans, waiver)
		_, err := f.docs.PutDocument(ctx, f.owner, f.app, loans, "s", waiverPage, content, rev)
		return err
	}
	approve := func(err error) {
		t.Helper()
		var e *plxerr.Error
		if !errors.As(err, &e) || e.Code != plxerr.ApprovalRequired {
			t.Fatalf("want an approval to be required: %v", err)
		}
		for _, p := range []auth.Principal{first, second} {
			if _, err := engine.Decide(ctx, p, f.app, e.Details["request"], approval.DecisionApprove, ""); err != nil {
				t.Fatal(err)
			}
		}
	}

	setProfile("strict")
	if err := save("Decorative picture, one."); err != nil {
		t.Fatalf("a waiver under strict asks nothing: %v", err)
	}
	setProfile("maximum")
	if err := save("Decorative picture, one."); err != nil {
		t.Fatalf("an unchanged waiver asks nothing: %v", err)
	}
	approve(save("Decorative picture, two."))
	if err := save("Decorative picture, three."); err == nil {
		t.Fatal("an approval carried over to another justification")
	}
	approve(save("Decorative picture, two."))
	if err := save("Decorative picture, two."); err != nil {
		t.Fatalf("the approved waiver: %v", err)
	}
	if err := save(""); err != nil {
		t.Fatalf("removing a waiver asks nothing: %v", err)
	}
}
