// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package plxtest

import (
	"crypto/rand"
	"strings"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/bundle"
	"github.com/nightCode42/plux3/backend/internal/compiler"
	"github.com/nightCode42/plux3/backend/internal/schema"
	"github.com/nightCode42/plux3/backend/internal/signing/runkey"
)

const dialogScenario = `schemaVersion: 1.0.0
kind: scenarios
scenarios:
  - name: confirm
    page: confirm-delete
    expect:
      - visible: confirm
`

// A release with device functions runs them on the device function engine,
// and a dialog or sheet page opens as the runtime presents it, so the
// harness renders the release as a host does (build log row 299).
//
// Verifies: TST-002, TST-003.
func TestHarnessRunsDeviceFunctionsAndPresentsDialogs(t *testing.T) {
	t.Parallel()
	f, findings := parse(t, "tests/confirm.scenario.yaml", dialogScenario)
	if len(findings) > 0 {
		t.Fatal(findings)
	}
	rel := &Release{
		AppID: "app", KeyID: "k", PublicKey: []byte{1, 2}, Files: map[string][]byte{},
		Kinds:           map[string]schema.PageKind{"confirm-delete": schema.PageKindDialog},
		DeviceFunctions: true,
	}
	files, _, err := Generate(Input{Release: rel, Files: []*File{f}, Runtime: Dependency{Path: "/src/plux_flutter"}})
	if err != nil {
		t.Fatal(err)
	}
	pub := string(files["pubspec.yaml"])
	for _, want := range []string{
		"  plux_functions:\n    path: '/src/plux_functions'\n",
		"dependency_overrides:\n  plux_flutter:\n    path: '/src/plux_flutter'\n",
	} {
		if !strings.Contains(pub, want) {
			t.Errorf("pubspec lacks %q:\n%s", want, pub)
		}
	}
	h := string(files["test/plux_harness.dart"])
	for _, want := range []string{"import 'package:plux_functions/plux_functions.dart';", "functionEngine: PluxFunctions(),", "PluxPresentation.values.byName(kind)"} {
		if !strings.Contains(h, want) {
			t.Errorf("harness lacks %q", want)
		}
	}
	if strings.Contains(h, "{{") {
		t.Error("harness: a placeholder left")
	}
	if test := string(files["test/01_tests_confirm_scenario_yaml_test.dart"]); !strings.Contains(test, "page: 'confirm-delete',\n        kind: 'dialog',\n") {
		t.Errorf("the scenario does not open the dialog as one:\n%s", test)
	}

	golden := GenerateGolden(GoldenInput{Release: rel, Runtime: Dependency{Version: "^0.6.0", FunctionsVersion: "^0.1.0"}, Cases: []GoldenCase{
		{ID: "confirm", Route: "confirm-delete", Width: 360, Height: 640, Ratio: 1, TextScale: 1, Locale: "en"},
		{ID: "home", Route: "home", Width: 360, Height: 640, Ratio: 1, TextScale: 1, Locale: "en"},
	}})
	if p := string(golden["pubspec.yaml"]); !strings.Contains(p, "  plux_functions: ^0.1.0\n") || strings.Contains(p, "dependency_overrides") {
		t.Errorf("published pubspec:\n%s", p)
	}
	if g := string(golden["test/golden_test.dart"]); strings.Count(g, "kind: 'dialog',") != 1 {
		t.Errorf("only the dialog case is presented as one:\n%s", g)
	}
}

// Without device functions or other page kinds, nothing more is generated.
//
// Verifies: TST-002.
func TestHarnessWithoutDeviceFunctionsLeavesTheEngineOut(t *testing.T) {
	t.Parallel()
	f, _ := parse(t, "tests/confirm.scenario.yaml", dialogScenario)
	files, _, err := Generate(Input{Release: &Release{AppID: "app", KeyID: "k", PublicKey: []byte{1, 2}, Files: map[string][]byte{}}, Files: []*File{f}, Runtime: Dependency{Path: "/src/plux_flutter"}})
	if err != nil {
		t.Fatal(err)
	}
	if p := string(files["pubspec.yaml"]); strings.Contains(p, "plux_functions") || strings.Contains(p, "dependency_overrides") {
		t.Errorf("pubspec:\n%s", p)
	}
	if h := string(files["test/plux_harness.dart"]); strings.Contains(h, "PluxFunctions") || strings.Contains(h, "{{") {
		t.Error("harness registers an engine or keeps a placeholder")
	}
	if test := string(files["test/01_tests_confirm_scenario_yaml_test.dart"]); strings.Contains(test, "kind:") {
		t.Errorf("a screen passes a kind:\n%s", test)
	}
}

// BuildRelease says which bundles carry device functions and which routes
// are not screens.
//
// Verifies: TST-003.
func TestBuildReleaseRecordsDeviceFunctionsAndPageKinds(t *testing.T) {
	t.Parallel()
	k, err := runkey.New(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	page := func(route string, kind schema.PageKind) schema.Loaded[schema.PageDocument] {
		return schema.Loaded[schema.PageDocument]{Doc: &schema.PageDocument{Route: route, PageKind: kind}}
	}
	res := &compiler.Result{
		App:     &compiler.Bundle{Key: "app", Data: []byte{1}},
		Plugins: []*compiler.Bundle{{Key: "plain", Data: []byte{2}}, {Key: "fn", Data: []byte{3}, Features: []string{bundle.FeatureWasm}}},
		Project: &schema.Project{Plugins: []schema.Plugin{{Pages: []schema.Loaded[schema.PageDocument]{
			page("home", schema.PageKindScreen), page("sheet", schema.PageKindBottomSheet), page("full", schema.PageKindFullscreenDialog), page("", schema.PageKindDialog),
		}}}},
	}
	rel, err := BuildRelease(res, k)
	if err != nil {
		t.Fatal(err)
	}
	if !rel.DeviceFunctions {
		t.Error("a bundle with a wasm section is not recorded")
	}
	if len(rel.Kinds) != 2 || rel.Kinds["sheet"] != schema.PageKindBottomSheet || rel.Kinds["full"] != schema.PageKindFullscreenDialog {
		t.Errorf("kinds = %v", rel.Kinds)
	}
	res.Plugins = res.Plugins[:1]
	if rel, _ := BuildRelease(res, k); rel.DeviceFunctions {
		t.Error("a release without device functions asks for the engine")
	}
}
