// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package document_test

import (
	"context"
	"slices"
	"sync"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/document"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// follower records the writes it is told of.
type follower struct {
	mu     sync.Mutex
	writes [][]string
}

func (f *follower) DraftWritten(_ context.Context, _ auth.Principal, _, _ string, paths []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.writes = append(f.writes, slices.Clone(paths))
}

// Verifies: STU-130, DEV-010.
// Every accepted write of a plugin's document tells the follower, which
// pushes the stored draft to the Studio dev sessions; a refused write does
// not.
func TestWritesTellTheFollower_STU_130(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	plugin := f.plugin(t)
	rec := &follower{}
	f.docs.FollowDrafts(rec)
	const session = "tab-1"
	if _, _, err := f.docs.AcquireLock(ctx, f.developer, f.app, plugin, session, false); err != nil {
		t.Fatal(err)
	}
	path := "plugins/loans/pages/home.page.json"
	if _, err := f.docs.PutDocument(ctx, f.developer, f.app, plugin, session, path, pageJSON("home", "Hello"), 0); err != nil {
		t.Fatal(err)
	}
	patch := []document.Op{{Op: "replace", Path: "/root/props/data", Value: "Patched"}}
	if _, err := f.docs.PatchDocument(ctx, f.developer, f.app, plugin, session, path, patch, 5); code(err) != plxerr.RevisionConflict {
		t.Fatalf("a stale patch: %v", err)
	}
	if _, err := f.docs.PatchDocument(ctx, f.developer, f.app, plugin, session, path, patch, 1); err != nil {
		t.Fatal(err)
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.writes) != 2 || !slices.Equal(rec.writes[1], []string{path}) {
		t.Errorf("the follower was told of %v, want two writes of %s", rec.writes, path)
	}
}
