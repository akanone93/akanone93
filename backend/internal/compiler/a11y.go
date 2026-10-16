// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema"
	"github.com/nightCode42/plux3/backend/internal/schema/registry"
)

// Touch target sizes in logical pixels (A11Y-004).
const (
	targetMin        = 44.0
	targetMinAndroid = 48.0
	// minWaiverJustification is the shortest justification of a waiver, in
	// characters (A11Y-008).
	minWaiverJustification = 10
)

// accessibility runs the accessibility checks of the policy stage
// (A11Y-002 to A11Y-006, A11Y-008) except the accessible name, which
// lintNode runs. Every finding goes through a11yFinding, which applies the
// profile's severity and the waivers.
func (u *unit) accessibility() {
	u.a11y.ran = true
	for _, pl := range u.plugins {
		for _, pg := range pl.pages {
			if u.inFocus(pg.file) {
				u.pageWaivers(pg)
				u.focusOrder(pg.file, pg.root, pg.root)
				u.headingLevels(pg)
			}
		}
		for _, c := range pl.components {
			if u.inFocus(c.file) {
				u.focusOrder(c.file, c.root, c.root)
			}
		}
	}
	for _, c := range u.shared {
		if u.inFocus(c.file) {
			u.focusOrder(c.file, c.root, c.root)
		}
	}
	for _, n := range u.allNodes() {
		if !u.inFocus(n.owner.file()) {
			continue
		}
		u.nodeWaivers(n)
		if n.widget == nil {
			continue
		}
		u.imageAlternative(n)
		u.contrast(n)
		u.touchTarget(n)
		u.fixedHeightText(n)
		u.liveRegion(n)
	}
}

// strictA11y reports whether the app's security profile turns the
// accessibility findings into errors (ADR-0069).
func (u *unit) strictA11y() bool {
	app := u.project.App.Doc
	return app.SecurityProfile == schema.SecurityProfileStrict || app.SecurityProfile == schema.SecurityProfileMaximum
}

// androidTargeted reports whether the app targets Android. The app document
// declares no platforms, so every app is taken to target both.
func (u *unit) androidTargeted() bool { return true }

// knownCheck reports whether name is a check that can be waived.
func knownCheck(name string) bool {
	for _, c := range a11yChecks {
		if c.name == name {
			return true
		}
	}
	return false
}

// validWaiver reports whether a waiver names a known check and carries a
// justification of at least ten characters.
func validWaiver(check, justification string) bool {
	return knownCheck(check) && utf8.RuneCountInString(strings.TrimSpace(justification)) >= minWaiverJustification
}

// waiverMaps returns the waivers that apply to a node: its own and its
// page's (A11Y-008).
func waiverMaps(n *node) []map[string]string {
	maps := []map[string]string{n.doc.A11yWaivers}
	if n.owner.page != nil {
		maps = append(maps, n.owner.page.doc.A11yWaivers)
	}
	return maps
}

// waived reports whether a valid waiver for the check applies to the node.
func waived(n *node, check string) bool {
	for _, m := range waiverMaps(n) {
		if j, ok := m[check]; ok && validWaiver(check, j) {
			return true
		}
	}
	return false
}

// nodeWaivers validates and records the waivers of one node.
func (u *unit) nodeWaivers(n *node) {
	u.declaredWaivers(n.owner.file(), n.ptr, n.doc.A11yWaivers)
}

// pageWaivers validates and records the waivers of a page.
func (u *unit) pageWaivers(pg *page) {
	u.declaredWaivers(pg.file, "", pg.doc.A11yWaivers)
}

// declaredWaivers reports the invalid waivers of a document part (PLX-1410)
// and records the valid ones for the report.
func (u *unit) declaredWaivers(file, ptr string, waivers map[string]string) {
	names := make([]string, 0, len(waivers))
	for name := range waivers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		path := ptr + plxerr.Pointer("a11yWaivers", name)
		switch {
		case !knownCheck(name):
			u.report(plxerr.A11yWaiverInvalid, file, path, "%q is not an accessibility check that can be waived", name)
		case !validWaiver(name, waivers[name]):
			u.report(plxerr.A11yWaiverInvalid, file, path, "the justification for waiving %s needs at least %d characters", name, minWaiverJustification)
		default:
			u.a11y.waivers = append(u.a11y.waivers, Waiver{Check: name, File: file, Path: path, Justification: waivers[name]})
		}
	}
}

// a11yFinding reports a finding of a check at a node, unless a waiver covers
// it. Under the strict and maximum profiles it is an error, except for the
// checks that are warnings by design (PLX-1405, PLX-1408).
func (u *unit) a11yFinding(n *node, check string, code plxerr.Code, ptr string, format string, args ...any) {
	if waived(n, check) {
		return
	}
	d := plxerr.NewDiagnostic(code, plxerr.Location{File: n.owner.file(), Path: ptr}, format, args...)
	if u.strictA11y() && code != plxerr.FixedHeightText && code != plxerr.LiveRegionStatic {
		d = d.WithSeverity(plxerr.SeverityError)
	}
	u.diags = append(u.diags, d)
	u.a11y.violation(check, d)
}

// notChecked reports once for a node that a colour could not be resolved
// (PLX-1409).
func (u *unit) notChecked(n *node, reason string, hostTheme bool) {
	u.a11y.notChecked++
	if hostTheme {
		// The theme comes from the host: counted in the report, not
		// listed node by node.
		return
	}
	u.report(plxerr.ContrastNotChecked, n.owner.file(), n.ptr, "contrast not checked: %s", reason)
}

// interactiveAncestor reports whether a node sits inside an interactive
// widget, whose own name covers it.
func interactiveAncestor(n *node) bool {
	for a := n.parent; a != nil; a = a.parent {
		if a.widget != nil && a.widget.Interactive {
			return true
		}
	}
	return false
}

// imageAlternative checks that an image has alternative text or is marked
// decorative (PLX-1402, A11Y-002).
func (u *unit) imageAlternative(n *node) {
	if n.widget.Role != registry.RoleImage || interactiveAncestor(n) {
		return
	}
	u.a11y.examined(checkImageAlternative)
	if sem := n.doc.Semantics; sem != nil {
		if len(sem.Label) > 0 || sem.Decorative != nil && *sem.Decorative || sem.ExcludeSemantics != nil && *sem.ExcludeSemantics {
			return
		}
	}
	if len(n.doc.Props["semanticLabel"]) > 0 || propBool(n, "excludeFromSemantics") {
		return
	}
	u.a11yFinding(n, checkImageAlternative, plxerr.ImageAlternativeMissing, n.ptr,
		"%s has no semanticLabel, no semantics.label and is not decorative", n.widget.Type)
}

// propBool reads a literal bool prop.
func propBool(n *node, name string) bool {
	var b bool
	return json.Unmarshal(n.doc.Props[name], &b) == nil && b
}

// propNumber reads a literal number prop.
func propNumber(n *node, name string) (float64, bool) {
	var f float64
	if raw := n.doc.Props[name]; len(raw) > 0 && json.Unmarshal(raw, &f) == nil {
		return f, true
	}
	return 0, false
}

// isTextNode reports whether a node shows text it owns: a text-role widget
// with a text prop.
func isTextNode(n *node) bool {
	if n.widget == nil || n.widget.Role != registry.RoleText {
		return false
	}
	for _, p := range n.props {
		if textNames[p.name] {
			return true
		}
	}
	return false
}

// contrast checks the contrast of a text node and of a UI component's
// boundary in the light, dark and high-contrast themes (PLX-1403, PLX-1409,
// A11Y-003).
func (u *unit) contrast(n *node) {
	var s contrastSubject
	switch {
	case isTextNode(n):
		style := n.doc.Props["style"]
		size, bold, known := u.textSize(style)
		if !known {
			u.a11y.examined(checkContrast)
			u.notChecked(n, "the font size is not a literal or a typography token", false)
			return
		}
		threshold := contrastText
		if size >= largeTextSize || size >= largeBoldTextSize && bold {
			threshold = contrastLargeText
		}
		own := objectFields(style)["color"]
		s = contrastSubject{
			what: "text",
			fg: func(v variant) colorRes {
				if len(own) > 0 {
					return u.resolveColor(own, v)
				}
				return u.defaultTextColor(n, v)
			},
			bg:        func(v variant) background { return u.backgroundBehind(n, v) },
			threshold: threshold,
		}
	case isFilledButton(n):
		fill := func(v variant) colorRes {
			if c, ok := stateValue(objectFields(n.doc.Props["style"])["backgroundColor"]); ok {
				return u.resolveColor(c, v)
			}
			return u.roleColor("primary", v)
		}
		s = contrastSubject{what: n.widget.Type + " fill", fg: fill, bg: func(v variant) background { return u.backgroundBehind(n, v) }, threshold: contrastComponent}
	case n.widget.Type == "IconButton" || n.widget.Type == "Icon" && !interactiveAncestor(n):
		colour := func(v variant) colorRes {
			if c := n.doc.Props["color"]; len(c) > 0 {
				return u.resolveColor(c, v)
			}
			return u.roleColor("onSurface", v)
		}
		s = contrastSubject{what: n.widget.Type, fg: colour, bg: func(v variant) background { return u.backgroundBehind(n, v) }, threshold: contrastComponent}
	default:
		return
	}
	u.a11y.examined(checkContrast)
	out := checkContrastPair(s)
	switch {
	case out.reason != "":
		u.notChecked(n, out.reason, out.hostTheme)
	case out.failure != "":
		u.a11yFinding(n, checkContrast, plxerr.ContrastInsufficient, n.ptr, "%s", out.failure)
	}
}

// defaultTextColor is the colour a text node has without its own: the
// foreground of the surface behind it, such as onPrimary on a filled
// button (the button's own foregroundColor wins) or onSurface.
func (u *unit) defaultTextColor(n *node, v variant) colorRes {
	bg := u.backgroundBehind(n, v)
	if bg.src != nil && isFilledButton(bg.src) {
		if c, ok := stateValue(objectFields(bg.src.doc.Props["style"])["foregroundColor"]); ok {
			return u.resolveColor(c, v)
		}
	}
	return u.roleColor(bg.fgRole, v)
}

// fixedSizes returns the width and height a node is fixed to by its own
// props and by the sizing wrappers directly around it; 0 means not fixed.
func fixedSizes(n *node) (w, h float64) {
	take := func(cur *float64, v float64) {
		if *cur == 0 || v < *cur {
			*cur = v
		}
	}
	if v, ok := propNumber(n, "width"); ok {
		take(&w, v)
	}
	if v, ok := propNumber(n, "height"); ok {
		take(&h, v)
	}
	for a := n.parent; a != nil && a.widget != nil; a = a.parent {
		switch a.widget.Type {
		case "SizedBox":
			if v, ok := propNumber(a, "width"); ok {
				take(&w, v)
			}
			if v, ok := propNumber(a, "height"); ok {
				take(&h, v)
			}
		case "ConstrainedBox":
			cf := objectFields(a.doc.Props["constraints"])
			if v, ok := number(cf["maxWidth"]); ok {
				take(&w, v)
			}
			if v, ok := number(cf["maxHeight"]); ok {
				take(&h, v)
			}
		default:
			return w, h
		}
	}
	return w, h
}

// number reads a JSON number.
func number(raw json.RawMessage) (float64, bool) {
	var f float64
	if len(raw) > 0 && json.Unmarshal(raw, &f) == nil {
		return f, true
	}
	return 0, false
}

// touchTarget checks the explicit size of an interactive node (PLX-1404,
// A11Y-004): below 44 in a dimension, or below 48 for Android.
func (u *unit) touchTarget(n *node) {
	if !n.widget.Interactive {
		return
	}
	w, h := fixedSizes(n)
	if w == 0 && h == 0 {
		return
	}
	u.a11y.examined(checkTouchTarget)
	small := func(limit float64) bool { return w != 0 && w < limit || h != 0 && h < limit }
	switch {
	case small(targetMin):
		u.a11yFinding(n, checkTouchTarget, plxerr.TouchTargetTooSmall, n.ptr, "%s is fixed to %s, below the %g x %g minimum target", n.widget.Type, sizeText(w, h), targetMin, targetMin)
	case u.androidTargeted() && small(targetMinAndroid):
		u.a11yFinding(n, checkTouchTarget, plxerr.TouchTargetTooSmall, n.ptr, "%s is fixed to %s, below the %g x %g minimum target of Android", n.widget.Type, sizeText(w, h), targetMinAndroid, targetMinAndroid)
	}
}

// sizeText formats a fixed size; an unfixed dimension is a dash.
func sizeText(w, h float64) string {
	f := func(v float64) string {
		if v == 0 {
			return "-"
		}
		return fmt.Sprintf("%g", v)
	}
	return f(w) + " x " + f(h)
}

// fixedHeightText warns about a text node under an ancestor with a literal
// height (PLX-1405, A11Y-005 SHOULD).
func (u *unit) fixedHeightText(n *node) {
	if !isTextNode(n) {
		return
	}
	u.a11y.examined(checkFixedHeightText)
	for a := n.parent; a != nil; a = a.parent {
		if a.widget == nil {
			continue
		}
		var h float64
		var ok bool
		switch a.widget.Type {
		case "SizedBox", "Container":
			h, ok = propNumber(a, "height")
		case "ConstrainedBox":
			h, ok = number(objectFields(a.doc.Props["constraints"])["maxHeight"])
		}
		if ok {
			u.a11yFinding(n, checkFixedHeightText, plxerr.FixedHeightText, n.ptr, "the text sits under %s with a fixed height of %g and is clipped when the user scales text", a.widget.Type, h)
			return
		}
	}
}

// focusOrder checks that the focusOrder values below a scope (a page or a
// group) are unique and dense (PLX-1406, A11Y-006). Nested groups are
// scopes of their own.
func (u *unit) focusOrder(file string, root, scope *node) {
	if root == nil {
		return
	}
	type scoped struct {
		scope  *node
		values []float64
	}
	var scopes []*scoped
	index := map[*node]*scoped{}
	var walk func(n, cur *node)
	walk = func(n, cur *node) {
		if sem := n.doc.Semantics; sem != nil && sem.FocusOrder != nil {
			s := index[cur]
			if s == nil {
				s = &scoped{scope: cur}
				index[cur] = s
				scopes = append(scopes, s)
			}
			s.values = append(s.values, *sem.FocusOrder)
		}
		next := cur
		if sem := n.doc.Semantics; sem != nil && sem.Group != nil && *sem.Group {
			next = n
		}
		for _, c := range n.children {
			walk(c, next)
		}
		for _, sf := range n.slots {
			for _, c := range sf.nodes {
				walk(c, next)
			}
		}
	}
	walk(root, scope)
	for _, s := range scopes {
		u.a11y.examined(checkFocusOrder)
		if problem := focusOrderProblem(s.values); problem != "" {
			u.a11yFinding(s.scope, checkFocusOrder, plxerr.FocusOrderInvalid, s.scope.ptr, "%s", problem)
		}
	}
}

// focusOrderProblem describes why values are not 1..n, each once; "" when
// they are.
func focusOrderProblem(values []float64) string {
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	for i, v := range sorted {
		if i > 0 && v == sorted[i-1] {
			return fmt.Sprintf("focusOrder %g is used more than once", v)
		}
	}
	for i, v := range sorted {
		if v != float64(i+1) {
			return fmt.Sprintf("focusOrder is not dense: expected %d, found %g", i+1, v)
		}
	}
	return ""
}

// headingLevels checks that heading levels in document order do not skip
// more than one level (PLX-1407, A11Y-006).
func (u *unit) headingLevels(pg *page) {
	prev := int64(0)
	var walk func(n *node)
	walk = func(n *node) {
		if sem := n.doc.Semantics; sem != nil && sem.HeadingLevel != nil {
			level := *sem.HeadingLevel
			u.a11y.examined(checkHeadingLevel)
			if prev != 0 && level > prev+1 {
				u.a11yFinding(n, checkHeadingLevel, plxerr.HeadingLevelInvalid, n.ptr+"/semantics/headingLevel", "heading level %d follows level %d", level, prev)
			}
			prev = level
		}
		for _, c := range n.children {
			walk(c)
		}
		for _, sf := range n.slots {
			for _, c := range sf.nodes {
				walk(c)
			}
		}
	}
	if pg.root != nil {
		walk(pg.root)
	}
}

// liveRegion warns about a live region whose content is constant
// (PLX-1408, A11Y-006).
func (u *unit) liveRegion(n *node) {
	sem := n.doc.Semantics
	if sem == nil || sem.LiveRegion == nil || !*sem.LiveRegion {
		return
	}
	var texts []json.RawMessage
	if len(sem.Label) > 0 {
		texts = append(texts, sem.Label)
	} else {
		texts = subtreeTexts(n, texts)
	}
	if len(texts) == 0 {
		return
	}
	u.a11y.examined(checkLiveRegion)
	for _, t := range texts {
		if bytes.Contains(t, []byte(`"$expr"`)) {
			return
		}
	}
	u.a11yFinding(n, checkLiveRegion, plxerr.LiveRegionStatic, n.ptr+"/semantics/liveRegion", "the live region's content is constant, so it never announces a change")
}

// subtreeTexts collects the text props of a subtree.
func subtreeTexts(n *node, out []json.RawMessage) []json.RawMessage {
	names := make([]string, 0, len(n.doc.Props))
	for name := range n.doc.Props {
		if textNames[name] {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		out = append(out, n.doc.Props[name])
	}
	for _, c := range n.children {
		out = subtreeTexts(c, out)
	}
	for _, sf := range n.slots {
		for _, c := range sf.nodes {
			out = subtreeTexts(c, out)
		}
	}
	return out
}
