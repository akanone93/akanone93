// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Verifies: FN-016.
// A build cache is for development builds only: a sandboxed builder refuses
// one, since sandboxed builds share no writable state, and a cache needs its
// size limit.
func TestBuildCacheIsForDevelopmentBuildsOnly_FN_016(t *testing.T) {
	f := newFixture(t)
	cfg := f.cfg
	cfg.CacheDir, cfg.CacheBytes = t.TempDir(), 1<<30
	cfg.Sandbox = LinuxSandbox{}
	if _, err := New(cfg); err == nil || !strings.Contains(err.Error(), "development builds only") {
		t.Errorf("a sandboxed builder took a cache: %v", err)
	}
	cfg.Sandbox, cfg.CacheBytes = NoSandbox{}, 0
	if _, err := New(cfg); err == nil {
		t.Error("a cache without a size limit was accepted")
	}
}

// Verifies: FN-019.
// A cached build keeps its workspace: a file whose bytes did not change keeps
// its modification time, so Cargo reuses what it compiled from it; the
// schema build and the artefact build use the cache's target directory, and
// Verify builds its second copy from scratch.
func TestCachedBuildKeepsUnchangedFiles_FN_019(t *testing.T) {
	f := newFixture(t)
	cache := t.TempDir()
	f.cfg.CacheDir, f.cfg.CacheBytes = cache, 1<<30
	if _, err := f.build(Options{}, twoFunctions()...); err != nil {
		t.Fatal(err)
	}
	lib := filepath.Join(cache, "ws", "vendor", "serde-1.0.229", "src", "lib.rs")
	before, err := os.Stat(lib)
	if err != nil {
		t.Fatalf("the cached workspace lacks the vendored crate: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	if _, err := f.build(Options{Verify: true}, twoFunctions()...); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(lib)
	if err != nil || !after.ModTime().Equal(before.ModTime()) {
		t.Errorf("an unchanged vendored file was rewritten: %v -> %v (%v)", before.ModTime(), after.ModTime(), err)
	}
	log := f.read("log")
	if strings.Count(log, "--target-dir "+filepath.Join(cache, sharedTarget)) != 4 {
		t.Errorf("the schema and artefact builds of both runs should use the cached target:\n%s", log)
	}
	if !strings.Contains(log, "target-fresh") {
		t.Errorf("Verify did not build its second copy from scratch:\n%s", log)
	}
}

// Verifies: FN-003.
// What an earlier build left in the cached workspace and no longer belongs to
// the build is removed, so a crate of another build cannot take part in
// resolving this one.
func TestCachedBuildRemovesWhatTheBuildNoLongerHas_FN_003(t *testing.T) {
	f := newFixture(t)
	cache := t.TempDir()
	f.cfg.CacheDir, f.cfg.CacheBytes = cache, 1<<30
	if _, err := f.build(Options{}, twoFunctions()...); err != nil {
		t.Fatal(err)
	}
	f.write("lock.out", lockText(lockPackage{Name: "loan", Version: "0.1.0"}))
	f.write("tree.txt", "plux-module v0.1.0 (/x)\nloan v0.1.0 (/x)\n")
	if _, err := f.build(Options{}, fnSource("loan.schedule", "loan", PlacementServer)); err != nil {
		t.Fatal(err)
	}
	for _, gone := range []string{"vendor/serde-1.0.229", "functions/fee.compute"} {
		if _, err := os.Stat(filepath.Join(cache, "ws", filepath.FromSlash(gone))); !os.IsNotExist(err) {
			t.Errorf("%s of the earlier build is still in the cached workspace: %v", gone, err)
		}
	}
}

// Verifies: FN-019.
// The cache is bounded: a target directory larger than the limit is emptied,
// one within it is kept.
func TestPruneCacheBoundsTheTargetDirectory_FN_019(t *testing.T) {
	target := filepath.Join(t.TempDir(), "target")
	if err := os.MkdirAll(target, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "a.rlib"), make([]byte, 100), 0o600); err != nil {
		t.Fatal(err)
	}
	pruneCache(target, 100)
	if _, err := os.Stat(target); err != nil {
		t.Errorf("a cache within its limit was emptied: %v", err)
	}
	pruneCache(target, 99)
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Errorf("a cache above its limit was kept: %v", err)
	}
}
