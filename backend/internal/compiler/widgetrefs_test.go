// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

const (
	animationPage = "plugins/gallery/pages/animation.page.json"
	uploaderPage  = "plugins/gallery/pages/components-b.page.json"
)

// findByTestID returns the pointer of the node with a test ID and the
// node's props, searching a page document.
func findByTestID(doc any, ptr, id string) (string, map[string]any) {
	switch x := doc.(type) {
	case map[string]any:
		if x["testId"] == id {
			props, _ := x["props"].(map[string]any)
			if props == nil {
				props = map[string]any{}
				x["props"] = props
			}
			return ptr, props
		}
		for k, v := range x {
			if p, props := findByTestID(v, ptr+"/"+k, id); p != "" {
				return p, props
			}
		}
	case []any:
		for i, v := range x {
			if p, props := findByTestID(v, ptr+"/"+strconv.Itoa(i), id); p != "" {
				return p, props
			}
		}
	}
	return "", nil
}

// setProp sets a prop of the node with a test ID and returns the node's
// pointer.
func setNodeProp(t *testing.T, m fstest.MapFS, file, id, prop string, value any) string {
	t.Helper()
	var ptr string
	edit(t, m, file, func(doc map[string]any) {
		p, props := findByTestID(doc["root"], "/root", id)
		if p == "" {
			t.Fatalf("no node %s in %s", id, file)
		}
		props[prop] = value
		ptr = p
	})
	return ptr
}

// hasEdge reports whether the reference graph has an edge of a kind at a
// location.
func hasEdge(res *Result, kind EdgeKind, file, ptr string) bool {
	for _, e := range res.Graph.Edges {
		if e.Kind == kind && e.File == file && e.Path == ptr {
			return true
		}
	}
	return false
}

// TestFileUploaderReferences checks that a FileUploader's literal data
// source and operation resolve at compile time and appear in the
// reference graph.
// Verifies: SCH-041, REL-003.
func TestFileUploaderReferences(t *testing.T) {
	t.Parallel()
	m := project(t, widgetsDir)
	res := compileFS(m)
	if res.Diagnostics.HasErrors() {
		t.Fatalf("diagnostics:\n%s", list(res.Diagnostics))
	}
	var base string
	edit(t, m, uploaderPage, func(doc map[string]any) { base, _ = findByTestID(doc["root"], "/root", "file-uploader") })
	if !hasEdge(res, EdgeUsesDataSource, uploaderPage, base+"/props/dataSource") {
		t.Errorf("no usesDataSource edge at %s", base)
	}

	m = project(t, widgetsDir)
	ptr := setNodeProp(t, m, uploaderPage, "file-uploader", "dataSource", "nowhere")
	wantDiag(t, compileFS(m), plxerr.UnresolvedReference, uploaderPage, ptr+"/props/dataSource")

	m = project(t, widgetsDir)
	ptr = setNodeProp(t, m, uploaderPage, "file-uploader", "operation", "nowhere")
	wantDiag(t, compileFS(m), plxerr.UnresolvedReference, uploaderPage, ptr+"/props/operation")

	m = project(t, widgetsDir)
	setNodeProp(t, m, uploaderPage, "file-uploader", "dataSource", map[string]any{"$expr": "\"nowhere\""})
	for _, d := range compileFS(m).Diagnostics {
		if d.Code == plxerr.UnresolvedReference {
			t.Errorf("a bound value is not checked: %s", d.Message)
		}
	}
}

// TestTimelineReferences checks that the timeline and track a Timeline or
// a transition widget names resolve on the page and appear in the
// reference graph.
// Verifies: SCH-041, REL-003.
func TestTimelineReferences(t *testing.T) {
	t.Parallel()
	m := project(t, widgetsDir)
	res := compileFS(m)
	if res.Diagnostics.HasErrors() {
		t.Fatalf("diagnostics:\n%s", list(res.Diagnostics))
	}
	var base string
	edit(t, m, animationPage, func(doc map[string]any) { base, _ = findByTestID(doc["root"], "/root", "fade-transition") })
	if !hasEdge(res, EdgeUsesTimeline, animationPage, base+"/props/timeline") {
		t.Errorf("no usesTimeline edge at %s", base)
	}

	m = project(t, widgetsDir)
	ptr := setNodeProp(t, m, animationPage, "timeline", "timeline", "nowhere")
	wantDiag(t, compileFS(m), plxerr.UnresolvedReference, animationPage, ptr+"/props/timeline")

	m = project(t, widgetsDir)
	ptr = setNodeProp(t, m, animationPage, "fade-transition", "timeline", "nowhere")
	wantDiag(t, compileFS(m), plxerr.UnresolvedReference, animationPage, ptr+"/props/timeline")

	m = project(t, widgetsDir)
	ptr = setNodeProp(t, m, animationPage, "fade-transition", "track", 99)
	wantDiag(t, compileFS(m), plxerr.UnresolvedReference, animationPage, ptr+"/props/track")
}

// TestHeroWidgetTags checks that two Hero widgets with one literal tag on
// a page are reported with the node animation hero tags.
// Verifies: SCH-041, ANI-004.
func TestHeroWidgetTags(t *testing.T) {
	t.Parallel()
	m := project(t, widgetsDir)
	edit(t, m, animationPage, func(doc map[string]any) {
		_, props := findByTestID(doc["root"], "/root", "hero")
		props["tag"] = "again"
	})
	ptr := ""
	edit(t, m, animationPage, func(doc map[string]any) {
		body := at(t, doc, "root/slots/body/slots/child")
		kids := body["children"].([]any)
		dup := map[string]any{
			"id": "01c0c450-6c00-7000-8a1c-0000000fe002", "type": "Hero", "props": map[string]any{"tag": "again"},
			"slots": map[string]any{"child": map[string]any{"id": "01c0c450-6c00-7000-8a1c-0000000fe003", "type": "Text", "props": map[string]any{"data": "x"}}},
		}
		body["children"] = append(kids, dup)
		ptr = "/root/slots/body/slots/child/children/" + strconv.Itoa(len(kids))
	})
	res := compileFS(m)
	found := false
	for _, d := range res.Diagnostics {
		found = found || (d.File == animationPage && d.Path == ptr+"/props/tag" && strings.Contains(d.Message, "hero tag"))
	}
	if !found {
		t.Errorf("no duplicate hero tag finding at %s\n%s", ptr, list(res.Diagnostics))
	}
}
