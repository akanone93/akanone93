// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/devsession"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

// ServerTimingHeader carries the durations of a push's stages, for the
// latency benchmark (NFR-030).
const ServerTimingHeader = "Server-Timing"

// PushPatch applies document patches to the session's draft, compiles them
// and delivers the development bundle to the paired devices (DEV-010). It is
// not replayed from the idempotency store: a repeated push is a new bundle.
// A patch that does not compile answers with its diagnostics and no results.
func (s DevSession) PushPatch(ctx context.Context, req *connect.Request[pluxv1.DevSessionServicePushPatchRequest]) (*connect.Response[pluxv1.DevSessionServicePushPatchResponse], error) {
	svc, err := s.service()
	if err != nil {
		return nil, err
	}
	p, err := s.h.principal(ctx, req.Header(), "")
	if err != nil {
		return nil, err
	}
	depth := int(s.h.Limits.Get(limits.DocumentJSONDepth))
	patches := make([]devsession.DocumentPatch, 0, len(req.Msg.GetPatches()))
	for _, dp := range req.Msg.GetPatches() {
		ops, err := patchOps(dp.GetPatch(), depth)
		if err != nil {
			return nil, err
		}
		patches = append(patches, devsession.DocumentPatch{PluginID: dp.GetPluginId(), Path: dp.GetPath(), Ops: ops, IfRevision: dp.GetIfRevision()})
	}
	res, err := svc.PushPatch(ctx, p, req.Msg.GetSessionId(), patches)
	if code, ok := plxerr.CodeOf(err); ok && code == plxerr.DevPatchRejected && len(res.Diagnostics) > 0 {
		err = nil // the diagnostics are the answer
	}
	if err != nil {
		return nil, err //nolint:wrapcheck // a domain error
	}
	out := connect.NewResponse(&pluxv1.DevSessionServicePushPatchResponse{
		BundleSha256: res.BundleSHA256, Results: res.Results, Diagnostics: diagnosticProtos(res.Diagnostics),
	})
	out.Header().Set(ServerTimingHeader, serverTiming(res.Timings))
	return out, nil
}

// serverTiming renders the stage durations as a Server-Timing value.
func serverTiming(t devsession.Timings) string {
	ms := func(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }
	return fmt.Sprintf("compile;dur=%.3f, sign;dur=%.3f, deliver;dur=%.3f", ms(t.Compile), ms(t.Sign), ms(t.Deliver))
}
