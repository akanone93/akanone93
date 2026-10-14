// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/nightCode42/plux3/backend/internal/builder"
	"github.com/nightCode42/plux3/backend/internal/fnhost"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/schema/uuid7"
)

// fnUsage lists the subcommands of plux fn.
const fnUsage = `Usage: plux fn <command> [flags]

Commands:
  new <name>                      Scaffold functions/<name>/ with a typed handler and a test case
  build <name>                    Build the module locally, without a sandbox (development only)
  test <name>                     Run cargo test, then every stored case on the built module
  invoke <name> --input json      Run the built module locally with the declared limits
  deploy <name>                   Upload the source, build it on the server and deploy the version
  alias <name> <alias> <version>  Point an alias at a version
  versions <name>                 List the versions and their aliases
  logs <name>                     Show the retained logs of a function

Run 'plux fn <command> -h' for the flags of a command.
`

// fn runs plux fn (FN-030, FN-031, FN-032).
func (e env) fn(args []string) int {
	if len(args) > 0 {
		sub := map[string]func([]string) int{
			"new": e.fnNew, "build": e.fnBuild, "test": e.fnTest, "invoke": e.fnInvoke,
			"deploy": e.fnDeploy, "alias": e.fnAlias, "versions": e.fnVersions, "logs": e.fnLogs,
		}
		if f, ok := sub[args[0]]; ok {
			return f(args[1:])
		}
		if args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
			_, _ = fmt.Fprint(e.stdout, fnUsage)
			return exitOK
		}
	}
	_, _ = fmt.Fprint(e.stderr, fnUsage)
	return exitUsage
}

// context returns the context of a long-running command: the environment's,
// else one that ends at an interrupt.
func (e env) context() (context.Context, context.CancelFunc) {
	if e.ctx != nil {
		return context.WithCancel(e.ctx)
	}
	return signal.NotifyContext(context.Background(), os.Interrupt)
}

// parseFn parses the flags of a plux fn command wherever they stand, so that
// both `plux fn deploy --env production name` and `plux fn deploy name --env
// production` work; the standard flag package stops at the first argument.
func parseFn(set *flag.FlagSet, args []string, stderr io.Writer, usage string, positional int) ([]string, int, bool) {
	var flags, rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			rest = append(rest, args[i+1:]...)
			break
		}
		if len(a) < 2 || a[0] != '-' {
			rest = append(rest, a)
			continue
		}
		flags = append(flags, a)
		name, _, hasValue := strings.Cut(strings.TrimLeft(a, "-"), "=")
		if f := set.Lookup(name); f != nil && !hasValue && i+1 < len(args) {
			if b, ok := f.Value.(interface{ IsBoolFlag() bool }); !ok || !b.IsBoolFlag() {
				i++
				flags = append(flags, args[i])
			}
		}
	}
	return parse(set, append(flags, rest...), stderr, usage, positional) //nolint:wrapcheck // parse reports its own failures
}

// localFlags are the flags of the commands that work on a local crate.
type localFlags struct {
	project, sdk string
	json         bool
}

func (l *localFlags) register(set *flag.FlagSet, withSDK bool) {
	set.StringVar(&l.project, "C", ".", "the project `directory` holding functions/")
	set.BoolVar(&l.json, "json", false, "print the result as JSON on stdout")
	if withSDK {
		set.StringVar(&l.sdk, "sdk", "", "the pinned SDK crate `directory` (default $"+sdkEnv+", then the copy next to the binary)")
	}
}

// fnNew scaffolds a function.
func (e env) fnNew(args []string) int {
	var l localFlags
	set := flag.NewFlagSet("fn new", flag.ContinueOnError)
	l.register(set, true)
	placement := set.String("placement", "server", "where the function runs: `server` or device (FN-008)")
	pos, code, ok := parseFn(set, args, e.stderr, "Usage: plux fn new <name> [--placement server|device] [--sdk dir] [-C dir]\n\nCreates functions/<name>/ with Cargo.toml, a typed handler, function.json with a test case and a .cargo/config.toml that resolves plux-fn to the SDK.", 1)
	if !ok {
		return code
	}
	name := pos[0]
	if err := checkFunctionName(name); err != nil {
		return e.fail("fn new", err)
	}
	if *placement != "server" && *placement != "device" {
		return e.fail("fn new", usageError("the placement is server or device"))
	}
	sdk, err := resolveSDK(l.sdk)
	if err != nil {
		return e.fail("fn new", err)
	}
	dir := crateDir(l.project, name)
	if _, err := os.Stat(dir); err == nil {
		return e.fail("fn new", fmt.Errorf("%s exists", dir))
	}
	id, err := uuid7.NewGenerator(time.Now, rand.Reader).New()
	if err != nil {
		return e.fail("fn new", fmt.Errorf("generate an identifier: %w", err))
	}
	files, err := scaffold(name, *placement, id.String(), sdk)
	if err != nil {
		return e.fail("fn new", err)
	}
	for rel, data := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			return e.fail("fn new", fmt.Errorf("create %s: %w", filepath.Dir(p), err))
		}
		if err := os.WriteFile(p, data, 0o600); err != nil {
			return e.fail("fn new", fmt.Errorf("write %s: %w", p, err))
		}
	}
	return e.emit(l.json, map[string]string{"directory": dir, "name": name, "placement": *placement, "id": id.String()},
		fmt.Sprintf("created %s\nnext: plux fn test %s", dir, name))
}

// fnBuild builds a function locally.
func (e env) fnBuild(args []string) int {
	var l localFlags
	set := flag.NewFlagSet("fn build", flag.ContinueOnError)
	l.register(set, true)
	verify := set.Bool("verify", false, "build twice and fail unless the two modules are byte-identical (FN-019)")
	pos, code, ok := parseFn(set, args, e.stderr, "Usage: plux fn build <name> [--verify] [--sdk dir] [-C dir] [--json]\n\nVendors the dependencies and builds the module without a sandbox, for development only: the server rebuilds in its sandbox on deploy. Writes the module, the schemas and the provenance to functions/<name>/"+outputDir+"/.", 1)
	if !ok {
		return code
	}
	ctx, cancel := e.context()
	defer cancel()
	_, _ = fmt.Fprintln(e.stderr, "warning: unsandboxed development build; the server rebuilds in its sandbox on deploy")
	b, err := e.buildFunction(ctx, l.project, pos[0], l.sdk, *verify)
	if err != nil {
		return e.fail("fn build", err)
	}
	a := b.Artifact
	return e.emit(l.json, map[string]any{"module": filepath.Join(b.Dir, moduleName), "moduleSha256": a.ModuleSHA256, "bytes": len(a.Module), "sandboxed": false},
		fmt.Sprintf("built %s (%d bytes, sha256 %s)\nwrote %s", pos[0], len(a.Module), a.ModuleSHA256, b.Dir))
}

// caseOutcome is one stored case on the engines it ran on.
type caseOutcome struct {
	Name        string `json:"name"`
	Wasm        bool   `json:"wasm"`
	Interpreter *bool  `json:"interpreter,omitempty"`
	Message     string `json:"message,omitempty"`
}

// fnTest runs the native tests and every stored case on the built module.
func (e env) fnTest(args []string) int {
	var l localFlags
	set := flag.NewFlagSet("fn test", flag.ContinueOnError)
	l.register(set, true)
	skipNative := set.Bool("skip-native", false, "do not run cargo test")
	pos, code, ok := parseFn(set, args, e.stderr, "Usage: plux fn test <name> [--skip-native] [--sdk dir] [-C dir] [--json]\n\nRuns cargo test natively, builds the module and runs every stored case of function.json on the function host. A device function also runs on the interpreter that stands in for the device's engine; any difference fails (FN-030, FN-015).", 1)
	if !ok {
		return code
	}
	name := pos[0]
	ctx, cancel := e.context()
	defer cancel()
	if err := checkFunctionName(name); err != nil {
		return e.fail("fn test", err)
	}
	if !*skipNative {
		if err := runTool(ctx, crateDir(l.project, name), e.stderr, e.stderr, "cargo", "test", "--quiet"); err != nil {
			return e.fail("fn test", fmt.Errorf("native tests failed: %w", err))
		}
	}
	b, err := e.buildFunction(ctx, l.project, name, l.sdk, false)
	if err != nil {
		return e.fail("fn test", err)
	}
	outcomes, err := runStoredCases(ctx, b.Artifact.Module, b.Decl)
	if err != nil {
		return e.fail("fn test", err)
	}
	failed := 0
	for _, o := range outcomes {
		if !o.Wasm || (o.Interpreter != nil && !*o.Interpreter) {
			failed++
		}
	}
	text := caseTable(outcomes, b.Decl.Placement() == builder.PlacementDevice)
	if code := e.emit(l.json, map[string]any{"function": name, "cases": outcomes, "failed": failed}, text); code != exitOK {
		return code
	}
	if failed > 0 {
		return e.fail("fn test", fmt.Errorf("%d of %d stored cases failed", failed, len(outcomes)))
	}
	return exitOK
}

// runStoredCases runs the stored cases of the declaration on the compiling
// engine and, for a device function, on the interpreter (FN-030). A case
// passes only if the engines agree with the case.
func runStoredCases(ctx context.Context, module []byte, decl *builder.Declaration) ([]caseOutcome, error) {
	set := limits.Defaults()
	run := func(interpreter bool) ([]builder.TestResult, error) {
		engine, err := newEngine(ctx, set, interpreter)
		if err != nil {
			return nil, err
		}
		defer func() { _ = engine.Close(context.WithoutCancel(ctx)) }()
		return builder.RunTests(ctx, engine, module, decl, set) //nolint:wrapcheck // reported as is
	}
	wasm, err := run(false)
	if err != nil {
		return nil, err
	}
	out := make([]caseOutcome, len(wasm))
	for i, r := range wasm {
		out[i] = caseOutcome{Name: r.Name, Wasm: r.Passed, Message: r.Message}
	}
	if decl.Placement() != builder.PlacementDevice {
		return out, nil
	}
	interp, err := run(true)
	if err != nil {
		return nil, err
	}
	for i, r := range interp {
		passed := r.Passed
		out[i].Interpreter = &passed
		if !r.Passed && out[i].Message == "" {
			out[i].Message = "interpreter: " + r.Message
		}
	}
	return out, nil
}

// caseTable renders the outcomes of the stored cases.
func caseTable(outcomes []caseOutcome, withInterpreter bool) string {
	verdict := func(ok bool) string {
		if ok {
			return "pass"
		}
		return "FAIL"
	}
	var b bytes.Buffer
	w := tabwriter.NewWriter(&b, 0, 4, 2, ' ', 0)
	if withInterpreter {
		_, _ = fmt.Fprintln(w, "CASE\tWASM\tINTERPRETER")
	} else {
		_, _ = fmt.Fprintln(w, "CASE\tWASM")
	}
	for _, o := range outcomes {
		if o.Interpreter != nil {
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\n", o.Name, verdict(o.Wasm), verdict(*o.Interpreter))
		} else {
			_, _ = fmt.Fprintf(w, "%s\t%s\n", o.Name, verdict(o.Wasm))
		}
	}
	_ = w.Flush()
	for _, o := range outcomes {
		if o.Message != "" {
			_, _ = fmt.Fprintf(&b, "\n%s: %s", o.Name, o.Message)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// fnInvoke runs the built module locally.
func (e env) fnInvoke(args []string) int {
	var l localFlags
	set := flag.NewFlagSet("fn invoke", flag.ContinueOnError)
	l.register(set, false)
	input := set.String("input", "", "the JSON input, or @file to read it from a file (@- reads standard input)")
	claims := set.String("claims", "", "the verified user claims as JSON, or @file (server functions)")
	pos, code, ok := parseFn(set, args, e.stderr, "Usage: plux fn invoke <name> --input json|@file [--claims json|@file] [-C dir] [--json]\n\nRuns the module that plux fn build wrote on the function host, under the limits and capabilities function.json declares, with an in-memory key-value store. Invoking a deployed function from a device is the runtime's job.", 1)
	if !ok {
		return code
	}
	in, err := e.jsonArgument(*input)
	if err != nil || in == nil {
		return e.fail("fn invoke", firstError(err, usageError("pass the input with --input")))
	}
	cl, err := e.jsonArgument(*claims)
	if err != nil {
		return e.fail("fn invoke", err)
	}
	module, decl, err := loadBuilt(l.project, pos[0])
	if err != nil {
		return e.fail("fn invoke", err)
	}
	ctx, cancel := e.context()
	defer cancel()
	set2 := limits.Defaults()
	engine, err := newEngine(ctx, set2, false)
	if err != nil {
		return e.fail("fn invoke", err)
	}
	defer func() { _ = engine.Close(context.WithoutCancel(ctx)) }()
	out, stats, err := builder.RunLocal(ctx, engine, module, decl, set2, in, cl, func(r fnhost.LogRecord) {
		_, _ = fmt.Fprintf(e.stderr, "[%s] %s\n", logLevels[r.Level], r.Message)
	})
	if err != nil {
		return e.fail("fn invoke", describeHostError(err))
	}
	_, _ = fmt.Fprintf(e.stderr, "took %s, peak memory %d bytes\n", stats.Duration.Round(time.Microsecond), stats.PeakMemoryBytes)
	if out.Business != nil {
		if l.json {
			e.emit(true, map[string]any{"error": out.Business}, "")
		}
		return e.fail("fn invoke", fmt.Errorf("business error %s (%s)", out.Business.Code, out.Business.MessageKey))
	}
	return e.emit(l.json, map[string]any{"output": out.Output}, string(out.Output))
}

// logLevels names the levels of a function's log records.
var logLevels = map[fnhost.LogLevel]string{fnhost.LogDebug: "debug", fnhost.LogInfo: "info", fnhost.LogWarn: "warn", fnhost.LogError: "error"}

// describeHostError names the Plux code of a system failure.
func describeHostError(err error) error {
	if pe, ok := plxerr.CodeOf(err); ok {
		return fmt.Errorf("PLX-%d: %w", uint16(pe), err)
	}
	return err
}

// firstError returns the first non-nil error.
func firstError(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// jsonArgument reads a JSON argument given inline or as @file; empty gives
// nil. The text must be JSON.
func (e env) jsonArgument(v string) ([]byte, error) {
	if v == "" {
		return nil, nil
	}
	data := []byte(v)
	if rest, ok := strings.CutPrefix(v, "@"); ok {
		var err error
		if rest == "-" {
			data, err = io.ReadAll(e.stdin)
		} else {
			data, err = os.ReadFile(rest) //nolint:gosec // G304: the developer names the file.
		}
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", rest, err)
		}
	}
	if !json.Valid(data) {
		return nil, usageError("the argument is not JSON")
	}
	return data, nil
}
