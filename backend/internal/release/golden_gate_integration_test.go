// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package release_test

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/golden"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/release"
	"github.com/nightCode42/plux3/backend/internal/schema/uuid7"
)

// setGoldensBlockPublish turns goldens.blockPublish on in the app document.
func setGoldensBlockPublish(t *testing.T, f *fixture) {
	t.Helper()
	ctx := context.Background()
	app, err := f.docs.GetDocument(ctx, f.owner, f.app, "", "app.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(app.Content, &doc); err != nil {
		t.Fatal(err)
	}
	doc["goldens"] = map[string]any{"blockPublish": true}
	content, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.docs.PutDocument(ctx, f.owner, f.app, "", "s", "app.json", content, app.Revision); err != nil {
		t.Fatal(err)
	}
}

var bundleHashes = regexp.MustCompile(`bundle ([0-9a-f]{64}) with app bundle ([0-9a-f]{64})`)

// hashesOf reads the bundle hashes a PLX-3061 diagnostic names.
func hashesOf(t *testing.T, job release.PublishJob) (bundle, app []byte) {
	t.Helper()
	for _, d := range job.Diagnostics {
		if d.Code != plxerr.GoldenRunMissing {
			continue
		}
		m := bundleHashes.FindStringSubmatch(d.Message)
		if m == nil {
			t.Fatalf("PLX-3061 names no hashes: %q", d.Message)
		}
		bundle, _ = hex.DecodeString(m[1])
		app, _ = hex.DecodeString(m[2])
		return bundle, app
	}
	t.Fatalf("no PLX-3061 in %+v", job.Diagnostics)
	return nil, nil
}

func goldenRun(plugin string, bundle, app []byte, canonical bool, images ...string) golden.Run {
	run := golden.Run{AppID: "", PluginKey: plugin, Bundle: bundle, App: app, Canonical: canonical, Renderer: "linux/amd64"}
	for i, img := range images {
		sum := sha256.Sum256([]byte(img))
		run.Cases = append(run.Cases, golden.Case{
			ID: "home/small-360x640/en/light/x1/" + string(rune('a'+i)), Frame: "small-360x640", Locale: "en", Theme: "light", TextScale: 1,
			SHA256: sum[:], Image: []byte(img),
		})
	}
	return run
}

// Verifies: TST-003.
// With goldens.blockPublish a publish needs a canonical golden run for the
// exact bundle hashes (PLX-3061), and one whose differences are approved
// (PLX-3062); a run that is not canonical never counts, and without the
// setting nothing is checked.
func TestGoldensBlockPublish_TST_003(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	svc := golden.New(golden.Options{
		DB: f.db, Audit: audit.NewLog(ids{g: uuid7.NewGenerator(time.Now, rand.Reader)}, nil), Objects: f.store,
		IDs: ids{g: uuid7.NewGenerator(time.Now, rand.Reader)},
	})
	upload := func(run golden.Run) string {
		t.Helper()
		run.AppID = f.app
		id, _, err := svc.Upload(ctx, f.owner, run)
		if err != nil {
			t.Fatalf("Upload: %v", err)
		}
		return id
	}

	setGoldensBlockPublish(t, f)

	// The app bundle: no run is PLX-3061, a non-canonical run does not help.
	job := f.publish(t, "", false)
	if job.State != release.StateFailed || !hasCode(job.Diagnostics, plxerr.GoldenRunMissing) {
		t.Fatalf("no run: %+v", job)
	}
	bundle, app := hashesOf(t, job)
	upload(goldenRun("", bundle, app, false, "a"))
	if job := f.publish(t, "", false); !hasCode(job.Diagnostics, plxerr.GoldenRunMissing) {
		t.Fatalf("a non-canonical run was accepted: %+v", job.Diagnostics)
	}

	// A canonical run with a difference: PLX-3062 until it is approved.
	runID := upload(goldenRun("", bundle, app, true, "a", "b"))
	job = f.publish(t, "", false)
	if job.State != release.StateFailed || !hasCode(job.Diagnostics, plxerr.GoldenDifferenceUnapproved) || hasCode(job.Diagnostics, plxerr.GoldenRunMissing) {
		t.Fatalf("unapproved differences: %+v", job)
	}
	if _, err := svc.Approve(ctx, f.viewer, f.app, runID, nil); err == nil {
		t.Fatal("a viewer approved golden images")
	}
	if approved, err := svc.Approve(ctx, f.owner, f.app, runID, nil); err != nil || len(approved) != 2 {
		t.Fatalf("Approve = %v, %v", approved, err)
	}
	if job := f.publish(t, "", false); job.State != release.StateSucceeded {
		t.Fatalf("approved: %+v", job.Diagnostics)
	}

	// A plugin has its own run, keyed by its key and both hashes.
	job = f.publish(t, f.loans, false)
	if !hasCode(job.Diagnostics, plxerr.GoldenRunMissing) {
		t.Fatalf("plugin without a run: %+v", job)
	}
	pluginBundle, pluginApp := hashesOf(t, job)
	upload(goldenRun("loans", pluginBundle, pluginApp, true, "c"))
	if job := f.publish(t, f.loans, false); !hasCode(job.Diagnostics, plxerr.GoldenDifferenceUnapproved) {
		t.Fatalf("plugin differences: %+v", job.Diagnostics)
	}
}
