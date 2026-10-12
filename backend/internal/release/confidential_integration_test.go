// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package release_test

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/hex"
	"slices"
	"strings"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/bundle"
	"github.com/nightCode42/plux3/backend/internal/device"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/release"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/seccfg"
	"github.com/nightCode42/plux3/backend/internal/signing"
	"github.com/nightCode42/plux3/backend/internal/storage/objects"
)

// recipients is a device registry of one: it hands out key, or refuses.
type recipients struct {
	key *ecdh.PublicKey
	err error
}

func (r *recipients) ConfidentialRecipient(context.Context, device.Identity) (*ecdh.PublicKey, error) {
	return r.key, r.err
}

// Verifies: SEC-053, SEC-052, REL-032.
// In an environment that requires confidential bundles, the signed manifest
// names the ciphertext of every bundle and the feature that opens it, the
// plan is a full download whatever the device holds, and the content key
// arrives wrapped for the device and opens the stored ciphertext; a device
// that may not have the key is refused.
func TestConfidentialRelease(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	device1, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	reg := &recipients{key: device1.PublicKey()}
	f := newFixtureWith(t, "loan-calculator", limits.Set{}, func(o *release.Options) {
		o.Crypter = o.Signer.(signing.Crypter)
		o.Recipients = reg
	})
	prod := f.envs["production"]
	if _, err := f.sec.Set(ctx, f.owner, seccfg.Change{
		AppID: f.app, EnvironmentID: prod.ID, Profile: "strict", Overrides: []byte(`{"confidentialBundles":"required"}`),
	}); err != nil {
		t.Fatal(err)
	}
	f.publish(t, "", false)
	f.publish(t, f.loans, false)
	rel, _, err := f.rel.CreateRelease(ctx, f.owner, f.app, f.envs["development"].ID, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.rel.PromoteRelease(ctx, f.owner, f.app, rel.Sequence, prod.ID, ""); err != nil {
		t.Fatal(err)
	}
	f.run(t)

	req := release.ManifestRequest{OrganizationID: f.org, AppID: f.app, EnvironmentID: prod.ID, DeviceID: "device-1"}
	m, err := f.rel.GetManifest(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	doc := m.Document
	if doc.AppBundle.Hash == "sha256:"+rel.AppBundleSHA256 {
		t.Fatal("the manifest names the plain app bundle")
	}
	for name, b := range map[string]release.SignedBundle{"app": doc.AppBundle, "loans": doc.Plugins[0].SignedBundle} {
		if !slices.Contains(b.RequiredFeatures, bundle.FeatureConfidential) {
			t.Errorf("%s does not require %s: %v", name, bundle.FeatureConfidential, b.RequiredFeatures)
		}
	}
	if m.ContentKey == nil {
		t.Fatal("no content key")
	}
	// The ciphertext is stored under the hash the manifest names, and the
	// wrapped key opens it to the plain bundle that was published.
	info := bundle.KeyInfo(req.DeviceID, doc.App, doc.Environment, doc.ReleaseSequence)
	key, err := bundle.UnwrapContentKey(*m.ContentKey, device1, info)
	if err != nil {
		t.Fatal(err)
	}
	name, _ := objects.Key(objects.KindBundle, strings.TrimPrefix(doc.AppBundle.Hash, "sha256:"))
	sealed, _, err := f.store.Get(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	if hash, err := bundle.HashOf(sealed); err != nil || "sha256:"+hex.EncodeToString(hash[:]) != doc.AppBundle.Hash {
		t.Errorf("the stored bundle is not the one the manifest names: %v", err)
	}
	plainName, _ := objects.Key(objects.KindBundle, rel.AppBundleSHA256)
	plain, _, err := f.store.Get(ctx, plainName)
	if err != nil {
		t.Fatal(err)
	}
	if opened, err := bundle.Decrypt(sealed, key); err != nil || !bytes.Equal(opened, plain) {
		t.Errorf("the content key does not open the app bundle: %v", err)
	}
	// Wrapped for another device ID, the key does not open.
	if _, err := bundle.UnwrapContentKey(*m.ContentKey, device1, bundle.KeyInfo("device-2", doc.App, doc.Environment, doc.ReleaseSequence)); err == nil {
		t.Error("the key opens for another device ID")
	}

	// No delta, even for a device that holds the plain bundle.
	req.Installed = map[string][]byte{"": mustHex(t, rel.AppBundleSHA256)}
	m, err = f.rel.GetManifest(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if step := m.Plan[""]; step.Action != release.SyncFull {
		t.Errorf("a confidential bundle's plan: %+v", step)
	}

	// Signing again seals nothing again: the same ciphertext is named.
	f.run(t)
	if again, err := f.rel.GetManifest(ctx, req); err != nil || again.Document.AppBundle.Hash != doc.AppBundle.Hash {
		t.Errorf("a second manifest names another ciphertext: %v", err)
	}

	// A device the registry refuses gets no key and no manifest.
	reg.key, reg.err = nil, plxerr.New(plxerr.ConfidentialKeyUnavailable, "below the level")
	if _, err := f.rel.GetManifest(ctx, req); code(err) != plxerr.ConfidentialKeyUnavailable {
		t.Errorf("an ineligible device: %v", err)
	}
}

// Verifies: SEC-053.
// An environment that does not ask for confidential bundles serves plain
// ones, and refuses to create a confidential release.
func TestConfidentialIsOffByDefault(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newFixtureWith(t, "loan-calculator", limits.Set{}, func(o *release.Options) {
		o.Crypter = o.Signer.(signing.Crypter)
	})
	f.publish(t, "", false)
	f.publish(t, f.loans, false)
	dev := f.envs["development"].ID
	if _, _, err := f.rel.CreateRelease(ctx, f.owner, f.app, dev, nil, "", release.WithConfidential()); code(err) != plxerr.PreconditionFailed {
		t.Errorf("a confidential release under off: %v", err)
	}
	rel, _, err := f.rel.CreateRelease(ctx, f.owner, f.app, dev, nil, "")
	if err != nil || rel.Confidential {
		t.Fatalf("a plain release: %+v, %v", rel, err)
	}
	if _, err := f.rel.PromoteRelease(ctx, f.owner, f.app, rel.Sequence, f.envs["production"].ID, ""); err != nil {
		t.Fatal(err)
	}
	f.run(t)
	m, err := f.rel.GetManifest(ctx, release.ManifestRequest{OrganizationID: f.org, AppID: f.app, EnvironmentID: f.envs["production"].ID})
	if err != nil || m.ContentKey != nil || m.Document.AppBundle.Hash != "sha256:"+rel.AppBundleSHA256 {
		t.Errorf("a plain manifest: %+v, %v", m.Document.AppBundle, err)
	}
}
