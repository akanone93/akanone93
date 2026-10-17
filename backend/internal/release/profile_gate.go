// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package release

import (
	"context"
	"fmt"
	"slices"

	"github.com/nightCode42/plux3/backend/internal/bundle"
	"github.com/nightCode42/plux3/backend/internal/bundle/fbs"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/security/settings"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// profileRank orders the security profiles by strictness (SEC-180).
func profileRank(p string) int {
	switch p {
	case string(settings.Maximum):
		return 2
	case string(settings.Strict):
		return 1
	}
	return 0
}

// environmentProfile is the effective security profile of an environment;
// without a security configuration service it is standard.
func (s *Service) environmentProfile(ctx context.Context, appID, environmentID string) (string, error) {
	if s.o.SecurityConfig == nil {
		return string(settings.Standard), nil
	}
	values, _, err := s.o.SecurityConfig.Effective(ctx, appID, environmentID)
	if err != nil {
		return "", fmt.Errorf("release: %w", err)
	}
	return string(values.Profile()), nil
}

// requireProfileFloor refuses with PLX-3063 a release whose profile is less
// strict than the environment's effective profile (SEC-180, A11Y-002); a
// stricter release may go into a looser environment.
func (s *Service) requireProfileFloor(ctx context.Context, appID, environmentID, releaseProfile string) error {
	envProfile, err := s.environmentProfile(ctx, appID, environmentID)
	if err != nil {
		return err
	}
	if profileRank(releaseProfile) < profileRank(envProfile) {
		if releaseProfile == "" {
			releaseProfile = string(settings.Standard)
		}
		return plxerr.New(plxerr.ReleaseProfileBelowEnvironment,
			"the release uses the %s security profile, below the environment's %s profile", releaseProfile, envProfile)
	}
	return nil
}

// appMeta reads the meta section of a release's app bundle from storage.
func (s *Service) appMeta(ctx context.Context, rel dbgen.Release, versions []dbgen.PluginVersion) (*fbs.Meta, error) {
	i := slices.IndexFunc(versions, func(v dbgen.PluginVersion) bool { return v.ID == rel.AppVersionID })
	if i < 0 {
		return nil, fmt.Errorf("release: release %d has no app bundle", rel.Sequence)
	}
	data, err := s.load(ctx, versions[i].BundleSha256)
	if err != nil {
		return nil, err
	}
	b, err := bundle.ReadStructure(data)
	if err != nil {
		return nil, fmt.Errorf("release: read the app bundle of release %d: %w", rel.Sequence, err)
	}
	for _, sec := range b.Sections {
		if sec.Kind == bundle.SectionMeta {
			return fbs.GetRootAsMeta(sec.Data, 0), nil
		}
	}
	return nil, fmt.Errorf("release: the app bundle of release %d has no meta section", rel.Sequence)
}

// gatePromotion applies the profile floor and the pseudo-locale rule to
// promoting a release into an environment (SEC-180, I18N-002).
func (s *Service) gatePromotion(ctx context.Context, appID string, env dbgen.Environment, rel dbgen.Release, versions []dbgen.PluginVersion) error {
	if s.o.SecurityConfig == nil && !env.Production {
		return nil
	}
	meta, err := s.appMeta(ctx, rel, versions)
	if err != nil {
		return err
	}
	if err := s.requireProfileFloor(ctx, appID, storage.ID(env.ID), string(meta.SecurityProfile())); err != nil {
		return err
	}
	if !env.Production {
		return nil
	}
	for i := range meta.SupportedLocalesLength() {
		if tag := string(meta.SupportedLocales(i)); tag == "en-XA" || tag == "ar-XB" {
			return plxerr.New(plxerr.PseudoLocalesInProduction,
				"the release supports the pseudo-locale %s, which is for development and cannot go into production", tag)
		}
	}
	return nil
}
