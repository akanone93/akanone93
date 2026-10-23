// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"archive/zip"
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/compiler"
	"github.com/nightCode42/plux3/backend/internal/generator"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
)

// serveRelease makes the fake serve the compiled routing project as its
// release: an app with push and deep links, and its plugin.
func serveRelease(t *testing.T, f *fake) {
	t.Helper()
	res := compiler.Compile(os.DirFS(filepath.Join("..", "..", "..", "schema", "testdata", "documents", "routing")), compiler.DefaultOptions())
	if res.Diagnostics.HasErrors() {
		t.Fatal(res.Diagnostics)
	}
	f.blobs = map[string][]byte{}
	for _, b := range append([]*compiler.Bundle{res.App}, res.Plugins...) {
		h := hex.EncodeToString(b.Hash[:])
		f.blobs[h] = b.Data
		key := b.Key
		if b == res.App {
			key = ""
		}
		f.release = append(f.release, &pluxv1.PluginVersion{PluginKey: key, Version: 1, BundleSha256: h, KeyId: "k1", Algorithm: "ed25519", Signature: []byte{7}})
	}
}

// TestCreate generates a project from a release into a directory, a zip
// and a Git remote: the shell from the release's app bundle and the
// flags, the release embedded with the environment's keys.
// Verifies: GEN-001, GEN-003.
func TestCreate(t *testing.T) { //nolint:paralleltest // the environment is process-wide
	t.Setenv(tokenEnv, "plux_pat_test")
	t.Setenv(serverEnv, "")
	t.Setenv(orgEnv, "")
	f, url := newFake(t)
	serveRelease(t, f)
	out := filepath.Join(t.TempDir(), "app")
	base := []string{"create", "--server", url, "--application-id", "com.example.demo", "--bundle-id", "com.example.demo"}
	code, stdout, stderr := cli(t, t.TempDir(), append(base, "--out", out, "demo")...)
	if code != exitOK || !strings.Contains(stdout, "release 3 embedded") {
		t.Fatalf("create: %d %s %s", code, stdout, stderr)
	}
	shell, err := os.ReadFile(filepath.Join(out, "plux.shell.json")) //nolint:gosec // the test's own file
	if err != nil {
		t.Fatal(err)
	}
	var spec generator.ShellSpec
	if err := json.Unmarshal(shell, &spec); err != nil {
		t.Fatal(err)
	}
	if spec.Package != "demo" || spec.Plux.EntryRoute != "home" || spec.Plux.Endpoint != url || spec.Plux.AppID != "a1" || !spec.Push ||
		len(spec.DeepLinkHosts) == 0 || spec.ApplicationID != "com.example.demo" {
		t.Errorf("the shell: %+v", spec)
	}
	for _, p := range []string{"assets/plux/bundles/_app.pxb", "assets/plux/bundles/nav.pxb", "assets/plux/keys.json", "assets/plux/baseline.json", "lib/main.dart", "ios/Runner/Runner.entitlements"} {
		if _, err := os.Stat(filepath.Join(out, p)); err != nil {
			t.Errorf("no %s: %v", p, err)
		}
	}
	if code, _, stderr := cli(t, t.TempDir(), append(base, "--out", out, "demo")...); code != exitFailed || !strings.Contains(stderr, "not empty") {
		t.Errorf("a non-empty directory: %d %s", code, stderr)
	}

	zipFile := filepath.Join(t.TempDir(), "demo.zip")
	if code, _, stderr := cli(t, t.TempDir(), append(base, "--zip", zipFile, "demo")...); code != exitOK {
		t.Fatalf("zip: %d %s", code, stderr)
	}
	data, _ := os.ReadFile(zipFile) //nolint:gosec // the test's own file
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil || len(r.File) < 50 || !strings.HasPrefix(r.File[0].Name, "demo/") {
		t.Errorf("the zip: %v", err)
	}

	if _, err := exec.LookPath("git"); err == nil {
		remote := filepath.Join(t.TempDir(), "remote.git")
		if msg, err := exec.CommandContext(t.Context(), "git", "init", "--quiet", "--bare", remote).CombinedOutput(); err != nil { //nolint:gosec // G204: the test's own directory
			t.Fatalf("git init: %v %s", err, msg)
		}
		t.Setenv("GIT_AUTHOR_NAME", "Ada")
		t.Setenv("GIT_AUTHOR_EMAIL", "ada@example.com")
		t.Setenv("GIT_COMMITTER_NAME", "Ada")
		t.Setenv("GIT_COMMITTER_EMAIL", "ada@example.com")
		// Run from a git hook, git exports where its repository is; the
		// project must not be added to that repository's index.
		caller := filepath.Join(t.TempDir(), "caller.git")
		t.Setenv("GIT_DIR", caller)
		t.Setenv("GIT_INDEX_FILE", filepath.Join(caller, "index"))
		if code, stdout, stderr := cli(t, t.TempDir(), append(base, "--git", remote, "--branch", "shell", "demo")...); code != exitOK {
			t.Fatalf("git: %d %s %s", code, stdout, stderr)
		}
		if _, err := os.Stat(caller); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("git wrote into the caller's repository: %v", err)
		}
		files, err := exec.CommandContext(t.Context(), "git", "--git-dir", remote, "ls-tree", "-r", "--name-only", "shell").Output() //nolint:gosec // G204: the test's own directory
		if err != nil || !strings.Contains(string(files), "plux.shell.json") {
			t.Errorf("the pushed branch: %v %s", err, files)
		}
	}

	for _, bad := range [][]string{
		append(base, "demo"), // no output
		append(base, "--out", t.TempDir(), "--zip", zipFile, "demo"), // two outputs
		append(base, "--zip", zipFile),                               // no app
	} {
		if code, _, _ := cli(t, t.TempDir(), bad...); code != exitUsage {
			t.Errorf("%v: %d", bad, code)
		}
	}
	if code, _, stderr := cli(t, t.TempDir(), append(base, "--zip", zipFile, "--version", "1", "demo")...); code != exitFailed || !strings.Contains(stderr, "version") {
		t.Errorf("a bad version: %d %s", code, stderr)
	}
}

// TestCreateRouterFlavours generates each router flavour: the adapter and
// its router in the pubspec, the router wired in lib/main.dart as the
// adapter's example wires it, and the choice in plux.shell.json. none adds
// neither, and an unknown router is a usage error before anything is
// fetched.
// Verifies: GEN-001, GEN-003, HST-034.
func TestCreateRouterFlavours(t *testing.T) { //nolint:paralleltest // the environment is process-wide
	t.Setenv(tokenEnv, "plux_pat_test")
	t.Setenv(serverEnv, "")
	t.Setenv(orgEnv, "")
	f, url := newFake(t)
	serveRelease(t, f)
	base := []string{"create", "--server", url, "--application-id", "com.example.demo", "--bundle-id", "com.example.demo"}
	read := func(path string) string {
		t.Helper()
		data, err := os.ReadFile(path) //nolint:gosec // the test's own file
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	flavours := []struct {
		router               string
		pubspec, main, shell []string
		absent, mainAbsent   []string
	}{
		{
			router:     "none",
			pubspec:    []string{"  plux_flutter: " + generator.RuntimeConstraint + "\n"},
			absent:     []string{"plux_go_router", "plux_auto_route", "go_router", "auto_route"},
			main:       []string{"PluxOptions.config(navigatorKey: navigatorKey)", "home: Scaffold("},
			mainAbsent: []string{"MaterialApp.router(", "PluxGoRoutes()", "RootStackRouter.build("},
			shell:      nil,
		},
		{
			router:     "go_router",
			pubspec:    []string{"  plux_flutter: " + generator.RuntimeConstraint + "\n", "  plux_go_router: ^0.1.0\n", "  go_router: ^18.0.2\n"},
			absent:     []string{"auto_route"},
			main:       []string{"import 'package:plux_go_router/plux_go_router.dart';", "final plux = PluxGoRoutes();", "GoRoute(path: '/', builder:", "router: PluxGoRouter(router),", "MaterialApp.router(", "PluxView(\n      'home',"},
			mainAbsent: []string{"PluxOptions.config(", "navigatorKey"},
			shell:      []string{`"router": "go_router"`},
		},
		{
			router:     "auto_route",
			pubspec:    []string{"  plux_flutter: " + generator.RuntimeConstraint + "\n", "  plux_auto_route: ^0.1.0\n", "  auto_route: ^11.2.0\n"},
			absent:     []string{"go_router"},
			main:       []string{"import 'package:plux_auto_route/plux_auto_route.dart';", "final plux = PluxAutoRoutes();", "RootStackRouter.build(", "name: 'home',", "router: PluxAutoRoute(router),", "routerConfig: router.config(),", "PluxView(\n      'home',"},
			mainAbsent: []string{"PluxOptions.config(", "navigatorKey"},
			shell:      []string{`"router": "auto_route"`},
		},
	}
	for _, fl := range flavours {
		out := filepath.Join(t.TempDir(), fl.router)
		args := append(append(append([]string{}, base...), "--out", out, "--router", fl.router), "demo")
		if code, stdout, stderr := cli(t, t.TempDir(), args...); code != exitOK {
			t.Fatalf("%s: %d %s %s", fl.router, code, stdout, stderr)
		}
		pubspec, main, shell := read(filepath.Join(out, "pubspec.yaml")), read(filepath.Join(out, "lib", "main.dart")), read(filepath.Join(out, "plux.shell.json"))
		for _, w := range fl.pubspec {
			if !strings.Contains(pubspec, w) {
				t.Errorf("%s: pubspec.yaml lacks %q", fl.router, w)
			}
		}
		for _, w := range fl.absent {
			if strings.Contains(pubspec, w) {
				t.Errorf("%s: pubspec.yaml holds %q", fl.router, w)
			}
		}
		for _, w := range fl.main {
			if !strings.Contains(main, w) {
				t.Errorf("%s: lib/main.dart lacks %q", fl.router, w)
			}
		}
		for _, w := range fl.mainAbsent {
			if strings.Contains(main, w) {
				t.Errorf("%s: lib/main.dart holds %q", fl.router, w)
			}
		}
		if fl.router == "none" && strings.Contains(shell, `"router"`) {
			t.Errorf("none: plux.shell.json names a router: %s", shell)
		}
		for _, w := range fl.shell {
			if !strings.Contains(shell, w) {
				t.Errorf("%s: plux.shell.json lacks %s", fl.router, w)
			}
		}
	}

	// With --plux-path the adapter is read from the directory beside
	// plux_flutter's, and go_router stays a pub dependency.
	pathOut := filepath.Join(t.TempDir(), "path")
	args := append(append(append([]string{}, base...), "--out", pathOut, "--router", "go_router", "--plux-path", "/repo/packages/plux_flutter"), "demo")
	if code, stdout, stderr := cli(t, t.TempDir(), args...); code != exitOK {
		t.Fatalf("path: %d %s %s", code, stdout, stderr)
	}
	if pubspec := read(filepath.Join(pathOut, "pubspec.yaml")); !strings.Contains(pubspec, "  plux_go_router:\n    path: /repo/packages/plux_go_router\n") || !strings.Contains(pubspec, "  go_router: ^18.0.2\n") {
		t.Errorf("the adapter's path dependency: %s", pubspec)
	}

	bad := filepath.Join(t.TempDir(), "bad")
	if code, _, stderr := cli(t, t.TempDir(), append(append(append([]string{}, base...), "--out", bad, "--router", "tabs"), "demo")...); code != exitUsage || !strings.Contains(stderr, `unknown router "tabs"`) {
		t.Errorf("an unknown router: %d %s", code, stderr)
	}
	if _, err := os.Stat(bad); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("an unknown router wrote a project: %v", err)
	}
}
