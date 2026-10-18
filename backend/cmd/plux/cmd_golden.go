// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"crypto/rand"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"text/tabwriter"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/compiler"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/plxgolden"
	"github.com/nightCode42/plux3/backend/internal/plxtest"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

const goldenUsage = "Usage: plux golden run|approve|status [flags]\n"

// golden runs plux golden run|approve|status (TST-003, ADR-0070).
func (e env) golden(args []string) int {
	if len(args) < 1 {
		_, _ = fmt.Fprint(e.stderr, goldenUsage)
		return exitUsage
	}
	switch args[0] {
	case "run":
		return e.goldenRun(args[1:])
	case "approve":
		return e.goldenApprove(args[1:])
	case "status":
		return e.goldenStatus(args[1:])
	}
	_, _ = fmt.Fprint(e.stderr, goldenUsage)
	return exitUsage
}

// goldenTarget is the app a golden command acts on, with its client.
type goldenTarget struct {
	c      common
	client pluxv1connect.GoldenServiceClient
}

// goldenConnect resolves the common flags and connects.
func (e env) goldenConnect(c common) (goldenTarget, error) {
	if err := c.resolve(); err != nil {
		return goldenTarget{}, err
	}
	if err := needApp(c); err != nil {
		return goldenTarget{}, err
	}
	cl, err := e.connect(c, true)
	if err != nil {
		return goldenTarget{}, err
	}
	client := pluxv1connect.NewGoldenServiceClient(e.httpClient, c.server, connect.WithInterceptors(headers{token: cl.token, org: c.org}))
	return goldenTarget{c: c, client: client}, nil
}

// goldenHarnessError is a failure reported with its PLX code.
func (e env) goldenHarnessError(cmd string, code plxerr.Code, format string, args ...any) int {
	d := plxerr.NewDiagnostic(code, plxerr.Location{}, format, args...)
	_, _ = fmt.Fprintf(e.stderr, "%s golden %s: %s\n", name, cmd, d.String())
	return exitFailed
}

// goldenRun compiles the project, renders every case with the real runtime,
// compares the images with the server's baselines and, with --upload,
// records the run.
func (e env) goldenRun(args []string) int {
	var c common
	set := flag.NewFlagSet("golden run", flag.ContinueOnError)
	c.register(set, true)
	plugin := set.String("plugin", "", "render only the plugin with this `key`")
	upload := set.Bool("upload", false, "record the run on the server")
	flutter := set.String("flutter", "", "the `path` of the Flutter executable (default flutter on the PATH)")
	rt := set.String("runtime", "", "use the plux_flutter checkout in `dir` instead of the published package")
	work := set.String("work", "", "generate the Flutter test project in `dir` and keep it (default a temporary directory)")
	fonts := set.String("fonts", "", "the vendored fonts `dir` (default schema/goldens/fonts of the checkout above the project)")
	out := set.String("out", "", "write the images that differ to `dir` (default .plux/golden in the project)")
	if _, code, ok := parse(set, args, e.stderr, "Usage: plux golden run [-C dir] [--app id] [--plugin key] [--upload] [--flutter path]\n\n"+
		"Renders every page in every device frame, locale, theme and text scale with the real\n"+
		"runtime, compares the images with the approved baselines and, with --upload, records the\n"+
		"run on the server. Exit codes: 0 no differences, 1 differences or a failure.", 0); !ok {
		return code
	}
	t, err := e.goldenConnect(c)
	if err != nil {
		return e.fail("golden run", err)
	}
	res, err := compileProject(t.c.dir, false, nil)
	if err != nil {
		return e.fail("golden run", err)
	}
	for _, d := range res.Diagnostics {
		_, _ = fmt.Fprintln(e.stderr, d.String())
	}
	if res.Diagnostics.HasErrors() {
		return exitFailed
	}
	flutterPath := *flutter
	if flutterPath == "" {
		p, err := exec.LookPath("flutter")
		if err != nil {
			return e.goldenHarnessError("run", plxerr.FlutterSDKUnavailable, "flutter is not on the PATH; install it or pass --flutter")
		}
		flutterPath = p
	}
	fontDir := *fonts
	if fontDir == "" {
		found, ok := plxgolden.FindFonts(t.c.dir)
		if !ok {
			cwd, _ := os.Getwd()
			found, ok = plxgolden.FindFonts(cwd)
		}
		if !ok {
			return e.goldenHarnessError("run", plxerr.GoldenHarnessFailed, "the vendored fonts (%s) were not found; pass --fonts", plxgolden.FontsPath)
		}
		fontDir = found
	}
	fontData, err := plxgolden.LoadFonts(fontDir)
	if err != nil {
		return e.goldenHarnessError("run", plxerr.GoldenHarnessFailed, "%v", err)
	}
	workDir := *work
	if workDir == "" {
		tmp, err := os.MkdirTemp("", "plux-golden-")
		if err != nil {
			return e.fail("golden run", err)
		}
		defer func() { _ = os.RemoveAll(tmp) }()
		workDir = tmp
	}
	workDir, err = filepath.Abs(workDir)
	if err != nil {
		return e.fail("golden run", err)
	}
	plan, err := plxgolden.Prepare(plxgolden.Options{
		Result: res, Plugin: *plugin, Fonts: fontData, Entropy: rand.Reader,
		Runtime: plxtest.Dependency{Version: runtimeConstraint, Path: *rt},
	}, filepath.Join(workDir, "_images"))
	if err != nil {
		return e.fail("golden run", err)
	}
	rendered, err := plxgolden.Execute(context.Background(), plan, workDir, flutterPath, e.stderr, limits.Defaults())
	if err != nil {
		return e.fail("golden run", err)
	}
	ctx := context.Background()
	var verdicts []plxgolden.Verdict
	for _, key := range pluginsOf(rendered) {
		base, err := t.client.GetBaselines(ctx, connect.NewRequest(&pluxv1.GetBaselinesRequest{AppId: t.c.app, PluginKey: key}))
		if err != nil {
			return e.fail("golden run", err)
		}
		verdicts = append(verdicts, plxgolden.Compare(filterPlugin(rendered, key), base.Msg.GetImageSha256())...)
	}
	outDir := *out
	if outDir == "" {
		outDir = filepath.Join(t.c.dir, ".plux", "golden")
	}
	differing, err := writeDifferences(outDir, verdicts)
	if err != nil {
		return e.fail("golden run", err)
	}
	var text strings.Builder
	tw := tabwriter.NewWriter(&text, 0, 4, 2, ' ', 0)
	view := map[string]any{"cases": len(verdicts), "differences": differing}
	runs := map[string]string{}
	for _, key := range plxgolden.Plugins(verdicts) {
		vs := plxgolden.ForPlugin(verdicts, key)
		counts := map[plxgolden.Status]int{}
		for _, v := range vs {
			counts[v.Status]++
		}
		line := fmt.Sprintf("%s\t%d cases\t%d match\t%d new\t%d changed", pluginLabel(key), len(vs), counts[plxgolden.Match], counts[plxgolden.New], counts[plxgolden.Changed])
		if *upload {
			id, err := e.goldenUpload(ctx, t, res, key, vs)
			if err != nil {
				return e.fail("golden run", err)
			}
			runs[key] = id
			line += "\trun " + id
		}
		_, _ = fmt.Fprintln(tw, line)
	}
	_ = tw.Flush()
	if *upload {
		view["runs"] = runs
	}
	if differing > 0 {
		fmt.Fprintf(&text, "%d case(s) differ from the approved baselines; images are in %s.\n", differing, outDir)
	}
	if code := e.emit(t.c.json, view, text.String()); code != exitOK {
		return code
	}
	if differing > 0 {
		return exitFailed
	}
	return exitOK
}

func pluginLabel(key string) string {
	if key == "" {
		return "(app)"
	}
	return key
}

func pluginsOf(rs []plxgolden.Rendered) []string {
	seen := map[string]bool{}
	for _, r := range rs {
		seen[r.Plugin] = true
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func filterPlugin(rs []plxgolden.Rendered, key string) []plxgolden.Rendered {
	var out []plxgolden.Rendered
	for _, r := range rs {
		if r.Plugin == key {
			out = append(out, r)
		}
	}
	return out
}

// writeDifferences writes the images of the cases that differ below dir and
// returns how many there are.
func writeDifferences(dir string, vs []plxgolden.Verdict) (int, error) {
	n := 0
	for _, v := range vs {
		if v.Status == plxgolden.Match {
			continue
		}
		n++
		file := filepath.Join(dir, pluginLabelDir(v.Plugin), strings.ReplaceAll(v.ID, "/", "__")+".png")
		if err := os.MkdirAll(filepath.Dir(file), 0o750); err != nil {
			return 0, fmt.Errorf("write %s: %w", file, err)
		}
		if err := writeFile(file, v.PNG); err != nil {
			return 0, err
		}
	}
	return n, nil
}

func pluginLabelDir(key string) string {
	if key == "" {
		return "_app"
	}
	return key
}

// goldenUpload records the run of one plugin.
func (e env) goldenUpload(ctx context.Context, t goldenTarget, res *compiler.Result, key string, vs []plxgolden.Verdict) (string, error) {
	bundle := res.App
	if key != "" {
		bundle = nil
		for _, p := range res.Plugins {
			if p.Key == key {
				bundle = p
			}
		}
	}
	if bundle == nil {
		return "", fmt.Errorf("the compilation made no bundle for plugin %q", key)
	}
	req := &pluxv1.UploadRunRequest{
		AppId: t.c.app, PluginKey: key, BundleSha256: bundle.Hash[:], AppSha256: res.App.Hash[:],
		Canonical: runtime.GOOS == "linux", Renderer: runtime.GOOS + "/" + runtime.GOARCH,
	}
	for _, v := range vs {
		c := &pluxv1.GoldenCase{
			CaseId: v.ID, Frame: v.Frame.ID, Locale: v.Locale, Theme: v.Theme, TextScale: v.Scale, ImageSha256: v.SHA256,
		}
		if v.Status != plxgolden.Match {
			c.Image = v.PNG
		}
		req.Cases = append(req.Cases, c)
	}
	out, err := t.client.UploadRun(ctx, connect.NewRequest(req))
	if err != nil {
		return "", err //nolint:wrapcheck // reported by the caller
	}
	return out.Msg.GetRunId(), nil
}

// goldenApprove promotes the images of a run's differences to baselines.
func (e env) goldenApprove(args []string) int {
	var c common
	set := flag.NewFlagSet("golden approve", flag.ContinueOnError)
	c.register(set, true)
	run := set.String("run", "", "the `id` of the run (printed by plux golden run --upload)")
	var cases stringList
	set.Var(&cases, "case", "approve only the case with this `id`; repeatable (default every difference)")
	if _, code, ok := parse(set, args, e.stderr, "Usage: plux golden approve --run id [--case id]... [-C dir] [--app id]\n\n"+
		"Approves the images of a run as the baselines. It needs goldens.approve and a second factor.", 0); !ok {
		return code
	}
	if *run == "" {
		return e.fail("golden approve", usageError("no run: pass --run"))
	}
	t, err := e.goldenConnect(c)
	if err != nil {
		return e.fail("golden approve", err)
	}
	out, err := t.client.Approve(context.Background(), connect.NewRequest(&pluxv1.ApproveRequest{AppId: t.c.app, RunId: *run, CaseIds: cases}))
	if err != nil {
		return e.fail("golden approve", stepUp(err))
	}
	return e.emit(t.c.json, map[string]any{"approved": out.Msg.GetApprovedCaseIds()}, fmt.Sprintf("Approved %d case(s).", len(out.Msg.GetApprovedCaseIds())))
}

// goldenStatus says what the publish gate sees for the project's bundles.
func (e env) goldenStatus(args []string) int {
	var c common
	set := flag.NewFlagSet("golden status", flag.ContinueOnError)
	c.register(set, true)
	plugin := set.String("plugin", "", "the plugin `key` (default every plugin)")
	if _, code, ok := parse(set, args, e.stderr, "Usage: plux golden status [-C dir] [--app id] [--plugin key] [--json]\n\n"+
		"Compiles the project and asks the server whether a canonical golden run exists for the exact\n"+
		"bundles and has differences nobody approved. Exit codes: 0 every plugin is approved, 1 not.", 0); !ok {
		return code
	}
	t, err := e.goldenConnect(c)
	if err != nil {
		return e.fail("golden status", err)
	}
	res, err := compileProject(t.c.dir, false, nil)
	if err != nil {
		return e.fail("golden status", err)
	}
	if res.Diagnostics.HasErrors() || res.App == nil {
		for _, d := range res.Diagnostics {
			_, _ = fmt.Fprintln(e.stderr, d.String())
		}
		return exitFailed
	}
	var (
		text    strings.Builder
		views   []map[string]any
		blocked bool
	)
	for _, p := range res.Plugins {
		if *plugin != "" && p.Key != *plugin {
			continue
		}
		out, err := t.client.GetStatus(context.Background(), connect.NewRequest(&pluxv1.GetGoldenStatusRequest{
			AppId: t.c.app, PluginKey: p.Key, BundleSha256: p.Hash[:], AppSha256: res.App.Hash[:],
		}))
		if err != nil {
			return e.fail("golden status", err)
		}
		m := out.Msg
		state := "approved"
		switch {
		case !m.GetFound():
			state, blocked = "no canonical run (PLX-3061)", true
		case m.GetDifferences() > 0:
			state, blocked = fmt.Sprintf("%d of %d cases unapproved (PLX-3062)", m.GetDifferences(), m.GetCases()), true
		}
		views = append(views, map[string]any{"plugin": p.Key, "found": m.GetFound(), "run": m.GetRunId(), "cases": m.GetCases(), "differences": m.GetDifferences()})
		fmt.Fprintf(&text, "%s: %s\n", p.Key, state)
	}
	if code := e.emit(t.c.json, map[string]any{"plugins": views}, text.String()); code != exitOK {
		return code
	}
	if blocked {
		return exitFailed
	}
	return exitOK
}

// stringList is a repeatable string flag.
type stringList []string

func (s *stringList) String() string     { return strings.Join(*s, ",") }
func (s *stringList) Set(v string) error { *s = append(*s, v); return nil }
