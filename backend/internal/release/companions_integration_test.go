// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package release_test

import (
	"bytes"
	"context"
	"slices"
	"sync"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/release"
)

// renameLoanSchedule renames the app's LoanSchedule type in the app document
// and in the loans plugin's documents that use it, in the drafts.
func renameLoanSchedule(t *testing.T, f *fixture, to string) {
	t.Helper()
	ctx := context.Background()
	edit := func(pluginID, path string) {
		t.Helper()
		doc, err := f.docs.GetDocument(ctx, f.owner, f.app, pluginID, path)
		if err != nil {
			t.Fatal(err)
		}
		changed := bytes.ReplaceAll(doc.Content, []byte(`"LoanSchedule`), []byte(`"`+to))
		if bytes.Equal(changed, doc.Content) {
			t.Fatalf("%s does not name LoanSchedule", path)
		}
		if _, err := f.docs.PutDocument(ctx, f.owner, f.app, pluginID, "s", path, changed, doc.Revision); err != nil {
			t.Fatal(err)
		}
	}
	edit("", "app.json")
	edit(f.loans, "plugins/loans/plugin.json")
	edit(f.loans, "plugins/loans/pages/result.page.json")
}

// publishParts publishes the parts of one release request as Studio's build
// step does: every part names the others as its companions, all are started
// at once, and then the worker runs them. It returns the finished jobs by
// part.
func (f *fixture) publishParts(t *testing.T, parts ...string) map[string]release.PublishJob {
	t.Helper()
	ctx := context.Background()
	jobs := make([]release.PublishJob, len(parts))
	errs := make([]error, len(parts))
	var wg sync.WaitGroup
	for i, part := range parts {
		companions := slices.DeleteFunc(slices.Clone(parts), func(p string) bool { return p == part })
		wg.Go(func() {
			jobs[i], errs[i] = f.rel.Publish(ctx, f.owner, release.PublishRequest{
				AppID: f.app, PluginID: part, EnvironmentID: f.envs["development"].ID, Label: "l", Companions: companions,
			})
		})
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("Publish %q with its companions: %v", parts[i], err)
		}
	}
	f.run(t)
	out := make(map[string]release.PublishJob, len(parts))
	for i, j := range jobs {
		got, err := f.rel.GetPublishJob(ctx, f.viewer, j.ID)
		if err != nil {
			t.Fatal(err)
		}
		out[parts[i]] = got
	}
	return out
}

// Verifies: REL-003.
// A change that spans the app and a plugin builds when its parts are
// published together (build log row 309). Alone, each part compiles the
// other at its newest version and fails: the app no longer declares the type
// the plugin's version names, and the plugin names a type the app's version
// lacks. Named as companions, each compiles the other's draft frozen with its
// own, so both build. Started at once, as Studio starts them, the parts'
// publishes keep the same snapshots without deadlocking, round after round.
func TestCompanionsBuildAgainstEachOthersDrafts_REL_003(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	for _, part := range []string{"", f.loans} {
		if j := f.publish(t, part, false); j.State != release.StateSucceeded {
			t.Fatalf("the first publish of %q: %s %v", part, j.State, j.Diagnostics)
		}
	}
	renameLoanSchedule(t, f, "AmortisationSchedule")
	if j := f.publish(t, "", false); j.State != release.StateFailed {
		t.Errorf("the app alone, against the plugin's version: %s, want failed", j.State)
	}
	if j := f.publish(t, f.loans, false); j.State != release.StateFailed {
		t.Errorf("the plugin alone, against the app's version: %s, want failed", j.State)
	}
	for round := range 5 {
		for part, j := range f.publishParts(t, "", f.loans) {
			if j.State != release.StateSucceeded {
				t.Fatalf("round %d: %q with its companion: %s %v", round, part, j.State, j.Diagnostics)
			}
		}
	}
	// The plugin's version was compiled at the app snapshot the app's own
	// version holds, so a release of the two is consistent (PLX-8050).
	if _, diags, err := f.rel.CreateRelease(context.Background(), f.owner, f.app, f.envs["development"].ID, nil, ""); err != nil || diags.HasErrors() {
		t.Fatalf("CreateRelease of the companion builds: %v %v", diags, err)
	}
}
