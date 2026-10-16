// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

// Command catalogue fails when spec Appendix C and D disagree with the widget
// and action registries, the renderer's builders or the action handlers.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/nightCode42/plux3/tools/internal/catalogue"
)

// Exit codes: 0 clean (or -report), 1 findings, 2 usage or I/O error.
const (
	exitOK       = 0
	exitFindings = 1
	exitError    = 2
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("catalogue", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", "", "repository root (required)")
	phaseFlag := fs.String("phase", "", "check entries up to this phase, e.g. P8 (required)")
	report := fs.Bool("report", false, "print findings and exit 0")
	if err := fs.Parse(args); err != nil {
		return exitError
	}
	if *root == "" || *phaseFlag == "" {
		fmt.Fprintln(stderr, "catalogue: -root and -phase are required")
		return exitError
	}
	phase, err := catalogue.ParsePhase(*phaseFlag)
	if err != nil {
		fmt.Fprintln(stderr, "catalogue:", err)
		return exitError
	}
	findings, err := catalogue.Check(*root, phase)
	if err != nil {
		fmt.Fprintln(stderr, "catalogue:", err)
		return exitError
	}
	for _, f := range findings {
		fmt.Fprintln(stdout, f)
	}
	fmt.Fprintf(stdout, "%d findings\n", len(findings))
	if len(findings) > 0 && !*report {
		return exitFindings
	}
	return exitOK
}
