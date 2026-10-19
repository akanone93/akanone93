// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Contrast thresholds of WCAG 2.2 AA (A11Y-003).
const (
	contrastText       = 4.5
	contrastLargeText  = 3.0
	contrastComponent  = 3.0
	largeTextSize      = 24.0
	largeBoldTextSize  = 18.66
	boldWeight         = 700
	defaultTextSize    = 14.0 // bodyMedium
	highContrastPrefix = "contrast.high."
	maxAliasDepth      = 8
)

// variant is a theme variant a colour is resolved in.
type variant int

const (
	variantLight variant = iota
	variantDark
	variantHigh
)

// name returns the variant as the report words it.
func (v variant) name() string {
	return [...]string{"light", "dark", "high-contrast"}[v]
}

// rgb is an opaque sRGB colour.
type rgb struct{ r, g, b uint8 }

// colorRes is a resolved colour: the colour or the reason it is unknown,
// and whether a dark or high-contrast value was used.
type colorRes struct {
	c       rgb
	opaque  bool
	reason  string
	usedVar bool
	// hostTheme says the colour is unknown because the token is not in the
	// app's theme, which the host may provide.
	hostTheme bool
}

// unknown reports whether the colour could not be resolved.
func (r colorRes) unknown() bool { return r.reason != "" }

// luminance is the WCAG 2.2 relative luminance.
func (c rgb) luminance() float64 {
	lin := func(v uint8) float64 {
		s := float64(v) / 255
		if s <= 0.04045 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c.r) + 0.7152*lin(c.g) + 0.0722*lin(c.b)
}

// contrastRatio is the WCAG 2.2 contrast ratio of two colours, 1 to 21.
func contrastRatio(a, b rgb) float64 {
	la, lb := a.luminance(), b.luminance()
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// parseHex parses #RGB, #RRGGBB and #RRGGBBAA.
func parseHex(s string) (rgb, uint8, bool) {
	if !strings.HasPrefix(s, "#") {
		return rgb{}, 0, false
	}
	h := s[1:]
	if len(h) == 3 {
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	}
	if len(h) != 6 && len(h) != 8 {
		return rgb{}, 0, false
	}
	v, err := strconv.ParseUint(h, 16, 32)
	if err != nil {
		return rgb{}, 0, false
	}
	alpha := uint8(255)
	if len(h) == 8 {
		alpha = uint8(v & 0xff) //nolint:gosec // G115: masked to 8 bits.
		v >>= 8
	}
	return rgb{uint8(v >> 16), uint8(v >> 8), uint8(v)}, alpha, true //nolint:gosec // G115: bytes of a 24-bit value.
}

// resolveColor resolves a colour prop in a variant: a literal, a token
// reference or a W3C alias. Anything else is unknown, never guessed.
func (u *unit) resolveColor(raw json.RawMessage, v variant) colorRes {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return colorRes{reason: "no colour is set"}
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		if c, a, ok := parseHex(s); ok {
			return colorRes{c: c, opaque: a == 255}
		}
		return colorRes{reason: fmt.Sprintf("the colour %q is not a hex literal", s)}
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil {
		return colorRes{reason: "the colour is not a literal"}
	}
	if t, ok := obj["$token"]; ok {
		var path string
		if json.Unmarshal(t, &path) != nil {
			return colorRes{reason: "the token path is not a string"}
		}
		return u.tokenColor(path, v, 0)
	}
	if _, ok := obj["$expr"]; ok {
		return colorRes{reason: "the colour is an expression"}
	}
	return colorRes{reason: "the colour is not a literal"}
}

// roleColor resolves a theme role such as color.onSurface.
func (u *unit) roleColor(role string, v variant) colorRes {
	return u.tokenColor("color."+role, v, 0)
}

// tokenColor resolves a colour token in a variant. Dark falls back to the
// light value; high contrast to the base token's value for the variant.
func (u *unit) tokenColor(path string, v variant, depth int) colorRes {
	if depth > maxAliasDepth {
		return colorRes{reason: fmt.Sprintf("token %q is an alias cycle", path)}
	}
	tok, ok := u.tokens[path]
	used := false
	if v == variantHigh {
		if hc, found := u.tokens[highContrastPrefix+path]; found {
			tok, ok, used = hc, true, true
		}
	}
	if !ok {
		return colorRes{reason: fmt.Sprintf("token %q is not in the app's theme (it may come from the host)", path), hostTheme: true}
	}
	if tok.typ != "color" {
		return colorRes{reason: fmt.Sprintf("token %q is not a colour", path)}
	}
	val := tok.light
	if v == variantDark && len(tok.dark) > 0 {
		val, used = tok.dark, true
	}
	var s string
	if json.Unmarshal(val, &s) != nil {
		return colorRes{reason: fmt.Sprintf("token %q is not a hex literal", path)}
	}
	if strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}") {
		r := u.tokenColor(strings.TrimSuffix(strings.TrimPrefix(s, "{"), "}"), v, depth+1)
		r.usedVar = r.usedVar || used
		return r
	}
	c, a, ok := parseHex(s)
	if !ok {
		return colorRes{reason: fmt.Sprintf("token %q is not a hex literal", path)}
	}
	return colorRes{c: c, opaque: a == 255, usedVar: used}
}

// background is the nearest opaque background behind a node and the role
// colour that is the default foreground on it.
type background struct {
	colorRes
	fgRole string
	// src is the ancestor that paints the background; nil for the theme's
	// surface.
	src *node
}

// backgroundBehind walks the ancestors of n for the nearest opaque colour.
// A gradient or an image on the way, or an unresolvable colour, makes the
// background unknown.
func (u *unit) backgroundBehind(n *node, v variant) background {
	for a := n.parent; a != nil; a = a.parent {
		if a.widget == nil {
			continue
		}
		raw, role, set, stop := ownFill(a)
		if stop != "" {
			return background{colorRes: colorRes{reason: stop}, fgRole: "onSurface"}
		}
		if !set && role == "" {
			continue
		}
		fg := "onSurface"
		if isFilledButton(a) {
			fg = "onPrimary"
		}
		var r colorRes
		switch {
		case set:
			r = u.resolveColor(raw, v)
		case role == "surfaceContainerLow":
			// A Card without a colour: the theme's container colour, or the
			// surface where the theme has none.
			if r = u.roleColor(role, v); r.unknown() {
				r = u.roleColor("surface", v)
			}
		default:
			r = u.roleColor(role, v)
		}
		if !r.unknown() && !r.opaque {
			continue
		}
		return background{colorRes: r, fgRole: fg, src: a}
	}
	return background{colorRes: u.roleColor("surface", v), fgRole: "onSurface"}
}

// isFilledButton reports the buttons whose label is onPrimary on primary.
func isFilledButton(n *node) bool {
	return n.widget != nil && (n.widget.Type == "FilledButton" || n.widget.Type == "ElevatedButton")
}

// ownFill returns the fill a widget paints behind its child: a colour prop
// (set), or a theme role it paints by default. stop is a reason the
// background cannot be known.
func ownFill(n *node) (raw json.RawMessage, role string, set bool, stop string) {
	p := n.doc.Props
	switch n.widget.Type {
	case "Container", "DecoratedBox":
		return boxFill(p)
	case "ColoredBox":
		return propFill(p, "color", "")
	case "Card":
		return propFill(p, "color", "surfaceContainerLow")
	case "Scaffold":
		return propFill(p, "backgroundColor", "")
	case "FilledButton", "ElevatedButton":
		if c, ok := stateValue(objectFields(p["style"])["backgroundColor"]); ok {
			return c, "", true, ""
		}
		return nil, "primary", false, ""
	}
	return nil, "", false, ""
}

// propFill is the fill a colour prop sets; without the prop, the theme role
// the widget paints by default (empty for none).
func propFill(p map[string]json.RawMessage, key, defaultRole string) (raw json.RawMessage, role string, set bool, stop string) {
	if c, ok := p[key]; ok {
		return c, "", true, ""
	}
	return nil, defaultRole, false, ""
}

// boxFill is the fill of a Container or DecoratedBox: a decoration colour,
// a colour prop, or a reason the background cannot be known.
func boxFill(p map[string]json.RawMessage) (raw json.RawMessage, role string, set bool, stop string) {
	if dec, ok := p["decoration"]; ok {
		f := objectFields(dec)
		if _, g := f["gradient"]; g {
			return nil, "", false, "the background is a gradient"
		}
		if c, ok := f["color"]; ok {
			return c, "", true, ""
		}
	}
	if c, ok := p["color"]; ok {
		return c, "", true, ""
	}
	if _, ok := p["backgroundImage"]; ok {
		return nil, "", false, "the background is an image"
	}
	return nil, "", false, ""
}

// objectFields decodes a JSON object's fields; nil for anything else.
func objectFields(raw json.RawMessage) map[string]json.RawMessage {
	var m map[string]json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &m) != nil {
		return nil
	}
	return m
}

// stateValue returns the colour of a WidgetStateColor: the colour itself
// or its `value` field.
func stateValue(raw json.RawMessage) (json.RawMessage, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	if f := objectFields(raw); f != nil {
		if _, tok := f["$token"]; tok {
			return raw, true
		}
		if _, ex := f["$expr"]; ex {
			return raw, true
		}
		v, ok := f["value"]
		return v, ok
	}
	return raw, true
}

// textSize returns a text node's font size and whether it is bold, from the
// style literal or its typography token; the default is bodyMedium.
func (u *unit) textSize(style json.RawMessage) (size float64, bold bool, known bool) {
	size = defaultTextSize
	f := objectFields(style)
	if f == nil {
		return size, false, true
	}
	if t, ok := f["$token"]; ok {
		var path string
		_ = json.Unmarshal(t, &path)
		tok, found := u.tokens[path]
		if !found || tok.typ != "typography" {
			return 0, false, false
		}
		tf := objectFields(tok.light)
		if s, ok := dimensionPx(tf["fontSize"]); ok {
			size = s
		}
		return size, weightOf(tf["fontWeight"]) >= boldWeight, true
	}
	if raw, ok := f["fontSize"]; ok {
		s, ok := dimensionPx(raw)
		if !ok {
			return 0, false, false
		}
		size = s
	}
	return size, weightOf(f["fontWeight"]) >= boldWeight, true
}

// dimensionPx reads a number or a {unit, value} dimension in pixels.
func dimensionPx(raw json.RawMessage) (float64, bool) {
	if len(raw) == 0 {
		return 0, false
	}
	var n float64
	if json.Unmarshal(raw, &n) == nil {
		return n, true
	}
	var d struct {
		Unit  string  `json:"unit"`
		Value float64 `json:"value"`
	}
	if json.Unmarshal(raw, &d) == nil && d.Unit == "px" {
		return d.Value, true
	}
	return 0, false
}

// weightOf reads a font weight: a number, or w100 to w900, or bold.
func weightOf(raw json.RawMessage) float64 {
	var n float64
	if json.Unmarshal(raw, &n) == nil {
		return n
	}
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return 0
	}
	if s == "bold" {
		return 700
	}
	if w, err := strconv.Atoi(strings.TrimPrefix(s, "w")); err == nil {
		return float64(w)
	}
	return 0
}

// contrastSubject is one pair of colours to compare.
type contrastSubject struct {
	what      string
	fg        func(v variant) colorRes
	bg        func(v variant) background
	threshold float64
}

// contrastOutcome is the result of checking a subject in all variants.
type contrastOutcome struct {
	// reason is why the subject was not checked; "" when it was.
	reason string
	// hostTheme says the reason is a token missing from the app's theme.
	hostTheme bool
	// failure describes the variants that fail, "" when none does.
	failure string
}

// checkContrast compares the pair in every variant that differs from the
// light one: dark when a dark value is used, high contrast when a
// high-contrast token is.
func checkContrastPair(s contrastSubject) contrastOutcome {
	var fails []string
	for v := variantLight; v <= variantHigh; v++ {
		fg, bg := s.fg(v), s.bg(v)
		if fg.unknown() {
			return contrastOutcome{reason: s.what + ": " + fg.reason, hostTheme: fg.hostTheme}
		}
		if !fg.opaque {
			return contrastOutcome{reason: s.what + ": the foreground colour is translucent"}
		}
		if bg.unknown() {
			return contrastOutcome{reason: s.what + ": " + bg.reason, hostTheme: bg.hostTheme}
		}
		if v != variantLight && !fg.usedVar && !bg.usedVar {
			continue
		}
		if ratio := contrastRatio(fg.c, bg.c); ratio < s.threshold {
			fails = append(fails, fmt.Sprintf("%s %.2f:1", v.name(), ratio))
		}
	}
	if len(fails) == 0 {
		return contrastOutcome{}
	}
	return contrastOutcome{failure: fmt.Sprintf("%s has a contrast of %s, below %.1f:1", s.what, strings.Join(fails, ", "), s.threshold)}
}
