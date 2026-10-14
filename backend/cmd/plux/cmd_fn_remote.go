// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/nightCode42/plux3/backend/internal/pluxv1"
)

// uploadChunk is the size of one message of a source upload (FN-016).
const uploadChunk = 256 << 10

// cliSession names the CLI in the session field of the function calls.
const cliSession = "plux-cli"

// fnTarget is the app and environment a function command acts on.
type fnTarget struct {
	cl    *clients
	app   string
	envID string
}

// fnConnect resolves the common flags and the environment key.
func (e env) fnConnect(c common, envKey string) (fnTarget, error) {
	if err := c.resolve(); err != nil {
		return fnTarget{}, err
	}
	if err := needApp(c); err != nil {
		return fnTarget{}, err
	}
	cl, err := e.connect(c, true)
	if err != nil {
		return fnTarget{}, err
	}
	id, err := environmentID(context.Background(), cl, c.app, firstOf(envKey, projectEnvironment(c.dir), "development"))
	if err != nil {
		return fnTarget{}, err
	}
	return fnTarget{cl: cl, app: c.app, envID: id}, nil
}

// deployResult is what plux fn deploy reports with --json.
type deployResult struct {
	Build   string                       `json:"build"`
	State   string                       `json:"state"`
	Tests   []*pluxv1.FunctionTestResult `json:"tests"`
	Version *pluxv1.FunctionVersion      `json:"version,omitempty"`
}

// fnDeploy uploads the source, builds it on the server and deploys the
// version (FN-006, FN-016, FN-031).
func (e env) fnDeploy(args []string) int {
	var c common
	set := flag.NewFlagSet("fn deploy", flag.ContinueOnError)
	c.register(set, true)
	envKey := set.String("env", "", "the environment `key` (default plux.json, then development)")
	alias := set.String("alias", "", "point this `alias` at the new version")
	pos, code, ok := parseFn(set, args, e.stderr, "Usage: plux fn deploy <name> [--env key] [--alias name] [-C dir] [--app id] [--json]\n\nPacks the crate with its vendored dependencies, uploads it, waits for the server's build and tests, and deploys the version. Production needs a second factor: sign in again with plux login when the server asks.", 1)
	if !ok {
		return code
	}
	name := pos[0]
	if err := checkFunctionName(name); err != nil {
		return e.fail("fn deploy", err)
	}
	t, err := e.fnConnect(c, *envKey)
	if err != nil {
		return e.fail("fn deploy", err)
	}
	ctx, cancel := e.context()
	defer cancel()
	archive, err := e.packFunction(ctx, c.dir, name)
	if err != nil {
		return e.fail("fn deploy", err)
	}
	sum := sha256.Sum256(archive)
	_, _ = fmt.Fprintf(e.stderr, "packed %d bytes, sha256 %s\n", len(archive), hex.EncodeToString(sum[:]))
	source, err := e.upload(ctx, t, name, archive)
	if err != nil {
		return e.fail("fn deploy", err)
	}
	built, err := t.cl.fn.Build(ctx, connect.NewRequest(&pluxv1.BuildRequest{SourceId: source}))
	if err != nil {
		return e.fail("fn deploy", stepUp(err))
	}
	build, err := e.awaitBuild(ctx, t.cl, built.Msg.GetBuild().GetId())
	if err != nil {
		return e.fail("fn deploy", err)
	}
	result := deployResult{Build: build.GetId(), State: stateName(build.GetState()), Tests: build.GetTestResults()}
	failed := e.reportBuild(build)
	if failed {
		e.emitJSONOnly(c.json, result)
		return exitFailed
	}
	dep, err := t.cl.fn.Deploy(ctx, connect.NewRequest(&pluxv1.DeployRequest{BuildId: build.GetId(), Alias: *alias, Session: cliSession}))
	if err != nil {
		return e.fail("fn deploy", stepUp(err))
	}
	v := dep.Msg.GetVersion()
	result.Version = v
	text := fmt.Sprintf("deployed %s version %d (%s, module %s)", name, v.GetVersion(), v.GetPlacement(), shortHash(v.GetModuleSha256()))
	if *alias != "" {
		text += ", alias " + *alias
	}
	return e.emit(c.json, result, text)
}

// emitJSONOnly prints v with --json and nothing otherwise.
func (e env) emitJSONOnly(jsonOut bool, v any) {
	if jsonOut {
		e.emit(true, v, "")
	}
}

// stepUp explains a refusal for a missing second factor, which the server
// returns for a production deploy (FN-007).
func stepUp(err error) error {
	var ce *connect.Error
	if errors.As(err, &ce) && strings.Contains(ce.Message(), "second factor") {
		return fmt.Errorf("%w\nstep-up required: run plux login to sign in with a second factor, then repeat the command", err)
	}
	return err
}

// packFunction vendors the crate when needed and packs its source.
func (e env) packFunction(ctx context.Context, project, name string) ([]byte, error) {
	dir := crateDir(project, name)
	if err := e.vendorCrate(ctx, dir); err != nil {
		return nil, err
	}
	files, err := collectSource(dir)
	if err != nil {
		return nil, err
	}
	return packArchive(files)
}

// upload streams the archive in chunks and returns the source identifier.
func (e env) upload(ctx context.Context, t fnTarget, name string, archive []byte) (string, error) {
	stream := t.cl.fn.UploadSource(ctx)
	if err := stream.Send(&pluxv1.UploadSourceRequest{AppId: t.app, EnvironmentId: t.envID, Name: name, Session: cliSession}); err != nil {
		_, _ = stream.CloseAndReceive()
		return "", fmt.Errorf("upload the source: %w", err)
	}
	for rest := archive; len(rest) > 0; {
		n := min(len(rest), uploadChunk)
		if err := stream.Send(&pluxv1.UploadSourceRequest{Chunk: rest[:n]}); err != nil {
			_, _ = stream.CloseAndReceive()
			return "", fmt.Errorf("upload the source: %w", err)
		}
		rest = rest[n:]
	}
	res, err := stream.CloseAndReceive()
	if err != nil {
		return "", fmt.Errorf("upload the source: %w", err)
	}
	_, _ = fmt.Fprintf(e.stderr, "uploaded source %s\n", res.Msg.GetSourceId())
	return res.Msg.GetSourceId(), nil
}

// awaitBuild polls the build with a growing wait until it ends.
func (e env) awaitBuild(ctx context.Context, cl *clients, id string) (*pluxv1.Build, error) {
	wait := e.pollInterval
	if wait <= 0 {
		wait = 500 * time.Millisecond
	}
	most, last := wait*8, ""
	for {
		res, err := cl.fn.GetBuild(ctx, connect.NewRequest(&pluxv1.GetBuildRequest{BuildId: id}))
		if err != nil {
			return nil, err //nolint:wrapcheck // reported as is
		}
		b := res.Msg.GetBuild()
		if s := stateName(b.GetState()); s != last {
			_, _ = fmt.Fprintf(e.stderr, "build %s: %s\n", id, s)
			last = s
		}
		if b.GetState() == pluxv1.BuildState_BUILD_STATE_SUCCEEDED || b.GetState() == pluxv1.BuildState_BUILD_STATE_FAILED {
			return b, nil
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("waiting for build %s: %w", id, ctx.Err())
		case <-time.After(wait):
		}
		wait = min(wait*2, most)
	}
}

// reportBuild prints the diagnostics and test results of a build and reports
// whether the function must not be deployed.
func (e env) reportBuild(b *pluxv1.Build) bool {
	for _, d := range b.GetDiagnostics() {
		where := d.GetLocation().GetFile()
		if where == "" {
			where = "build"
		}
		_, _ = fmt.Fprintf(e.stderr, "%s: %s %s: %s\n", where, strings.ToLower(strings.TrimPrefix(d.GetSeverity().String(), "SEVERITY_")), d.GetCode(), d.GetMessage())
		if d.GetFix() != "" {
			_, _ = fmt.Fprintf(e.stderr, "  fix: %s\n", d.GetFix())
		}
	}
	failedTests := 0
	for _, t := range b.GetTestResults() {
		verdict := "pass"
		if !t.GetPassed() {
			verdict = "FAIL"
			failedTests++
		}
		line := fmt.Sprintf("test %s: %s", t.GetName(), verdict)
		if t.GetMessage() != "" {
			line += " (" + t.GetMessage() + ")"
		}
		_, _ = fmt.Fprintln(e.stderr, line)
	}
	switch {
	case b.GetState() == pluxv1.BuildState_BUILD_STATE_FAILED:
		_, _ = fmt.Fprintln(e.stderr, "the build failed; nothing was deployed")
		return true
	case failedTests > 0:
		_, _ = fmt.Fprintf(e.stderr, "%d stored test cases failed; nothing was deployed (FN-031)\n", failedTests)
		return true
	}
	return false
}

// stateName is the lower-case name of a build state.
func stateName(s pluxv1.BuildState) string {
	return strings.ToLower(strings.TrimPrefix(s.String(), "BUILD_STATE_"))
}

// shortHash shortens a hex digest for display.
func shortHash(h string) string {
	if len(h) > 12 {
		return h[:12]
	}
	return h
}

// fnAlias points an alias at a version (FN-006).
func (e env) fnAlias(args []string) int {
	var c common
	set := flag.NewFlagSet("fn alias", flag.ContinueOnError)
	c.register(set, true)
	envKey := set.String("env", "", "the environment `key` (default plux.json, then development)")
	pos, code, ok := parseFn(set, args, e.stderr, "Usage: plux fn alias <name> <alias> <version> [--env key] [-C dir] [--app id] [--json]\n\nPoints an alias at an existing version.", 3)
	if !ok {
		return code
	}
	version, err := strconv.ParseUint(pos[2], 10, 32)
	if err != nil || version == 0 {
		return e.fail("fn alias", usageError("the version is a positive number"))
	}
	t, err := e.fnConnect(c, *envKey)
	if err != nil {
		return e.fail("fn alias", err)
	}
	res, err := t.cl.fn.SetAlias(context.Background(), connect.NewRequest(&pluxv1.SetAliasRequest{
		AppId: t.app, EnvironmentId: t.envID, Name: pos[0], Alias: &pluxv1.Alias{Name: pos[1], Version: uint32(version)}, Session: cliSession,
	}))
	if err != nil {
		return e.fail("fn alias", stepUp(err))
	}
	return e.emit(c.json, res.Msg.GetAliases(), aliasText(res.Msg.GetAliases()))
}

// fnPromote copies a version to another environment without rebuilding it
// (FN-006, FN-007, FN-031).
func (e env) fnPromote(args []string) int {
	var c common
	set := flag.NewFlagSet("fn promote", flag.ContinueOnError)
	c.register(set, true)
	from := set.String("from", "", "the `environment` key that holds the version")
	to := set.String("to", "", "the `environment` key that receives the version")
	alias := set.String("alias", "", "point this `alias` at the version in the target")
	pos, code, ok := parseFn(set, args, e.stderr, "Usage: plux fn promote <name>@<version> --from <env> --to <env> [--alias name] [-C dir] [--app id] [--json]\n\nCopies a version to another environment without rebuilding it: the number, the module and the signature stay the same. A release can be promoted to an environment once every function version it pins is there. Production needs a second factor: sign in again with plux login when the server asks.", 1)
	if !ok {
		return code
	}
	name, number, found := strings.Cut(pos[0], "@")
	version, err := strconv.ParseUint(number, 10, 32)
	if !found || err != nil || version == 0 {
		return e.fail("fn promote", usageError("name the function as <name>@<version>, with a positive version"))
	}
	if *from == "" || *to == "" {
		return e.fail("fn promote", usageError("--from and --to name the environments"))
	}
	t, err := e.fnConnect(c, *from)
	if err != nil {
		return e.fail("fn promote", err)
	}
	toID, err := environmentID(context.Background(), t.cl, t.app, *to)
	if err != nil {
		return e.fail("fn promote", err)
	}
	res, err := t.cl.fn.PromoteFunction(context.Background(), connect.NewRequest(&pluxv1.PromoteFunctionRequest{
		AppId: t.app, FromEnvironmentId: t.envID, ToEnvironmentId: toID, Name: name, Version: uint32(version), Alias: *alias, Session: cliSession,
	}))
	if err != nil {
		return e.fail("fn promote", stepUp(err))
	}
	v := res.Msg.GetVersion()
	text := fmt.Sprintf("promoted %s version %d from %s to %s (%s, module %s)", name, v.GetVersion(), *from, *to, v.GetPlacement(), shortHash(v.GetModuleSha256()))
	if *alias != "" {
		text += ", alias " + *alias
	}
	return e.emit(c.json, v, text)
}

// aliasText lists aliases as "name -> version".
func aliasText(as []*pluxv1.Alias) string {
	lines := make([]string, 0, len(as))
	for _, a := range as {
		lines = append(lines, fmt.Sprintf("%s -> %d", a.GetName(), a.GetVersion()))
	}
	return strings.Join(lines, "\n")
}

// fnVersions lists the versions of a function with their aliases.
func (e env) fnVersions(args []string) int {
	var c common
	set := flag.NewFlagSet("fn versions", flag.ContinueOnError)
	c.register(set, true)
	envKey := set.String("env", "", "the environment `key` (default plux.json, then development)")
	pos, code, ok := parseFn(set, args, e.stderr, "Usage: plux fn versions <name> [--env key] [-C dir] [--app id] [--json]\n\nLists the versions of a function, newest first, with their aliases.", 1)
	if !ok {
		return code
	}
	t, err := e.fnConnect(c, *envKey)
	if err != nil {
		return e.fail("fn versions", err)
	}
	ctx := context.Background()
	var versions []*pluxv1.FunctionVersion
	for token := ""; ; {
		res, err := t.cl.fn.ListVersions(ctx, connect.NewRequest(&pluxv1.ListVersionsRequest{
			AppId: t.app, EnvironmentId: t.envID, Name: pos[0], Page: &pluxv1.Page{PageSize: 100, PageToken: token},
		}))
		if err != nil {
			return e.fail("fn versions", err)
		}
		versions = append(versions, res.Msg.GetVersions()...)
		if token = res.Msg.GetPage().GetNextPageToken(); token == "" {
			break
		}
	}
	aliases := map[uint32][]string{}
	if got, err := t.cl.fn.GetFunction(ctx, connect.NewRequest(&pluxv1.GetFunctionRequest{AppId: t.app, EnvironmentId: t.envID, Name: pos[0]})); err == nil {
		for _, a := range got.Msg.GetAliases() {
			aliases[a.GetVersion()] = append(aliases[a.GetVersion()], a.GetName())
		}
	}
	var b bytes.Buffer
	w := tabwriter.NewWriter(&b, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "VERSION\tPLACEMENT\tMODULE\tTESTS\tALIASES\tCREATED")
	for _, v := range versions {
		names := aliases[v.GetVersion()]
		slices.Sort(names)
		_, _ = fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\n", v.GetVersion(), v.GetPlacement(), shortHash(v.GetModuleSha256()), passFail(v.GetTestsPassed()),
			strings.Join(names, ","), v.GetCreatedAt().AsTime().Format(time.RFC3339))
	}
	_ = w.Flush()
	return e.emit(c.json, map[string]any{"versions": versions, "aliases": aliases}, b.String())
}

// passFail renders a test verdict.
func passFail(ok bool) string {
	if ok {
		return "passed"
	}
	return "failed"
}

// fnLogs shows the retained logs of a function, oldest first (FN-032).
func (e env) fnLogs(args []string) int {
	var c common
	set := flag.NewFlagSet("fn logs", flag.ContinueOnError)
	c.register(set, true)
	envKey := set.String("env", "", "the environment `key` (default plux.json, then development)")
	version := set.Uint("version", 0, "only this `version`")
	since := set.String("since", "", "only lines newer than this `duration` (such as 15m) or RFC 3339 time")
	limit := set.Int("n", 100, "show at most this many `lines`")
	follow := set.Bool("follow", false, "keep waiting for new lines until interrupted")
	pos, code, ok := parseFn(set, args, e.stderr, "Usage: plux fn logs <name> [--version n] [--since 15m] [-n lines] [--follow] [--env key] [-C dir] [--app id] [--json]\n\nShows the lines a function logged, oldest first. Logs are kept for the retention the operator configures.", 1)
	if !ok {
		return code
	}
	from, err := parseSince(*since, time.Now())
	if err != nil {
		return e.fail("fn logs", err)
	}
	t, err := e.fnConnect(c, *envKey)
	if err != nil {
		return e.fail("fn logs", err)
	}
	ctx, cancel := e.context()
	defer cancel()
	q := logQuery{t: t, name: pos[0], version: uint32(*version)} //nolint:gosec // G115: a version number
	lines, err := q.fetch(ctx, from, max(*limit, 1))
	if err != nil {
		return e.fail("fn logs", err)
	}
	if !*follow {
		return e.emit(c.json, lines, strings.Join(logTexts(lines), "\n"))
	}
	return e.followLogs(ctx, c.json, q, lines)
}

// logQuery is the selection of a function's log lines.
type logQuery struct {
	t       fnTarget
	name    string
	version uint32
}

// fetch returns up to limit lines at or after since, oldest first.
func (q logQuery) fetch(ctx context.Context, since time.Time, limit int) ([]*pluxv1.FunctionLogLine, error) {
	var lines []*pluxv1.FunctionLogLine
	for token := ""; len(lines) < limit; {
		req := &pluxv1.ListLogsRequest{
			AppId: q.t.app, EnvironmentId: q.t.envID, Name: q.name, Version: q.version,
			Page: &pluxv1.Page{PageSize: int32(min(limit-len(lines), 1000)), PageToken: token}, //nolint:gosec // G115: bounded
		}
		if !since.IsZero() {
			req.Since = timestamppb.New(since)
		}
		res, err := q.t.cl.fn.ListLogs(ctx, connect.NewRequest(req))
		if err != nil {
			return nil, err //nolint:wrapcheck // reported as is
		}
		lines = append(lines, res.Msg.GetLines()...)
		if token = res.Msg.GetPage().GetNextPageToken(); token == "" {
			break
		}
	}
	slices.Reverse(lines)
	return lines, nil
}

// logFollower prints each log line once. Lines arrive oldest first and
// `seen` holds only the lines at the newest time, which is all a later fetch
// from that time can repeat.
type logFollower struct {
	e       env
	jsonOut bool
	seen    map[string]bool
	newest  time.Time
}

// show prints the lines it has not printed yet.
func (f *logFollower) show(lines []*pluxv1.FunctionLogLine) {
	for _, l := range lines {
		at := l.GetTime().AsTime()
		key := at.Format(time.RFC3339Nano) + "\x00" + strconv.Itoa(int(l.GetVersion())) + "\x00" + l.GetLevel() + "\x00" + l.GetMessage()
		if f.seen[key] || at.Before(f.newest) {
			continue
		}
		if at.After(f.newest) {
			f.newest = at
			clear(f.seen)
		}
		f.seen[key] = true
		if f.jsonOut {
			f.e.emit(true, l, "")
		} else {
			_, _ = fmt.Fprintln(f.e.stdout, logText(l))
		}
	}
}

// followLogs prints the lines already fetched and then every new one, until
// the context ends.
func (e env) followLogs(ctx context.Context, jsonOut bool, q logQuery, first []*pluxv1.FunctionLogLine) int {
	wait := 2 * e.pollInterval
	if wait <= 0 {
		wait = time.Second
	}
	f := &logFollower{e: e, jsonOut: jsonOut, seen: map[string]bool{}}
	f.show(first)
	for {
		select {
		case <-ctx.Done():
			return exitOK
		case <-time.After(wait):
		}
		lines, err := q.fetch(ctx, f.newest, 1000)
		if err != nil {
			if ctx.Err() != nil {
				return exitOK
			}
			return e.fail("fn logs", err)
		}
		f.show(lines)
	}
}

// logText renders a log line.
func logText(l *pluxv1.FunctionLogLine) string {
	return fmt.Sprintf("%s v%d %-5s %s", l.GetTime().AsTime().Format(time.RFC3339), l.GetVersion(), strings.ToUpper(l.GetLevel()), l.GetMessage())
}

// logTexts renders log lines.
func logTexts(lines []*pluxv1.FunctionLogLine) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = logText(l)
	}
	return out
}

// parseSince reads a duration back from now, or an RFC 3339 time; empty is
// the beginning of the retention.
func parseSince(v string, now time.Time) (time.Time, error) {
	if v == "" {
		return time.Time{}, nil
	}
	if d, err := time.ParseDuration(v); err == nil && d > 0 {
		return now.Add(-d), nil
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return time.Time{}, usageError("--since is a duration such as 15m or an RFC 3339 time")
	}
	return t, nil
}
