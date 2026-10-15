// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package function_test

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/buildclient"
	"github.com/nightCode42/plux3/backend/internal/compiler"
	"github.com/nightCode42/plux3/backend/internal/function"
	"github.com/nightCode42/plux3/backend/internal/schema"
	"github.com/nightCode42/plux3/backend/internal/signing"
)

// loanManifest is the standalone manifest of the loan example: the workspace
// form of sdk/fn/examples/loan, with the patch that `cargo vendor` needs
// while plux-fn is unpublished.
const loanManifest = `[package]
name = "loan"
version = "0.1.0"
edition = "2024"
publish = false

[dependencies]
plux-fn = "0.1"
serde = { version = "1", features = ["derive"] }
serde_json = "1"
schemars = "1.2"
rust_decimal = { version = "1.43", default-features = false, features = ["std", "serde-str", "maths"] }

[patch.crates-io]
plux-fn = { path = %q }
`

// loanSource lays out the loan example as an uploadable crate with its
// vendored dependencies.
func loanSource(t *testing.T, sdk, examples string) string {
	t.Helper()
	proj := filepath.Join(t.TempDir(), "loan")
	if err := os.MkdirAll(proj, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.CopyFS(filepath.Join(proj, "src"), os.DirFS(filepath.Join(examples, "loan", "src"))); err != nil {
		t.Fatal(err)
	}
	doc, err := os.ReadFile(filepath.Join(examples, "loan", "function.json")) //nolint:gosec // G304: a fixed path in the repository.
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{"function.json": doc, "Cargo.toml": []byte(fmt.Sprintf(loanManifest, sdk))} {
		if err := os.WriteFile(filepath.Join(proj, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	runIn(t, proj, "cargo", "vendor", "--quiet")
	return proj
}

// Verifies: FN-051, FN-004, FN-006, FN-019.
// The loan example, a device function, goes through the real builder role
// (Rust toolchain and Binaryen) as a build, is deployed, appears in the
// catalogue with its placement and derived schemas, and is linked by the
// builder role into the plugin's device module, which is found in the cache
// the second time and, linked again from nothing, is byte-identical.
func TestRustLinkEndToEnd_FN_051(t *testing.T) {
	if os.Getenv("PLUX_RUST_E2E") != "1" {
		t.Skip("set PLUX_RUST_E2E=1 to build with the real Rust toolchain and Binaryen")
	}
	root, err := filepath.Abs("../../../sdk/fn")
	if err != nil {
		t.Fatal(err)
	}
	archive := packSource(t, loanSource(t, filepath.Join(root, "plux-fn"), filepath.Join(root, "examples")))
	wasmOpt := filepath.Join(t.TempDir(), "binaryen")
	if err := os.MkdirAll(wasmOpt, 0o750); err != nil {
		t.Fatal(err)
	}
	runIn(t, wasmOpt, "bun", "add", "--exact", "binaryen@132.0.0")

	w := newWorld(t, nil, nil)
	backend, err := signing.NewFile(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	signer, err := buildclient.NewSigner(backend, buildclient.DefaultKeyRef)
	if err != nil {
		t.Fatal(err)
	}
	client, err := buildclient.NewClient(buildclient.Options{
		HTTP: &http.Client{Timeout: 30 * time.Minute}, URL: startBuilderRole(t, filepath.Join(root, "plux-fn"), wasmOpt),
		Objects: w.store, MaxResponseBytes: 80 << 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	w.builder.delegate, w.signer.delegate = client, signer
	w.deviceFunctions.Store(true)

	ctx := t.Context()
	const name = "loan.calculateSchedule"
	src, err := w.svc.UploadSource(ctx, w.developer, w.app, w.envs["development"], name, bytes.NewReader(archive))
	if err != nil {
		t.Fatal(err)
	}
	b, err := w.svc.RequestBuild(ctx, w.developer, src.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if err := w.svc.RunBuild(ctx, w.queue.last(t)); err != nil {
		t.Fatalf("RunBuild: %v", err)
	}
	t.Logf("build job of the loan example took %s", time.Since(start).Round(time.Second))
	v, err := w.svc.Deploy(ctx, w.developer, b.ID, "")
	if err != nil || v.Placement != function.PlacementDevice {
		t.Fatalf("Deploy: %v %+v", err, v)
	}

	linking := w.service(t, func(o *function.Options) { o.Linker = client })
	cat, err := linking.Catalogue(ctx, w.org, w.app, w.envs["development"])
	if err != nil {
		t.Fatal(err)
	}
	info, ok := cat.Function(name, "")
	if !ok || info.Placement != schema.PlacementDevice || info.Version != 1 || len(info.InputSchema) == 0 || len(info.OutputSchema) == 0 || !cat.DeviceFunctionsEnabled() {
		t.Fatalf("catalogue entry = %+v, %v", info, ok)
	}

	mods := linking.DeviceModules(ctx, w.org, w.app, w.envs["development"])
	refs := []compiler.DeviceFunctionRef{{Name: name, Version: 1}}
	start = time.Now()
	first, err := mods.Module("loans", refs)
	if err != nil {
		t.Fatalf("Module: %v", err)
	}
	t.Logf("link of one device function (cargo, schema build, wasm-opt) took %s; module %d bytes", time.Since(start).Round(time.Second), len(first.Wasm))
	if !bytes.HasPrefix(first.Wasm, []byte("\x00asm")) || first.ABIVersion != 1 || first.Index[name] != 0 || len(first.Index) != 1 {
		t.Fatalf("module = %d bytes, ABI %d, index %v", len(first.Wasm), first.ABIVersion, first.Index)
	}
	start = time.Now()
	cached, err := mods.Module("loans", refs)
	if err != nil || !bytes.Equal(cached.Wasm, first.Wasm) {
		t.Fatalf("cache hit: %v", err)
	}
	t.Logf("the second request took %s (cache hit)", time.Since(start).Round(time.Millisecond))

	// A set linked from nothing yields the same bytes: linking is deterministic.
	fresh := w.service(t, func(o *function.Options) { o.Linker = client })
	w.removeLinked(t)
	again, err := fresh.DeviceModules(ctx, w.org, w.app, w.envs["development"]).Module("loans", refs)
	if err != nil || !bytes.Equal(again.Wasm, first.Wasm) {
		t.Errorf("linking again gave other bytes (%d and %d): %v", len(again.Wasm), len(first.Wasm), err)
	}
}
