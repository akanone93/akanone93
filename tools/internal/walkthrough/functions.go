// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package walkthrough

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// functionExamples maps each function the Plux Bank project declares to the
// example crate of the SDK that implements it (sdk/fn/examples).
var functionExamples = map[string]string{"bank.transferFee": "fee"}

// publishEnv is the environment `plux publish` goes to when none is named.
const publishEnv = "development"

// standaloneManifest is an example crate's manifest outside the SDK's
// workspace, with the patch `cargo vendor` needs while plux-fn is
// unpublished: the crate name and the path of plux-fn.
const standaloneManifest = `[package]
name = %q
version = "0.1.0"
edition = "2024"
publish = false

[dependencies]
plux-fn = "0.1"
serde = { version = "1", features = ["derive"] }
serde_json = "1"
schemars = "1.2"

[patch.crates-io]
plux-fn = { path = %q }
`

// deployFunctions builds and deploys the project's server functions, which
// its pages call, before anything is published (FN-016): each is laid out
// from its example crate beside the project's declaration, deployed with
// `plux fn deploy` to the environment publishes go to, and promoted, without
// a rebuild, to the fleet's and production.
func (e *Env) deployFunctions(ctx context.Context, w *walk, actor string) error {
	entries, err := os.ReadDir(filepath.Join(e.Project, "functions"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read the project's functions: %w", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		crate, ok := functionExamples[name]
		if !ok {
			return fmt.Errorf("the project declares the function %s, which has no example crate", name)
		}
		dir := filepath.Join(e.Project, "functions", name)
		if err := os.CopyFS(filepath.Join(dir, "src"), os.DirFS(filepath.Join(e.FnSDK, "examples", crate, "src"))); err != nil {
			return fmt.Errorf("lay out %s: %w", name, err)
		}
		manifest := fmt.Sprintf(standaloneManifest, crate, filepath.Join(e.FnSDK, "plux-fn"))
		if err := os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte(manifest), 0o600); err != nil {
			return fmt.Errorf("lay out %s: %w", name, err)
		}
		out, err := w.plux(ctx, actor, "fn", "deploy", name, "--env", publishEnv, "--app", e.App, "-C", e.Project, "--json")
		if err != nil {
			return err
		}
		var deployed struct {
			Version struct {
				Version uint32 `json:"version"`
			} `json:"version"`
		}
		if err := json.Unmarshal([]byte(out), &deployed); err != nil || deployed.Version.Version == 0 {
			return fmt.Errorf("`plux fn deploy %s` printed no version: %s", name, out)
		}
		for _, env := range []string{e.FleetEnv, e.ProdEnv} {
			ref := fmt.Sprintf("%s@%d", name, deployed.Version.Version)
			if _, err := w.plux(ctx, actor, "fn", "promote", ref, "--from", publishEnv, "--to", env, "--app", e.App, "-C", e.Project); err != nil {
				return err
			}
		}
	}
	return nil
}
