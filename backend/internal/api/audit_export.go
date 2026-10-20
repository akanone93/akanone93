// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/auditexport"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

// Audit serves AuditService: the export of the audit chain to a SIEM
// (SEC-143).
type Audit struct {
	pluxv1connect.UnimplementedAuditServiceHandler
	h *Handlers
}

var _ pluxv1connect.AuditServiceHandler = Audit{}

// Audit returns the AuditService handler.
func (h *Handlers) Audit() Audit { return Audit{h: h} }

// CreateExportDestination registers a SIEM destination.
func (s Audit) CreateExportDestination(ctx context.Context, req *connect.Request[pluxv1.CreateExportDestinationRequest]) (*connect.Response[pluxv1.CreateExportDestinationResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.CreateExportDestinationResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		m := req.Msg
		d, err := s.h.AuditExport.Create(ctx, p, auditexport.NewDestination{
			Name: m.GetName(), Kind: m.GetKind(), Address: m.GetAddress(), AllowPrivate: m.GetAllowPrivate(), CAPEM: m.GetCaPem(),
			Credentials: auditexport.Credentials{
				ClientCertificatePEM: m.GetClientCertificatePem(), ClientKeyPEM: m.GetClientKeyPem(), SigningSecret: m.GetSigningSecret(),
			},
			StartAfterSequence: m.GetStartAfterSequence(),
		})
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.CreateExportDestinationResponse{Destination: exportDestinationProto(d)}, nil
	})
}

// GetExportDestination returns a destination with its status.
func (s Audit) GetExportDestination(ctx context.Context, req *connect.Request[pluxv1.GetExportDestinationRequest]) (*connect.Response[pluxv1.GetExportDestinationResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.GetExportDestinationResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		d, head, err := s.h.AuditExport.Get(ctx, p, req.Msg.GetId())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.GetExportDestinationResponse{Destination: exportDestinationProto(d), ChainHeadSequence: head}, nil
	})
}

// ListExportDestinations lists the organisation's destinations.
func (s Audit) ListExportDestinations(ctx context.Context, req *connect.Request[pluxv1.ListExportDestinationsRequest]) (*connect.Response[pluxv1.ListExportDestinationsResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.ListExportDestinationsResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		q := Query{OrderBy: "name", Filters: map[string]func(Item) string{
			"name": func(m Item) string { return m.(*pluxv1.ExportDestination).GetName() },
		}}
		page := req.Msg.GetPage()
		clauses, err := q.Parse(page)
		if err != nil {
			return nil, err
		}
		after, size, err := s.h.Pages.Request(req.Spec().Procedure, p.OrganizationID, page)
		if err != nil {
			return nil, err
		}
		destinations, err := s.h.AuditExport.List(ctx, p, after.Key, size)
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		out := &pluxv1.ListExportDestinationsResponse{Page: &pluxv1.PageResult{}}
		var last storage.Cursor
		for _, d := range destinations {
			last = storage.Cursor{Key: d.Name}
			if item := exportDestinationProto(d); q.Keep(clauses, item) {
				out.Destinations = append(out.Destinations, item)
			}
		}
		out.Page.NextPageToken = s.h.Pages.Next(req.Spec().Procedure, p.OrganizationID, page, len(destinations), size, last)
		return out, Mask(page.GetReadMask(), out.Destinations)
	})
}

// UpdateExportDestination replaces a destination's settings.
func (s Audit) UpdateExportDestination(ctx context.Context, req *connect.Request[pluxv1.UpdateExportDestinationRequest]) (*connect.Response[pluxv1.UpdateExportDestinationResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.UpdateExportDestinationResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		m := req.Msg
		d, err := s.h.AuditExport.Update(ctx, p, auditexport.Change{
			ID: m.GetId(), Address: m.GetAddress(), Enabled: m.GetEnabled(), AllowPrivate: m.GetAllowPrivate(), CAPEM: m.GetCaPem(),
			Credentials: auditexport.Credentials{
				ClientCertificatePEM: m.GetClientCertificatePem(), ClientKeyPEM: m.GetClientKeyPem(), SigningSecret: m.GetSigningSecret(),
			},
			ClearCredentials: m.GetClearCredentials(),
		})
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.UpdateExportDestinationResponse{Destination: exportDestinationProto(d)}, nil
	})
}

// DeleteExportDestination removes a destination.
func (s Audit) DeleteExportDestination(ctx context.Context, req *connect.Request[pluxv1.DeleteExportDestinationRequest]) (*connect.Response[pluxv1.DeleteExportDestinationResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.DeleteExportDestinationResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		if err := s.h.AuditExport.Delete(ctx, p, req.Msg.GetId()); err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.DeleteExportDestinationResponse{}, nil
	})
}

// exportDestinationProto converts a destination for the wire. The
// credentials are not in it.
func exportDestinationProto(d auditexport.Destination) *pluxv1.ExportDestination {
	return &pluxv1.ExportDestination{
		Id: d.ID, Name: d.Name, Kind: d.Kind, Address: d.Address, Enabled: d.Enabled, AllowPrivate: d.AllowPrivate,
		CaPem: d.CAPEM, CredentialsHint: d.CredentialsHint, CursorSequence: d.CursorSequence, CursorHash: d.CursorHash,
		ExportedCount: d.ExportedCount, Failures: d.Failures, LastError: d.LastError, LastErrorAt: ts(d.LastErrorAt),
		LastSuccessAt: ts(d.LastSuccessAt), NextAttemptAt: ts(d.NextAttemptAt), Version: d.Version,
		CreatedAt: ts(d.CreatedAt), UpdatedAt: ts(d.UpdatedAt), Warnings: d.Warnings(),
	}
}
