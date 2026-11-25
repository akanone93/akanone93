// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	"slices"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/bundle"
	"github.com/nightCode42/plux3/backend/internal/bundle/fbs"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// The stage page's node with the implicit animation, its entering and
// leaving node, and the first track of the pulse timeline.
const (
	implicitNode = "root/slots/body/slots/child/children/3"
	enterNode    = "root/slots/body/slots/child/children/4"
)

// cubicOf is the control points a curve was encoded with.
func cubicOf(n int, at func(int) float32) []float32 {
	out := make([]float32, n)
	for i := range n {
		out[i] = at(i)
	}
	return out
}

// pageOf is the page section of the plugin bundle.
func pageOf(t *testing.T, res *Result) *fbs.Page {
	t.Helper()
	pb := readAll(t, res)[1]
	for i := range pb.Sections {
		if pb.Sections[i].Kind == bundle.SectionPage {
			return fbs.GetRootAsPage(pb.Sections[i].Data, 0)
		}
	}
	t.Fatal("no page section")
	return nil
}

// Verifies: ANI-001, ANI-002, ANI-003, BND-008.
// A curve may be a cubic Bézier's control points instead of a name (build
// log row 48): the implicit animation's, a transition's and a keyframe's
// are encoded as four floats, and the bundle needs anim.v2 and
// anim.timelines.v2 (runtime 0.6.0).
func TestCubicCurvesAreEncoded(t *testing.T) {
	t.Parallel()
	res := edited(t, func(p map[string]any) {
		a := at(t, p, implicitNode+"/animation")
		delete(a, "curve")
		a["cubic"] = []any{0.2, 0, 0, 1}
		at(t, p, enterNode+"/animation/enter")["cubic"] = []any{0.3, 0, 0.8, 0.15}
		kf := trackDoc(t, p, 0)["keyframes"].([]any)[1].(map[string]any)
		delete(kf, "curve")
		kf["cubic"] = []any{0.05, 0.7, 0.1, 1}
	})
	onlyFeatureWarnings(t, res)
	page := pageOf(t, res)
	var implicit, enter []float32
	var n fbs.Node
	for i := range page.NodesLength() {
		page.Nodes(&n, i)
		a := n.Animation(nil)
		switch {
		case a == nil:
		case a.CubicLength() > 0:
			implicit = cubicOf(a.CubicLength(), a.Cubic)
		case a.Enter(nil) != nil && a.Enter(nil).CubicLength() > 0:
			e := a.Enter(nil)
			enter = cubicOf(e.CubicLength(), e.Cubic)
		}
	}
	if !slices.Equal(implicit, []float32{0.2, 0, 0, 1}) || !slices.Equal(enter, []float32{0.3, 0, 0.8, 0.15}) {
		t.Errorf("implicit %v, enter %v", implicit, enter)
	}
	tls, str := timelinesOf(t, res)
	var tl fbs.Timeline
	var tr fbs.Track
	var kf fbs.Keyframe
	for i := range tls.TimelinesLength() {
		if tls.Timelines(&tl, i); str(tl.Name()) == "pulse" {
			tl.Tracks(&tr, 0)
			tr.Keyframes(&kf, 1)
		}
	}
	if got := cubicOf(kf.CubicLength(), kf.Cubic); !slices.Equal(got, []float32{0.05, 0.7, 0.1, 1}) || kf.Curve() != 0 {
		t.Errorf("keyframe curve %d, cubic %v", kf.Curve(), got)
	}
	features := res.Plugins[0].Features
	if !slices.Contains(features, "anim.v2") || !slices.Contains(features, "anim.timelines.v2") {
		t.Errorf("features %v lack anim.v2 and anim.timelines.v2", features)
	}
}

// Verifies: ANI-001, ANI-002.
// A curve is a name or four control points, not both, with x1 and x2 from
// 0 to 1 so that time only moves forward.
func TestCubicCurveErrors(t *testing.T) {
	t.Parallel()
	const page = stagePage
	for _, c := range []struct {
		name string
		code plxerr.Code
		ptr  string
		do   func(t *testing.T, p map[string]any)
	}{
		{"name and points", plxerr.NodeAnimationInvalid, "/" + implicitNode + "/animation/cubic", func(t *testing.T, p map[string]any) {
			at(t, p, implicitNode+"/animation")["cubic"] = []any{0.2, 0, 0, 1}
		}},
		{"x outside 0 to 1", plxerr.NodeAnimationInvalid, "/" + implicitNode + "/animation/cubic", func(t *testing.T, p map[string]any) {
			a := at(t, p, implicitNode+"/animation")
			delete(a, "curve")
			a["cubic"] = []any{1.2, 0, 0, 1}
		}},
		{"keyframe x outside 0 to 1", plxerr.AnimationInvalid, "/animations/0/tracks/0/keyframes/1/cubic", func(t *testing.T, p map[string]any) {
			kf := trackDoc(t, p, 0)["keyframes"].([]any)[1].(map[string]any)
			delete(kf, "curve")
			kf["cubic"] = []any{0.5, 0, -0.1, 1}
		}},
	} {
		wantDiag(t, edited(t, func(p map[string]any) { c.do(t, p) }), c.code, page, c.ptr)
	}
}

// Verifies: ANI-001, THM-001.
// A Curve prop may be a cubic Bézier: four control points, encoded as a
// list of doubles, or a cubicBezier design token, which binds no other
// prop type.
func TestCubicCurveProps(t *testing.T) {
	t.Parallel()
	const node = "root/slots/body/slots/child/children/0"
	for _, c := range []struct {
		name, curve string
		code        plxerr.Code
	}{
		{"points", `[0.2, 0, 0, 1]`, 0},
		{"token", `{"$token": "motion.easing.standard"}`, 0},
		{"name", `"easeIn"`, 0},
		{"three numbers", `[0.2, 0, 1]`, plxerr.PropTypeMismatch},
	} {
		m := stageProject(t)
		edit(t, m, "theme.json", func(doc map[string]any) {
			doc["tokens"].(map[string]any)["motion"] = raw(t, `{"easing": {"standard": {"$type": "cubicBezier", "$value": [0.2, 0, 0, 1]}}}`)
		})
		edit(t, m, stagePage, func(p map[string]any) {
			at(t, p, "root/slots/body/slots/child")["children"] = append([]any{raw(t, `{"id": "01d0c450-6c00-7000-8000-0000000fe001", "type": "AnimatedOpacity",
				"props": {"opacity": 1, "duration": 300, "curve": `+c.curve+`}}`)}, at(t, p, "root/slots/body/slots/child")["children"].([]any)...)
		})
		res := compileFS(m)
		if c.code != 0 {
			wantDiag(t, res, c.code, stagePage, "/"+node+"/props/curve")
			continue
		}
		onlyFeatureWarnings(t, res)
	}

	// A cubicBezier token cannot be a number.
	m := stageProject(t)
	edit(t, m, "theme.json", func(doc map[string]any) {
		doc["tokens"].(map[string]any)["motion"] = raw(t, `{"easing": {"standard": {"$type": "cubicBezier", "$value": [0.2, 0, 0, 1]}}}`)
	})
	edit(t, m, stagePage, func(p map[string]any) {
		at(t, p, implicitNode)["props"].(map[string]any)["opacity"] = raw(t, `{"$token": "motion.easing.standard"}`)
	})
	wantDiag(t, compileFS(m), plxerr.PropTypeMismatch, stagePage, "/"+implicitNode+"/props/opacity/$token")
}
