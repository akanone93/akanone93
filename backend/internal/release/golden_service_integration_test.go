// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package release_test

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/golden"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/schema/uuid7"
)

// Verifies: TST-003, LIM-001.
// A run is compared with the approved baselines: a case that matches needs
// no image, one that differs must carry it, approving promotes only the
// cases named, and golden.baselineBytesPerPlugin bounds what is kept.
func TestGoldenBaselines_TST_003(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	gen := ids{g: uuid7.NewGenerator(time.Now, rand.Reader)}
	opts := golden.Options{DB: f.db, Audit: audit.NewLog(gen, nil), Objects: f.store, IDs: gen}
	svc := golden.New(opts)
	hash := sha256.Sum256([]byte("bundle"))
	run := func(cases ...golden.Case) golden.Run {
		return golden.Run{AppID: f.app, PluginKey: "loans", Bundle: hash[:], App: hash[:], Canonical: true, Cases: cases}
	}
	image := func(id, content string, send bool) golden.Case {
		sum := sha256.Sum256([]byte(content))
		c := golden.Case{ID: id, Frame: "small-360x640", Locale: "en", Theme: "light", TextScale: 1, SHA256: sum[:]}
		if send {
			c.Image = []byte(content)
		}
		return c
	}

	if _, _, err := svc.Upload(ctx, f.owner, run(image("home/a", "A", false))); err == nil {
		t.Fatal("a case that differs from its baseline was accepted without its image")
	}
	first, differences, err := svc.Upload(ctx, f.owner, run(image("home/a", "A", true), image("home/b", "B", true)))
	if err != nil || differences != 2 {
		t.Fatalf("Upload = %v, %d differences", err, differences)
	}
	if base, err := svc.Baselines(ctx, f.owner, f.app, "loans"); err != nil || len(base) != 0 {
		t.Fatalf("baselines before approval = %v, %v", base, err)
	}
	if _, err := svc.Approve(ctx, f.owner, f.app, first, []string{"home/nope"}); err == nil {
		t.Fatal("approved a case the run does not have")
	}
	if approved, err := svc.Approve(ctx, f.owner, f.app, first, []string{"home/a"}); err != nil || len(approved) != 1 {
		t.Fatalf("Approve = %v, %v", approved, err)
	}
	base, err := svc.Baselines(ctx, f.owner, f.app, "loans")
	sumA := sha256.Sum256([]byte("A"))
	if err != nil || len(base) != 1 || string(base["home/a"]) != string(sumA[:]) {
		t.Fatalf("baselines = %v, %v", base, err)
	}
	if other, err := svc.Baselines(ctx, f.owner, f.app, ""); err != nil || len(other) != 0 {
		t.Fatalf("another plugin's baselines = %v, %v", other, err)
	}
	diffs, err := svc.Differences(ctx, f.owner, f.app, first)
	if err != nil || len(diffs) != 1 || diffs[0].ID != "home/b" || diffs[0].Baseline != nil {
		t.Fatalf("differences = %+v, %v", diffs, err)
	}

	// The next run matches home/a without an image and changes home/b.
	second, differences, err := svc.Upload(ctx, f.owner, run(image("home/a", "A", false), image("home/b", "B2", true)))
	if err != nil || differences != 1 {
		t.Fatalf("second Upload = %v, %d differences", err, differences)
	}
	st, err := svc.StatusOf(ctx, f.owner, f.app, "loans", hash[:], hash[:])
	if err != nil || !st.Found || st.RunID != second || st.Cases != 2 || st.Differences != 1 {
		t.Fatalf("StatusOf = %+v, %v", st, err)
	}
	if st, err := svc.StatusOf(ctx, f.owner, f.app, "loans", make([]byte, 32), hash[:]); err != nil || st.Found {
		t.Fatalf("StatusOf for other bundles = %+v, %v", st, err)
	}

	// Approving the first run's other image leaves the second run's changed
	// image a difference: only an identical image would be approved with it.
	if _, err := svc.Approve(ctx, f.owner, f.app, first, nil); err != nil {
		t.Fatalf("Approve rest of first: %v", err)
	}
	if st, _ := svc.StatusOf(ctx, f.owner, f.app, "loans", hash[:], hash[:]); st.Differences != 1 {
		t.Errorf("the second run's changed image counts as approved: %+v", st)
	}
	if approved, err := svc.Approve(ctx, f.owner, f.app, second, nil); err != nil || len(approved) != 1 {
		t.Fatalf("Approve second = %v, %v", approved, err)
	}
	if st, _ := svc.StatusOf(ctx, f.owner, f.app, "loans", hash[:], hash[:]); st.Differences != 0 {
		t.Errorf("approved run still has differences: %+v", st)
	}

	// The baseline bytes of a plugin are bounded by the registry.
	small, err := limits.Defaults().Tighten(limits.GoldenBaselineBytesPerPlugin, limits.ScopeInstallation, 2)
	if err != nil {
		t.Fatal(err)
	}
	opts.Limits = small
	third, _, err := svc.Upload(ctx, f.owner, run(image("home/c", "C", true)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := golden.New(opts).Approve(ctx, f.owner, f.app, third, nil); err == nil {
		t.Error("an approval beyond golden.baselineBytesPerPlugin was accepted")
	}
	if _, _, err := svc.Upload(ctx, f.viewer, run(image("home/a", "A", false))); err == nil {
		t.Error("a viewer uploaded a golden run")
	}
}
