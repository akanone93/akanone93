// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
	"github.com/nightCode42/plux3/backend/internal/preferences"
)

// Preferences serves PreferencesService (STU-014, STU-015, STU-058).
type Preferences struct{ h *Handlers }

var _ pluxv1connect.PreferencesServiceHandler = Preferences{}

// Preferences returns the handler.
func (h *Handlers) Preferences() Preferences { return Preferences{h: h} }

// GetPreferences returns the caller's preferences in the organisation of the call.
func (s Preferences) GetPreferences(ctx context.Context, req *connect.Request[pluxv1.GetPreferencesRequest]) (*connect.Response[pluxv1.GetPreferencesResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.GetPreferencesResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		got, err := s.h.UserPreferences.Get(ctx, p)
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.GetPreferencesResponse{Preferences: preferencesProto(got)}, nil
	})
}

// UpdatePreferences replaces the fields its mask names.
func (s Preferences) UpdatePreferences(ctx context.Context, req *connect.Request[pluxv1.UpdatePreferencesRequest]) (*connect.Response[pluxv1.UpdatePreferencesResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.UpdatePreferencesResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		got, err := s.h.UserPreferences.Update(ctx, p, preferencesFromProto(req.Msg.GetPreferences()), req.Msg.GetUpdateMask().GetPaths())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.UpdatePreferencesResponse{Preferences: preferencesProto(got)}, nil
	})
}

// GetOnboarding returns the organisation's onboarding steps.
func (s Preferences) GetOnboarding(ctx context.Context, req *connect.Request[pluxv1.GetOnboardingRequest]) (*connect.Response[pluxv1.GetOnboardingResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.GetOnboardingResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		steps, err := s.h.UserPreferences.Onboarding(ctx, p)
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		out := &pluxv1.GetOnboardingResponse{}
		for _, st := range steps {
			out.Steps = append(out.Steps, &pluxv1.OnboardingStep{Key: st.Key, Done: st.Done})
		}
		return out, nil
	})
}

func preferencesProto(p preferences.Preferences) *pluxv1.Preferences {
	out := &pluxv1.Preferences{
		Theme: p.Theme, Language: p.Language, Density: p.Density, Shortcuts: p.Shortcuts,
		FavouriteAppIds: p.FavouriteAppIDs, Pinned: p.Pinned, OnboardingDismissed: p.OnboardingDismissed, MutedNotificationTypes: p.MutedTypes,
	}
	if len(p.Preview) > 0 {
		out.Preview = make(map[string]*pluxv1.PreviewSwitches, len(p.Preview))
		for app, sw := range p.Preview {
			out.Preview[app] = &pluxv1.PreviewSwitches{
				DeviceFrame: sw.DeviceFrame, Theme: sw.Theme, BrandOverlay: sw.BrandOverlay, Locale: sw.Locale,
				TextScale: sw.TextScale, Platform: sw.Platform, MockStates: sw.MockStates,
			}
		}
	}
	return out
}

func preferencesFromProto(p *pluxv1.Preferences) preferences.Preferences {
	out := preferences.Preferences{
		Theme: p.GetTheme(), Language: p.GetLanguage(), Density: p.GetDensity(), Shortcuts: p.GetShortcuts(),
		FavouriteAppIDs: p.GetFavouriteAppIds(), Pinned: p.GetPinned(), OnboardingDismissed: p.GetOnboardingDismissed(), MutedTypes: p.GetMutedNotificationTypes(),
	}
	if len(p.GetPreview()) > 0 {
		out.Preview = make(map[string]preferences.PreviewSwitches, len(p.GetPreview()))
		for app, sw := range p.GetPreview() {
			out.Preview[app] = preferences.PreviewSwitches{
				DeviceFrame: sw.GetDeviceFrame(), Theme: sw.GetTheme(), BrandOverlay: sw.GetBrandOverlay(), Locale: sw.GetLocale(),
				TextScale: sw.GetTextScale(), Platform: sw.GetPlatform(), MockStates: sw.GetMockStates(),
			}
		}
	}
	return out
}
