// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"
	"slices"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/edition"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
)

// License serves LicenseService: the state of the installation's
// enterprise licence (GOV-030).
type License struct {
	pluxv1connect.UnimplementedLicenseServiceHandler
	h *Handlers
}

var _ pluxv1connect.LicenseServiceHandler = License{}

// License returns the LicenseService handler.
func (h *Handlers) License() License { return License{h: h} }

// GetLicenseState reports the licence state to an organisation
// administrator. The state concerns the installation, not the organisation,
// and holds nothing secret: the licence's identifier, its organisation, its
// dates and the features it names.
func (s License) GetLicenseState(ctx context.Context, req *connect.Request[pluxv1.GetLicenseStateRequest]) (*connect.Response[pluxv1.GetLicenseStateResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.GetLicenseStateResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		if err := p.Authorize(auth.OrganizationManage); err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		provider := edition.Community()
		if s.h.Edition != nil {
			provider = s.h.Edition
		}
		return licenseResponse(provider.Status()), nil
	})
}

// licenseResponse converts a licence status for the wire.
func licenseResponse(st edition.Status) *pluxv1.GetLicenseStateResponse {
	out := &pluxv1.GetLicenseStateResponse{
		State: string(st.State), LicenseId: st.LicenseID, Organization: st.Organization,
		ExpiresAt: ts(st.ExpiresAt), GraceEndsAt: ts(st.GraceEndsAt),
		DaysRemaining: int32(st.DaysRemaining), //nolint:gosec // whole days, bounded by the licence's dates
		Limits:        st.Limits, Message: st.Message,
	}
	for _, f := range st.Features {
		out.Features = append(out.Features, string(f))
	}
	slices.Sort(out.Features)
	return out
}
