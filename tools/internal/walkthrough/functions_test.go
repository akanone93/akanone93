// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package walkthrough

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Verifies: FN-016.
// The project's functions are laid out from their example crates, deployed
// once where publishes go, and promoted without a rebuild to the fleet's
// environment and production.
func TestDeployFunctions(t *testing.T) {
	env, plux, _ := testEnv(t)
	env.FnSDK = filepath.Join(t.TempDir(), "sdk")
	for path, data := range map[string]string{
		filepath.Join(env.FnSDK, "examples", "fee", "src", "lib.rs"):                 "// fee",
		filepath.Join(env.Project, "functions", "bank.transferFee", "function.json"): `{"name": "bank.transferFee"}`,
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	plux.rules = []rule{
		{[]string{"alice|fn deploy bank.transferFee --env development"}, out(`{"build": "b1", "state": "succeeded", "version": {"version": 3}}`)},
		{[]string{"alice|fn promote bank.transferFee@3 --from development"}, out("")},
	}
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	step := Step{ID: "functions", Title: "functions", Run: func(ctx context.Context, run *Run) error {
		return env.deployFunctions(ctx, env.newWalk(run), "alice")
	}}
	if _, err := (&Runner{Steps: []Step{step}, Store: store, Now: newFakeClock().Now}).Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	crate := filepath.Join(env.Project, "functions", "bank.transferFee")
	manifest, err := os.ReadFile(filepath.Join(crate, "Cargo.toml"))
	if err != nil || !strings.Contains(string(manifest), `name = "fee"`) || !strings.Contains(string(manifest), filepath.Join(env.FnSDK, "plux-fn")) {
		t.Errorf("Cargo.toml = %q, %v", manifest, err)
	}
	if _, err := os.Stat(filepath.Join(crate, "src", "lib.rs")); err != nil {
		t.Errorf("the example's sources were not copied: %v", err)
	}
	for _, to := range []string{"staging", "production"} {
		if !plux.called("fn promote bank.transferFee@3", "--to "+to) {
			t.Errorf("not promoted to %s: %v", to, plux.calls)
		}
	}
}
