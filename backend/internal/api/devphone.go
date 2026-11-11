// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"
	"errors"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/devsession"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/tenancy"
)

// DevPhone serves DevPhoneService: connecting a phone to the Plux Dev app
// and its joining its person's Studio sessions (DEV-002, ADR-0104). The
// handlers hold no logic: auth signs phones in, devsession joins them.
type DevPhone struct {
	pluxv1connect.UnimplementedDevPhoneServiceHandler
	h *Handlers
}

var _ pluxv1connect.DevPhoneServiceHandler = DevPhone{}

// DevPhone returns the handler.
func (h *Handlers) DevPhone() DevPhone { return DevPhone{h: h} }

// phoneAppsMax bounds the apps a phone lists.
const phoneAppsMax = 200

func (s DevPhone) sessions() (*devsession.Service, error) {
	if s.h.DevSessions == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("this server serves no dev sessions"))
	}
	return s.h.DevSessions, nil
}

// phone returns the calling phone's principal and credential.
func (s DevPhone) phone(ctx context.Context, req connect.AnyRequest) (auth.Principal, PhoneCaller, error) {
	caller, ok := PhoneFrom(ctx)
	if !ok {
		return auth.Principal{}, PhoneCaller{}, plxerr.New(plxerr.AuthenticationRequired, "this call needs a phone token and a DPoP proof")
	}
	p, err := s.h.principal(ctx, req.Header(), "")
	return p, caller, err
}

// proofKey returns the key of a proof-only call.
func proofKeyOf(ctx context.Context) (string, error) {
	proof, ok := ProofFrom(ctx)
	if !ok || proof.JKT == "" {
		return "", plxerr.New(plxerr.AuthenticationRequired, "this call needs a DPoP proof")
	}
	return proof.JKT, nil
}

// CreatePhoneSignIn starts connecting a phone, after a step-up, and returns the QR code's link once.
func (s DevPhone) CreatePhoneSignIn(ctx context.Context, req *connect.Request[pluxv1.CreatePhoneSignInRequest]) (*connect.Response[pluxv1.CreatePhoneSignInResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.CreatePhoneSignInResponse, error) {
		svc, err := s.sessions()
		if err != nil {
			return nil, err
		}
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		grant, err := s.h.Auth.CreatePhoneSignIn(ctx, p)
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.CreatePhoneSignInResponse{SignInId: grant.ID, Link: svc.SignInLink(grant.Grant), ExpiresAt: ts(grant.ExpiresAt)}, nil
	})
}

// GetPhoneSignIn reports one of the caller's sign-ins, with the number while a phone has claimed it.
func (s DevPhone) GetPhoneSignIn(ctx context.Context, req *connect.Request[pluxv1.GetPhoneSignInRequest]) (*connect.Response[pluxv1.GetPhoneSignInResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.GetPhoneSignInResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		st, err := s.h.Auth.GetPhoneSignIn(ctx, p, req.Msg.GetSignInId())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.GetPhoneSignInResponse{
			State: phoneSignInState(st.State), Number: uint32(st.Number), //nolint:gosec // two digits
			PhoneName: st.PhoneName, Platform: st.Platform, ExpiresAt: ts(st.ExpiresAt),
		}, nil
	})
}

// CancelPhoneSignIn ends one of the caller's sign-ins that is not finished.
func (s DevPhone) CancelPhoneSignIn(ctx context.Context, req *connect.Request[pluxv1.CancelPhoneSignInRequest]) (*connect.Response[pluxv1.CancelPhoneSignInResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.CancelPhoneSignInResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		if err := s.h.Auth.CancelPhoneSignIn(ctx, p, req.Msg.GetSignInId()); err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.CancelPhoneSignInResponse{}, nil
	})
}

// ListPhones lists the caller's signed-in phones.
func (s DevPhone) ListPhones(ctx context.Context, req *connect.Request[pluxv1.ListPhonesRequest]) (*connect.Response[pluxv1.ListPhonesResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.ListPhonesResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		phones, err := s.h.Auth.ListPhones(ctx, p)
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		out := &pluxv1.ListPhonesResponse{}
		for _, ph := range phones {
			out.Phones = append(out.Phones, &pluxv1.DevPhone{
				Id: ph.ID, Name: ph.Name, Platform: ph.Platform, SignedInAt: ts(ph.SignedInAt),
				LastUsedAt: ts(ph.LastUsedAt), LapsesAt: ts(ph.LapsesAt),
			})
		}
		return out, nil
	})
}

// RevokePhone signs out one of the caller's phones.
func (s DevPhone) RevokePhone(ctx context.Context, req *connect.Request[pluxv1.RevokePhoneRequest]) (*connect.Response[pluxv1.RevokePhoneResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.RevokePhoneResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		if err := s.h.Auth.RevokePhone(ctx, p, req.Msg.GetPhoneId()); err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.RevokePhoneResponse{}, nil
	})
}

// GetJoinRequest reports a phone's request to join a session of the caller's, with its number.
func (s DevPhone) GetJoinRequest(ctx context.Context, req *connect.Request[pluxv1.GetJoinRequestRequest]) (*connect.Response[pluxv1.GetJoinRequestResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.GetJoinRequestResponse, error) {
		svc, err := s.sessions()
		if err != nil {
			return nil, err
		}
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		st, err := svc.GetJoinRequest(ctx, p, req.Msg.GetRequestId())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.GetJoinRequestResponse{
			SessionId: st.SessionID, State: st.State, Number: uint32(st.Number), //nolint:gosec // two digits
			PhoneName: st.PhoneName, ExpiresAt: ts(st.ExpiresAt),
		}, nil
	})
}

// DenyJoinRequest refuses a phone's request to join a session of the caller's.
func (s DevPhone) DenyJoinRequest(ctx context.Context, req *connect.Request[pluxv1.DenyJoinRequestRequest]) (*connect.Response[pluxv1.DenyJoinRequestResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.DenyJoinRequestResponse, error) {
		svc, err := s.sessions()
		if err != nil {
			return nil, err
		}
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		if err := svc.DenyJoinRequest(ctx, p, req.Msg.GetRequestId()); err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.DenyJoinRequestResponse{}, nil
	})
}

// AllowJoinRetry lets a phone that picked a wrong number ask to join again.
func (s DevPhone) AllowJoinRetry(ctx context.Context, req *connect.Request[pluxv1.AllowJoinRetryRequest]) (*connect.Response[pluxv1.AllowJoinRetryResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.AllowJoinRetryResponse, error) {
		svc, err := s.sessions()
		if err != nil {
			return nil, err
		}
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		if err := svc.AllowJoinRetry(ctx, p, req.Msg.GetRequestId()); err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.AllowJoinRetryResponse{}, nil
	})
}

// ClaimPhoneSignIn is a phone's scan of the QR code, with a DPoP proof of its key.
func (s DevPhone) ClaimPhoneSignIn(ctx context.Context, req *connect.Request[pluxv1.ClaimPhoneSignInRequest]) (*connect.Response[pluxv1.ClaimPhoneSignInResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.ClaimPhoneSignInResponse, error) {
		jkt, err := proofKeyOf(ctx)
		if err != nil {
			return nil, err
		}
		m := req.Msg
		claim, err := s.h.Auth.ClaimPhoneSignIn(ctx, m.GetGrant(), jkt, m.GetName(), m.GetPlatform())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.ClaimPhoneSignInResponse{
			Options: numbers(claim.Options), ExpiresAt: ts(claim.ExpiresAt),
			UserDisplay: claim.UserDisplay, OrganizationName: claim.OrganizationName,
		}, nil
	})
}

// CompletePhoneSignIn is the phone's pick; the right number returns its token once.
func (s DevPhone) CompletePhoneSignIn(ctx context.Context, req *connect.Request[pluxv1.CompletePhoneSignInRequest]) (*connect.Response[pluxv1.CompletePhoneSignInResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.CompletePhoneSignInResponse, error) {
		jkt, err := proofKeyOf(ctx)
		if err != nil {
			return nil, err
		}
		token, err := s.h.Auth.CompletePhoneSignIn(ctx, req.Msg.GetGrant(), jkt, int(req.Msg.GetNumber()))
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.CompletePhoneSignInResponse{
			Token: token.Secret, PhoneId: token.PhoneID, OrganizationId: token.OrganizationID,
			UserDisplay: token.UserDisplay, OrganizationName: token.OrganizationName,
		}, nil
	})
}

// ListPhoneApps lists the apps a signed-in phone's person may open, and which are open in Studio.
func (s DevPhone) ListPhoneApps(ctx context.Context, req *connect.Request[pluxv1.ListPhoneAppsRequest]) (*connect.Response[pluxv1.ListPhoneAppsResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.ListPhoneAppsResponse, error) {
		svc, err := s.sessions()
		if err != nil {
			return nil, err
		}
		p, _, err := s.phone(ctx, req)
		if err != nil {
			return nil, err
		}
		apps, err := s.h.Tenancy.ListApps(ctx, p, tenancy.Page{Size: phoneAppsMax})
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		ids := make([]string, 0, len(apps))
		for _, a := range apps {
			ids = append(ids, a.ID)
		}
		live, err := svc.LiveApps(ctx, p, ids)
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		out := &pluxv1.ListPhoneAppsResponse{UserDisplay: p.Display}
		for _, a := range apps {
			out.Apps = append(out.Apps, &pluxv1.PhoneApp{Id: a.ID, Name: a.Name, Live: live[a.ID]})
		}
		return out, nil
	})
}

// RequestJoin is a signed-in phone's request to join its person's Studio session of an app.
func (s DevPhone) RequestJoin(ctx context.Context, req *connect.Request[pluxv1.RequestJoinRequest]) (*connect.Response[pluxv1.RequestJoinResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.RequestJoinResponse, error) {
		svc, err := s.sessions()
		if err != nil {
			return nil, err
		}
		p, caller, err := s.phone(ctx, req)
		if err != nil {
			return nil, err
		}
		jr, err := svc.RequestJoin(ctx, p, p.ID, caller.Name, req.Msg.GetAppId())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		out := &pluxv1.RequestJoinResponse{
			RequestId: jr.ID, SessionId: jr.SessionID, Environment: jr.Environment, Options: numbers(jr.Options),
		}
		if !jr.ExpiresAt.IsZero() {
			out.ExpiresAt = ts(jr.ExpiresAt)
		}
		if jr.Pairing != nil {
			out.PairingLink = jr.Pairing.URI
		}
		return out, nil
	})
}

// CompleteJoin is the phone's pick; the right number returns a pairing link.
func (s DevPhone) CompleteJoin(ctx context.Context, req *connect.Request[pluxv1.CompleteJoinRequest]) (*connect.Response[pluxv1.CompleteJoinResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.CompleteJoinResponse, error) {
		svc, err := s.sessions()
		if err != nil {
			return nil, err
		}
		p, _, err := s.phone(ctx, req)
		if err != nil {
			return nil, err
		}
		joined, err := svc.CompleteJoin(ctx, p, p.ID, req.Msg.GetRequestId(), int(req.Msg.GetNumber()))
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.CompleteJoinResponse{PairingLink: joined.Pairing.URI, SessionId: joined.SessionID, Environment: joined.Environment}, nil
	})
}

// SignOutPhone revokes the calling phone's own token.
func (s DevPhone) SignOutPhone(ctx context.Context, req *connect.Request[pluxv1.SignOutPhoneRequest]) (*connect.Response[pluxv1.SignOutPhoneResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.SignOutPhoneResponse, error) {
		if _, ok := PhoneFrom(ctx); !ok {
			return nil, plxerr.New(plxerr.AuthenticationRequired, "this call needs a phone token and a DPoP proof")
		}
		id, err := identity(ctx)
		if err != nil {
			return nil, err
		}
		if err := s.h.Auth.SignOutPhone(ctx, id); err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.SignOutPhoneResponse{}, nil
	})
}

func numbers(v []int) []uint32 {
	out := make([]uint32, len(v))
	for i, n := range v {
		out[i] = uint32(n) //nolint:gosec // two digits
	}
	return out
}

func phoneSignInState(state string) pluxv1.PhoneSignInState {
	switch state {
	case auth.PhoneSignInWaiting:
		return pluxv1.PhoneSignInState_PHONE_SIGN_IN_STATE_WAITING
	case auth.PhoneSignInClaimed:
		return pluxv1.PhoneSignInState_PHONE_SIGN_IN_STATE_CLAIMED
	case auth.PhoneSignInSignedIn:
		return pluxv1.PhoneSignInState_PHONE_SIGN_IN_STATE_SIGNED_IN
	case auth.PhoneSignInFailed:
		return pluxv1.PhoneSignInState_PHONE_SIGN_IN_STATE_FAILED
	case auth.PhoneSignInCancelled:
		return pluxv1.PhoneSignInState_PHONE_SIGN_IN_STATE_CANCELLED
	case auth.PhoneSignInExpired:
		return pluxv1.PhoneSignInState_PHONE_SIGN_IN_STATE_EXPIRED
	default:
		return pluxv1.PhoneSignInState_PHONE_SIGN_IN_STATE_UNSPECIFIED
	}
}
