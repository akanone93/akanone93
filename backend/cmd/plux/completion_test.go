// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"slices"
	"strings"
	"testing"
)

// usageNames returns the first word of each indented line of a usage text's
// Commands section.
func usageNames(text string) []string {
	_, list, _ := strings.Cut(text, "Commands:\n")
	var names []string
	for line := range strings.SplitSeq(list, "\n") {
		if !strings.HasPrefix(line, "  ") {
			break
		}
		names = append(names, strings.Fields(line)[0])
	}
	return names
}

// Every command the top-level help lists completes in the shell, and every
// command completion offers is in the help.
func TestCompletionOffersEveryCommandOfTheHelp(t *testing.T) {
	help := usageNames(usage)
	if len(help) == 0 {
		t.Fatal("no commands read from the usage text")
	}
	for _, c := range help {
		if !slices.Contains(commands, c) {
			t.Errorf("completion lacks %q", c)
		}
	}
	for _, c := range commands {
		if !slices.Contains(help, c) {
			t.Errorf("help lacks %q", c)
		}
	}
}

// The top-level help's line for plux fn names every command plux fn lists,
// so plux --help shows promote and triggers too.
func TestHelpNamesEveryFnCommand(t *testing.T) {
	var line string
	for l := range strings.SplitSeq(usage, "\n") {
		if strings.HasPrefix(l, "  fn ") {
			line = l
		}
	}
	listed := strings.Fields(strings.ReplaceAll(line, "|", " "))
	sub := usageNames(fnUsage)
	if len(sub) == 0 {
		t.Fatal("no commands read from plux fn's usage text")
	}
	for _, c := range sub {
		if !slices.Contains(listed, c) {
			t.Errorf("plux --help does not name fn %s: %q", c, line)
		}
	}
}
