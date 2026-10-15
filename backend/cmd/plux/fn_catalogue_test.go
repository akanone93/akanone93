// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/schema"
)

// documents is the directory of the conformance projects.
var documents = filepath.Join("..", "..", "..", "schema", "testdata", "documents")

// copyProject copies a conformance project to a temporary directory.
func copyFixture(t *testing.T, fixture string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS(filepath.Join(documents, fixture))); err != nil {
		t.Fatal(err)
	}
	return dir
}

// execute runs the CLI and returns its exit code and streams.
func execute(args ...string) (code int, stdout, stderr string) {
	var out, errOut bytes.Buffer
	code = run(args, &out, &errOut)
	return code, out.String(), errOut.String()
}

// The reference projects declare functions whose build output is committed:
// they validate, build and generate code with no server (CLI-005).
//
// Verifies: CLI-005, FN-004.
func TestOfflineFunctionProjects_CLI_005(t *testing.T) {
	t.Parallel()
	for _, fixture := range []string{"plux_express", "plux_bank"} {
		dir := filepath.Join(documents, fixture)
		t.Run(fixture+" validate", func(t *testing.T) {
			t.Parallel()
			if code, _, stderr := execute("validate", dir); code != exitOK || !strings.Contains(stderr, "0 errors") {
				t.Errorf("validate = %d, stderr:\n%s", code, stderr)
			}
		})
		t.Run(fixture+" build", func(t *testing.T) {
			t.Parallel()
			code, stdout, stderr := execute("build", "--json", "-o", t.TempDir(), dir)
			if code != exitOK {
				t.Fatalf("build = %d, stderr:\n%s", code, stderr)
			}
			var rep report
			if err := json.Unmarshal([]byte(stdout), &rep); err != nil {
				t.Fatal(err)
			}
			packed := false
			for _, b := range rep.Bundles {
				packed = packed || slices.Contains(b.Features, "section.wasm.v1")
			}
			if want := fixture == "plux_express"; packed != want {
				t.Errorf("a bundle carries the device module = %v, want %v", packed, want)
			}
		})
		t.Run(fixture+" codegen", func(t *testing.T) {
			t.Parallel()
			if code, _, stderr := execute("codegen", "--host", t.TempDir(), dir); code != exitOK {
				t.Errorf("codegen = %d, stderr:\n%s", code, stderr)
			}
		})
	}
}

// The module of a device function is linked into the bundle as built: the
// bundle of the plugin that grants it holds the committed module's bytes.
//
// Verifies: CLI-005, FN-051.
func TestOfflineBuildPacksTheBuiltModule_FN_051(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(documents, "plux_express")
	out := t.TempDir()
	if code, _, stderr := execute("build", "-o", out, dir); code != exitOK {
		t.Fatalf("build = %d, stderr:\n%s", code, stderr)
	}
	module, err := os.ReadFile(filepath.Join(builtDir(dir, "loan.calculateSchedule"), moduleName))
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := os.ReadFile(filepath.Join(out, "plugins", "shop.pxb"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(bundle, module) {
		t.Error("the plugin bundle does not contain the built module")
	}
}

// A function without build output is named, with the command that makes it.
//
// Verifies: CLI-005.
func TestOfflineFunctionWithoutBuildOutput_CLI_005(t *testing.T) {
	t.Parallel()
	dir := copyFixture(t, "plux_bank")
	if err := os.RemoveAll(builtDir(dir, "bank.transferFee")); err != nil {
		t.Fatal(err)
	}
	for _, cmd := range [][]string{{"validate", dir}, {"build", "-o", t.TempDir(), dir}, {"codegen", "--host", t.TempDir(), dir}} {
		code, _, stderr := execute(cmd...)
		if code != exitFailed || !strings.Contains(stderr, "run plux fn build bank.transferFee") {
			t.Errorf("%s = %d, stderr:\n%s", cmd[0], code, stderr)
		}
	}
}

// A device function whose module is missing validates and generates code,
// which need no module, and fails to build with the command that makes it.
//
// Verifies: CLI-005, FN-051.
func TestOfflineDeviceFunctionWithoutModule_FN_051(t *testing.T) {
	t.Parallel()
	dir := copyFixture(t, "plux_express")
	if err := os.Remove(filepath.Join(builtDir(dir, "loan.calculateSchedule"), moduleName)); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := execute("validate", dir); code != exitOK {
		t.Errorf("validate = %d, stderr:\n%s", code, stderr)
	}
	if code, _, stderr := execute("codegen", "--host", t.TempDir(), dir); code != exitOK {
		t.Errorf("codegen = %d, stderr:\n%s", code, stderr)
	}
	code, _, stderr := execute("build", "-o", t.TempDir(), dir)
	if code != exitFailed || !strings.Contains(stderr, "run plux fn build loan.calculateSchedule") {
		t.Errorf("build = %d, stderr:\n%s", code, stderr)
	}
}

// The schemas of the build type the function steps: an input that does not
// fit the input schema is refused offline, as it is when publishing
// (FN-004, ACT-001).
//
// Verifies: CLI-005, FN-004.
func TestOfflineCatalogueTypesFunctionInput_FN_004(t *testing.T) {
	t.Parallel()
	dir := copyFixture(t, "plux_bank")
	path := filepath.Join(builtDir(dir, "bank.transferFee"), inputSchemaFile)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	doc["required"] = append(doc["required"].([]any), "noSuchProperty")                     //nolint:forcetypeassert // the schema of the fixture lists required properties.
	doc["properties"].(map[string]any)["noSuchProperty"] = map[string]any{"type": "string"} //nolint:forcetypeassert // as above.
	edited, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, edited, 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := execute("validate", dir)
	if code != exitFailed || !strings.Contains(stderr, "noSuchProperty") {
		t.Errorf("validate = %d, stderr:\n%s", code, stderr)
	}
}

// The offline catalogue describes a function by its document and answers
// for any alias with the one local build.
//
// Verifies: CLI-005, FN-006, FN-020.
func TestLocalCatalogue_CLI_005(t *testing.T) {
	t.Parallel()
	cat, err := loadLocalCatalogue(filepath.Join(documents, "plux_bank"))
	if err != nil {
		t.Fatal(err)
	}
	info, ok := cat.Function("bank.transferFee", "stable")
	if !ok {
		t.Fatal("the function is unknown")
	}
	if info.Placement != schema.PlacementServer || info.Version != localVersion || !info.RequiresAttestation || !info.RequiresSCA ||
		info.Capabilities == nil || info.Capabilities.Claims == nil || !*info.Capabilities.Claims {
		t.Errorf("info = %+v", info)
	}
	if len(info.InputSchema) == 0 || len(info.OutputSchema) == 0 {
		t.Error("the schemas of the build are missing")
	}
	if _, ok := cat.Function("bank.other", ""); ok {
		t.Error("an unknown function was found")
	}
	if !cat.DeviceFunctionsEnabled() {
		t.Error("device placement is refused offline")
	}
}

// A project without functions has an empty catalogue.
//
// Verifies: CLI-005.
func TestLocalCatalogueWithoutFunctions_CLI_005(t *testing.T) {
	t.Parallel()
	cat, err := loadLocalCatalogue(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cat.Function("any.thing", ""); ok {
		t.Error("an empty catalogue knows a function")
	}
}
