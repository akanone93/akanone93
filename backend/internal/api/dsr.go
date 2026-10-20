// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/dsr"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

// DataSubject serves DataSubjectService: the export and erasure of the data
// of a device or a pseudonymous user (SEC-162).
type DataSubject struct {
	pluxv1connect.UnimplementedDataSubjectServiceHandler
	h *Handlers
}

var _ pluxv1connect.DataSubjectServiceHandler = DataSubject{}

// DataSubject returns the DataSubjectService handler.
func (h *Handlers) DataSubject() DataSubject { return DataSubject{h: h} }

// ExportSubject returns the archive of a subject's data. It does not go
// through the idempotency store: that would keep a copy of the personal data
// it is meant to hand over once.
func (s DataSubject) ExportSubject(ctx context.Context, req *connect.Request[pluxv1.ExportSubjectRequest]) (*connect.Response[pluxv1.ExportSubjectResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.ExportSubjectResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		subject, err := subjectOf(req.Msg.GetKind(), req.Msg.GetSubjectId())
		if err != nil {
			return nil, err
		}
		a, err := s.h.DataSubjects.Export(ctx, p, subject)
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.ExportSubjectResponse{RequestId: a.RequestID, Archive: a.JSON, RowCount: a.Rows}, nil
	})
}

// EraseSubject erases a subject's data.
func (s DataSubject) EraseSubject(ctx context.Context, req *connect.Request[pluxv1.EraseSubjectRequest]) (*connect.Response[pluxv1.EraseSubjectResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.EraseSubjectResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		subject, err := subjectOf(req.Msg.GetKind(), req.Msg.GetSubjectId())
		if err != nil {
			return nil, err
		}
		e, err := s.h.DataSubjects.Erase(ctx, p, subject)
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.EraseSubjectResponse{RequestId: e.RequestID, Erased: storeCountsProto(e.Erased), AlreadyErased: e.AlreadyErased}, nil
	})
}

// GetRequest returns one completed request.
func (s DataSubject) GetRequest(ctx context.Context, req *connect.Request[pluxv1.GetRequestRequest]) (*connect.Response[pluxv1.GetRequestResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.GetRequestResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		r, err := s.h.DataSubjects.Get(ctx, p, req.Msg.GetId())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.GetRequestResponse{Request: dsrRequestProto(r)}, nil
	})
}

// ListRequests lists the organisation's completed requests, newest first.
func (s DataSubject) ListRequests(ctx context.Context, req *connect.Request[pluxv1.ListRequestsRequest]) (*connect.Response[pluxv1.ListRequestsResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.ListRequestsResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		page := req.Msg.GetPage()
		after, size, err := s.h.Pages.Request(req.Spec().Procedure, p.OrganizationID, page)
		if err != nil {
			return nil, err
		}
		requests, err := s.h.DataSubjects.List(ctx, p, after, size)
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		out := &pluxv1.ListRequestsResponse{Page: &pluxv1.PageResult{}}
		var last storage.Cursor
		for _, r := range requests {
			last = storage.Cursor{Time: r.RequestedAt, ID: r.ID}
			out.Requests = append(out.Requests, dsrRequestProto(r))
		}
		out.Page.NextPageToken = s.h.Pages.Next(req.Spec().Procedure, p.OrganizationID, page, len(requests), size, last)
		return out, nil
	})
}

// GetPersonalDataMap returns the map of where personal data is stored.
func (s DataSubject) GetPersonalDataMap(ctx context.Context, req *connect.Request[pluxv1.GetPersonalDataMapRequest]) (*connect.Response[pluxv1.GetPersonalDataMapResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.GetPersonalDataMapResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		stores, err := s.h.DataSubjects.Map(p)
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		out := &pluxv1.GetPersonalDataMapResponse{}
		for _, st := range stores {
			out.Stores = append(out.Stores, &pluxv1.PersonalDataStore{
				Name: st.Name, Description: st.Description, DeviceKey: st.DeviceKey, UserKey: st.UserKey,
				PersonalFields: st.Personal, Retention: st.Retention, Erase: string(st.Erase), Note: st.Note,
			})
		}
		return out, nil
	})
}

// subjectOf converts the wire kind and identifier to a subject.
func subjectOf(kind pluxv1.SubjectKind, id string) (dsr.Subject, error) {
	switch kind {
	case pluxv1.SubjectKind_SUBJECT_KIND_DEVICE:
		return dsr.Subject{Kind: dsr.KindDevice, ID: id}, nil
	case pluxv1.SubjectKind_SUBJECT_KIND_USER:
		return dsr.Subject{Kind: dsr.KindUser, ID: id}, nil
	default:
		return dsr.Subject{}, plxerr.New(plxerr.InvalidFormat, "the subject kind must be device or user")
	}
}

// kindProto converts a subject kind for the wire.
func kindProto(k dsr.Kind) pluxv1.SubjectKind {
	if k == dsr.KindUser {
		return pluxv1.SubjectKind_SUBJECT_KIND_USER
	}
	return pluxv1.SubjectKind_SUBJECT_KIND_DEVICE
}

// storeCountsProto converts per-store row counts for the wire.
func storeCountsProto(counts []dsr.StoreCount) []*pluxv1.StoreCount {
	out := make([]*pluxv1.StoreCount, len(counts))
	for i, c := range counts {
		out[i] = &pluxv1.StoreCount{Store: c.Store, Rows: c.Rows}
	}
	return out
}

// dsrRequestProto converts a request for the wire.
func dsrRequestProto(r dsr.Request) *pluxv1.DataSubjectRequest {
	return &pluxv1.DataSubjectRequest{
		Id: r.ID, Action: r.Action, Kind: kindProto(r.Kind), SubjectHash: r.SubjectHash, Actor: r.Actor,
		RowCount: r.RowCount, ArchiveBytes: r.ArchiveBytes, RequestedAt: ts(r.RequestedAt), Tombstones: storeCountsProto(r.Tombstones),
	}
}
