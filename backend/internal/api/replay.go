// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/replay"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

// ReplayHandler serves ReplayService (ANL-050, ADR-0101).
type ReplayHandler struct{ h *Handlers }

var _ pluxv1connect.ReplayServiceHandler = ReplayHandler{}

// Replay returns the ReplayService handler.
func (h *Handlers) Replay() ReplayHandler { return ReplayHandler{h: h} }

// IngestReplay stores a device's replay. The device must name its own app and
// declare the replay consent; the service checks both again, the security
// profile and the replay.* limits.
func (s ReplayHandler) IngestReplay(ctx context.Context, req *connect.Request[pluxv1.IngestReplayRequest]) (*connect.Response[pluxv1.IngestReplayResponse], error) {
	d, err := deviceOf(ctx)
	if err != nil {
		return nil, err
	}
	m := req.Msg
	if id := m.GetAppId(); id != "" && id != d.AppID {
		return nil, plxerr.New(plxerr.PermissionDenied, "a device reports only for its own app")
	}
	u := replay.Upload{
		ID: m.GetId(), DurationMs: m.GetDurationMs(), ReleaseSequence: m.GetReleaseSequence(), Truncated: m.GetTruncated(),
		Events: make([]replay.Event, 0, len(m.GetEvents())),
	}
	if m.GetStartedAt() != nil {
		u.StartedAt = m.GetStartedAt().AsTime()
	}
	for _, e := range m.GetEvents() {
		u.Events = append(u.Events, replayEventOf(e))
	}
	res, err := s.h.Replays.Ingest(ctx, replay.Source{
		OrganizationID: d.OrganizationID, AppID: d.AppID, EnvironmentID: d.EnvironmentID, DeviceID: d.DeviceID,
		ReplayConsent: m.GetConsent().GetReplay(), Bytes: proto.Size(m),
	}, u)
	if err != nil {
		return nil, err //nolint:wrapcheck // a domain error
	}
	return connect.NewResponse(&pluxv1.IngestReplayResponse{Stored: res.Stored, Dropped: int32(res.Dropped)}), nil //nolint:gosec // G115: bounded by replay.eventsPerReplay.
}

// ListReplays lists an environment's replays, newest first.
func (s ReplayHandler) ListReplays(ctx context.Context, req *connect.Request[pluxv1.ListReplaysRequest]) (*connect.Response[pluxv1.ListReplaysResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.ListReplaysResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		m := req.Msg
		env, err := s.h.Tenancy.GetEnvironmentByKey(ctx, p, m.GetAppId(), m.GetEnvironment())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		page := m.GetPage()
		after, size, err := s.h.Pages.Request(req.Spec().Procedure, p.OrganizationID, page)
		if err != nil {
			return nil, err
		}
		rs, err := s.h.Replays.List(ctx, p, m.GetAppId(), env.ID, after, size)
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		out := &pluxv1.ListReplaysResponse{Page: &pluxv1.PageResult{}}
		var last storage.Cursor
		for _, r := range rs {
			last = storage.Cursor{Time: r.StartedAt, ID: r.ID}
			out.Replays = append(out.Replays, replayProto(r))
		}
		out.Page.NextPageToken = s.h.Pages.Next(req.Spec().Procedure, p.OrganizationID, page, len(rs), size, last)
		return out, nil
	})
}

// GetReplay returns one replay with its events.
func (s ReplayHandler) GetReplay(ctx context.Context, req *connect.Request[pluxv1.GetReplayRequest]) (*connect.Response[pluxv1.GetReplayResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.GetReplayResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		r, events, err := s.h.Replays.Get(ctx, p, req.Msg.GetAppId(), req.Msg.GetId())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		out := &pluxv1.GetReplayResponse{Replay: replayProto(r), Events: make([]*pluxv1.ReplayEvent, 0, len(events))}
		for _, e := range events {
			out.Events = append(out.Events, replayEventProto(e))
		}
		return out, nil
	})
}

func replayProto(r replay.Replay) *pluxv1.Replay {
	return &pluxv1.Replay{
		Id: r.ID, AppId: r.AppID, DeviceId: r.DeviceID, StartedAt: ts(r.StartedAt), DurationMs: r.DurationMs,
		EventCount: r.EventCount, Bytes: r.Bytes, ReleaseSequence: r.ReleaseSequence, Truncated: r.Truncated,
		Route: r.Route, ExpiresAt: ts(r.ExpiresAt),
	}
}

func replayEventOf(e *pluxv1.ReplayEvent) replay.Event {
	out := replay.Event{
		AtMs: e.GetAtMs(), Kind: e.GetKind(), Route: e.GetRoute(), PluginKey: e.GetPluginKey(), NodePath: e.GetNodePath(),
		FX: e.GetFx(), FY: e.GetFy(), Name: e.GetName(), ValueType: e.GetValueType(), Value: e.GetValue(),
	}
	for _, n := range e.GetNodes() {
		out.Nodes = append(out.Nodes, replay.Node{
			Path: n.GetPath(), Widget: n.GetWidget(), X: n.GetX(), Y: n.GetY(), Width: n.GetWidth(), Height: n.GetHeight(), Masked: n.GetMasked(),
		})
	}
	return out
}

func replayEventProto(e replay.Event) *pluxv1.ReplayEvent {
	out := &pluxv1.ReplayEvent{
		AtMs: e.AtMs, Kind: e.Kind, Route: e.Route, PluginKey: e.PluginKey, NodePath: e.NodePath,
		Fx: e.FX, Fy: e.FY, Name: e.Name, ValueType: e.ValueType, Value: e.Value,
	}
	for _, n := range e.Nodes {
		out.Nodes = append(out.Nodes, &pluxv1.ReplayNode{
			Path: n.Path, Widget: n.Widget, X: n.X, Y: n.Y, Width: n.Width, Height: n.Height, Masked: n.Masked,
		})
	}
	return out
}
