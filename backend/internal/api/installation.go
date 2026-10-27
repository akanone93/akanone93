// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// HealthResult is the answer of one readiness check.
type HealthResult struct {
	Name    string
	Healthy bool
}

// JobDepth is how many jobs wait in one queue.
type JobDepth struct {
	Queue string
	Count int64
}

// TrustedIssuer is one CI identity provider the installation trusts.
type TrustedIssuer struct {
	Issuer         string
	Audience       string
	SubjectPattern string
}

// InstallationReader supplies what InstallationService reports. The server
// wires it from the pieces it already owns, so the handler holds no
// database credentials of its own. It returns nothing secret.
type InstallationReader interface {
	Health(ctx context.Context) []HealthResult
	DatabaseBytes(ctx context.Context) (int64, error)
	Jobs(ctx context.Context) ([]JobDepth, error)
	Issuers() []TrustedIssuer
}

// Installation serves InstallationService (STU-116, P11 §4.3 #22).
type Installation struct {
	pluxv1connect.UnimplementedInstallationServiceHandler
	h *Handlers
}

var _ pluxv1connect.InstallationServiceHandler = Installation{}

// Installation returns the InstallationService handler.
func (h *Handlers) Installation() Installation { return Installation{h: h} }

// administrator refuses everyone but an installation administrator and
// returns the reader; a server that wired none reports nothing.
func (s Installation) administrator(ctx context.Context) (InstallationReader, error) {
	id, err := identity(ctx)
	if err != nil {
		return nil, err
	}
	if id.Kind != auth.KindUser || !id.InstallationAdmin {
		return nil, plxerr.New(plxerr.PermissionDenied, "only an installation administrator may read the installation's state")
	}
	return s.h.InstallationState, nil
}

// GetInstallationHealth reports each readiness check.
func (s Installation) GetInstallationHealth(ctx context.Context, _ *connect.Request[pluxv1.GetInstallationHealthRequest]) (*connect.Response[pluxv1.GetInstallationHealthResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.GetInstallationHealthResponse, error) {
		r, err := s.administrator(ctx)
		if err != nil || r == nil {
			return &pluxv1.GetInstallationHealthResponse{}, err
		}
		out := &pluxv1.GetInstallationHealthResponse{}
		for _, c := range r.Health(ctx) {
			out.Checks = append(out.Checks, &pluxv1.HealthCheck{Name: c.Name, Healthy: c.Healthy})
		}
		return out, nil
	})
}

// GetInstallationStorage reports the database's size.
func (s Installation) GetInstallationStorage(ctx context.Context, _ *connect.Request[pluxv1.GetInstallationStorageRequest]) (*connect.Response[pluxv1.GetInstallationStorageResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.GetInstallationStorageResponse, error) {
		r, err := s.administrator(ctx)
		if err != nil || r == nil {
			return &pluxv1.GetInstallationStorageResponse{}, err
		}
		size, err := r.DatabaseBytes(ctx)
		if err != nil {
			return nil, err //nolint:wrapcheck // the reader wraps its own failures
		}
		return &pluxv1.GetInstallationStorageResponse{Areas: []*pluxv1.StorageArea{{Area: "database", Bytes: size}}}, nil
	})
}

// ListInstallationJobs counts the jobs waiting in each queue.
func (s Installation) ListInstallationJobs(ctx context.Context, _ *connect.Request[pluxv1.ListInstallationJobsRequest]) (*connect.Response[pluxv1.ListInstallationJobsResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.ListInstallationJobsResponse, error) {
		r, err := s.administrator(ctx)
		if err != nil || r == nil {
			return &pluxv1.ListInstallationJobsResponse{}, err
		}
		depths, err := r.Jobs(ctx)
		if err != nil {
			return nil, err //nolint:wrapcheck // the reader wraps its own failures
		}
		out := &pluxv1.ListInstallationJobsResponse{}
		for _, d := range depths {
			out.Jobs = append(out.Jobs, &pluxv1.JobCount{Queue: d.Queue, State: "waiting", Count: d.Count})
		}
		return out, nil
	})
}

// ListTrustedIssuers lists the CI identity providers the installation trusts.
func (s Installation) ListTrustedIssuers(ctx context.Context, _ *connect.Request[pluxv1.ListTrustedIssuersRequest]) (*connect.Response[pluxv1.ListTrustedIssuersResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.ListTrustedIssuersResponse, error) {
		r, err := s.administrator(ctx)
		if err != nil || r == nil {
			return &pluxv1.ListTrustedIssuersResponse{}, err
		}
		out := &pluxv1.ListTrustedIssuersResponse{}
		for _, i := range r.Issuers() {
			out.Issuers = append(out.Issuers, &pluxv1.TrustedIssuer{Issuer: i.Issuer, Audience: i.Audience, SubjectPattern: i.SubjectPattern})
		}
		return out, nil
	})
}

// ListInstallationOrganizations lists every organisation with its member
// and app counts. It reads through the tenancy service, which refuses
// everyone but an installation administrator and records the read.
func (s Installation) ListInstallationOrganizations(ctx context.Context, _ *connect.Request[pluxv1.ListInstallationOrganizationsRequest]) (*connect.Response[pluxv1.ListInstallationOrganizationsResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.ListInstallationOrganizationsResponse, error) {
		id, err := identity(ctx)
		if err != nil {
			return nil, err
		}
		orgs, err := s.h.Tenancy.InstallationOrganizations(ctx, id)
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		out := &pluxv1.ListInstallationOrganizationsResponse{}
		for _, o := range orgs {
			out.Organizations = append(out.Organizations, &pluxv1.InstallationOrganization{
				Id: o.ID, Name: o.Name, CreatedAtMs: o.CreatedAt.UnixMilli(), MemberCount: o.MemberCount, AppCount: o.AppCount,
			})
		}
		return out, nil
	})
}
