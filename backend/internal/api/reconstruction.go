// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"
	"errors"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/reconstruct"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

// reconstruction returns the reconstruction service, or the unimplemented
// failure of an installation without an object store.
func (s Audit) reconstruction() (*reconstruct.Service, error) {
	if s.h.Reconstruction == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("reconstruction is not available on this installation"))
	}
	return s.h.Reconstruction, nil
}

// Reconstruct makes a reconstruction release of what a device ran at a
// moment (SEC-142).
func (s Audit) Reconstruct(ctx context.Context, req *connect.Request[pluxv1.ReconstructRequest]) (*connect.Response[pluxv1.ReconstructResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.ReconstructResponse, error) {
		svc, err := s.reconstruction()
		if err != nil {
			return nil, err
		}
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		m := req.Msg
		if m.GetAt() == nil {
			return nil, plxerr.New(plxerr.InvalidFormat, "the moment to reconstruct is required")
		}
		r, err := svc.Reconstruct(ctx, p, m.GetAppId(), reconstruct.Subject{DeviceID: m.GetDeviceId(), UserRef: m.GetUserRef()}, m.GetAt().AsTime())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.ReconstructResponse{Reconstruction: reconstructionProto(r)}, nil
	})
}

// GetReconstruction opens a reconstruction release; every call is audited.
func (s Audit) GetReconstruction(ctx context.Context, req *connect.Request[pluxv1.GetReconstructionRequest]) (*connect.Response[pluxv1.GetReconstructionResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.GetReconstructionResponse, error) {
		svc, err := s.reconstruction()
		if err != nil {
			return nil, err
		}
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		o, err := svc.Get(ctx, p, req.Msg.GetId())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		out := &pluxv1.GetReconstructionResponse{Reconstruction: reconstructionProto(o.Reconstruction), Manifest: o.Manifest}
		for _, sig := range o.Signatures {
			out.Signatures = append(out.Signatures, &pluxv1.ReconstructionSignature{KeyId: sig.KeyID, Algorithm: sig.Algorithm, Signature: sig.Signature})
		}
		for _, b := range o.Bundles {
			out.Bundles = append(out.Bundles, &pluxv1.ReconstructionBundle{Key: b.Key, Sha256: b.Hash, Size: b.Size, Url: b.URL})
		}
		return out, nil
	})
}

// ListReconstructions lists an app's reconstruction releases, newest first.
func (s Audit) ListReconstructions(ctx context.Context, req *connect.Request[pluxv1.ListReconstructionsRequest]) (*connect.Response[pluxv1.ListReconstructionsResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.ListReconstructionsResponse, error) {
		svc, err := s.reconstruction()
		if err != nil {
			return nil, err
		}
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		page := req.Msg.GetPage()
		after, size, err := s.h.Pages.Request(req.Spec().Procedure, p.OrganizationID, page)
		if err != nil {
			return nil, err
		}
		rs, err := svc.List(ctx, p, req.Msg.GetAppId(), after, size)
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		out := &pluxv1.ListReconstructionsResponse{Page: &pluxv1.PageResult{}}
		var last storage.Cursor
		for _, r := range rs {
			last = storage.Cursor{Time: r.CreatedAt, ID: r.ID}
			out.Reconstructions = append(out.Reconstructions, reconstructionProto(r))
		}
		out.Page.NextPageToken = s.h.Pages.Next(req.Spec().Procedure, p.OrganizationID, page, len(rs), size, last)
		return out, nil
	})
}

// reconstructionProto converts a reconstruction release for the wire.
func reconstructionProto(r reconstruct.Reconstruction) *pluxv1.Reconstruction {
	state := "pending"
	if r.Signed {
		state = "signed"
	}
	return &pluxv1.Reconstruction{
		Id: r.ID, AppId: r.AppID, EnvironmentId: r.EnvironmentID, DeviceId: r.DeviceID, UserRef: r.UserRef,
		Moment: ts(r.Moment), ReleaseSequence: r.ReleaseSequence, State: state, CreatedAt: ts(r.CreatedAt),
		SignedAt: ts(r.SignedAt), CreatedBy: r.CreatedBy.Display, OpenUri: r.OpenURI,
	}
}
