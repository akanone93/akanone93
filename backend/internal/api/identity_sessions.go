// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

// BeginStepUp starts a WebAuthn step-up in the caller's browser session, or
// for a request of the CLI's that the session performs (SEC-103). It is not
// kept for replay under an idempotency key: its answer is a challenge.
func (s Identity) BeginStepUp(ctx context.Context, req *connect.Request[pluxv1.BeginStepUpRequest]) (*connect.Response[pluxv1.BeginStepUpResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.BeginStepUpResponse, error) {
		id, err := identity(ctx)
		if err != nil {
			return nil, err
		}
		o, err := s.h.Auth.BeginStepUp(ctx, id, req.Msg.GetCliRequestId())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.BeginStepUpResponse{RequestId: o.RequestID, OptionsJson: string(o.Options), ExpiresAt: ts(o.ExpiresAt)}, nil
	})
}

// CompleteStepUp verifies the assertion that answers a step-up ceremony. An
// assertion answers once, so the call is never replayed from a kept result.
func (s Identity) CompleteStepUp(ctx context.Context, req *connect.Request[pluxv1.CompleteStepUpRequest]) (*connect.Response[pluxv1.CompleteStepUpResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.CompleteStepUpResponse, error) {
		id, err := identity(ctx)
		if err != nil {
			return nil, err
		}
		m := req.Msg
		at, err := s.h.Auth.CompleteStepUp(ctx, id, m.GetRequestId(), auth.WebAuthnAssertion{
			CredentialID: m.GetCredentialId(), ClientDataJSON: m.GetClientDataJson(),
			AuthenticatorData: m.GetAuthenticatorData(), Signature: m.GetSignature(),
		})
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.CompleteStepUpResponse{StepUpAt: ts(at)}, nil
	})
}

// ListSessions lists the sessions of the organisation's members and the
// organisation's tokens (GOV-005).
func (s Identity) ListSessions(ctx context.Context, req *connect.Request[pluxv1.ListSessionsRequest]) (*connect.Response[pluxv1.ListSessionsResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.ListSessionsResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), req.Msg.GetOrganizationId())
		if err != nil {
			return nil, err
		}
		page := req.Msg.GetPage()
		after, size, err := s.h.Pages.Request(req.Spec().Procedure, p.OrganizationID, page)
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		sessions, err := s.h.Auth.ListSessions(ctx, p, after, size)
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		out := &pluxv1.ListSessionsResponse{Page: &pluxv1.PageResult{}}
		var last storage.Cursor
		for _, v := range sessions {
			last = storage.Cursor{Time: v.CreatedAt, ID: v.ID}
			out.Sessions = append(out.Sessions, sessionInfoProto(v))
		}
		out.Page.NextPageToken = s.h.Pages.Next(req.Spec().Procedure, p.OrganizationID, page, len(sessions), size, last)
		return out, Mask(page.GetReadMask(), out.Sessions)
	})
}

// RevokeSession ends a session or revokes a token of the organisation.
func (s Identity) RevokeSession(ctx context.Context, req *connect.Request[pluxv1.RevokeSessionRequest]) (*connect.Response[pluxv1.RevokeSessionResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.RevokeSessionResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		if err := s.h.Auth.RevokeSession(ctx, p, req.Msg.GetId()); err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.RevokeSessionResponse{}, nil
	})
}

// sessionInfoProto converts a session or token for an administrator.
func sessionInfoProto(v auth.SessionInfo) *pluxv1.SessionInfo {
	return &pluxv1.SessionInfo{
		Id: v.ID, UserId: v.UserID, ClientKind: v.ClientKind, Name: v.Name,
		CreatedAt: ts(v.CreatedAt), LastSeenAt: ts(v.LastSeenAt), ExpiresAt: ts(v.ExpiresAt),
		IpPrefix: v.IPPrefix, UserAgent: v.UserAgent, StepUpAt: ts(v.StepUpAt),
	}
}
