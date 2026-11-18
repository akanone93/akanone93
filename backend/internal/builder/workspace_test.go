// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"encoding/json"
	"flag"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

var update = flag.Bool("update", false, "rewrite the golden files under testdata")

// golden compares got with testdata/<name>, or rewrites it with -update.
func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	p := filepath.Join("testdata", name)
	if *update {
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, got, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != string(got) {
		t.Errorf("%s differs from the golden file (run with -update to refresh):\n--- got\n%s", name, got)
	}
}

// prepared returns the two fixture functions, sorted.
func prepared(t *testing.T) []preparedFunction {
	t.Helper()
	funcs, err := prepareSources(twoFunctions(), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	return funcs
}

// Verifies: FN-003, FN-004, FN-019.
func TestGeneratedWorkspaceFilesMatchTheGoldenFiles_FN_019(t *testing.T) {
	files := generatedFiles(prepared(t))
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		golden(t, "workspace/"+strings.ReplaceAll(n, "/", "__"), files[n])
	}
}

// Verifies: FN-003.
func TestStageWorkspaceMergesVendorDirectories_FN_003(t *testing.T) {
	other := registryCrate("itoa", "1.0.18")
	srcs := []FunctionSource{
		fnSource("a.one", "one", PlacementDevice, serdeLock),
		fnSource("b.two", "two", PlacementDevice, serdeLock, other),
	}
	funcs, err := prepareSources(srcs, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	f := newFixture(t)
	h, err := stageWorkspace(root, os.DirFS(f.sdk), funcs, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Cargo.toml", ".cargo/config.toml", "plux-module/src/lib.rs", "plux-fn/src/lib.rs",
		"functions/a.one/src/lib.rs", "functions/b.two/Cargo.lock",
		"vendor/serde-1.0.229/Cargo.toml", "vendor/serde-1.0.229/src/lib.rs", "vendor/itoa-1.0.18/.cargo-checksum.json",
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(want))); err != nil {
			t.Errorf("workspace lacks %s", want)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "functions", "a.one", "vendor")); err == nil {
		t.Error("vendor/ must not be copied into the function crate")
	}
	if len(h.source) != 64 || len(h.sdk) != 64 {
		t.Errorf("hashes = %+v", h)
	}
	again, err := stageWorkspace(t.TempDir(), os.DirFS(f.sdk), funcs, 1<<20)
	if err != nil || again != h {
		t.Errorf("hashes are not stable: %+v %+v %v", again, h, err)
	}
}

// Verifies: FN-003.
func TestSourceHashCoversEveryFileIncludingVendor_FN_003(t *testing.T) {
	hash := func(s FunctionSource) string {
		funcs, err := prepareSources([]FunctionSource{s}, 1<<20)
		if err != nil {
			t.Fatal(err)
		}
		h, err := stageWorkspace(t.TempDir(), os.DirFS(newFixture(t).sdk), funcs, 1<<20)
		if err != nil {
			t.Fatal(err)
		}
		return h.source
	}
	base := hash(fnSource("a.one", "one", PlacementDevice, serdeLock))
	edited := withCrate(fnSource("a.one", "one", PlacementDevice, serdeLock), "serde", "1.0.229", "// another release\n")
	if hash(edited) == base {
		t.Error("another archive of a dependency did not change the source hash")
	}
	renamed := hash(fnSource("a.two", "one", PlacementDevice, serdeLock))
	if renamed == base {
		t.Error("the function name must be part of the source hash")
	}
}

// Verifies: FN-003.
func TestVendorChecksumConflictIsPLX7201_FN_003(t *testing.T) {
	funcs, err := prepareSources([]FunctionSource{
		fnSource("a.one", "one", PlacementDevice, serdeLock),
		withCrate(fnSource("b.two", "two", PlacementDevice, serdeLock), "serde", "1.0.229", "// another archive\n"),
	}, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	_, err = mergeVendorPlan(funcs)
	wantCode(t, err, plxerr.FunctionDependencyNotVendored)
	if !strings.Contains(err.Error(), "serde 1.0.229") {
		t.Errorf("error does not name the crate: %v", err)
	}
	_, err = stageWorkspace(t.TempDir(), os.DirFS(newFixture(t).sdk), funcs, 1<<20)
	wantCode(t, err, plxerr.FunctionDependencyNotVendored)
}

// Verifies: FN-003.
// vendor/ is the builder's own: its checksum files are written from the bytes
// the builder extracted, with the archive's SHA-256 as the package checksum,
// so Cargo checks every vendored file against the verified archive.
func TestVendorIsWrittenFromTheVerifiedArchives_FN_003(t *testing.T) {
	funcs, err := prepareSources([]FunctionSource{fnSource("a", "a", PlacementDevice, serdeLock)}, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := fs.ReadFile(funcs[0].Files, "vendor/serde-1.0.229/.cargo-checksum.json")
	if err != nil {
		t.Fatal(err)
	}
	var sums struct {
		Files   map[string]string `json:"files"`
		Package string            `json:"package"`
	}
	if err := json.Unmarshal(raw, &sums); err != nil {
		t.Fatal(err)
	}
	if sums.Package != serdeLock.Checksum || sums.Files["src/lib.rs"] != hexSum([]byte(defaultLib("serde"))) || len(sums.Files) != 2 {
		t.Errorf("checksum file %s", raw)
	}
	lib, err := fs.ReadFile(funcs[0].Files, "vendor/serde-1.0.229/src/lib.rs")
	if err != nil || string(lib) != defaultLib("serde") {
		t.Errorf("vendored lib.rs %q, %v", lib, err)
	}
}

// Verifies: FN-003.
func TestLockDriftCheckIsExact_FN_003(t *testing.T) {
	funcs := prepared(t)
	if err := checkLockAgainst(funcs, []lockPackage{{Name: "loan", Version: "0.1.0"}, serdeLock}); err != nil {
		t.Errorf("matching lock: %v", err)
	}
	drift := serdeLock
	drift.Version = "1.0.230"
	wantCode(t, checkLockAgainst(funcs, []lockPackage{drift}), plxerr.FunctionDependencyNotVendored)
	drift = serdeLock
	drift.Checksum = "tampered"
	wantCode(t, checkLockAgainst(funcs, []lockPackage{drift}), plxerr.FunctionDependencyNotVendored)
}
