// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package replay

import (
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/nightCode42/plux3/backend/internal/telemetry"
)

// Event kinds of a replay.
const (
	KindScreen = "screen"
	KindLayout = "layout"
	KindTap    = "tap"
	KindInput  = "input"
	KindState  = "state"
)

const (
	maxText      = 256
	maxPluginKey = 64
	maxWidget    = 64
	// maxCoordinate bounds a rectangle in logical pixels.
	maxCoordinate = 1e6
	// maxRecordedDigits is the longest integer whose value is kept: a longer
	// one may be a card, phone or account number.
	maxRecordedDigits = 9
)

var (
	statePattern  = regexp.MustCompile(`^[a-z][A-Za-z0-9_]{0,63}$`)
	widgetPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,63}$`)
)

// maskedWidgetWords mark a widget that takes or shows what a user entered or
// a secret: only its rectangle is kept (SCH-012).
var maskedWidgetWords = []string{"input", "field", "password", "passcode", "pincode", "secure", "secret", "otp", "editable", "textarea"}

// Node is a node of a layout snapshot.
type Node struct {
	Path   string  `json:"p"`
	Widget string  `json:"w"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
	Masked bool    `json:"m,omitempty"`
}

// Event is one step of a replay.
type Event struct {
	AtMs      int32   `json:"at"`
	Kind      string  `json:"k"`
	Route     string  `json:"r,omitempty"`
	PluginKey string  `json:"pl,omitempty"`
	Nodes     []Node  `json:"n,omitempty"`
	NodePath  string  `json:"np,omitempty"`
	FX        float64 `json:"fx,omitempty"`
	FY        float64 `json:"fy,omitempty"`
	Name      string  `json:"s,omitempty"`
	ValueType string  `json:"vt,omitempty"`
	Value     string  `json:"v,omitempty"`
}

// Sanitise returns the events a replay may hold and how many it removed. It
// keeps only the fields of each kind, drops a state event whose entry name
// marks sensitive data, keeps a state value only for a bool, an int or a
// double, and masks every input, secure widget and node inside one.
func Sanitise(in []Event) ([]Event, int) {
	out := make([]Event, 0, len(in))
	dropped := 0
	var masked []string
	for _, e := range in {
		clean, ok := sanitizeEvent(e, &masked)
		if !ok {
			dropped++
			continue
		}
		out = append(out, clean)
	}
	return out, dropped
}

func sanitizeEvent(e Event, masked *[]string) (Event, bool) {
	if e.AtMs < 0 {
		return Event{}, false
	}
	switch e.Kind {
	case KindScreen:
		if !text(e.Route, maxText) || !text(e.PluginKey, maxPluginKey) {
			return Event{}, false
		}
		return Event{AtMs: e.AtMs, Kind: e.Kind, Route: e.Route, PluginKey: e.PluginKey}, true
	case KindLayout:
		nodes, ok := sanitizeNodes(e.Nodes)
		if !ok {
			return Event{}, false
		}
		*masked = (*masked)[:0]
		for _, n := range nodes {
			if n.Masked {
				*masked = append(*masked, n.Path)
			}
		}
		return Event{AtMs: e.AtMs, Kind: e.Kind, Nodes: nodes}, true
	case KindTap:
		if !text(e.NodePath, maxText) || !unit(e.FX) || !unit(e.FY) {
			return Event{}, false
		}
		return Event{AtMs: e.AtMs, Kind: e.Kind, NodePath: e.NodePath, FX: e.FX, FY: e.FY}, true
	case KindInput:
		if !text(e.NodePath, maxText) {
			return Event{}, false
		}
		return Event{AtMs: e.AtMs, Kind: e.Kind, NodePath: e.NodePath}, true
	case KindState:
		return sanitizeState(e)
	default:
		return Event{}, false
	}
}

func sanitizeNodes(in []Node) ([]Node, bool) {
	out := make([]Node, 0, len(in))
	var maskedPaths []string
	for _, n := range in {
		if !validNode(n) {
			return nil, false
		}
		masked := n.Masked || maskedWidget(n.Widget) || insideAny(n.Path, maskedPaths)
		if masked {
			maskedPaths = append(maskedPaths, n.Path)
		}
		out = append(out, Node{Path: n.Path, Widget: n.Widget, X: n.X, Y: n.Y, Width: n.Width, Height: n.Height, Masked: masked})
	}
	return out, true
}

func validNode(n Node) bool {
	if !text(n.Path, maxText) || !widgetPattern.MatchString(n.Widget) || len(n.Widget) > maxWidget {
		return false
	}
	for _, v := range []float64{n.X, n.Y, n.Width, n.Height} {
		if math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v) > maxCoordinate {
			return false
		}
	}
	return n.Width >= 0 && n.Height >= 0
}

func insideAny(path string, parents []string) bool {
	for _, p := range parents {
		if strings.HasPrefix(path, p+"/") {
			return true
		}
	}
	return false
}

func sanitizeState(e Event) (Event, bool) {
	if !statePattern.MatchString(e.Name) || telemetry.Sensitive(e.Name) {
		return Event{}, false
	}
	out := Event{AtMs: e.AtMs, Kind: KindState, Name: e.Name}
	switch e.ValueType {
	case "bool":
		if e.Value == "true" || e.Value == "false" {
			out.Value = e.Value
		}
	case "int":
		if v, err := strconv.ParseInt(e.Value, 10, 64); err == nil && len(strconv.FormatInt(abs(v), 10)) <= maxRecordedDigits {
			out.Value = e.Value
		}
	case "double":
		if v, err := strconv.ParseFloat(e.Value, 64); err == nil && !math.IsNaN(v) && !math.IsInf(v, 0) && len(e.Value) <= 24 {
			out.Value = e.Value
		}
	case "string", "list", "map", "null":
	default:
		return Event{}, false
	}
	out.ValueType = e.ValueType
	return out, true
}

func abs(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

func maskedWidget(w string) bool {
	lower := strings.ToLower(w)
	for _, word := range maskedWidgetWords {
		if strings.Contains(lower, word) {
			return true
		}
	}
	return false
}

func text(s string, max int) bool {
	if len(s) > max {
		return false
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

func unit(v float64) bool { return !math.IsNaN(v) && v >= 0 && v <= 1 }
