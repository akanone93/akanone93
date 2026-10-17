// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package plxtest

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/nightCode42/plux3/backend/internal/codegen"
	"github.com/nightCode42/plux3/backend/internal/schema"
)

// GoldenGroup is the group the golden tests are reported under, so
// ParseReport finds a case by Case{File: GoldenGroup, Name: id}.
const GoldenGroup = "golden"

// goldenFamilies are the families the golden harness registers from the
// vendored Noto fonts (TST-003, ADR-0070), each with its font files below
// fonts/: the platform families the runtime asks for, which stand for the
// system fonts, and the script fallbacks.
var goldenFamilies = []struct {
	family string
	files  []string
}{
	{"Roboto", []string{"NotoSans_400Regular.ttf", "NotoSans_700Bold.ttf"}},
	{".SF UI Text", []string{"NotoSans_400Regular.ttf", "NotoSans_700Bold.ttf"}},
	{".SF UI Display", []string{"NotoSans_400Regular.ttf", "NotoSans_700Bold.ttf"}},
	{"NotoSans", []string{"NotoSans_400Regular.ttf", "NotoSans_700Bold.ttf"}},
	{"NotoNaskhArabic", []string{"NotoNaskhArabic_400Regular.ttf", "NotoNaskhArabic_700Bold.ttf"}},
	{"NotoSansEthiopic", []string{"NotoSansEthiopic_400Regular.ttf", "NotoSansEthiopic_700Bold.ttf"}},
}

// GoldenCase is one screen the golden harness renders.
type GoldenCase struct {
	// ID names the case; it is the test's name.
	ID string
	// Route is the page's route.
	Route string
	// Width and Height are the frame's logical size, Ratio its pixel ratio.
	Width, Height int
	Ratio         float64
	// TextScale is the text scale factor.
	TextScale float64
	// Dark renders the dark theme.
	Dark bool
	// Locale is the BCP 47 tag the runtime is set to.
	Locale string
	// Params are the page's inputs.
	Params map[string]json.RawMessage
}

// GoldenInput is everything the golden test project is generated from.
type GoldenInput struct {
	Release *Release
	// Natives is the project's native catalogue, or nil.
	Natives *schema.NativeCatalogueDocument
	Runtime Dependency
	// Cases are rendered in this order; case i is written to Out/i.png.
	Cases []GoldenCase
	// DataSources are the names of every data source of the app and its
	// plugins, which the harness mocks with their design-time mocks.
	DataSources []string
	// Out is the absolute directory the images are written to.
	Out string
	// Fonts are the vendored font files by name.
	Fonts map[string][]byte
}

// GenerateGolden writes the Flutter test project of a golden run: the
// pubspec, the release, the harness and the one test file that renders
// every case with the clock frozen (the widget tester's) and all data
// sources mocked. The files are in a fixed order for the same input.
func GenerateGolden(in GoldenInput) map[string][]byte {
	files := map[string][]byte{}
	for p, data := range in.Release.Files {
		files[BaselineDir+"/"+p] = data
	}
	for name, data := range in.Fonts {
		files["fonts/"+name] = data
	}
	files["pubspec.yaml"] = []byte(pubspec(in.Runtime))
	files["test/plux_harness.dart"] = []byte(harness(len(in.DataSources) > 0))
	files["test/plux_project.dart"] = []byte(project(Input{Release: in.Release, Natives: in.Natives}))
	files["test/golden_test.dart"] = []byte(goldenTest(in))
	return files
}

func goldenTest(in GoldenInput) string {
	var b strings.Builder
	b.WriteString("// Written by plux golden (TST-003); do not edit.\n// ignore_for_file: implementation_imports\n\n")
	b.WriteString("import 'dart:io';\nimport 'dart:typed_data';\n\nimport 'package:flutter/material.dart';\nimport 'package:flutter/services.dart' show FontLoader;\nimport 'package:flutter_test/flutter_test.dart';\n\nimport 'plux_harness.dart';\n\n")
	fmt.Fprintf(&b, "const _out = %s;\n\n", codegen.DartString(in.Out))
	b.WriteString("Future<void> _loadFonts() async {\n")
	for _, f := range goldenFamilies {
		fmt.Fprintf(&b, "  await _load(%s, [", codegen.DartString(f.family))
		for i, name := range f.files {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(codegen.DartString(name))
		}
		b.WriteString("]);\n")
	}
	b.WriteString("}\n\nFuture<void> _load(String family, List<String> files) async {\n  final loader = FontLoader(family);\n  for (final f in files) {\n    loader.addFont(Future.value(ByteData.sublistView(File('fonts/$f').readAsBytesSync())));\n  }\n  await loader.load();\n}\n\n")
	b.WriteString("/// The host theme: Roboto and the system families stand for the phone's own\n/// fonts, and the script fonts follow for what Noto Sans lacks.\nThemeData _themeFor(Brightness brightness) => ThemeData(\n  brightness: brightness,\n  fontFamily: 'Roboto',\n  fontFamilyFallback: ['NotoSans', 'NotoNaskhArabic', 'NotoSansEthiopic'],\n);\n\n")
	b.WriteString("const _dataSources = <String, String>{")
	for i, name := range slices.Sorted(slices.Values(in.DataSources)) {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%s: 'success'", codegen.DartString(name))
	}
	b.WriteString("};\n\nvoid main() {\n  setUpAll(_loadFonts);\n  group(" + codegen.DartString(GoldenGroup) + ", () {\n")
	for i, c := range in.Cases {
		fmt.Fprintf(&b, "    testWidgets(%s, (tester) async {\n", codegen.DartString(c.ID))
		fmt.Fprintf(&b, "      final s = await ScenarioSession.start(\n        tester,\n        page: %s,\n        params: %s,\n        dataSources: _dataSources,\n", codegen.DartString(c.Route), object(c.Params))
		fmt.Fprintf(&b, "        frame: const Size(%d, %d),\n        pixelRatio: %s,\n        textScale: %s,\n        themeMode: %s,\n        locale: %s,\n        theme: _themeFor(%s),\n      );\n",
			c.Width, c.Height, number(c.Ratio), number(c.TextScale), themeMode(c.Dark), codegen.DartString(c.Locale), brightness(c.Dark))
		fmt.Fprintf(&b, "      try {\n        final png = await s.capture();\n        await tester.runAsync(() => File('$_out/%04d.png').writeAsBytes(png));\n      } finally {\n        await s.finish();\n      }\n    });\n", i)
	}
	b.WriteString("  });\n}\n")
	return b.String()
}

func themeMode(dark bool) string {
	if dark {
		return "ThemeMode.dark"
	}
	return "ThemeMode.light"
}

func brightness(dark bool) string {
	if dark {
		return "Brightness.dark"
	}
	return "Brightness.light"
}
