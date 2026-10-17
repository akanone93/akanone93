// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package golden

import (
	"crypto/sha256"
	"errors"
	"strings"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

func caseOf(id, image string) Case {
	sum := sha256.Sum256([]byte(image))
	return Case{ID: id, SHA256: sum[:], Image: []byte(image)}
}

func codeOf(t *testing.T, err error) plxerr.Code {
	t.Helper()
	var e *plxerr.Error
	if !errors.As(err, &e) {
		t.Fatalf("%v is not a plxerr", err)
	}
	return e.Code
}

// Verifies: TST-003, LIM-001.
// A run is held to golden.casesPerRun and golden.imageBytes from the limits
// registry, and its hashes and case IDs must be well formed.
func TestValidateRun(t *testing.T) {
	t.Parallel()
	hash := make([]byte, sha256.Size)
	run := func(cases ...Case) Run { return Run{Bundle: hash, App: hash, Cases: cases} }
	s := New(Options{})
	if err := s.validate(run(caseOf("a", "x"), caseOf("b", "y"))); err != nil {
		t.Fatalf("a valid run: %v", err)
	}

	for name, tc := range map[string]struct {
		run  Run
		want plxerr.Code
	}{
		"no cases":        {run(), plxerr.InvalidFormat},
		"short bundle":    {Run{Bundle: hash[:5], App: hash, Cases: []Case{caseOf("a", "x")}}, plxerr.InvalidFormat},
		"empty ID":        {run(caseOf("", "x")), plxerr.InvalidFormat},
		"duplicate ID":    {run(caseOf("a", "x"), caseOf("a", "y")), plxerr.InvalidFormat},
		"hash mismatch":   {run(Case{ID: "a", SHA256: hash, Image: []byte("x")}), plxerr.InvalidFormat},
		"missing hash":    {run(Case{ID: "a"}), plxerr.InvalidFormat},
		"very long ID":    {run(caseOf(strings.Repeat("a", 513), "x")), plxerr.InvalidFormat},
		"image too large": {run(caseOf("a", strings.Repeat("x", int(limits.Defaults().Get(limits.GoldenImageBytes))+1))), plxerr.LimitExceeded},
	} {
		if got := codeOf(t, s.validate(tc.run)); got != tc.want {
			t.Errorf("%s: %v, want %v", name, got, tc.want)
		}
	}

	small, err := limits.Defaults().Tighten(limits.GoldenCasesPerRun, limits.ScopeInstallation, 2)
	if err != nil {
		t.Fatal(err)
	}
	tight := New(Options{Limits: small})
	if got := codeOf(t, tight.validate(run(caseOf("a", "1"), caseOf("b", "2"), caseOf("c", "3")))); got != plxerr.LimitExceeded {
		t.Errorf("too many cases: %v", got)
	}
}

// Verifies: TST-003.
// An image is stored under a key of its own plugin: the same image in
// another plugin, app or organisation has another key.
func TestObjectKeyBelongsToAPlugin(t *testing.T) {
	t.Parallel()
	sum := sha256.Sum256([]byte("png"))
	keys := map[string]bool{}
	for _, in := range [][3]string{{"o1", "a1", "p1"}, {"o1", "a1", "p2"}, {"o1", "a1", ""}, {"o1", "a2", "p1"}, {"o2", "a1", "p1"}} {
		k, err := objectKey(in[0], in[1], in[2], sum[:])
		if err != nil {
			t.Fatal(err)
		}
		if keys[k] {
			t.Errorf("key %s is shared", k)
		}
		keys[k] = true
		if !strings.HasPrefix(k, "goldens/") {
			t.Errorf("key %q is not under goldens/", k)
		}
	}
	again, err := objectKey("o1", "a1", "p1", sum[:])
	if err != nil {
		t.Fatal(err)
	}
	if !keys[again] {
		t.Error("the key of an image is not stable")
	}
}
