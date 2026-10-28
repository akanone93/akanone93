// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Package renderclient is the worker role's side of a visual diff (O-9,
// ADR-0097): the releasereq.Renderer that sends both projects of a release
// request to the render worker over mutual TLS and collects the screens whose
// image changed.
//
// The render worker image holds the Flutter toolchain and no credential of the
// server (L-4), so the client sends it everything it needs and stores what it
// returns: the worker role keeps the database, the object store and the queue.
// What the worker returns is untrusted until it has passed the bounds below:
// the images are PNGs of at most golden.imageBytes, and there are at most
// golden.casesPerRun screens. The client is safe for concurrent use.
package renderclient

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/releasereq"
	"github.com/nightCode42/plux3/backend/internal/renderpb"
	"github.com/nightCode42/plux3/backend/internal/renderpb/renderpbconnect"
	"github.com/nightCode42/plux3/backend/internal/renderworker"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

// pngSignature opens every PNG file.
var pngSignature = []byte("\x89PNG\r\n\x1a\n")

// Timeout is the longest a render may take: Flutter compiles and draws two
// projects. The HTTP client's own timeout must be at least this.
const Timeout = 15 * time.Minute

// Options configures a Client.
type Options struct {
	// HTTP is the mutual-TLS client (httpx.NewInternalClient); its own timeout
	// must be at least Timeout.
	HTTP *http.Client
	// URL is the base URL of the render worker, such as "https://render:8443".
	URL string
}

// Client calls the render worker. It implements releasereq.Renderer.
type Client struct {
	rpc renderpbconnect.RenderWorkerServiceClient
}

var _ releasereq.Renderer = (*Client)(nil)

// New returns a Client.
func New(o Options) (*Client, error) {
	if o.HTTP == nil || o.URL == "" {
		return nil, errors.New("renderclient.New: HTTP and URL are required")
	}
	return &Client{rpc: renderpbconnect.NewRenderWorkerServiceClient(o.HTTP, o.URL)}, nil
}

// Render implements releasereq.Renderer. A failure of the worker, of the
// network or of the projects is an error the job retries; after its last
// attempt the job fails (releasereq.FailRender).
func (c *Client) Render(ctx context.Context, in releasereq.RenderInput) ([]releasereq.Screen, error) {
	if in.After == nil {
		return nil, errors.New("renderclient.Render: the input has no project to render")
	}
	set := in.Limits
	if set.IsZero() {
		set = limits.Defaults()
	}
	lim := renderworker.LimitsOf(set).Clamped()
	req := &renderpb.RenderRequest{
		JobId: in.JobID, Devices: in.Devices,
		Limits: &renderpb.RenderLimits{
			ProjectBytes: uint64(lim.ProjectBytes), Entries: uint32(min(lim.Entries, 1<<32-1)), //nolint:gosec // G115: bounded and positive.
			Cases: uint32(min(lim.Cases, 1<<32-1)), ImageBytes: uint64(lim.ImageBytes), //nolint:gosec // G115: bounded and positive.
		},
	}
	var err error
	if req.After, err = renderworker.Pack(in.After, lim); err != nil {
		return nil, fmt.Errorf("renderclient.Render: pack the new project: %w", err)
	}
	if in.Before != nil {
		if req.Before, err = renderworker.Pack(in.Before, lim); err != nil {
			return nil, fmt.Errorf("renderclient.Render: pack the served project: %w", err)
		}
	}
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	stream, err := c.rpc.Render(ctx, connect.NewRequest(req))
	if err != nil {
		return nil, fmt.Errorf("renderclient.Render: %w", err)
	}
	defer func() { _ = stream.Close() }()
	var out []releasereq.Screen
	for stream.Receive() {
		if int64(len(out)) >= lim.Cases {
			return nil, fmt.Errorf("renderclient.Render: the worker returned more than %d screens", lim.Cases)
		}
		s, err := screenOf(stream.Msg(), lim.ImageBytes)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	if err := stream.Err(); err != nil {
		return nil, fmt.Errorf("renderclient.Render: %w", err)
	}
	return out, nil
}

// screenOf checks one screen of a response.
func screenOf(m *renderpb.RenderedScreen, maxImage int64) (releasereq.Screen, error) {
	for _, img := range [][]byte{m.GetBeforePng(), m.GetAfterPng()} {
		if int64(len(img)) > maxImage {
			return releasereq.Screen{}, fmt.Errorf("renderclient.Render: an image of %s has %d bytes; golden.imageBytes allows %d", m.GetScreen(), len(img), maxImage)
		}
		if len(img) > 0 && !bytes.HasPrefix(img, pngSignature) {
			return releasereq.Screen{}, fmt.Errorf("renderclient.Render: an image of %s is not a PNG", m.GetScreen())
		}
	}
	if m.GetScreen() == "" || m.GetDevice() == "" || (len(m.GetBeforePng()) == 0 && len(m.GetAfterPng()) == 0) {
		return releasereq.Screen{}, errors.New("renderclient.Render: the worker returned a screen with no name, device or image")
	}
	return releasereq.Screen{PluginKey: m.GetPluginKey(), Screen: m.GetScreen(), Device: m.GetDevice(), Before: m.GetBeforePng(), After: m.GetAfterPng()}, nil
}
