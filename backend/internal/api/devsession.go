// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"
	"errors"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/devsession"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// DevSession serves DevSessionService: sessions and pairing (DEV-002,
// DEV-003, ADR-0087), the push of patches (DEV-010), and the relay's
// commands, stream and trace export (DEV-020, DEV-021).
type DevSession struct {
	pluxv1connect.UnimplementedDevSessionServiceHandler
	h *Handlers
}

var _ pluxv1connect.DevSessionServiceHandler = DevSession{}

// DevSession returns the DevSessionService handler.
func (h *Handlers) DevSession() DevSession { return DevSession{h: h} }

// service returns the domain service, or Unimplemented when this server
// has none.
func (s DevSession) service() (*devsession.Service, error) {
	if s.h.DevSessions == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("this server serves no dev sessions"))
	}
	return s.h.DevSessions, nil
}

// CreateSession opens a dev session and returns its first pairing token. It
// is not replayed from the idempotency store: the answer carries a
// credential that is shown once (DEV-002).
func (s DevSession) CreateSession(ctx context.Context, req *connect.Request[pluxv1.DevSessionServiceCreateSessionRequest]) (*connect.Response[pluxv1.DevSessionServiceCreateSessionResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.DevSessionServiceCreateSessionResponse, error) {
		svc, err := s.service()
		if err != nil {
			return nil, err
		}
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		session, pairing, err := svc.CreateSession(ctx, p, req.Msg.GetAppId(), req.Msg.GetEnvironmentId())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.DevSessionServiceCreateSessionResponse{Session: devSessionProto(session), Pairing: devPairingProto(pairing)}, nil
	})
}

// GetPairing issues a fresh pairing token; like CreateSession it is never
// replayed.
func (s DevSession) GetPairing(ctx context.Context, req *connect.Request[pluxv1.DevSessionServiceGetPairingRequest]) (*connect.Response[pluxv1.DevSessionServiceGetPairingResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.DevSessionServiceGetPairingResponse, error) {
		svc, err := s.service()
		if err != nil {
			return nil, err
		}
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		pairing, err := svc.GetPairing(ctx, p, req.Msg.GetSessionId())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.DevSessionServiceGetPairingResponse{Pairing: devPairingProto(pairing)}, nil
	})
}

// Pair pairs the calling device, authenticated by its access token and DPoP
// proof, with the session its pairing token names (DEV-002, SEC-021).
func (s DevSession) Pair(ctx context.Context, req *connect.Request[pluxv1.DevSessionServicePairRequest]) (*connect.Response[pluxv1.DevSessionServicePairResponse], error) {
	svc, err := s.service()
	if err != nil {
		return nil, err
	}
	d, err := deviceOf(ctx)
	if err != nil {
		return nil, err
	}
	jkt, ok := DeviceKeyFrom(ctx)
	if !ok {
		return nil, plxerr.New(plxerr.AuthenticationRequired, "this call needs a device token and its proof")
	}
	m := req.Msg
	res, err := svc.Pair(ctx,
		devsession.Caller{DeviceID: d.DeviceID, OrganizationID: d.OrganizationID, AppID: d.AppID, EnvironmentID: d.EnvironmentID, Thumbprint: jkt},
		devsession.PairRequest{Token: m.GetPairingToken(), Code: m.GetPairingCode(), Name: m.GetName(), Platform: m.GetPlatform(), RuntimeVersion: m.GetRuntimeVersion()})
	if err != nil {
		return nil, err //nolint:wrapcheck // a domain error
	}
	return connect.NewResponse(&pluxv1.DevSessionServicePairResponse{
		SessionId: res.SessionID, DeviceId: res.DeviceID, SocketTicket: res.Ticket, SocketTicketExpiresAt: ts(res.TicketExpiresAt),
	}), nil
}

// RenewSocketTicket gives the calling device, authenticated by its access
// token and DPoP proof, a new socket ticket for the session it is paired
// with (DEV-021).
func (s DevSession) RenewSocketTicket(ctx context.Context, req *connect.Request[pluxv1.DevSessionServiceRenewSocketTicketRequest]) (*connect.Response[pluxv1.DevSessionServiceRenewSocketTicketResponse], error) {
	svc, err := s.service()
	if err != nil {
		return nil, err
	}
	d, err := deviceOf(ctx)
	if err != nil {
		return nil, err
	}
	jkt, ok := DeviceKeyFrom(ctx)
	if !ok {
		return nil, plxerr.New(plxerr.AuthenticationRequired, "this call needs a device token and its proof")
	}
	t, err := svc.RenewSocketTicket(ctx,
		devsession.Caller{DeviceID: d.DeviceID, OrganizationID: d.OrganizationID, AppID: d.AppID, EnvironmentID: d.EnvironmentID, Thumbprint: jkt},
		req.Msg.GetSessionId(), req.Msg.GetDeviceId())
	if err != nil {
		return nil, err //nolint:wrapcheck // a domain error
	}
	return connect.NewResponse(&pluxv1.DevSessionServiceRenewSocketTicketResponse{
		SocketTicket: t.Ticket, SocketTicketExpiresAt: ts(t.ExpiresAt),
	}), nil
}

// SendCommand sends a command to a device of the session, or to all of
// them (DEV-011, DEV-031).
func (s DevSession) SendCommand(ctx context.Context, req *connect.Request[pluxv1.DevSessionServiceSendCommandRequest]) (*connect.Response[pluxv1.DevSessionServiceSendCommandResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.DevSessionServiceSendCommandResponse, error) {
		svc, err := s.service()
		if err != nil {
			return nil, err
		}
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		sent, err := svc.SendCommand(ctx, p, req.Msg)
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.DevSessionServiceSendCommandResponse{CommandId: sent.CommandID, DeviceIds: sent.DeviceIDs}, nil
	})
}

// WatchEvents streams the session's events, resuming after a sequence
// (DEV-021).
func (s DevSession) WatchEvents(ctx context.Context, req *connect.Request[pluxv1.DevSessionServiceWatchEventsRequest], stream *connect.ServerStream[pluxv1.DevSessionServiceWatchEventsResponse]) error {
	svc, err := s.service()
	if err != nil {
		return err
	}
	p, err := s.h.principal(ctx, req.Header(), "")
	if err != nil {
		return err
	}
	m := req.Msg
	return svc.Watch(ctx, p, m.GetSessionId(), m.GetDeviceIds(), m.GetAfterSequence(), func(ev *pluxv1.DevEvent) error { //nolint:wrapcheck // a domain error
		return stream.Send(&pluxv1.DevSessionServiceWatchEventsResponse{Event: ev}) //nolint:wrapcheck // the stream's own error
	})
}

// ExportTrace returns the session's retained events as a trace file
// (DEV-021).
func (s DevSession) ExportTrace(ctx context.Context, req *connect.Request[pluxv1.DevSessionServiceExportTraceRequest]) (*connect.Response[pluxv1.DevSessionServiceExportTraceResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.DevSessionServiceExportTraceResponse, error) {
		svc, err := s.service()
		if err != nil {
			return nil, err
		}
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		trace, err := svc.ExportTrace(ctx, p, req.Msg.GetSessionId(), req.Msg.GetDeviceIds())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.DevSessionServiceExportTraceResponse{Trace: trace.File, EventCount: trace.Events}, nil
	})
}

// ListDevices lists the devices paired with a session.
func (s DevSession) ListDevices(ctx context.Context, req *connect.Request[pluxv1.DevSessionServiceListDevicesRequest]) (*connect.Response[pluxv1.DevSessionServiceListDevicesResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.DevSessionServiceListDevicesResponse, error) {
		svc, err := s.service()
		if err != nil {
			return nil, err
		}
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		devices, err := svc.ListDevices(ctx, p, req.Msg.GetSessionId())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		out := &pluxv1.DevSessionServiceListDevicesResponse{}
		for _, d := range devices {
			out.Devices = append(out.Devices, &pluxv1.DevDevice{
				Id: d.ID, DeviceId: d.DeviceID, Name: d.Name, Platform: d.Platform, RuntimeVersion: d.RuntimeVersion, PairedAt: ts(d.PairedAt),
			})
		}
		return out, nil
	})
}

// EndSession ends a session and unpairs its devices.
func (s DevSession) EndSession(ctx context.Context, req *connect.Request[pluxv1.DevSessionServiceEndSessionRequest]) (*connect.Response[pluxv1.DevSessionServiceEndSessionResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.DevSessionServiceEndSessionResponse, error) {
		svc, err := s.service()
		if err != nil {
			return nil, err
		}
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		session, err := svc.EndSession(ctx, p, req.Msg.GetSessionId())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.DevSessionServiceEndSessionResponse{Session: devSessionProto(session)}, nil
	})
}

// devSessionProto converts a session.
func devSessionProto(s devsession.Session) *pluxv1.DevSession {
	return &pluxv1.DevSession{
		Id: s.ID, AppId: s.AppID, EnvironmentId: s.EnvironmentID, CreatedBy: actorProto(s.CreatedBy),
		CreatedAt: ts(s.CreatedAt), ExpiresAt: ts(s.ExpiresAt), EndedAt: ts(s.EndedAt),
	}
}

// devPairingProto converts an issued pairing token.
func devPairingProto(p devsession.Pairing) *pluxv1.DevPairing {
	return &pluxv1.DevPairing{Token: p.Token, Uri: p.URI, ExpiresAt: ts(p.ExpiresAt), Code: p.Code}
}
