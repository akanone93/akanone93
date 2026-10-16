// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema"
)

const (
	themeFile   = "theme.json"
	pageColumn  = "root/slots/body/slots/child"
	assetSource = `{"asset": {"$asset": "01a0c450-6c00-700d-8000-000000019223"}}`
)

// a11yProject returns the fixture with the given profile, a theme that has
// the role colours the checks need (light, dark), and the page's column
// replaced by the nodes in kids, a JSON list. "%ID%" in kids is replaced by
// unique node IDs.
func a11yProject(t *testing.T, profile, onSurface, onSurfaceDark, kids string) fstest.MapFS {
	t.Helper()
	m := fixture(t)
	edit(t, m, "app.json", func(doc map[string]any) { doc["securityProfile"] = profile })
	edit(t, m, themeFile, func(doc map[string]any) {
		colors := at(t, doc, "tokens/color")
		colors["onSurface"] = map[string]any{"$value": onSurface, "$extensions": map[string]any{"dev.plux.dark": onSurfaceDark}}
	})
	n := 0
	for strings.Contains(kids, "%ID%") {
		n++
		kids = strings.Replace(kids, "%ID%", fmt.Sprintf("01a0c450-6c00-7fff-8000-%012x", n), 1)
	}
	onPage(func(t *testing.T, doc map[string]any) {
		col := at(t, doc, pageColumn)
		col["children"] = append(col["children"].([]any), raw(t, kids).([]any)...)
	})(t, m)
	return m
}

// text returns a Text node with a style.
func text(style, extra string) string {
	if style != "" {
		style = `, "style": ` + style
	}
	if extra != "" {
		extra = ", " + extra
	}
	return `{"id": "%ID%", "type": "Text", "props": {"data": "Hello"` + style + `}` + extra + `}`
}

// ownChildren is the number of children the fixture's column has before
// a11yProject appends its nodes.
const ownChildren = 6

// mine reports whether a diagnostic location is in a node a11yProject added.
func mine(file, path string) bool {
	rest, ok := strings.CutPrefix(path, "/"+pageColumn+"/children/")
	if !ok || file != calculatorPage {
		return false
	}
	idx, _, _ := strings.Cut(rest, "/")
	n, err := strconv.Atoi(idx)
	return err == nil && n >= ownChildren
}

// findings returns the diagnostics of a code.
func findings(res *Result, code plxerr.Code) []plxerr.Diagnostic {
	var out []plxerr.Diagnostic
	for _, d := range res.Diagnostics {
		if d.Code == code && mine(d.File, d.Path) {
			out = append(out, d)
		}
	}
	return out
}

// Verifies: A11Y-003.
func TestContrastRatio_A11Y_003(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		fg, bg string
		want   float64
	}{
		{"#777777", "#FFFFFF", 4.48},
		{"#767676", "#FFFFFF", 4.54},
		{"#000000", "#FFFFFF", 21},
		{"#FFFFFF", "#FFFFFF", 1},
	} {
		fg, _, _ := parseHex(tc.fg)
		bg, _, _ := parseHex(tc.bg)
		got := contrastRatio(fg, bg)
		if math.Abs(got-tc.want) > 0.01 || contrastRatio(bg, fg) != got {
			t.Errorf("%s on %s = %.3f, want %.2f", tc.fg, tc.bg, got, tc.want)
		}
	}
}

// Verifies: A11Y-003.
func TestTextContrast_A11Y_003(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		onSurface string
		dark      string
		style     string
		want      int
	}{
		{"normal text fails at 4.48", "#777777", "#EEEEEE", "", 1},
		{"normal text passes at 4.54", "#767676", "#EEEEEE", "", 0},
		{"24px text passes at 4.48", "#777777", "#EEEEEE", `{"fontSize": 24}`, 0},
		{"18.66px bold passes", "#777777", "#EEEEEE", `{"fontSize": 19, "fontWeight": "w700"}`, 0},
		{"19px regular fails", "#777777", "#EEEEEE", `{"fontSize": 19}`, 1},
		{"large text below 3:1 fails", "#AAAAAA", "#EEEEEE", `{"fontSize": 30}`, 1},
		{"own colour wins", "#777777", "#EEEEEE", `{"color": {"$token": "color.primary"}}`, 0},
		{"dark theme fails", "#000000", "#777777", "", 1},
		{"typography token size", "#777777", "#EEEEEE", `{"$token": "type.headlineMedium"}`, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := a11yProject(t, "standard", tc.onSurface, tc.dark, "["+text(tc.style, "")+"]")
			res := compileFS(m)
			if got := len(findings(res, plxerr.ContrastInsufficient)); got != tc.want {
				t.Errorf("got %d PLX-1403, want %d:\n%s", got, tc.want, list(res.Diagnostics))
			}
		})
	}
}

// Verifies: A11Y-003.
func TestButtonAndBackgroundContrast_A11Y_003(t *testing.T) {
	t.Parallel()
	button := `{"id": "%ID%", "type": "FilledButton", "slots": {"child": ` + text("", "") + `}}`
	m := a11yProject(t, "standard", "#000000", "#FFFFFF", "["+button+"]")
	res := compileFS(m)
	// Fixture: onPrimary #FFF on primary #5B3DF5 passes in light; in dark
	// the primary is #8B78FF, where white is below 4.5:1.
	got := findings(res, plxerr.ContrastInsufficient)
	if len(got) != 1 || !strings.Contains(got[0].Message, "dark") || strings.Contains(got[0].Message, "light") {
		t.Errorf("want one dark-only finding:\n%s", list(res.Diagnostics))
	}
	// A Container colour is the nearest opaque background.
	box := `{"id": "%ID%", "type": "Container", "props": {"color": "#777777"}, "slots": {"child": ` + text(`{"color": "#888888"}`, "") + `}}`
	res = compileFS(a11yProject(t, "standard", "#000000", "#FFFFFF", "["+box+"]"))
	if len(findings(res, plxerr.ContrastInsufficient)) != 1 {
		t.Errorf("want a finding for #888 on #777:\n%s", list(res.Diagnostics))
	}
	// An icon button's colour against the surface is a component boundary.
	icon := `{"id": "%ID%", "type": "IconButton", "props": {"icon": {"name": "home"}, "color": "#DDDDDD"}, "semantics": {"label": "Home"}}`
	res = compileFS(a11yProject(t, "standard", "#000000", "#FFFFFF", "["+icon+"]"))
	if len(findings(res, plxerr.ContrastInsufficient)) != 1 {
		t.Errorf("want a finding for the icon colour:\n%s", list(res.Diagnostics))
	}
}

// Verifies: A11Y-003.
func TestUnresolvableColoursAreNotChecked_A11Y_003(t *testing.T) {
	t.Parallel()
	u := &unit{tokens: map[string]*token{
		"color.a":               {path: "color.a", typ: "color", light: json.RawMessage(`"#112233"`), dark: json.RawMessage(`"#445566"`)},
		"color.b":               {path: "color.b", typ: "color", light: json.RawMessage(`"{color.a}"`)},
		"color.c":               {path: "color.c", typ: "color", light: json.RawMessage(`"{color.c}"`)},
		"contrast.high.color.a": {path: "contrast.high.color.a", typ: "color", light: json.RawMessage(`"#000000"`)},
	}}
	for _, tc := range []struct {
		raw     string
		v       variant
		want    rgb
		unknown bool
	}{
		{`"#112233"`, variantLight, rgb{0x11, 0x22, 0x33}, false},
		{`{"$token": "color.a"}`, variantDark, rgb{0x44, 0x55, 0x66}, false},
		{`{"$token": "color.a"}`, variantHigh, rgb{}, false},
		{`{"$token": "color.b"}`, variantLight, rgb{0x11, 0x22, 0x33}, false},
		{`{"$token": "color.c"}`, variantLight, rgb{}, true},
		{`{"$token": "color.missing"}`, variantLight, rgb{}, true},
		{`{"$expr": "page.color"}`, variantLight, rgb{}, true},
		{`"transparent"`, variantLight, rgb{}, true},
		{`{"gradient": {}}`, variantLight, rgb{}, true},
	} {
		got := u.resolveColor(json.RawMessage(tc.raw), tc.v)
		if got.unknown() != tc.unknown || !tc.unknown && got.c != tc.want {
			t.Errorf("%s in %s: got %+v", tc.raw, tc.v.name(), got)
		}
	}
	if r := u.resolveColor(json.RawMessage(`"#11223380"`), variantLight); r.unknown() || r.opaque {
		t.Errorf("a translucent colour is known and not opaque: %+v", r)
	}
}

// Verifies: A11Y-003.

// Verifies: A11Y-002.
func TestImageAlternative_A11Y_002(t *testing.T) {
	t.Parallel()
	image := func(props, extra string) string {
		return `{"id": "%ID%", "type": "Image", "props": {"source": ` + assetSource + props + `}` + extra + `}`
	}
	for _, tc := range []struct {
		name string
		node string
		want int
	}{
		{"missing", image("", ""), 1},
		{"semanticLabel", image(`, "semanticLabel": "Logo"`, ""), 0},
		{"semantics label", image("", `, "semantics": {"label": "Logo"}`), 0},
		{"decorative", image("", `, "semantics": {"decorative": true}`), 0},
		{"excludeFromSemantics", image(`, "excludeFromSemantics": true`, ""), 0},
		{"decorative false", image("", `, "semantics": {"decorative": false}`), 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			res := compileFS(a11yProject(t, "standard", "#000000", "#FFFFFF", "["+tc.node+"]"))
			if got := len(findings(res, plxerr.ImageAlternativeMissing)); got != tc.want {
				t.Errorf("got %d PLX-1402, want %d:\n%s", got, tc.want, list(res.Diagnostics))
			}
		})
	}
}

// Verifies: A11Y-004.
func TestTouchTargets_A11Y_004(t *testing.T) {
	t.Parallel()
	sized := func(w, h string) string {
		return `{"id": "%ID%", "type": "SizedBox", "props": {"width": ` + w + `, "height": ` + h + `}, "slots": {"child":
			{"id": "%ID%", "type": "FilledButton", "slots": {"child": ` + text(`{"color": "#000000"}`, "") + `}}}}`
	}
	for _, tc := range []struct {
		name string
		node string
		want int
	}{
		{"40 x 40", sized("40", "40"), 1},
		{"wide but 30 high", sized("200", "30"), 1},
		{"46 x 46 is below Android's 48", sized("46", "46"), 1},
		{"48 x 48", sized("48", "48"), 0},
		{"own height", `{"id": "%ID%", "type": "IconButton", "props": {"icon": {"name": "home"}, "height": 20, "color": "#000000"}, "semantics": {"label": "Home"}}`, 1},
		{"unconstrained", `{"id": "%ID%", "type": "IconButton", "props": {"icon": {"name": "home"}, "color": "#000000"}, "semantics": {"label": "Home"}}`, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			res := compileFS(a11yProject(t, "standard", "#000000", "#FFFFFF", "["+tc.node+"]"))
			if got := len(findings(res, plxerr.TouchTargetTooSmall)); got != tc.want {
				t.Errorf("got %d PLX-1404, want %d:\n%s", got, tc.want, list(res.Diagnostics))
			}
		})
	}
}

// Verifies: A11Y-005.
func TestFixedHeightText_A11Y_005(t *testing.T) {
	t.Parallel()
	box := `{"id": "%ID%", "type": "SizedBox", "props": {"height": 20}, "slots": {"child": ` + text("", "") + `}}`
	for _, profile := range []string{"standard", "strict"} {
		res := compileFS(a11yProject(t, profile, "#000000", "#FFFFFF", "["+box+"]"))
		got := findings(res, plxerr.FixedHeightText)
		if len(got) != 1 || got[0].Severity != plxerr.SeverityWarning {
			t.Errorf("%s: want one warning PLX-1405:\n%s", profile, list(res.Diagnostics))
		}
	}
	free := `{"id": "%ID%", "type": "SizedBox", "props": {"width": 20}, "slots": {"child": ` + text("", "") + `}}`
	if res := compileFS(a11yProject(t, "standard", "#000000", "#FFFFFF", "["+free+"]")); len(findings(res, plxerr.FixedHeightText)) != 0 {
		t.Errorf("a fixed width is not a fixed height:\n%s", list(res.Diagnostics))
	}
}

// Verifies: A11Y-006.
func TestStructure_A11Y_006(t *testing.T) {
	t.Parallel()
	order := func(values ...int) string {
		var nodes []string
		for _, v := range values {
			nodes = append(nodes, text("", fmt.Sprintf(`"semantics": {"focusOrder": %d}`, v)))
		}
		return "[" + strings.Join(nodes, ",") + "]"
	}
	heading := func(levels ...int) string {
		var nodes []string
		for _, v := range levels {
			nodes = append(nodes, text("", fmt.Sprintf(`"semantics": {"headingLevel": %d}`, v)))
		}
		return "[" + strings.Join(nodes, ",") + "]"
	}
	for _, tc := range []struct {
		name string
		kids string
		code plxerr.Code
		want int
	}{
		{"dense order", order(2, 1, 3), plxerr.FocusOrderInvalid, 0},
		{"duplicate order", order(1, 1, 2), plxerr.FocusOrderInvalid, 1},
		{"gap in order", order(1, 3), plxerr.FocusOrderInvalid, 1},
		{"order not from 1", order(2, 3), plxerr.FocusOrderInvalid, 1},
		{"consecutive headings", heading(1, 2, 2, 3, 1), plxerr.HeadingLevelInvalid, 0},
		{"skipped heading", heading(1, 3), plxerr.HeadingLevelInvalid, 1},
		{"skip after going back", heading(1, 2, 3, 1, 3), plxerr.HeadingLevelInvalid, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			res := compileFS(a11yProject(t, "standard", "#000000", "#FFFFFF", tc.kids))
			// A focus-order finding is at the scope, the page root.
			got := len(findings(res, tc.code))
			if tc.code == plxerr.FocusOrderInvalid {
				got = 0
				for _, d := range res.Diagnostics {
					if d.Code == tc.code {
						got++
					}
				}
			}
			if got != tc.want {
				t.Errorf("got %d %s, want %d:\n%s", got, tc.code, tc.want, list(res.Diagnostics))
			}
		})
	}
}

// Verifies: A11Y-006.
func TestFocusOrderGroupsAreScopes_A11Y_006(t *testing.T) {
	t.Parallel()
	group := `{"id": "%ID%", "type": "Column", "semantics": {"group": true}, "children": [` +
		text("", `"semantics": {"focusOrder": 1}`) + `,` + text("", `"semantics": {"focusOrder": 2}`) + `]}`
	kids := "[" + group + "," + text("", `"semantics": {"focusOrder": 1}`) + "]"
	res := compileFS(a11yProject(t, "standard", "#000000", "#FFFFFF", kids))
	if got := findings(res, plxerr.FocusOrderInvalid); len(got) != 0 {
		t.Errorf("each group numbers from 1:\n%s", list(res.Diagnostics))
	}
}

// Verifies: A11Y-006.
func TestLiveRegion_A11Y_006(t *testing.T) {
	t.Parallel()
	constant := text("", `"semantics": {"liveRegion": true}`)
	res := compileFS(a11yProject(t, "strict", "#000000", "#FFFFFF", "["+constant+"]"))
	got := findings(res, plxerr.LiveRegionStatic)
	if len(got) != 1 || got[0].Severity != plxerr.SeverityWarning {
		t.Errorf("want one warning PLX-1408:\n%s", list(res.Diagnostics))
	}
	bound := `{"id": "%ID%", "type": "Text", "props": {"data": {"$expr": "string(page.amount)"}}, "semantics": {"liveRegion": true}}`
	res = compileFS(a11yProject(t, "strict", "#000000", "#FFFFFF", "["+bound+"]"))
	if got := findings(res, plxerr.LiveRegionStatic); len(got) != 0 {
		t.Errorf("a bound live region is fine:\n%s", list(res.Diagnostics))
	}
}

// Verifies: A11Y-002, A11Y-003, A11Y-004.
func TestSeverityFollowsTheProfile_A11Y_002(t *testing.T) {
	t.Parallel()
	image := `{"id": "%ID%", "type": "Image", "props": {"source": ` + assetSource + `}}`
	for profile, want := range map[string]plxerr.Severity{
		"standard": plxerr.SeverityWarning, "strict": plxerr.SeverityError, "maximum": plxerr.SeverityError,
	} {
		res := compileFS(a11yProject(t, profile, "#000000", "#FFFFFF", "["+image+"]"))
		got := findings(res, plxerr.ImageAlternativeMissing)
		if len(got) != 1 || got[0].Severity != want {
			t.Errorf("%s: PLX-1402 = %v, want severity %v", profile, got, want)
		}
	}
	nameless := `{"id": "%ID%", "type": "IconButton", "props": {"icon": {"name": "home"}, "color": "#000000"}}`
	for profile, want := range map[string]plxerr.Severity{"standard": plxerr.SeverityWarning, "strict": plxerr.SeverityError} {
		res := compileFS(a11yProject(t, profile, "#000000", "#FFFFFF", "["+nameless+"]"))
		got := findings(res, plxerr.AccessibleNameMissing)
		if len(got) != 1 || got[0].Severity != want {
			t.Errorf("%s: PLX-1401 = %v, want severity %v", profile, got, want)
		}
	}
}

// Verifies: A11Y-008.
func TestWaivers_A11Y_008(t *testing.T) {
	t.Parallel()
	image := func(extra string) string {
		return `{"id": "%ID%", "type": "Image", "props": {"source": ` + assetSource + `}` + extra + `}`
	}
	waiver := `, "a11yWaivers": {"imageAlternative": "A purely decorative photograph."}`
	m := a11yProject(t, "strict", "#000000", "#FFFFFF", "["+image(waiver)+"]")
	res := compileFS(m)
	if len(findings(res, plxerr.ImageAlternativeMissing)) != 0 {
		t.Fatalf("a node waiver suppresses the finding:\n%s", list(res.Diagnostics))
	}
	var own []Waiver
	for _, w := range res.Accessibility.Waivers {
		if mine(w.File, w.Path) {
			own = append(own, w)
		}
	}
	if len(own) != 1 || own[0].Check != checkImageAlternative || own[0].Justification != "A purely decorative photograph." ||
		!strings.HasSuffix(own[0].Path, "/a11yWaivers/imageAlternative") {
		t.Errorf("waivers = %+v", own)
	}
	for _, v := range res.Accessibility.Violations {
		if v.Check == checkImageAlternative {
			t.Errorf("a waived finding is not a violation: %+v", v)
		}
	}
	// A page waiver covers the whole page; another check is not covered.
	m = a11yProject(t, "strict", "#000000", "#FFFFFF", "["+image("")+","+image("")+`,{"id": "%ID%", "type": "IconButton", "props": {"icon": {"name": "home"}, "color": "#000000"}}]`)
	edit(t, m, calculatorPage, func(doc map[string]any) {
		doc["a11yWaivers"] = map[string]any{"imageAlternative": "Decorative photographs on this page."}
	})
	res = compileFS(m)
	if len(findings(res, plxerr.ImageAlternativeMissing)) != 0 || len(findings(res, plxerr.AccessibleNameMissing)) != 1 {
		t.Errorf("a page waiver covers its check only:\n%s", list(res.Diagnostics))
	}
}

// Verifies: A11Y-008.
func TestInvalidWaiversAreReported_A11Y_008(t *testing.T) {
	t.Parallel()
	u := &unit{}
	u.declaredWaivers("page.json", "/root", map[string]string{
		"contrast":         "too short",
		"nonsense":         "a justification that is long enough",
		"touchTarget":      "The target is large enough in practice.",
		"imageAlternative": "          ",
	})
	if len(u.diags) != 3 {
		t.Fatalf("diagnostics:\n%s", list(u.diags))
	}
	for _, d := range u.diags {
		if d.Code != plxerr.A11yWaiverInvalid {
			t.Errorf("unexpected %s", d)
		}
	}
	if len(u.a11y.waivers) != 1 || u.a11y.waivers[0].Check != checkTouchTarget || u.a11y.waivers[0].Path != "/root/a11yWaivers/touchTarget" {
		t.Errorf("only the valid waiver is recorded: %+v", u.a11y.waivers)
	}
	// An invalid waiver does not suppress.
	n := &node{doc: &schema.Node{A11yWaivers: map[string]string{"contrast": "too short"}}}
	if waived(n, checkContrast) {
		t.Error("a short justification suppressed the check")
	}
}

// Verifies: A11Y-008.
func TestReportIsDeterministicAndValid_A11Y_008(t *testing.T) {
	t.Parallel()
	kids := "[" + text("", "") + `,{"id": "%ID%", "type": "Image", "props": {"source": ` + assetSource + `}},` +
		`{"id": "%ID%", "type": "Image", "props": {"source": ` + assetSource + `}, "a11yWaivers": {"imageAlternative": "Decorative picture."}}]`
	m := a11yProject(t, "standard", "#777777", "#EEEEEE", kids)
	first := compileFS(m).Accessibility
	data, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		again, _ := json.Marshal(compileFS(m).Accessibility)
		if string(again) != string(data) {
			t.Fatal("the report differs between runs")
		}
	}
	if len(first.Checks) != len(a11yChecks) || first.Checks[0].Name != checkAccessibleName {
		t.Errorf("checks = %+v", first.Checks)
	}
	byName := map[string]CheckSummary{}
	for _, c := range first.Checks {
		byName[c.Name] = c
	}
	if c := byName[checkImageAlternative]; c.Run != 2 || c.Violations != 1 {
		t.Errorf("imageAlternative = %+v", c)
	}
	own := 0
	for _, v := range first.Violations {
		if v.Check == checkContrast && mine(v.File, v.Path) {
			own++
		}
	}
	if own != 1 {
		t.Errorf("want one contrast violation in the added nodes, got %+v", first.Violations)
	}

	path := filepath.Join("..", "..", "..", "schema", "json", "a11y-report.schema.json")
	f, err := os.Open(path) //nolint:gosec // G304: the repository's schema.
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	doc, err := jsonschema.UnmarshalJSON(f)
	if err != nil {
		t.Fatal(err)
	}
	c := jsonschema.NewCompiler()
	c.DefaultDraft(jsonschema.Draft2020)
	const url = "https://plux.dev/schema/a11y-report.schema.json"
	if err := c.AddResource(url, doc); err != nil {
		t.Fatal(err)
	}
	sch, err := c.Compile(url)
	if err != nil {
		t.Fatal(err)
	}
	inst, err := jsonschema.UnmarshalJSON(strings.NewReader(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	if err := sch.Validate(inst); err != nil {
		t.Errorf("the report does not match its schema: %v\n%s", err, data)
	}
	bad, _ := jsonschema.UnmarshalJSON(strings.NewReader(`{"checks": [], "violations": [], "waivers": []}`))
	if sch.Validate(bad) == nil {
		t.Error("the schema accepts a report without notChecked")
	}
}

// Verifies: A11Y-003.
func TestNotCheckedIsAnInfoOrCountedOnly_A11Y_003(t *testing.T) {
	t.Parallel()
	u := &unit{}
	n := &node{owner: owner{page: &page{file: "p.page.json"}}, ptr: "/root"}
	u.notChecked(n, "text: the colour is an expression", false)
	u.notChecked(n, "text: token \"color.onSurface\" is not in the app's theme", true)
	if len(u.diags) != 1 || u.diags[0].Code != plxerr.ContrastNotChecked || u.diags[0].Severity != plxerr.SeverityInfo {
		t.Errorf("want one info PLX-1409:\n%s", list(u.diags))
	}
	if u.a11y.notChecked != 2 {
		t.Errorf("notChecked = %d, want both counted", u.a11y.notChecked)
	}
}
