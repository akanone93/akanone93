// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package release

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/bundle"
	"github.com/nightCode42/plux3/backend/internal/device"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/security/settings"
	"github.com/nightCode42/plux3/backend/internal/signing"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
	"github.com/nightCode42/plux3/backend/internal/storage/objects"
)

// Confidential releases (SEC-053, ADR-0055). A release marked confidential
// is delivered with every bundle sealed under a content key drawn for that
// release alone. The compiler is untouched: the versions stay as published
// (build once, REL-004), and the worker seals a copy of each when it first
// signs a manifest for the release. The manifest names the sealed bundles
// by the hash of their ciphertext, so a device verifies before it decrypts
// (SEC-052). The content key is kept sealed under the installation's key
// (SEC-106) and wrapped, per request, to the key-agreement key of each
// device that may have it.

// Recipients says which devices may receive a content key and to which key
// (SEC-053); the device service is one.
type Recipients interface {
	// ConfidentialRecipient returns the key-agreement key to wrap to, or
	// refuses with PLX-6050.
	ConfidentialRecipient(ctx context.Context, d device.Identity) (*ecdh.PublicKey, error)
}

// CreateOption adjusts CreateRelease.
type CreateOption func(*createOptions)

type createOptions struct{ confidential bool }

// WithConfidential asks for a confidential release (SEC-053). Whether it
// may be asked for depends on the environment's confidentialBundles
// setting.
func WithConfidential() CreateOption { return func(o *createOptions) { o.confidential = true } }

// contentKeyBinding ties a sealed content key to its release, so a row
// copied to another release does not open.
func contentKeyBinding(releaseID string) []byte {
	return []byte("release-content-key:" + releaseID)
}

// isConfidential reports whether a signed bundle is delivered sealed.
func isConfidential(b SignedBundle) bool {
	return slices.Contains(b.RequiredFeatures, bundle.FeatureConfidential)
}

// confidentialSetting is the environment's confidentialBundles setting:
// "off", "optional" or "required". Without a security configuration
// service it is the standard profile's "off".
func (s *Service) confidentialSetting(ctx context.Context, env dbgen.Environment) (string, error) {
	if s.o.SecurityConfig == nil {
		return "off", nil
	}
	values, _, err := s.o.SecurityConfig.Effective(ctx, storage.ID(env.AppID), storage.ID(env.ID))
	if err != nil {
		return "", fmt.Errorf("release: %w", err)
	}
	return values.Get(settings.ConfidentialBundles).Text(), nil
}

// deliveredSealed reports whether the manifest of a release in env names
// sealed bundles: always under "required", for the releases created as
// confidential under "optional", never under "off". A release is built
// once and promoted between environments (REL-004), so the environment a
// device belongs to decides, not the one the release was created in.
func (s *Service) deliveredSealed(ctx context.Context, env dbgen.Environment, rel dbgen.Release) (bool, error) {
	setting, err := s.confidentialSetting(ctx, env)
	if err != nil {
		return false, err
	}
	return setting == "required" || (setting == "optional" && rel.Confidential), nil
}

// confidentialFor decides whether a new release in env is created as
// confidential: under confidentialBundles "required" every release is,
// under "optional" the ones asked for are, and under "off" none may be. A
// confidential release needs the means to seal its key, and none of its
// bundles may exceed confidential.bundleBytes.
func (s *Service) confidentialFor(ctx context.Context, tx pgx.Tx, orgID string, env dbgen.Environment, versions []dbgen.PluginVersion, requested bool) (bool, error) {
	setting, err := s.confidentialSetting(ctx, env)
	if err != nil {
		return false, err
	}
	switch {
	case setting == "off" && requested:
		return false, plxerr.New(plxerr.PreconditionFailed, "this environment does not allow confidential releases (confidentialBundles is off)")
	case setting == "off", setting == "optional" && !requested:
		return false, nil
	}
	if s.o.Crypter == nil {
		return false, errors.New("release: confidential releases need the installation's key encryption")
	}
	set, err := s.o.Tenancy.Effective(ctx, tx, orgID, storage.ID(env.AppID), "")
	if err != nil {
		return false, fmt.Errorf("release: %w", err)
	}
	return true, checkSealedSize(set, versions)
}

// checkSealedSize refuses a bundle larger than confidential.bundleBytes.
func checkSealedSize(set limits.Set, versions []dbgen.PluginVersion) error {
	max := set.Get(limits.ConfidentialBundleBytes)
	for _, v := range versions {
		if v.BundleSize > max {
			return plxerr.New(plxerr.LimitExceeded, "%s has %d bytes, more than %s = %d", nameOf(v), v.BundleSize, limits.ConfidentialBundleBytes, max)
		}
	}
	return nil
}

// sealedBundles returns the sealed copies of a confidential release's
// bundles by plugin key, making them, and the release's content key, the
// first time. It runs in the transaction that signs a manifest, under the
// release's row lock, so two channels pointing at one release seal it once.
func (s *Service) sealedBundles(ctx context.Context, q *dbgen.Queries, rel dbgen.Release, versions []dbgen.PluginVersion) (map[string]dbgen.ReleaseConfidentialBundle, error) {
	rows, err := q.ListReleaseConfidentialBundles(ctx, rel.ID)
	if err != nil {
		return nil, failure(err, "release")
	}
	if len(rows) < len(versions) {
		if _, err := q.LockRelease(ctx, rel.ID); err != nil {
			return nil, failure(err, "release")
		}
		if rows, err = q.ListReleaseConfidentialBundles(ctx, rel.ID); err != nil {
			return nil, failure(err, "release")
		}
	}
	if len(rows) < len(versions) {
		if rows, err = s.seal(ctx, q, rel, versions); err != nil {
			return nil, err
		}
	}
	out := make(map[string]dbgen.ReleaseConfidentialBundle, len(rows))
	for _, r := range rows {
		out[r.PluginKey] = r
	}
	return out, nil
}

// seal draws the release's content key, seals every bundle of the release
// with it and records both. The key row and the bundle rows are written
// together at the end, so a failed attempt leaves only unreferenced
// objects, and a key is never used by two attempts.
func (s *Service) seal(ctx context.Context, q *dbgen.Queries, rel dbgen.Release, versions []dbgen.PluginVersion) ([]dbgen.ReleaseConfidentialBundle, error) {
	if s.o.Crypter == nil {
		return nil, errors.New("release: confidential releases need the installation's key encryption")
	}
	if err := checkSealedSize(s.o.Limits, versions); err != nil {
		return nil, err
	}
	key := make([]byte, bundle.ContentKeySize)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("release: content key: %w", err)
	}
	defer clear(key)
	sealedKey, err := signing.Seal(ctx, s.o.Crypter, key, contentKeyBinding(storage.ID(rel.ID)))
	if err != nil {
		return nil, fmt.Errorf("release: %w", err)
	}
	ordered := slices.SortedFunc(slices.Values(versions), func(a, b dbgen.PluginVersion) int {
		switch {
		case a.PluginKey < b.PluginKey:
			return -1
		case a.PluginKey > b.PluginKey:
			return 1
		}
		return 0
	})
	nonces := &bundle.Nonces{}
	rows := make([]dbgen.ReleaseConfidentialBundle, 0, len(ordered))
	for _, v := range ordered {
		plain, err := s.load(ctx, v.BundleSha256)
		if err != nil {
			return nil, err
		}
		sealed, err := bundle.Encrypt(plain, key, nonces)
		if err != nil {
			return nil, fmt.Errorf("release: seal %s: %w", nameOf(v), err)
		}
		sum, err := bundle.HashOf(sealed)
		if err != nil {
			return nil, fmt.Errorf("release: seal %s: %w", nameOf(v), err)
		}
		if err := s.store(ctx, objects.KindBundle, sum[:], sealed, bundle.MediaType); err != nil {
			return nil, err
		}
		rows = append(rows, dbgen.ReleaseConfidentialBundle{
			ReleaseID: rel.ID, OrganizationID: rel.OrganizationID, PluginKey: v.PluginKey,
			PlainSha256: v.BundleSha256, BundleSha256: sum[:], Size: int64(len(sealed)),
		})
	}
	if err := q.InsertReleaseContentKey(ctx, dbgen.InsertReleaseContentKeyParams{
		ReleaseID: rel.ID, OrganizationID: rel.OrganizationID, Sealed: sealedKey.Encode(),
	}); err != nil {
		return nil, failure(err, "release")
	}
	for _, r := range rows {
		if err := q.InsertReleaseConfidentialBundle(ctx, dbgen.InsertReleaseConfidentialBundleParams(r)); err != nil {
			return nil, failure(err, "release")
		}
	}
	return rows, nil
}

// sealManifest makes a manifest name the sealed bundles of its release:
// their ciphertext hashes and sizes, and the feature a runtime needs to
// open them. Hashes in the signed part therefore cover ciphertext.
func (s *Service) sealManifest(ctx context.Context, q *dbgen.Queries, rel dbgen.Release, versions []dbgen.PluginVersion, doc *SignedManifest) error {
	sealed, err := s.sealedBundles(ctx, q, rel, versions)
	if err != nil {
		return err
	}
	apply := func(key string, b *SignedBundle) error {
		row, ok := sealed[key]
		if !ok {
			return fmt.Errorf("release: release %d has no sealed bundle for %q", rel.Sequence, key)
		}
		b.Hash, b.Size = hashRef(row.BundleSha256), row.Size
		b.RequiredFeatures = append(slices.Clone(b.RequiredFeatures), bundle.FeatureConfidential)
		slices.Sort(b.RequiredFeatures)
		return nil
	}
	if err := apply("", &doc.AppBundle); err != nil {
		return err
	}
	for i := range doc.Plugins {
		if err := apply(doc.Plugins[i].Key, &doc.Plugins[i].SignedBundle); err != nil {
			return err
		}
	}
	return nil
}

// contentKey wraps the content key of the release a served manifest names
// to the requesting device, or refuses with PLX-6050 when the device may
// not have it (SEC-053).
func (s *Service) contentKey(ctx context.Context, r ManifestRequest, doc SignedManifest) (*bundle.WrappedKey, error) {
	if s.o.Recipients == nil || s.o.Crypter == nil {
		return nil, plxerr.New(plxerr.ConfidentialKeyUnavailable, "this server cannot deliver confidential content keys")
	}
	recipient, err := s.o.Recipients.ConfidentialRecipient(ctx, device.Identity{
		DeviceID: r.DeviceID, OrganizationID: r.OrganizationID, AppID: r.AppID, EnvironmentID: r.EnvironmentID,
	})
	if err != nil {
		return nil, fmt.Errorf("release: %w", err)
	}
	var (
		releaseID string
		sealedKey []byte
	)
	err = s.o.DB.InTx(ctx, storage.Tenant{OrganizationID: r.OrganizationID}, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		rel, err := q.GetRelease(ctx, dbgen.GetReleaseParams{AppID: storage.MustUUID(r.AppID), Sequence: doc.ReleaseSequence})
		if err != nil {
			return failure(err, "release")
		}
		releaseID = storage.ID(rel.ID)
		if sealedKey, err = q.GetReleaseContentKey(ctx, rel.ID); err != nil {
			return failure(err, "content key")
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("release: %w", err)
	}
	sealed, err := signing.DecodeSealed(sealedKey)
	if err != nil {
		return nil, fmt.Errorf("release: %w", err)
	}
	key, err := signing.Open(ctx, s.o.Crypter, sealed, contentKeyBinding(releaseID))
	if err != nil {
		return nil, fmt.Errorf("release: %w", err)
	}
	defer clear(key)
	w, err := bundle.WrapContentKey(key, recipient, bundle.KeyInfo(r.DeviceID, doc.App, doc.Environment, doc.ReleaseSequence), nil)
	if err != nil {
		return nil, fmt.Errorf("release: %w", err)
	}
	return &w, nil
}
