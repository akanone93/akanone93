// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// Verifies: STU-100.
// plux render-worker needs its subcommand and the files of its identity and
// toolchain; without them it exits with a usage error before it opens a port,
// and a missing certificate is reported, not served around.
func TestRenderWorkerNeedsItsConfiguration_STU_100(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if code, _, stderr := cli(t, dir, "render-worker"); code != exitUsage || !strings.Contains(stderr, "plux render-worker serve") {
		t.Errorf("no subcommand: exit %d\n%s", code, stderr)
	}
	if code, _, stderr := cli(t, dir, "render-worker", "serve"); code != exitUsage || !strings.Contains(stderr, "--ca, --cert, --key and --fonts are required") {
		t.Errorf("no configuration: exit %d\n%s", code, stderr)
	}
	flutter := fakeFlutterRendering(t, "x")
	code, _, stderr := cli(t, dir, "render-worker", "serve", "--ca", filepath.Join(dir, "ca.pem"), "--cert", filepath.Join(dir, "c.pem"), "--key", filepath.Join(dir, "k.pem"),
		"--fonts", goldenFonts, "--flutter", flutter)
	if code != exitFailed || !strings.Contains(stderr, "render-worker serve") {
		t.Errorf("missing identity files: exit %d\n%s", code, stderr)
	}
	if code, _, stderr := cli(t, dir, "render-worker", "serve", "--ca", "a", "--cert", "b", "--key", "c", "--fonts", filepath.Join(dir, "none"), "--flutter", flutter); code != exitFailed {
		t.Errorf("missing fonts: exit %d\n%s", code, stderr)
	}
}
