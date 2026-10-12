// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package policy

import (
	"bufio"
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// inspectScript is test/release/inspect.sh, which CI runs on real release
// builds; these tests run it on small extracted trees.
var inspectScript = filepath.Join(repoRoot, "test", "release", "inspect.sh")

// inspect runs the script on dir and returns its output and exit status.
func inspect(t *testing.T, dir string) (string, int) {
	t.Helper()
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is not installed")
	}
	cmd := exec.CommandContext(t.Context(), bash, inspectScript, dir) //nolint:gosec // the repository's own script, on a temporary tree
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err = cmd.Run()
	if err == nil {
		return out.String(), 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return out.String(), ee.ExitCode()
	}
	t.Fatal(err)
	return "", -1
}

// release writes the tree of an unpacked release APK whose AOT snapshot
// holds snapshot, plus any extra files.
func release(t *testing.T, snapshot string, extra ...string) string {
	t.Helper()
	root := t.TempDir()
	write(t, root, "lib/arm64-v8a/libapp.so", "\x7fELF\x00\x01"+snapshot+"\x00")
	write(t, root, "lib/arm64-v8a/libflutter.so", "\x7fELF engine")
	write(t, root, "assets/flutter_assets/AssetManifest.bin", "manifest")
	for _, f := range extra {
		write(t, root, f, "x")
	}
	return root
}

// Verifies: SEC-074, DEV-050.
func TestInspectPassesACleanReleaseBuild_SEC_074(t *testing.T) {
	t.Parallel()
	out, code := inspect(t, release(t, "PluxView Sync now Plux.start"))
	if code != 0 || !strings.HasPrefix(out, "ok ") {
		t.Fatalf("clean release: exit %d\n%s", code, out)
	}
}

// Verifies: SEC-074, DEV-050.
func TestInspectFailsOnPlantedDevtoolsAndLoggingStrings_SEC_074(t *testing.T) {
	t.Parallel()
	for _, probe := range []string{"Open Plux devtools", "_LogTab", "where actions do not run"} {
		out, code := inspect(t, release(t, "PluxView "+probe+" more"))
		if code != 1 || !strings.Contains(out, `"`+probe+`"`) {
			t.Errorf("planted %q: exit %d\n%s", probe, code, out)
		}
	}
}

// Verifies: SEC-074.
func TestInspectFailsOnSourceMapsAndDebugArtifacts_SEC_074(t *testing.T) {
	t.Parallel()
	for _, file := range []string{
		"assets/flutter_assets/kernel_blob.bin",
		"assets/flutter_assets/vm_snapshot_data",
		"assets/flutter_assets/isolate_snapshot_data",
		"assets/app.dill",
		"assets/main.dart.js.map",
		"assets/app.android-arm64.symbols",
	} {
		out, code := inspect(t, release(t, "PluxView", file))
		if code != 1 || !strings.Contains(out, file) {
			t.Errorf("planted %s: exit %d\n%s", file, code, out)
		}
	}
}

// Verifies: SEC-074.
func TestInspectFailsWithoutAnAOTSnapshot_SEC_074(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	write(t, root, "assets/flutter_assets/AssetManifest.bin", "manifest")
	out, code := inspect(t, root)
	if code != 1 || !strings.Contains(out, "no AOT snapshot") {
		t.Fatalf("no snapshot: exit %d\n%s", code, out)
	}
	// An iOS app keeps its snapshot in App.framework.
	ios := t.TempDir()
	write(t, ios, "Frameworks/App.framework/App", "Open Plux devtools")
	if out, code := inspect(t, ios); code != 1 || !strings.Contains(out, "Frameworks/App.framework/App") {
		t.Fatalf("iOS app: exit %d\n%s", code, out)
	}
}

// probe is one line of test/release/probes.txt.
type probe struct{ kind, text string }

func readProbes(t *testing.T) []probe {
	t.Helper()
	f, err := os.Open(filepath.Join(repoRoot, "test", "release", "probes.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var probes []probe
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		kind, text, ok := strings.Cut(line, "\t")
		if !ok || text == "" {
			t.Fatalf("probes.txt: %q is not kind<TAB>string", line)
		}
		probes = append(probes, probe{kind, text})
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return probes
}

// sourceText is every Dart file under dir, concatenated.
func sourceText(t *testing.T, dir string) string {
	t.Helper()
	var b strings.Builder
	err := filepath.WalkDir(filepath.Join(repoRoot, dir), func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".dart") {
			return err
		}
		data, err := os.ReadFile(p)
		b.Write(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// Verifies: SEC-074, DEV-050.
// A probe that no longer occurs in its source would pass every build
// without proving anything; so each is checked against the code that
// would leak it. The runtime must not depend on the devtools either.
func TestReleaseProbesNameRealStringsAndRuntimeHasNoDevtools_SEC_074(t *testing.T) {
	t.Parallel()
	devtools := sourceText(t, "packages/plux_devtools/lib")
	runtime := sourceText(t, "packages/plux_flutter/lib")
	probes := readProbes(t)
	kinds := map[string]bool{}
	for _, p := range probes {
		kinds[p.kind] = true
		source := map[string]string{"devtools": devtools, "logging": runtime}[p.kind]
		if source == "" {
			t.Errorf("probe kind %q has no source to check against", p.kind)
		} else if !strings.Contains(source, p.text) {
			t.Errorf("%s probe %q does not occur in its package any more", p.kind, p.text)
		}
		for _, r := range p.text {
			if r > 0x7f {
				t.Errorf("probe %q is not ASCII", p.text)
				break
			}
		}
	}
	if !kinds["devtools"] || !kinds["logging"] {
		t.Errorf("probes cover %v, want devtools and logging", kinds)
	}
	pubspec, err := os.ReadFile(filepath.Join(repoRoot, "packages", "plux_flutter", "pubspec.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(pubspec), "plux_devtools") {
		t.Error("plux_flutter depends on plux_devtools")
	}
}
