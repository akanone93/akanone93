// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package renderclient_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/releasereq"
	"github.com/nightCode42/plux3/backend/internal/renderclient"
	"github.com/nightCode42/plux3/backend/internal/renderpb"
	"github.com/nightCode42/plux3/backend/internal/renderpb/renderpbconnect"
	"github.com/nightCode42/plux3/backend/internal/renderworker"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

const png = "\x89PNG\r\n\x1a\nbody"

// fakeWorker answers Render with the screens it was given, or fails after
// them, and keeps the request it received.
type fakeWorker struct {
	renderpbconnect.UnimplementedRenderWorkerServiceHandler
	screens []*renderpb.RenderedScreen
	fail    error
	got     *renderpb.RenderRequest
}

func (w *fakeWorker) Render(_ context.Context, req *connect.Request[renderpb.RenderRequest], stream *connect.ServerStream[renderpb.RenderedScreen]) error {
	w.got = req.Msg
	for _, s := range w.screens {
		if err := stream.Send(s); err != nil {
			return err //nolint:wrapcheck // a test double
		}
	}
	return w.fail
}

func start(t *testing.T, w *fakeWorker) *renderclient.Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle(renderpbconnect.NewRenderWorkerServiceHandler(w))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c, err := renderclient.New(renderclient.Options{HTTP: srv.Client(), URL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func input() releasereq.RenderInput {
	return releasereq.RenderInput{
		JobID: "j1", Devices: []string{"small-360x640"}, Limits: limits.Defaults(),
		Before: fstest.MapFS{"app.json": {Data: []byte("1")}}, After: fstest.MapFS{"app.json": {Data: []byte("2")}},
	}
}

// Verifies: STU-100.
// The client sends both projects, the devices and the limits of the
// organisation, and returns the screens the worker streamed.
func TestRenderSendsProjectsAndReturnsScreens_STU_100(t *testing.T) {
	t.Parallel()
	w := &fakeWorker{screens: []*renderpb.RenderedScreen{
		{PluginKey: "loans", Screen: "home", Device: "small-360x640", BeforePng: []byte(png), AfterPng: []byte(png + "2")},
		{Screen: "new", Device: "small-360x640", AfterPng: []byte(png)},
	}}
	got, err := start(t, w).Render(context.Background(), input())
	if err != nil || len(got) != 2 {
		t.Fatalf("Render = %d screens, %v", len(got), err)
	}
	if got[0].PluginKey != "loans" || got[0].Screen != "home" || string(got[0].After) != png+"2" || got[1].Before != nil {
		t.Errorf("screens = %+v", got)
	}
	if w.got.GetJobId() != "j1" || len(w.got.GetBefore()) == 0 || len(w.got.GetAfter()) == 0 || w.got.GetDevices()[0] != "small-360x640" {
		t.Errorf("request = %+v", w.got)
	}
	lim := renderworker.LimitsOf(limits.Defaults())
	if int64(w.got.GetLimits().GetCases()) != lim.Cases || int64(w.got.GetLimits().GetImageBytes()) != lim.ImageBytes { //nolint:gosec // G115: bounded by the registry
		t.Errorf("limits = %+v, want %+v", w.got.GetLimits(), lim)
	}

	first := input()
	first.Before = nil
	if _, err := start(t, &fakeWorker{}).Render(context.Background(), first); err != nil {
		t.Errorf("a first release: %v", err)
	}
}

// Verifies: STU-100, SEC-043.
// Everything the worker returns is checked: a failure, an image that is not a
// PNG or is too large, a screen with no name and too many screens are errors
// that the job retries, and a missing project is refused before any call.
func TestRenderRejectsWhatTheWorkerGetsWrong_STU_100(t *testing.T) {
	t.Parallel()
	lim := renderworker.LimitsOf(limits.Defaults())
	many := make([]*renderpb.RenderedScreen, lim.Cases+1)
	for i := range many {
		many[i] = &renderpb.RenderedScreen{Screen: "s", Device: "d", AfterPng: []byte(png)}
	}
	for name, w := range map[string]*fakeWorker{
		"worker failure": {fail: connect.NewError(connect.CodeFailedPrecondition, errors.New("the project does not compile"))},
		"not a png":      {screens: []*renderpb.RenderedScreen{{Screen: "s", Device: "d", AfterPng: []byte("GIF89a")}}},
		"too large":      {screens: []*renderpb.RenderedScreen{{Screen: "s", Device: "d", AfterPng: append([]byte(png), make([]byte, lim.ImageBytes)...)}}},
		"unnamed":        {screens: []*renderpb.RenderedScreen{{AfterPng: []byte(png)}}},
		"no image":       {screens: []*renderpb.RenderedScreen{{Screen: "s", Device: "d"}}},
		"too many":       {screens: many},
	} {
		if _, err := start(t, w).Render(context.Background(), input()); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
	noAfter := input()
	noAfter.After = nil
	if _, err := start(t, &fakeWorker{}).Render(context.Background(), noAfter); err == nil || !strings.Contains(err.Error(), "no project") {
		t.Errorf("no project: %v", err)
	}
	huge := input()
	huge.Limits, _ = limits.Defaults().Tighten(limits.ArchiveEntries, limits.ScopeInstallation, 1)
	huge.After = fstest.MapFS{"a": {Data: []byte("1")}, "b": {Data: []byte("2")}}
	if _, err := start(t, &fakeWorker{}).Render(context.Background(), huge); !errors.Is(err, renderworker.ErrTooLarge) {
		t.Errorf("a project over the file limit: %v", err)
	}
	huge.After, huge.Before = fstest.MapFS{"a": {Data: []byte("1")}}, huge.After
	if _, err := start(t, &fakeWorker{}).Render(context.Background(), huge); !errors.Is(err, renderworker.ErrTooLarge) {
		t.Errorf("a served project over the file limit: %v", err)
	}
}

// Verifies: STU-100.
func TestNewNeedsATransportAndAnAddress_STU_100(t *testing.T) {
	t.Parallel()
	if _, err := renderclient.New(renderclient.Options{URL: "https://render:8443"}); err == nil {
		t.Error("no HTTP client was accepted")
	}
	if _, err := renderclient.New(renderclient.Options{HTTP: http.DefaultClient}); err == nil {
		t.Error("no address was accepted")
	}
}
