// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"slices"
	"strings"
	"testing"
)

// TestEveryCommandAnswersHelp asks every command for its usage with -h and
// --help, every subcommand group also with help, and each through plux help
// <command>: each answers with its usage and exit code 0, and does nothing else
// (the commands that act without arguments, such as login, are not run).
//
// Verifies: CLI-001.
func TestEveryCommandAnswersHelp(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("PLUX_TOKEN", "")
	t.Setenv("PLUX_SERVER", "http://127.0.0.1:1")
	t.Chdir(t.TempDir())
	cmds := []string{"validate", "build", "codegen", "test"}
	for c := range newEnv(nil, nil).commands() {
		cmds = append(cmds, c)
	}
	slices.Sort(cmds)
	for _, c := range cmds {
		asks := [][]string{{c, "-h"}, {c, "--help"}, {"help", c}}
		if groups[c] {
			asks = append(asks, []string{c, "help"})
		}
		for _, args := range asks {
			var stdout, stderr bytes.Buffer
			code := run(args, &stdout, &stderr)
			out := stdout.String() + stderr.String()
			if code != exitOK || !strings.Contains(out, "Usage") {
				t.Errorf("plux %s: exit %d, output %q", strings.Join(args, " "), code, firstLine(out))
			}
			if args[0] == "help" && (!strings.Contains(stdout.String(), "Usage") || stderr.Len() != 0) {
				t.Errorf("plux %s: the usage is not on standard output alone", strings.Join(args, " "))
			}
		}
	}
}

// TestNestedGroupsAnswerHelp asks the groups inside groups for their usage.
func TestNestedGroupsAnswerHelp(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PLUX_SERVER", "http://127.0.0.1:1")
	for _, args := range [][]string{
		{"git", "export"},
		{"security", "config"},
		{"analytics", "alerts"},
		{"analytics", "errors"},
		{"analytics", "retention"},
		{"analytics", "export"},
	} {
		for _, h := range []string{"-h", "--help", "help"} {
			var stdout, stderr bytes.Buffer
			full := append(slices.Clone(args), h)
			code := run(full, &stdout, &stderr)
			if code != exitOK || !strings.Contains(stdout.String(), "Usage") {
				t.Errorf("plux %s: exit %d, stdout %q, stderr %q", strings.Join(full, " "), code,
					firstLine(stdout.String()), firstLine(stderr.String()))
			}
		}
	}
}

// TestHelpOfAnUnknownCommand is a usage error that lists the commands.
func TestHelpOfAnUnknownCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"help", "deploy"}, &stdout, &stderr); code != exitUsage {
		t.Errorf("exit %d, want %d", code, exitUsage)
	}
	if !strings.Contains(stderr.String(), `unknown command "deploy"`) || stdout.Len() != 0 {
		t.Errorf("stdout %q, stderr %q", stdout.String(), firstLine(stderr.String()))
	}
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}
