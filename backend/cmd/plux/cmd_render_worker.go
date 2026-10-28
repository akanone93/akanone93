// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/nightCode42/plux3/backend/internal/httpx"
	"github.com/nightCode42/plux3/backend/internal/plxgolden"
	"github.com/nightCode42/plux3/backend/internal/plxtest"
	"github.com/nightCode42/plux3/backend/internal/renderworker"
)

const renderWorkerUsage = "Usage: plux render-worker serve [flags]\n"

// renderWorker runs plux render-worker serve, the process of the render
// worker image (O-9, ADR-0097).
func (e env) renderWorker(args []string) int {
	if len(args) < 1 || args[0] != "serve" {
		_, _ = fmt.Fprint(e.stderr, renderWorkerUsage)
		return exitUsage
	}
	return e.renderWorkerServe(args[1:])
}

// renderWorkerServe answers the worker role of the server's render calls
// over mutual TLS until it is stopped. The process reads no credential of the
// server: the requests carry the projects, and the images go back in the
// responses (L-4).
func (e env) renderWorkerServe(args []string) int {
	set := flag.NewFlagSet("render-worker serve", flag.ContinueOnError)
	listen := set.String("listen", ":8443", "the `address` of the mutual-TLS listener")
	ca := set.String("ca", "", "the internal CA `file` that must have signed every client's certificate (required)")
	cert := set.String("cert", "", "this worker's certificate chain `file` (required)")
	key := set.String("key", "", "this worker's private key `file` (required)")
	flutter := set.String("flutter", "", "the `path` of the Flutter executable (default flutter on the PATH)")
	runtime := set.String("runtime", "", "the plux_flutter checkout `dir` the generated test projects depend on (default the published package)")
	fonts := set.String("fonts", "", "the vendored golden fonts `dir` (required)")
	work := set.String("work", "", "the `dir` that holds the scratch directories of renders (default the system's)")
	concurrency := set.Int("concurrency", 1, "renders to run at once; a further one is refused and queued again by the server")
	if _, code, ok := parse(set, args, e.stderr, "Usage: plux render-worker serve --ca file --cert file --key file --fonts dir [--listen addr] [--flutter path] [--runtime dir]\n\n"+
		"Renders the visual diffs of release requests for the server's worker role: it compiles the\n"+
		"project before and after a request, draws its pages on the standard devices with the golden\n"+
		"harness and returns the screens that changed. It holds no credential of the server.", 0); !ok {
		return code
	}
	if *ca == "" || *cert == "" || *key == "" || *fonts == "" {
		return e.fail("render-worker serve", usageError("--ca, --cert, --key and --fonts are required"))
	}
	flutterPath, code, ok := e.locateFlutter(*flutter)
	if !ok {
		return code
	}
	fontData, err := plxgolden.LoadFonts(*fonts)
	if err != nil {
		return e.fail("render-worker serve", err)
	}
	tlsConfig, err := httpx.InternalServerTLS(httpx.InternalTLSOptions{CAFile: *ca, CertFile: *cert, KeyFile: *key})
	if err != nil {
		return e.fail("render-worker serve", err)
	}
	h, err := renderworker.New(renderworker.Options{
		Flutter: flutterPath, Fonts: fontData, Runtime: plxtest.Dependency{Version: runtimeConstraint, Path: *runtime},
		WorkDir: *work, Log: e.stderr, Concurrency: *concurrency,
	})
	if err != nil {
		return e.fail("render-worker serve", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log := slog.New(slog.NewJSONHandler(e.stderr, nil)).With("component", "render-worker")
	if err := renderworker.Serve(ctx, *listen, tlsConfig, h, log); err != nil {
		return e.fail("render-worker serve", err)
	}
	return exitOK
}
