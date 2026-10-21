// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/controldoc"
	"github.com/nightCode42/plux3/backend/internal/device"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/push"
	"github.com/nightCode42/plux3/backend/internal/release"
)

// scopeOfProto maps the wire enumeration to the scopes of a kill switch.
var scopeOfProto = map[pluxv1.KillSwitchScope]controldoc.Scope{
	pluxv1.KillSwitchScope_KILL_SWITCH_SCOPE_PLUGIN:       controldoc.ScopePlugin,
	pluxv1.KillSwitchScope_KILL_SWITCH_SCOPE_PAGE:         controldoc.ScopePage,
	pluxv1.KillSwitchScope_KILL_SWITCH_SCOPE_ACTION_GRAPH: controldoc.ScopeActionGraph,
	pluxv1.KillSwitchScope_KILL_SWITCH_SCOPE_DATA_SOURCE:  controldoc.ScopeDataSource,
	pluxv1.KillSwitchScope_KILL_SWITCH_SCOPE_FUNCTION:     controldoc.ScopeFunction,
}

// killSwitchOf reads a kill switch from the wire, refusing an unknown scope.
func killSwitchOf(e *pluxv1.KillSwitchEntry) (controldoc.KillSwitch, error) {
	scope, ok := scopeOfProto[e.GetScope()]
	if !ok {
		return controldoc.KillSwitch{}, plxerr.New(plxerr.InvalidFormat, "the kill switch scope is plugin, page, actionGraph, dataSource or function")
	}
	return controldoc.KillSwitch{Scope: scope, Target: e.GetTarget()}, nil
}

// channelRef names the channel of a request.
func channelRef(app, environment, channel string) release.ChannelRef {
	return release.ChannelRef{AppID: app, EnvironmentID: environment, ChannelKey: channel}
}

// breakGlassOptions turns the break-glass fields of a request into the
// option of the call (GOV-024). A justification without break_glass is
// refused rather than ignored, so that nobody believes a change was made as
// an emergency when it was not.
func breakGlassOptions(breakGlass bool, justification string) ([]release.ChangeOption, error) {
	switch {
	case breakGlass:
		return []release.ChangeOption{release.WithBreakGlass(justification)}, nil
	case justification != "":
		return nil, plxerr.New(plxerr.InvalidFormat, "a justification belongs to a break-glass action; set break_glass or leave it out")
	}
	return nil, nil
}

// controlResult answers a change with the document it produced.
func controlResult(state release.ControlState, err error) (*pluxv1.ControlDocument, error) {
	if err != nil {
		return nil, err
	}
	return controlDocumentProto(state), nil
}

// controlDocumentProto converts an operator's view of a control document.
func controlDocumentProto(s release.ControlState) *pluxv1.ControlDocument {
	out := &pluxv1.ControlDocument{
		AppId: s.AppID, EnvironmentId: s.EnvironmentID, ChannelKey: s.ChannelKey, Version: s.Version,
		UpdatedBy: actorProto(s.UpdatedBy), UpdatedAt: ts(s.UpdatedAt),
		SignedVersion: s.SignedVersion, IssuedAt: ts(s.IssuedAt), ExpiresAt: ts(s.ExpiresAt),
	}
	for _, k := range s.KillSwitches {
		out.KillSwitches = append(out.KillSwitches, &pluxv1.KillSwitchEntry{Scope: protoOfScope(k.Scope), Target: k.Target})
	}
	if m := s.MandatoryUpdate; m != nil {
		out.MandatoryUpdate = &pluxv1.ControlRelease{ReleaseId: m.ReleaseID, ReleaseSequence: int64(m.ReleaseSequence), MessageKey: m.MessageKey} //nolint:gosec // a sequence fits
	}
	if r := s.ForcedRollback; r != nil {
		out.ForcedRollback = &pluxv1.ControlRelease{ReleaseId: r.ReleaseID, ReleaseSequence: int64(r.ReleaseSequence)} //nolint:gosec // a sequence fits
	}
	return out
}

// protoOfScope is the inverse of scopeOfProto.
func protoOfScope(s controldoc.Scope) pluxv1.KillSwitchScope {
	for wire, scope := range scopeOfProto {
		if scope == s {
			return wire
		}
	}
	return pluxv1.KillSwitchScope_KILL_SWITCH_SCOPE_UNSPECIFIED
}

// GetControlDocument returns a channel's control document (REL-060).
func (s Control) GetControlDocument(ctx context.Context, req *connect.Request[pluxv1.GetControlDocumentRequest]) (*connect.Response[pluxv1.GetControlDocumentResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.GetControlDocumentResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		m := req.Msg
		state, err := s.h.Releases.ControlDocument(ctx, p, channelRef(m.GetAppId(), m.GetEnvironmentId(), m.GetChannelKey()))
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.GetControlDocumentResponse{Document: controlDocumentProto(state)}, nil
	})
}

// SetKillSwitch switches one thing off for a channel's devices (REL-060).
func (s Control) SetKillSwitch(ctx context.Context, req *connect.Request[pluxv1.SetKillSwitchRequest]) (*connect.Response[pluxv1.SetKillSwitchResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.SetKillSwitchResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		m := req.Msg
		k, err := killSwitchOf(m.GetKillSwitch())
		if err != nil {
			return nil, err
		}
		opts, err := breakGlassOptions(m.GetBreakGlass(), m.GetJustification())
		if err != nil {
			return nil, err
		}
		doc, err := controlResult(s.h.Releases.SetKillSwitch(ctx, p, channelRef(m.GetAppId(), m.GetEnvironmentId(), m.GetChannelKey()), k, opts...))
		if err != nil {
			return nil, err
		}
		return &pluxv1.SetKillSwitchResponse{Document: doc}, nil
	})
}

// ClearKillSwitch switches it back on.
func (s Control) ClearKillSwitch(ctx context.Context, req *connect.Request[pluxv1.ClearKillSwitchRequest]) (*connect.Response[pluxv1.ClearKillSwitchResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.ClearKillSwitchResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		m := req.Msg
		k, err := killSwitchOf(m.GetKillSwitch())
		if err != nil {
			return nil, err
		}
		opts, err := breakGlassOptions(m.GetBreakGlass(), m.GetJustification())
		if err != nil {
			return nil, err
		}
		doc, err := controlResult(s.h.Releases.ClearKillSwitch(ctx, p, channelRef(m.GetAppId(), m.GetEnvironmentId(), m.GetChannelKey()), k, opts...))
		if err != nil {
			return nil, err
		}
		return &pluxv1.ClearKillSwitchResponse{Document: doc}, nil
	})
}

// SetMandatoryUpdate makes a release mandatory (REL-070).
func (s Control) SetMandatoryUpdate(ctx context.Context, req *connect.Request[pluxv1.SetMandatoryUpdateRequest]) (*connect.Response[pluxv1.SetMandatoryUpdateResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.SetMandatoryUpdateResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		m := req.Msg
		doc, err := controlResult(s.h.Releases.SetMandatoryUpdate(ctx, p, channelRef(m.GetAppId(), m.GetEnvironmentId(), m.GetChannelKey()), m.GetReleaseSequence(), m.GetMessageKey()))
		if err != nil {
			return nil, err
		}
		return &pluxv1.SetMandatoryUpdateResponse{Document: doc}, nil
	})
}

// ClearMandatoryUpdate withdraws the mandatory update.
func (s Control) ClearMandatoryUpdate(ctx context.Context, req *connect.Request[pluxv1.ClearMandatoryUpdateRequest]) (*connect.Response[pluxv1.ClearMandatoryUpdateResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.ClearMandatoryUpdateResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		m := req.Msg
		doc, err := controlResult(s.h.Releases.ClearMandatoryUpdate(ctx, p, channelRef(m.GetAppId(), m.GetEnvironmentId(), m.GetChannelKey())))
		if err != nil {
			return nil, err
		}
		return &pluxv1.ClearMandatoryUpdateResponse{Document: doc}, nil
	})
}

// SetForcedRollback directs devices to an earlier release they retain.
func (s Control) SetForcedRollback(ctx context.Context, req *connect.Request[pluxv1.SetForcedRollbackRequest]) (*connect.Response[pluxv1.SetForcedRollbackResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.SetForcedRollbackResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		m := req.Msg
		opts, err := breakGlassOptions(m.GetBreakGlass(), m.GetJustification())
		if err != nil {
			return nil, err
		}
		doc, err := controlResult(s.h.Releases.SetForcedRollback(ctx, p, channelRef(m.GetAppId(), m.GetEnvironmentId(), m.GetChannelKey()), m.GetReleaseSequence(), opts...))
		if err != nil {
			return nil, err
		}
		return &pluxv1.SetForcedRollbackResponse{Document: doc}, nil
	})
}

// ClearForcedRollback withdraws the forced rollback.
func (s Control) ClearForcedRollback(ctx context.Context, req *connect.Request[pluxv1.ClearForcedRollbackRequest]) (*connect.Response[pluxv1.ClearForcedRollbackResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.ClearForcedRollbackResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		m := req.Msg
		opts, err := breakGlassOptions(m.GetBreakGlass(), m.GetJustification())
		if err != nil {
			return nil, err
		}
		doc, err := controlResult(s.h.Releases.ClearForcedRollback(ctx, p, channelRef(m.GetAppId(), m.GetEnvironmentId(), m.GetChannelKey()), opts...))
		if err != nil {
			return nil, err
		}
		return &pluxv1.ClearForcedRollbackResponse{Document: doc}, nil
	})
}

// GetSignedControl serves the signed control document of the device's
// channel, with the ETag handling of the manifest (SYN-060). The document
// is public to the devices of its channel and each device verifies it
// against its trusted root, so the DPoP check at the edge is the only
// authentication it needs.
func (s Manifest) GetSignedControl(ctx context.Context, req *connect.Request[pluxv1.GetSignedControlRequest]) (*connect.Response[pluxv1.GetSignedControlResponse], error) {
	d, err := deviceOf(ctx)
	if err != nil {
		return nil, err
	}
	served, err := s.h.Releases.GetControlDocument(ctx, release.ControlRequest{
		OrganizationID: d.OrganizationID, EnvironmentID: d.EnvironmentID, Channel: req.Msg.GetChannel(), IfNoneMatch: req.Msg.GetIfNoneMatch(),
	})
	if err != nil {
		return nil, err //nolint:wrapcheck // a domain error
	}
	res := connect.NewResponse(&pluxv1.GetSignedControlResponse{
		NotModified: served.NotModified, Etag: served.ETag, Version: served.Version, Signed: served.Signed,
	})
	res.Header().Set("ETag", served.ETag)
	return res, nil
}

// providerOfProto maps the wire enumeration to a push provider.
var providerOfProto = map[pluxv1.PushProvider]push.Provider{
	pluxv1.PushProvider_PUSH_PROVIDER_FCM:  push.FCM,
	pluxv1.PushProvider_PUSH_PROVIDER_APNS: push.APNs,
}

// pushDevice names the device a push call is about.
func pushDevice(d device.Identity) push.Device {
	return push.Device{OrganizationID: d.OrganizationID, AppID: d.AppID, EnvironmentID: d.EnvironmentID, DeviceID: d.DeviceID}
}

// RegisterPushToken stores the calling device's push token (SYN-060).
func (s Device) RegisterPushToken(ctx context.Context, req *connect.Request[pluxv1.RegisterPushTokenRequest]) (*connect.Response[pluxv1.RegisterPushTokenResponse], error) {
	d, err := deviceOf(ctx)
	if err != nil {
		return nil, err
	}
	provider, ok := providerOfProto[req.Msg.GetProvider()]
	if !ok {
		return nil, plxerr.New(plxerr.InvalidFormat, "the push provider is fcm or apns")
	}
	if s.h.Push == nil {
		return nil, plxerr.New(plxerr.PreconditionFailed, "silent push is not enabled on this server")
	}
	if err := s.h.Push.RegisterToken(ctx, pushDevice(d), provider, req.Msg.GetToken()); err != nil {
		return nil, err //nolint:wrapcheck // a domain error
	}
	return connect.NewResponse(&pluxv1.RegisterPushTokenResponse{}), nil
}

// ClearPushToken deletes the calling device's push token.
func (s Device) ClearPushToken(ctx context.Context, _ *connect.Request[pluxv1.ClearPushTokenRequest]) (*connect.Response[pluxv1.ClearPushTokenResponse], error) {
	d, err := deviceOf(ctx)
	if err != nil {
		return nil, err
	}
	if s.h.Push != nil {
		if err := s.h.Push.ClearToken(ctx, pushDevice(d)); err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
	}
	return connect.NewResponse(&pluxv1.ClearPushTokenResponse{}), nil
}
