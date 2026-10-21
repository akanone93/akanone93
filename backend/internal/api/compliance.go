// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/nightCode42/plux3/backend/internal/compliance"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

// Compliance serves ComplianceService: compliance reports of production
// changes (GOV-026, ADR-0076).
type Compliance struct{ h *Handlers }

var _ pluxv1connect.ComplianceServiceHandler = Compliance{}

// Compliance returns the ComplianceService handler.
func (h *Handlers) Compliance() Compliance { return Compliance{h: h} }

// CreateComplianceReport gathers the production changes of a period into a
// report that the worker signs.
func (s Compliance) CreateComplianceReport(ctx context.Context, req *connect.Request[pluxv1.CreateComplianceReportRequest]) (*connect.Response[pluxv1.CreateComplianceReportResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.CreateComplianceReportResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		m := req.Msg
		info, err := s.h.Reports.Create(ctx, p, instant(m.GetFrom()), instant(m.GetTo()))
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.CreateComplianceReportResponse{Report: complianceProto(info)}, nil
	})
}

// GetComplianceReport returns a report as the signed envelope or as PDF.
func (s Compliance) GetComplianceReport(ctx context.Context, req *connect.Request[pluxv1.GetComplianceReportRequest]) (*connect.Response[pluxv1.GetComplianceReportResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.GetComplianceReportResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		info, content, contentType, err := s.h.Reports.Get(ctx, p, req.Msg.GetId(), req.Msg.GetFormat())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.GetComplianceReportResponse{Report: complianceProto(info), Content: content, ContentType: contentType}, nil
	})
}

// ListComplianceReports lists the reports requested, newest first.
func (s Compliance) ListComplianceReports(ctx context.Context, req *connect.Request[pluxv1.ListComplianceReportsRequest]) (*connect.Response[pluxv1.ListComplianceReportsResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.ListComplianceReportsResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		page := req.Msg.GetPage()
		after, size, err := s.h.Pages.Request(req.Spec().Procedure, p.OrganizationID, page)
		if err != nil {
			return nil, err
		}
		infos, err := s.h.Reports.List(ctx, p, after, size)
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		out := &pluxv1.ListComplianceReportsResponse{Page: &pluxv1.PageResult{}}
		var last storage.Cursor
		for _, i := range infos {
			last = storage.Cursor{Time: i.RequestedAt, ID: i.ID}
			out.Reports = append(out.Reports, complianceProto(i))
		}
		out.Page.NextPageToken = s.h.Pages.Next(req.Spec().Procedure, p.OrganizationID, page, len(infos), size, last)
		return out, Mask(page.GetReadMask(), out.Reports)
	})
}

// instant reads a timestamp of a request; an absent one is the zero time,
// which the domain refuses.
func instant(t *timestamppb.Timestamp) time.Time {
	if t == nil {
		return time.Time{}
	}
	return t.AsTime()
}

// complianceProto converts a report's description.
func complianceProto(i compliance.Info) *pluxv1.ComplianceReportInfo {
	return &pluxv1.ComplianceReportInfo{
		Id: i.ID, PeriodFrom: ts(i.From), PeriodTo: ts(i.To), Changes: int32(i.Changes), //nolint:gosec // bounded by governance.reportChanges
		RequestedBy: i.RequestedBy, RequestedAt: ts(i.RequestedAt), Signed: i.Signed, KeyId: i.KeyID, SignedAt: ts(i.SignedAt),
	}
}
