// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package devsession

import (
	"fmt"
	"slices"

	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/pxl"
)

// consoleRoots are the state roots the PXL console offers (DEV-032). The
// server does not know the types of a device's live state, so each root
// reads as a map of text and the console is a read-only view: a value is
// reached by key and only text operations check.
var consoleRoots = []string{"app", "data", "device", "env", "flags", "page", "plugin", "state", "user"}

// consoleType is the type every console root has for the compiler.
const consoleType = "map<string,string>"

// consoleLocation is where the diagnostics of a console expression point.
var consoleLocation = plxerr.Location{File: "console", Path: "/expression"}

// compileConsole compiles the expression of an evaluatePxl command to PXL
// bytecode, the bytes the runtime evaluates (DEV-032). The console only
// reads: an assignment, like any other expression the compiler does not
// accept, is refused with the code of the first compile error. roots, when
// not empty, narrows the roots the expression may read.
func compileConsole(expression string, roots []string) ([]byte, error) {
	offered := consoleRoots
	if len(roots) > 0 {
		offered = nil
		for _, r := range roots {
			if !slices.Contains(consoleRoots, r) {
				return nil, plxerr.New(plxerr.DevCommandNotAllowed, "the console has no root %q", r)
			}
			offered = append(offered, r)
		}
	}
	spec := pxl.EnvSpec{Roots: map[string]string{}}
	for _, r := range offered {
		spec.Roots[r] = consoleType
	}
	env, err := pxl.NewEnv(spec)
	if err != nil {
		return nil, fmt.Errorf("devsession: the console environment: %w", err)
	}
	prog, _, diags := pxl.Compile(expression, env, pxl.DefaultOptions(), consoleLocation)
	if prog == nil || diags.HasErrors() {
		for _, d := range diags {
			if d.Severity == plxerr.SeverityError {
				return nil, plxerr.New(d.Code, "%s", d.Message)
			}
		}
		return nil, plxerr.New(plxerr.PXLSyntaxError, "the expression does not compile")
	}
	return prog.Encode(), nil
}

// prepareConsole replaces the program of an evaluatePxl command with the
// one the server compiles from its expression; a program a client sent is
// never trusted. Other commands pass through.
func prepareConsole(cmd *pluxv1.DevSessionServiceSendCommandRequest) error {
	eval := cmd.GetEvaluatePxl()
	if eval == nil {
		return nil
	}
	eval.Program = nil
	program, err := compileConsole(eval.GetExpression(), eval.GetRoots())
	if err != nil {
		return err
	}
	eval.Program = program
	return nil
}
