// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package release_test

import (
	"context"
	"testing"
)

// Verifies: SEC-051, CLI-004.
// A new app's environment has its manifest key's public half before anything was published, so a host project
// generated for it builds and verifies the first release; asking again returns the same key, and the first release
// is signed with it.
func TestRootKeysBeforeTheFirstRelease(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	dev := f.envs["development"]
	keys, err := f.rel.RootKeys(ctx, f.org, dev.ID)
	if err != nil || len(keys) != 1 || keys[0].Role != "targets" || len(keys[0].PublicKey) == 0 {
		t.Fatalf("RootKeys before a release: %+v %v", keys, err)
	}
	again, err := f.rel.RootKeys(ctx, f.org, dev.ID)
	if err != nil || len(again) != 1 || again[0].KeyID != keys[0].KeyID {
		t.Fatalf("RootKeys asked again: %+v %v", again, err)
	}
	f.publish(t, "", false)
	f.publish(t, f.loans, false)
	if _, _, err := f.rel.CreateRelease(ctx, f.owner, f.app, dev.ID, nil, ""); err != nil {
		t.Fatal(err)
	}
	f.run(t)
	after, err := f.rel.RootKeys(ctx, f.org, dev.ID)
	if err != nil || len(after) != 1 || after[0].KeyID != keys[0].KeyID {
		t.Errorf("the first release is signed with the key the project embeds: %+v %v", after, err)
	}
}
