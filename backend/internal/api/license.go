// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"
	"slices"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/edition"
	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
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

// maxLicenseBytes bounds an uploaded licence file; real ones are a few
// kilobytes.
const maxLicenseBytes = 64 << 10

// InstallLicense installs a licence file for an installation administrator
// (P11 §4.3 #23). The edition verifies it offline; an edition that cannot
// hold a licence refuses, and a refused file is never quoted back.
func (s License) InstallLicense(ctx context.Context, req *connect.Request[pluxv1.InstallLicenseRequest]) (*connect.Response[pluxv1.InstallLicenseResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.InstallLicenseResponse, error) {
		id, err := identity(ctx)
		if err != nil {
			return nil, err
		}
		if id.Kind != auth.KindUser || !id.InstallationAdmin {
			return nil, plxerr.New(plxerr.PermissionDenied, "only an installation administrator may install a licence")
		}
		file := req.Msg.GetLicense()
		if len(file) == 0 || len(file) > maxLicenseBytes {
			return nil, plxerr.New(plxerr.InvalidFormat, "a licence file is between 1 byte and %d bytes", maxLicenseBytes)
		}
		installer, ok := s.h.Edition.(edition.Installer)
		if !ok {
			return nil, plxerr.New(plxerr.PreconditionFailed, "this edition cannot hold a licence")
		}
		status, err := installer.Install(ctx, file)
		if err != nil {
			return nil, plxerr.New(plxerr.InvalidFormat, "the licence file did not verify")
		}
		if err := s.h.Tenancy.RecordInstallation(ctx, id, audit.LicenseStateChanged, "license", status.LicenseID); err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.InstallLicenseResponse{State: licenseResponse(status)}, nil
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
