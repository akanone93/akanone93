// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// Verifies: FN-016.
// A cache needs its size limit, and a sandboxed builder, which keeps one cache
// per key, also needs a limit for all of them together.
func TestBuildCacheNeedsItsLimits_FN_016(t *testing.T) {
	f := newFixture(t)
	cfg := f.cfg
	cfg.CacheDir, cfg.CacheBytes = t.TempDir(), 1<<30
	cfg.Sandbox = LinuxSandbox{Helper: "/x"}
	if _, err := New(cfg); err == nil || !strings.Contains(err.Error(), "total size limit") {
		t.Errorf("a sandboxed builder took caches without a total limit: %v", err)
	}
	cfg.CacheTotalBytes = 4 << 30
	if _, err := New(cfg); err != nil {
		t.Errorf("a sandboxed builder refused caches with their limits: %v", err)
	}
	cfg.Sandbox, cfg.CacheBytes = NoSandbox{}, 0
	if _, err := New(cfg); err == nil {
		t.Error("a cache without a size limit was accepted")
	}
}

// recordingSandbox runs commands without isolation and records the
// directories each may change; the builder treats it as a sandbox.
type recordingSandbox struct {
	mu       sync.Mutex
	writable [][]string
}

func (s *recordingSandbox) Run(ctx context.Context, c Command) (Result, error) {
	s.mu.Lock()
	s.writable = append(s.writable, slices.Clone(c.Writable))
	s.mu.Unlock()
	return NoSandbox{}.Run(ctx, c)
}

// take returns what was recorded since the last call.
func (s *recordingSandbox) take() [][]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.writable
	s.writable = nil
	return out
}

// Verifies: FN-016.
// A sandboxed builder keeps one cache per key, and a build's tools may change
// only their scratch directory and their own key's cache: never another key's,
// and none at all for a build without a key or with a key that is not a plain
// name.
func TestBuildCachesAreKeptApartByKey_FN_016(t *testing.T) {
	f := newFixture(t)
	rec := &recordingSandbox{}
	cache := filepath.Join(t.TempDir(), "cache")
	f.cfg.Sandbox, f.cfg.CacheDir, f.cfg.CacheBytes, f.cfg.CacheTotalBytes = rec, cache, 1<<30, 4<<30
	for _, key := range []string{"org-a", "org-b"} {
		if _, err := f.build(Options{CacheKey: key}, twoFunctions()...); err != nil {
			t.Fatal(err)
		}
		own := filepath.Join(cache, key)
		if _, err := os.Stat(filepath.Join(own, "ws")); err != nil {
			t.Errorf("the build of %s left no cache: %v", key, err)
		}
		for _, w := range rec.take() {
			if len(w) != 2 || w[1] != own {
				t.Errorf("a tool of %s could change %v; want its scratch directory and %s only", key, w, own)
			}
		}
	}
	for _, key := range []string{"", "../escape", "Org-A", "org/a", strings.Repeat("a", 65)} {
		if _, err := f.build(Options{CacheKey: key}, twoFunctions()...); err != nil {
			t.Fatal(err)
		}
		for _, w := range rec.take() {
			if len(w) != 1 {
				t.Errorf("a build with the key %q could change %v; want its scratch directory only", key, w)
			}
		}
	}
	entries, err := os.ReadDir(cache)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if !slices.Equal(names, []string{"org-a", "org-b"}) {
		t.Errorf("caches = %v; want org-a and org-b only", names)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(cache), "escape")); !os.IsNotExist(err) {
		t.Errorf("a key reached outside the cache directory: %v", err)
	}
}

// Verifies: FN-019.
// A build with an organisation's cache compiles the crates of its own
// workspace again — the functions and the module wrapper — so a module the
// server signs is never made from what an earlier build compiled of another
// source: before compiling, it removes them from the cached target
// directory, and keeps only crates.io crates. A developer's own cache, with
// no key, keeps them.
func TestCachedBuildCompilesTheWorkspaceCratesAgain_FN_019(t *testing.T) {
	f := newFixture(t)
	root := t.TempDir()
	cache := filepath.Join(root, "org-a")
	f.cfg.Sandbox, f.cfg.CacheDir, f.cfg.CacheBytes, f.cfg.CacheTotalBytes = &recordingSandbox{}, root, 1<<30, 4<<30
	f.write("lock.out", lockText(lockPackage{Name: "loan", Version: "0.1.0"}, lockPackage{Name: moduleName, Version: "0.1.0"}))
	f.write("tree.txt", "plux-module v0.1.0 (/x)\nloan v0.1.0 (/x)\n")
	src := fnSource("loan.schedule", "loan", PlacementServer)
	for range 2 {
		if _, err := f.build(Options{CacheKey: "org-a"}, src); err != nil {
			t.Fatal(err)
		}
	}
	want := "clean --offline --locked --release --target wasm32-wasip1 --target-dir " + filepath.Join(cache, sharedTarget) +
		" -p loan@0.1.0 -p " + moduleName + "@0.1.0"
	var sequence []string
	for _, line := range strings.Split(f.read("log"), "\n") {
		switch {
		case line == want:
			sequence = append(sequence, "clean")
		case strings.HasPrefix(line, "clean"):
			t.Errorf("an unexpected clean: %q", line)
		case strings.HasPrefix(line, "build "):
			sequence = append(sequence, "build")
		}
	}
	if got := strings.Join(sequence, " "); got != "clean build build clean build build" {
		t.Errorf("each build must forget the workspace crates before it compiles; got %q", got)
	}
	f.write("log", "")
	f.cfg.Sandbox, f.cfg.CacheDir = NoSandbox{}, t.TempDir()
	if _, err := f.build(Options{}, src); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(f.read("log"), "clean") {
		t.Errorf("a developer's own cache forgot the workspace crates:\n%s", f.read("log"))
	}
}

// Verifies: FN-019.
// The keyed caches stay within their total: those used least recently are
// removed first, a cache in use is kept, and what a stopped process left
// half removed is cleared.
func TestPruneCachesRemovesTheLeastRecentlyUsed_FN_019(t *testing.T) {
	root := t.TempDir()
	base := time.Now().Add(-time.Hour)
	for i, key := range []string{"org-a", "org-b", "org-c"} {
		dir := filepath.Join(root, key)
		if err := os.MkdirAll(filepath.Join(dir, "target"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "target", "lib.rlib"), make([]byte, 100), 0o600); err != nil {
			t.Fatal(err)
		}
		lock := filepath.Join(dir, "lock")
		if err := os.WriteFile(lock, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		used := base.Add(time.Duration(i) * time.Minute)
		if err := os.Chtimes(lock, used, used); err != nil {
			t.Fatal(err)
		}
	}
	for _, other := range []string{".removed-org-z/x", "Not-A-Key/x"} {
		if err := os.MkdirAll(filepath.Join(root, other), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	exists := func(name string) bool {
		_, err := os.Stat(filepath.Join(root, name))
		return err == nil
	}
	pruneCaches(root, 250)
	if exists("org-a") || !exists("org-b") || !exists("org-c") {
		t.Errorf("pruning to 250 bytes should remove only the least recently used cache: a %v, b %v, c %v", exists("org-a"), exists("org-b"), exists("org-c"))
	}
	if exists(".removed-org-z") || !exists("Not-A-Key") {
		t.Errorf("a half-removed cache should go and a directory that is no cache stay: %v, %v", exists(".removed-org-z"), exists("Not-A-Key"))
	}
	unlock, held := tryLock(filepath.Join(root, "org-b", "lock"))
	if !held {
		t.Fatal("could not take the lock of org-b")
	}
	pruneCaches(root, 100)
	unlock()
	if !exists("org-b") || exists("org-c") {
		t.Errorf("a cache in use was removed, or the one free was kept: b %v, c %v", exists("org-b"), exists("org-c"))
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
