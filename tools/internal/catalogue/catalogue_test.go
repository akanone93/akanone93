// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package catalogue

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

// Verifies: QA-073 (catalogue row parsing).
func TestParseWidgetRow(t *testing.T) {
	tests := []struct {
		name string
		line string
		want []Entry
	}{
		{"pair", "| `Container` | P3 | `Row` | P4 |", []Entry{{"Container", 3}, {"Row", 4}}},
		{"parens", "| `GridView` (item template) | P3 | `Image` (asset, network) | P5 |", []Entry{{"GridView", 3}, {"Image", 5}}},
		{"slash", "| `RichText` / `Text.rich` | P3 | `Text` | P3 |", []Entry{{"RichText", 3}, {"Text", 3}}},
		{"slash both widgets", "| `TextField` / `TextFormField` | P3 |", []Entry{{"TextField", 3}, {"TextFormField", 3}}},
		{"multi name cell", "| `TabBar`, `TabBarView` | P4 | `Tab` | P4 |", []Entry{{"TabBar", 4}, {"TabBarView", 4}, {"Tab", 4}}},
		{"c1 purpose ignored", "| `If` | Conditional with `Then` slots | P3 |", []Entry{{"If", 3}}},
		{"c9 shape", "| `LineChart`, `BarChart` | Charts | P5 | `plux_charts` |", []Entry{{"LineChart", 5}, {"BarChart", 5}}},
		{"header", "| Widget | Phase | Widget | Phase |", nil},
		{"separator", "|---|---|", nil},
		{"not a row", "text with `Foo` | P3", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ParseWidgetRow(tc.line); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// Verifies: QA-073 (catalogue row parsing).
func TestParseActionRow(t *testing.T) {
	tests := []struct {
		name string
		line string
		want []Entry
	}{
		{"single", "| `navigate` | Navigation | Push `Route` | P4 |", []Entry{{"navigate", 4}}},
		{"multi", "| `setState`, `resetState` | State | Write | P5 |", []Entry{{"setState", 5}, {"resetState", 5}}},
		{"uppercase ignored", "| `Widget` | X | Y | P5 |", nil},
		{"header", "| Action | Category | Description | Phase |", nil},
		{"no phase", "| `x` | a | b |", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ParseActionRow(tc.line); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// Verifies: QA-073 (builder detection).
func TestBuilderDetection(t *testing.T) {
	gen := "const Map<int, NodeBuilder> generatedBuilders = {\n  61: _buildActionChip, // ActionChip\n  8: _buildAlign, // Align\n};\nconst x = {\n  99: _other,\n};\n"
	ids := ScanGeneratedBuilders(gen)
	if !ids[61] || !ids[8] || ids[99] {
		t.Errorf("ids = %v", ids)
	}
	b := NewBuilders(ids)
	b.ScanHandWritten("WidgetIds.listView: _listView,\nWidgetIds.if_: _if,\n_widget('SecureTextField')\nformScopeWidgetId: _formScope,\n")
	for _, tc := range []struct {
		name string
		id   int
		want bool
	}{
		{"ActionChip", 61, true},
		{"ListView", 1, true},
		{"If", 2, true},
		{"SecureTextField", 3, true},
		{"FormScope", 5, true},
		{"TabBar", 4, false},
	} {
		if got := b.Has(tc.name, tc.id); got != tc.want {
			t.Errorf("Has(%s) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// Verifies: QA-073 (builders registered by optional packages).
func TestBuilderPackageKeys(t *testing.T) {
	b := NewBuilders(map[int]bool{})
	b.ScanHandWritten("abstract final class PluxChartIds {\n  static const int lineChart = 193;\n  static const int pieChart = 195;\n  static const int unused = 197;\n}\nabstract final class PluxRiveIds {\n  static const int riveView = 206;\n}\n")
	b.ScanHandWritten("final class PluxCharts implements PluxWidgetPackage {\n  @override\n  Map<int, NodeBuilder> get builders => {\n    PluxChartIds.lineChart: (c) => _cartesian(c, bars: false, widget: 'LineChart'),\n    PluxChartIds.pieChart: _pie,\n    42: _forty,\n  };\n}\nfinal class PluxRive {\n  Map<int, NodeBuilder> get builders => {PluxRiveIds.riveView: _riveView};\n}\n")
	for _, tc := range []struct {
		name string
		id   int
		want bool
	}{
		{"LineChart", 193, true},
		{"PieChart", 195, true},
		{"RiveView", 206, true},
		{"Forty", 42, true},
		{"Unused", 197, false},
		{"Sparkline", 1, false},
	} {
		if got := b.Has(tc.name, tc.id); got != tc.want {
			t.Errorf("Has(%s, %d) = %v, want %v", tc.name, tc.id, got, tc.want)
		}
	}
}

func TestConstName(t *testing.T) {
	for in, want := range map[string]string{"If": "if_", "Switch": "switch_", "ListView": "listView", "": ""} {
		if got := ConstName(in); got != want {
			t.Errorf("ConstName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseAppendices(t *testing.T) {
	doc := "## Appendix C — Widget Catalogue\n| `A` | P3 |\n## Appendix D — Built-in Action Catalogue\n| `go` | c | d | P4 |\n## Appendix E\n| `Z` | P3 |\n"
	w, a := ParseAppendices(doc)
	if !reflect.DeepEqual(w, map[string]int{"A": 3}) || !reflect.DeepEqual(a, map[string]int{"go": 4}) {
		t.Errorf("w=%v a=%v", w, a)
	}
}

// Verifies: QA-073 against the real repository.
func TestCheckRealRepository(t *testing.T) {
	findings, err := Check("../../..", 8)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.IsSorted(findings) {
		t.Errorf("findings not sorted: %v", findings)
	}
	for _, f := range findings {
		// Text is built by the generated renderer, PieChart by plux_charts'
		// builders map through a class constant.
		if strings.HasPrefix(f, "WIDGET Text:") || strings.HasPrefix(f, "WIDGET PieChart:") {
			t.Errorf("Text reported: %s", f)
		}
	}
	if !slices.IsSorted(findings) {
		t.Error("findings not sorted")
	}
}

func TestScanStructuralActions(t *testing.T) {
	src := "enum StructuralAction {\n  /// doc; with semicolon\n  forEach,\n\n  parallel, // x\n  callFlow;\n  const StructuralAction();\n}\n"
	if got, want := ScanStructuralActions(src), []string{"forEach", "parallel", "callFlow"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if got := ScanStructuralActions("enum Other { a }"); got != nil {
		t.Errorf("got %v", got)
	}
}
