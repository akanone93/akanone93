// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// Verifies: FN-003.
// A link gives each source, from the one pool of crate archives, exactly the
// archives its own lock file names.
func TestCratesForGivesEachSourceItsOwnCrates_FN_003(t *testing.T) {
	serde, itoa := fakeCrate("serde", "1.0.229", defaultLib("serde")), fakeCrate("itoa", "1.0.18", defaultLib("itoa"))
	src := fnSource("a.b", "a-b", PlacementDevice, serdeLock)
	got := CratesFor(src.Files, [][]byte{itoa, serde, serde})
	if len(got) != 1 || CrateSHA256(got[0]) != serdeLock.Checksum {
		t.Errorf("CratesFor gave %d archives; want serde once", len(got))
	}
	if got := CratesFor(fstest.MapFS{}, [][]byte{serde}); got != nil {
		t.Errorf("a source without a lock file was given %d archives", len(got))
	}
}

// Verifies: FN-003.
// LockedCrates lists the crates.io packages of a lock file and refuses any
// other source or a package without a checksum.
func TestLockedCratesAcceptsOnlyCratesIO_FN_003(t *testing.T) {
	itoa := registryCrate("itoa", "1.0.18")
	got, err := LockedCrates([]byte(lockText(lockPackage{Name: "fn", Version: "0.1.0"}, serdeLock, itoa)))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "itoa" || got[1].Checksum != serdeLock.Checksum {
		t.Errorf("LockedCrates = %+v; want itoa then serde", got)
	}
	for name, lock := range map[string]string{
		"a git source":   lockText(lockPackage{Name: "x", Version: "1.0.0", Source: "git+https://example.invalid/x", Checksum: serdeLock.Checksum}),
		"no checksum":    lockText(lockPackage{Name: "x", Version: "1.0.0", Source: cratesIOSource}),
		"a bad checksum": lockText(lockPackage{Name: "x", Version: "1.0.0", Source: cratesIOSource, Checksum: "c1"}),
	} {
		if _, err := LockedCrates([]byte(lock)); err == nil {
			t.Errorf("%s was accepted", name)
		} else {
			wantCode(t, err, plxerr.FunctionDependencyNotVendored)
		}
	}
}

// Verifies: FN-003.
// LocalCrates reads each locked archive from Cargo's cache, under whichever
// index directory holds a file with the right SHA-256, and says to run cargo
// fetch when one is missing.
func TestLocalCratesReadsCargosCacheByChecksum_FN_003(t *testing.T) {
	home := t.TempDir()
	write := func(index, name string, data []byte) {
		t.Helper()
		dir := filepath.Join(home, "registry", "cache", index)
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	lock := []byte(lockText(lockPackage{Name: "fn", Version: "0.1.0"}, serdeLock))
	if _, err := LocalCrates(home, lock); err == nil {
		t.Fatal("an empty cache was accepted")
	}
	write("index.a", "serde-1.0.229.crate", []byte("another archive with the same name"))
	write("index.b", "serde-1.0.229.crate", fakeCrate("serde", "1.0.229", defaultLib("serde")))
	got, err := LocalCrates(home, lock)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || CrateSHA256(got[0]) != serdeLock.Checksum {
		t.Errorf("LocalCrates gave %d archives", len(got))
	}
}
