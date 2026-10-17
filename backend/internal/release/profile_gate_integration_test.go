// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package release_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/release"
	"github.com/nightCode42/plux3/backend/internal/seccfg"
)

// setEnvProfile sets an environment's security profile.
func setEnvProfile(t *testing.T, f *fixture, env, profile string) {
	t.Helper()
	_, version, err := f.sec.Effective(context.Background(), f.app, f.envs[env].ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.sec.Set(context.Background(), f.owner, seccfg.Change{
		AppID: f.app, EnvironmentID: f.envs[env].ID, Profile: profile, Overrides: []byte(`{}`), ExpectedVersion: version,
	}); err != nil {
		t.Fatalf("Set(%s, %s): %v", env, profile, err)
	}
}

// Verifies: SEC-180, A11Y-002.
// A release compiled for a profile below the environment's is refused at
// publish and at promotion with PLX-3063; a stricter release may go into a
// looser environment.
func TestReleaseProfileFloor_SEC_180(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newFixture(t)
	dev, prod := f.envs["development"], f.envs["production"]
	setAppProfile(t, f, "strict")
	f.publish(t, "", false)
	f.publish(t, f.loans, false)
	rel, _, err := f.rel.CreateRelease(ctx, f.owner, f.app, dev.ID, nil, "")
	if err != nil {
		t.Fatal(err)
	}

	setAppProfile(t, f, "standard")
	setEnvProfile(t, f, "development", "strict")
	_, err = f.rel.Publish(ctx, f.owner, release.PublishRequest{AppID: f.app, EnvironmentID: dev.ID, Label: "l"})
	if code(err) != plxerr.ReleaseProfileBelowEnvironment {
		t.Fatalf("publish below the environment: %v", err)
	}

	setEnvProfile(t, f, "production", "maximum")
	if _, err := f.rel.PromoteRelease(ctx, f.owner, f.app, rel.Sequence, prod.ID, ""); code(err) != plxerr.ReleaseProfileBelowEnvironment {
		t.Fatalf("promote a strict release into maximum: %v", err)
	}
	setEnvProfile(t, f, "production", "standard")
	if _, err := f.rel.PromoteRelease(ctx, f.owner, f.app, rel.Sequence, prod.ID, ""); err != nil {
		t.Fatalf("promote a stricter release into a looser environment: %v", err)
	}
}

// Verifies: I18N-002.
// A release whose app bundle supports a pseudo-locale cannot be promoted
// into a production environment (PLX-3064).
func TestPseudoLocalesCannotBePromotedToProduction(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newFixture(t)
	app, err := f.docs.GetDocument(ctx, f.owner, f.app, "", "app.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(app.Content, &doc); err != nil {
		t.Fatal(err)
	}
	doc["localisation"] = map[string]any{"pseudoLocales": true}
	content, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.docs.PutDocument(ctx, f.owner, f.app, "", "s", "app.json", content, app.Revision); err != nil {
		t.Fatal(err)
	}
	f.publish(t, "", false)
	f.publish(t, f.loans, false)
	rel, _, err := f.rel.CreateRelease(ctx, f.owner, f.app, f.envs["development"].ID, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.rel.PromoteRelease(ctx, f.owner, f.app, rel.Sequence, f.envs["production"].ID, ""); code(err) != plxerr.PseudoLocalesInProduction {
		t.Fatalf("promote pseudo-locales into production: %v", err)
	}
}
