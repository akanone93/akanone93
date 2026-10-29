// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package renderworker

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/buildinfo"
	"github.com/nightCode42/plux3/backend/internal/compiler"
	"github.com/nightCode42/plux3/backend/internal/plxgolden"
	"github.com/nightCode42/plux3/backend/internal/plxtest"
	"github.com/nightCode42/plux3/backend/internal/renderpb"
	"github.com/nightCode42/plux3/backend/internal/renderpb/renderpbconnect"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

// Executor runs a prepared golden plan below a work directory and returns the
// images it drew. plxgolden.Execute with a Flutter executable is the real one.
type Executor func(ctx context.Context, plan *plxgolden.Plan, work string, lim limits.Set) ([]plxgolden.Rendered, error)

// Options configure a Handler.
type Options struct {
	// Flutter is the Flutter executable; it runs flutter_tester headless.
	Flutter string
	// Fonts are the vendored golden fonts (plxgolden.LoadFonts).
	Fonts map[string][]byte
	// Runtime says where the generated test project gets plux_flutter from.
	Runtime plxtest.Dependency
	// WorkDir is the parent of the scratch directories; empty means the
	// system's temporary directory. It is the only place the worker writes.
	WorkDir string
	// Log receives Flutter's output; nil discards it.
	Log io.Writer
	// Concurrency is the number of renders the process runs at once; a further
	// call is refused as exhausted and the server queues the job again. Zero
	// means one: a render saturates the machine.
	Concurrency int
	// Execute replaces Flutter in tests.
	Execute Executor
}

// Handler implements renderpbconnect.RenderWorkerServiceHandler.
type Handler struct {
	renderpbconnect.UnimplementedRenderWorkerServiceHandler
	o    Options
	slot chan struct{}
}

var _ renderpbconnect.RenderWorkerServiceHandler = (*Handler)(nil)

// New returns a Handler.
func New(o Options) (*Handler, error) {
	if o.Execute == nil {
		if o.Flutter == "" {
			return nil, errors.New("renderworker: the Flutter executable is required")
		}
		flutter, log := o.Flutter, o.Log
		o.Execute = func(ctx context.Context, plan *plxgolden.Plan, work string, lim limits.Set) ([]plxgolden.Rendered, error) {
			return plxgolden.Execute(ctx, plan, work, flutter, log, lim)
		}
	}
	if o.Log == nil {
		o.Log = io.Discard
	}
	if len(o.Fonts) == 0 {
		return nil, errors.New("renderworker: the golden fonts are required")
	}
	n := max(o.Concurrency, 1)
	return &Handler{o: o, slot: make(chan struct{}, n)}, nil
}

// Render implements renderpbconnect.RenderWorkerServiceHandler.
func (h *Handler) Render(ctx context.Context, req *connect.Request[renderpb.RenderRequest], stream *connect.ServerStream[renderpb.RenderResponse]) error {
	select {
	case h.slot <- struct{}{}:
		defer func() { <-h.slot }()
	default:
		return connect.NewError(connect.CodeResourceExhausted, errors.New("the render worker is busy"))
	}
	m := req.Msg
	lim := Limits{
		ProjectBytes: int64(min(m.GetLimits().GetProjectBytes(), math.MaxInt64)), Entries: int64(m.GetLimits().GetEntries()),
		Cases: int64(m.GetLimits().GetCases()), ImageBytes: int64(min(m.GetLimits().GetImageBytes(), math.MaxInt64)),
	}.Clamped()
	if len(m.GetAfter()) == 0 {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("the request has no project to render"))
	}
	after, err := Unpack(m.GetAfter(), lim)
	if err != nil {
		return connect.NewError(connect.CodeInvalidArgument, err)
	}
	var before fs.FS
	if len(m.GetBefore()) > 0 {
		if before, err = Unpack(m.GetBefore(), lim); err != nil {
			return connect.NewError(connect.CodeInvalidArgument, err)
		}
	}
	cat, err := DecodeCatalogue(m.GetFunctions(), m.GetDeviceFunctions())
	if err != nil {
		return connect.NewError(connect.CodeInvalidArgument, err)
	}
	screens, err := h.diff(ctx, cat, before, after, m.GetDevices(), lim)
	if err != nil {
		return err
	}
	for _, s := range screens {
		if err := stream.Send(s); err != nil {
			return fmt.Errorf("renderworker: send %s: %w", s.GetScreen(), err)
		}
	}
	return nil
}

// shot identifies one screen on one device.
type shot struct{ plugin, screen, device string }

// diff renders both projects and returns the screens whose image differs, in
// order of plugin, screen and device. A screen only in one project is new or
// removed and is returned with the other image empty.
func (h *Handler) diff(ctx context.Context, cat compiler.FunctionCatalogue, before, after fs.FS, devices []string, lim Limits) ([]*renderpb.RenderResponse, error) {
	now, err := h.render(ctx, cat, after, devices, lim)
	if err != nil {
		return nil, err
	}
	var was map[shot][]byte
	if before != nil {
		if was, err = h.render(ctx, cat, before, devices, lim); err != nil {
			return nil, err
		}
	}
	keys := make([]shot, 0, len(now)+len(was))
	for k := range now {
		keys = append(keys, k)
	}
	for k := range was {
		if _, ok := now[k]; !ok {
			keys = append(keys, k)
		}
	}
	slices.SortFunc(keys, func(a, b shot) int {
		return cmpStrings(a.plugin, b.plugin, a.screen, b.screen, a.device, b.device)
	})
	var out []*renderpb.RenderResponse
	for _, k := range keys {
		b, a := was[k], now[k]
		if bytes.Equal(b, a) {
			continue
		}
		out = append(out, &renderpb.RenderResponse{PluginKey: k.plugin, Screen: k.screen, Device: k.device, BeforePng: b, AfterPng: a})
	}
	return out, nil
}

func cmpStrings(pairs ...string) int {
	for i := 0; i+1 < len(pairs); i += 2 {
		if c := strings.Compare(pairs[i], pairs[i+1]); c != 0 {
			return c
		}
	}
	return 0
}

// render compiles a project against the function catalogue of the request, as
// a release build does, and draws its pages on the devices.
func (h *Handler) render(ctx context.Context, cat compiler.FunctionCatalogue, files fs.FS, devices []string, lim Limits) (map[shot][]byte, error) {
	opts := compiler.DefaultOptions()
	opts.Version = buildinfo.Get().Version
	opts.Functions = cat
	opts.DeviceModules = deviceStubs{}
	res := compiler.Compile(files, opts)
	if res.Diagnostics.HasErrors() {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("the project does not compile: %s", firstErrors(res)))
	}
	work, err := os.MkdirTemp(h.o.WorkDir, "render-")
	if err != nil {
		return nil, fmt.Errorf("renderworker: %w", err)
	}
	defer func() { _ = os.RemoveAll(work) }()
	set, err := golden(lim)
	if err != nil {
		return nil, err
	}
	plan, err := plxgolden.Prepare(plxgolden.Options{
		Result: res, Fonts: h.o.Fonts, Entropy: rand.Reader, Runtime: h.o.Runtime, Limits: set, Narrow: narrowTo(devices),
	}, filepath.Join(work, "_images"))
	if err != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, err)
	}
	rendered, err := h.o.Execute(ctx, plan, work, set)
	if err != nil {
		return nil, fmt.Errorf("renderworker: %w", err)
	}
	out := make(map[shot][]byte, len(rendered))
	for _, r := range rendered {
		out[shot{plugin: r.Plugin, screen: r.Route, device: r.Frame.ID}] = r.PNG
	}
	return out, nil
}

// golden is the registry set the harness is held to: the defaults, tightened
// to what the server resolved for the organisation where that is lower. A
// value above the default is not honoured: a visual diff renders a narrowed
// matrix, which the defaults bound generously.
func golden(lim Limits) (limits.Set, error) {
	set := limits.Defaults()
	for _, c := range []struct {
		key limits.Key
		v   int64
	}{{limits.GoldenCasesPerRun, lim.Cases}, {limits.GoldenImageBytes, lim.ImageBytes}} {
		if c.v >= set.Get(c.key) {
			continue
		}
		var err error
		if set, err = set.Tighten(c.key, limits.ScopeOrganization, c.v); err != nil {
			return limits.Set{}, fmt.Errorf("renderworker: %w", err)
		}
	}
	return set, nil
}

// narrowTo reduces the golden matrix to what a visual diff shows: the named
// devices (all of the matrix's when none is named), the first locale, the
// light theme and the text scale 1.
func narrowTo(devices []string) func(plxgolden.Matrix) plxgolden.Matrix {
	return func(m plxgolden.Matrix) plxgolden.Matrix {
		if len(devices) > 0 {
			var keep []plxgolden.Frame
			for _, f := range m.Frames {
				if slices.Contains(devices, f.ID) {
					keep = append(keep, f)
				}
			}
			m.Frames = keep
		}
		m.Locales = m.Locales[:min(len(m.Locales), 1)]
		m.Themes = []string{plxgolden.ThemeLight}
		m.TextScales = []float64{1}
		return m
	}
}

// firstErrors names the first few error diagnostics of a compilation.
func firstErrors(res *compiler.Result) string {
	var parts []string
	for _, d := range res.Diagnostics {
		if d.Severity.String() == "error" {
			parts = append(parts, d.String())
			if len(parts) == 3 {
				break
			}
		}
	}
	return strings.Join(parts, "; ")
}
