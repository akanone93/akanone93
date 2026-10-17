// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
)

// Localisation serves LocalisationService (I18N-008, I18N-009).
type Localisation struct{ h *Handlers }

var _ pluxv1connect.LocalisationServiceHandler = Localisation{}

// Localisation returns the LocalisationService handler.
func (h *Handlers) Localisation() Localisation { return Localisation{h: h} }

// ExportTranslations writes the translations of a locale in a format.
func (s Localisation) ExportTranslations(ctx context.Context, req *connect.Request[pluxv1.ExportTranslationsRequest]) (*connect.Response[pluxv1.ExportTranslationsResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.ExportTranslationsResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		m := req.Msg
		out, err := s.h.Translations.Export(ctx, p, m.GetAppId(), m.GetLocale(), m.GetFormat())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.ExportTranslationsResponse{Content: out.Content, SourceLocale: out.SourceLocale}, nil
	})
}

// ImportTranslations applies a translation file to a locale.
func (s Localisation) ImportTranslations(ctx context.Context, req *connect.Request[pluxv1.ImportTranslationsRequest]) (*connect.Response[pluxv1.ImportTranslationsResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.ImportTranslationsResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		m := req.Msg
		out, err := s.h.Translations.Import(ctx, p, m.GetAppId(), m.GetLocale(), m.GetFormat(), m.GetContent(), m.GetDryRun())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.ImportTranslationsResponse{Diagnostics: diagnosticProtos(out.Diagnostics), ChangedKeys: out.ChangedKeys, Saved: out.Saved}, nil
	})
}

// GetStatus counts the keys of each locale by review state.
func (s Localisation) GetStatus(ctx context.Context, req *connect.Request[pluxv1.GetStatusRequest]) (*connect.Response[pluxv1.GetStatusResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.GetStatusResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		st, err := s.h.Translations.Status(ctx, p, req.Msg.GetAppId())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		out := &pluxv1.GetStatusResponse{SourceLocale: st.SourceLocale}
		for _, l := range st.Locales {
			out.Locales = append(out.Locales, &pluxv1.LocaleStatus{
				Locale: l.Locale, Total: int32(l.Total), New: int32(l.New), Translated: int32(l.Translated), //nolint:gosec // counts of keys
				Reviewed: int32(l.Reviewed), Approved: int32(l.Approved), Missing: int32(l.Missing), //nolint:gosec // counts of keys
			})
		}
		return out, nil
	})
}

// SetStates sets the review state of keys of a locale.
func (s Localisation) SetStates(ctx context.Context, req *connect.Request[pluxv1.SetStatesRequest]) (*connect.Response[pluxv1.SetStatesResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.SetStatesResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		m := req.Msg
		changed, err := s.h.Translations.SetStates(ctx, p, m.GetAppId(), m.GetLocale(), m.GetStates())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.SetStatesResponse{ChangedKeys: changed}, nil
	})
}
