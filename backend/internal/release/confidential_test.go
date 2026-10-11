// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package release

import (
	"context"
	"crypto/sha256"
	"strings"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/bundle"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
	"github.com/nightCode42/plux3/backend/internal/storage/objects"
)

// Verifies: SEC-053.
// A confidential bundle downloads whole, whatever the device holds: the
// plan never reaches the delta cache, which a plain bundle's plan does
// (this service has no database, so reaching it would fail).
func TestPlanNeverDeltasAConfidentialBundle(t *testing.T) {
	t.Parallel()
	store, err := objects.NewFilesystem(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	s := &Service{o: Options{Objects: store, PublicBaseURL: "https://plux.example.com"}}
	to := sha256.Sum256([]byte("sealed"))
	from := sha256.Sum256([]byte("installed"))
	target := SignedBundle{Hash: hashRef(to[:]), Size: 99, RequiredFeatures: []string{bundle.FeatureConfidential, "pxl.v1"}}

	step, full, err := s.plan(context.Background(), "org", target, from[:])
	if err != nil {
		t.Fatal(err)
	}
	if step.Action != SyncFull || step.Size != 99 || step.URL != full || !strings.Contains(full, "/v1/objects/bundles/") {
		t.Errorf("a device holding another bundle: %+v", step)
	}
	step, _, err = s.plan(context.Background(), "org", target, to[:])
	if err != nil || step.Action != SyncKeep {
		t.Errorf("a device holding the sealed bundle: %+v, %v", step, err)
	}
}

// Verifies: SEC-053, LIM-001.
func TestSealedSizeComesFromTheRegistry(t *testing.T) {
	t.Parallel()
	max := limits.Defaults().Get(limits.ConfidentialBundleBytes)
	small := dbgen.PluginVersion{PluginKey: "loans", BundleSize: max}
	big := dbgen.PluginVersion{PluginKey: "loans", BundleSize: max + 1}
	if err := checkSealedSize(limits.Defaults(), []dbgen.PluginVersion{small}); err != nil {
		t.Errorf("a bundle at the limit: %v", err)
	}
	err := checkSealedSize(limits.Defaults(), []dbgen.PluginVersion{small, big})
	if code, _ := plxerr.CodeOf(err); code != plxerr.LimitExceeded {
		t.Errorf("a bundle over the limit: %v", err)
	}
}

// Verifies: SEC-053.
func TestContentKeyBindsToItsRelease(t *testing.T) {
	t.Parallel()
	if string(contentKeyBinding("a")) == string(contentKeyBinding("b")) {
		t.Error("two releases share a binding")
	}
	if isConfidential(SignedBundle{RequiredFeatures: []string{"pxl.v1"}}) || !isConfidential(SignedBundle{RequiredFeatures: []string{bundle.FeatureConfidential}}) {
		t.Error("isConfidential does not follow the feature")
	}
}
