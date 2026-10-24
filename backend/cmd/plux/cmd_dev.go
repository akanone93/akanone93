// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/pluxv1"
)

// devUsage lists the subcommands of plux dev.
const devUsage = `Usage: plux dev [--app id] [--env key] [--dir path]
       plux dev <command> [flags]

Without a command, opens a dev session, prints the pairing link, pushes every saved
JSON document to the paired devices and ends the session on Ctrl-C (DEV-041).

Commands:
  logs                          Watch the events of the session's devices
  trace export | show           Save the retained events as a trace file; read one offline
  open <route>                  Open a screen on a device
  set <scope> <path> <json>     Set a state value
  mock <source> <state>         Switch a data-source mock
  clock set|advance|reset       Fake the device's clock
  lifecycle <event>             Simulate pause, resume, inactive, detached or memory-pressure
  network <profile>             Apply a network profile
  security-detect <kind>        Make plux_security report a detection
  pxl <expression>              Evaluate a PXL expression against the live state
  screenshot --out file.png     Capture the screen
  inspect <node-path>           Highlight a node
  overlay <name> on|off         Switch frame-chart, rebuild-counters, touch-targets or contrast

Run 'plux dev <command> -h' for the flags of a command.
`

const (
	// devPollInterval is how often the project directory is scanned.
	devPollInterval = 50 * time.Millisecond
	// devPairingMargin is how long before a pairing token expires an unpaired
	// session asks for a fresh one.
	devPairingMargin = 10 * time.Second
	// devResultWait bounds the wait for a device's answer to a command.
	devResultWait = 10 * time.Second
	// devSessionFile is where plux dev records its session, relative to the
	// project directory, so that the other commands find it.
	devSessionFile = ".plux/dev-session"
)

// dev runs plux dev (DEV-041, DEV-021, DEV-011, DEV-031).
func (e env) dev(args []string) int {
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		sub := map[string]func([]string) int{
			"logs": e.devLogs, "trace": e.devTrace, "open": e.devOpen, "set": e.devSet, "mock": e.devMock, "clock": e.devClock,
			"lifecycle": e.devLifecycle, "network": e.devNetwork, "security-detect": e.devSecurityDetect, "pxl": e.devPXL,
			"screenshot": e.devScreenshot, "inspect": e.devInspect, "overlay": e.devOverlay,
		}
		if f, ok := sub[args[0]]; ok {
			return f(args[1:])
		}
		if args[0] == "help" {
			_, _ = fmt.Fprint(e.stdout, devUsage)
			return exitOK
		}
		_, _ = fmt.Fprint(e.stderr, devUsage)
		return exitUsage
	}
	return e.devSession(args)
}

// multiFlag collects a repeatable flag.
type multiFlag []string

func (m *multiFlag) String() string { return strings.Join(*m, ",") }

func (m *multiFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
}

// devCall is the target of a plux dev command: the common flags and the
// session and device they name.
type devCall struct {
	c       common
	session string
	device  string
}

// devParse parses the flags of a plux dev command; extra registers the
// flags of the command itself.
func (e env) devParse(name string, args []string, usage string, positional int, extra func(*flag.FlagSet)) (*devCall, []string, int, bool) {
	d := &devCall{}
	set := flag.NewFlagSet("dev "+name, flag.ContinueOnError)
	d.c.register(set, true)
	set.StringVar(&d.session, "session", "", "the dev session `id` (default: the session plux dev started in the project)")
	set.StringVar(&d.device, "device", "", "the session `device` to address (default: every device)")
	if extra != nil {
		extra(set)
	}
	pos, code, ok := parseFn(set, args, e.stderr, usage, positional)
	return d, pos, code, ok
}

// connect resolves the server, the credential and the session.
func (e env) devConnect(d *devCall) (*clients, string, error) {
	if err := d.c.resolve(); err != nil {
		return nil, "", err
	}
	cl, err := e.connect(d.c, true)
	if err != nil {
		return nil, "", err
	}
	id := d.session
	if id == "" {
		data, err := os.ReadFile(filepath.Join(d.c.dir, filepath.FromSlash(devSessionFile))) //nolint:gosec // G304: the user's own project.
		if err != nil {
			return nil, "", usageError("no session: pass --session or start one with plux dev")
		}
		id = strings.TrimSpace(string(data))
	}
	return cl, id, nil
}

// devOut serialises the output of the goroutines of a dev session.
type devOut struct {
	mu sync.Mutex
	w  io.Writer
}

func (o *devOut) printf(format string, args ...any) {
	o.mu.Lock()
	defer o.mu.Unlock()
	_, _ = fmt.Fprintf(o.w, format, args...)
}

// devSession opens a session and pushes the saved documents (DEV-041,
// DEV-002, DEV-010).
func (e env) devSession(args []string) int {
	var c common
	set := flag.NewFlagSet("dev", flag.ContinueOnError)
	c.register(set, true)
	set.StringVar(&c.dir, "dir", ".", "the project `directory` to watch")
	envKey := set.String("env", "", "the environment `key` (default plux.json, then development)")
	if _, code, ok := parse(set, args, e.stderr, devUsage, 0); !ok {
		return code
	}
	if c.json {
		return e.fail("dev", usageError("--json is for the other plux dev commands; the session prints for people"))
	}
	t, err := e.fnConnect(c, *envKey)
	if err != nil {
		return e.fail("dev", err)
	}
	ctx, cancel := e.context()
	defer cancel()
	created, err := t.cl.dev.CreateSession(ctx, connect.NewRequest(&pluxv1.DevSessionServiceCreateSessionRequest{AppId: t.app, EnvironmentId: t.envID}))
	if err != nil {
		return e.fail("dev", err)
	}
	s := &devRun{e: e, cl: t.cl, root: c.dir, out: &devOut{w: e.stdout}, id: created.Msg.GetSession().GetId()}
	file := filepath.Join(c.dir, filepath.FromSlash(devSessionFile))
	s.remember(file)
	defer func() { _ = os.Remove(file) }()
	s.out.printf("session %s (expires %s)\n", s.id, created.Msg.GetSession().GetExpiresAt().AsTime().UTC().Format(time.RFC3339))
	s.showPairing(created.Msg.GetPairing())
	s.out.printf("watching %s; press Ctrl-C to end the session\n", c.dir)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.watchDevices(ctx)
	}()
	s.loop(ctx, created.Msg.GetPairing().GetExpiresAt().AsTime())
	cancel()
	wg.Wait()

	end, endCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer endCancel()
	if _, err := t.cl.dev.EndSession(end, connect.NewRequest(&pluxv1.DevSessionServiceEndSessionRequest{SessionId: s.id})); err != nil {
		return e.fail("dev", fmt.Errorf("end the session: %w", err))
	}
	s.out.printf("session %s ended\n", s.id)
	return exitOK
}

// devRun is one running dev session.
type devRun struct {
	e    env
	cl   *clients
	root string
	out  *devOut
	id   string
	seen map[string]devStamp
}

// devStamp identifies a version of a file.
type devStamp struct {
	mod  time.Time
	size int64
}

// remember records the session so that the other commands find it.
func (s *devRun) remember(file string) {
	if err := os.MkdirAll(filepath.Dir(file), 0o750); err != nil {
		return
	}
	_ = os.WriteFile(file, []byte(s.id+"\n"), 0o600)
}

// showPairing prints the pairing link, which carries the single-use token,
// and its short code to type instead (DEV-002); both are for the person who
// started the session.
func (s *devRun) showPairing(p *pluxv1.DevPairing) {
	expires := p.GetExpiresAt().AsTime()
	s.out.printf("pairing link: %s\n", p.GetUri())
	if code := p.GetCode(); code != "" {
		s.out.printf("Code: %s (expires in %s)\n", code, countdown(time.Until(expires)))
	}
	s.out.printf("open the link or type the code on the device (valid until %s)\n", expires.UTC().Format(time.RFC3339))
}

// countdown shows a duration to the second as minutes and seconds, 2:00.
func countdown(d time.Duration) string {
	secs := max(int64(d.Round(time.Second)/time.Second), 0)
	return fmt.Sprintf("%d:%02d", secs/60, secs%60)
}

// loop scans the project and refreshes the pairing token until ctx ends.
func (s *devRun) loop(ctx context.Context, expires time.Time) {
	s.seen = map[string]devStamp{}
	s.scan() // the documents already there are the baseline, not changes
	tick := time.NewTicker(devPollInterval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		if changed := s.scan(); len(changed) > 0 {
			s.push(ctx, changed)
		}
		if time.Until(expires) < devPairingMargin {
			expires = s.refreshPairing(ctx, expires)
		}
	}
}

// refreshPairing asks for a new pairing token while no device is paired.
// It returns when the next one is due.
func (s *devRun) refreshPairing(ctx context.Context, expires time.Time) time.Time {
	devices, err := s.cl.dev.ListDevices(ctx, connect.NewRequest(&pluxv1.DevSessionServiceListDevicesRequest{SessionId: s.id}))
	if err != nil {
		return expires.Add(devPairingMargin)
	}
	if len(devices.Msg.GetDevices()) > 0 {
		// A paired device needs no token; look again later.
		return time.Now().Add(2 * devPairingMargin)
	}
	fresh, err := s.cl.dev.GetPairing(ctx, connect.NewRequest(&pluxv1.DevSessionServiceGetPairingRequest{SessionId: s.id}))
	if err != nil {
		if ctx.Err() == nil {
			s.out.printf("could not refresh the pairing link: %v\n", err)
		}
		return time.Now().Add(devPairingMargin)
	}
	s.out.printf("the pairing link expired; a new one:\n")
	s.showPairing(fresh.Msg.GetPairing())
	return fresh.Msg.GetPairing().GetExpiresAt().AsTime()
}

// devIgnoredDir reports whether a directory is neither watched nor
// searched: dot-directories and the usual build outputs.
func devIgnoredDir(name string) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	switch name {
	case "build", "dist", "out", "target", "node_modules", "vendor":
		return true
	}
	return false
}

// scan returns the documents that are new or changed since the last scan,
// in path order.
func (s *devRun) scan() []string {
	now := map[string]devStamp{}
	var changed []string
	_ = filepath.WalkDir(s.root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // a file that vanished during the walk is not a change
		}
		if d.IsDir() {
			if p != s.root && devIgnoredDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") || filepath.Ext(d.Name()) != ".json" {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil //nolint:nilerr // see above
		}
		rel, err := filepath.Rel(s.root, p)
		if err != nil {
			return nil //nolint:nilerr // see above
		}
		rel = filepath.ToSlash(rel)
		stamp := devStamp{mod: info.ModTime(), size: info.Size()}
		now[rel] = stamp
		if old, ok := s.seen[rel]; !ok || old != stamp {
			changed = append(changed, rel)
		}
		return nil
	})
	s.seen = now
	slices.Sort(changed)
	return changed
}

// pluginKeyOf returns the key of the plugin a document path belongs to.
func pluginKeyOf(rel string) string {
	parts := strings.Split(rel, "/")
	if len(parts) > 2 && parts[0] == "plugins" {
		return parts[1]
	}
	return ""
}

// push sends the changed documents as whole-document patches and prints
// what each device did with them (DEV-010).
func (s *devRun) push(ctx context.Context, paths []string) {
	var patches []*pluxv1.DevDocumentPatch
	var sent []string
	for _, rel := range paths {
		data, err := os.ReadFile(filepath.Join(s.root, filepath.FromSlash(rel))) //nolint:gosec // G304: the user's own project.
		if err != nil {
			s.out.printf("skipped %s: %v\n", rel, err)
			continue
		}
		if !json.Valid(data) {
			s.out.printf("skipped %s: not valid JSON yet\n", rel)
			continue
		}
		patches = append(patches, &pluxv1.DevDocumentPatch{
			PluginId: pluginKeyOf(rel), Path: rel,
			Patch: []*pluxv1.PatchOp{{Op: "replace", Path: "", Value: data}},
		})
		sent = append(sent, rel)
	}
	if len(patches) == 0 {
		return
	}
	start := time.Now()
	res, err := s.cl.dev.PushPatch(ctx, connect.NewRequest(&pluxv1.DevSessionServicePushPatchRequest{SessionId: s.id, Patches: patches}))
	took := time.Since(start).Round(time.Millisecond)
	if err != nil {
		if ctx.Err() == nil {
			s.out.printf("push refused after %s: %v\n", took, err)
		}
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "pushed %d %s in %s (bundle %s)\n", len(sent), plural(len(sent), "document"), took, shortHash(res.Msg.GetBundleSha256()))
	for _, rel := range sent {
		fmt.Fprintf(&b, "  %s\n", rel)
	}
	for _, r := range res.Msg.GetResults() {
		fmt.Fprintf(&b, "  device %s: %s\n", r.GetDeviceId(), patchResultText(r))
	}
	for _, d := range res.Msg.GetDiagnostics() {
		fmt.Fprintf(&b, "  %s %s: %s\n", strings.ToLower(strings.TrimPrefix(d.GetSeverity().String(), "SEVERITY_")), d.GetCode(), d.GetMessage())
	}
	s.out.printf("%s", b.String())
}

// plural appends an s to a noun unless n is one.
func plural(n int, noun string) string {
	if n == 1 {
		return noun
	}
	return noun + "s"
}

// patchResultText says what a device did with a pushed patch.
func patchResultText(r *pluxv1.DevPatchResult) string {
	status := strings.ToLower(strings.TrimPrefix(r.GetStatus().String(), "DEV_PATCH_STATUS_"))
	status = strings.ReplaceAll(status, "_", " ")
	var b strings.Builder
	b.WriteString(status)
	if len(r.GetPagesApplied()) > 0 {
		b.WriteString(", pages " + strings.Join(r.GetPagesApplied(), ", "))
	}
	if len(r.GetScopesReset()) > 0 {
		b.WriteString(", state reset: " + strings.Join(r.GetScopesReset(), ", "))
	}
	if r.GetError() != nil {
		b.WriteString(", " + devErrorText(r.GetError()))
	}
	return b.String()
}

// watchDevices prints the hot-apply timings and the errors the devices
// report while the session runs.
func (s *devRun) watchDevices(ctx context.Context) {
	stream, err := s.cl.dev.WatchEvents(ctx, connect.NewRequest(&pluxv1.DevSessionServiceWatchEventsRequest{SessionId: s.id}))
	if err != nil {
		return
	}
	defer func() { _ = stream.Close() }()
	for stream.Receive() {
		ev := stream.Msg().GetEvent()
		if k := devEventKind(ev); k == "hot_apply" || k == "error" || k == "security_detection" {
			s.out.printf("%s\n", devEventLine(ev))
		}
	}
}
