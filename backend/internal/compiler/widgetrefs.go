// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	"encoding/json"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// timelineWidgets are the widgets whose timeline prop names a timeline of
// the page.
var timelineWidgets = map[string]bool{
	"Timeline":           true,
	"FadeTransition":     true,
	"ScaleTransition":    true,
	"SlideTransition":    true,
	"RotationTransition": true,
}

// checkWidgetRefs resolves the references that widget props make by name
// (SCH-041, REL-003): the data source and operation of a FileUploader,
// and the timeline and track of a Timeline or a transition widget. Only
// literal values are checked; a bound value is resolved at run time. Each
// resolved reference is an edge of the reference graph.
func (u *unit) checkWidgetRefs(n *node, file string) {
	if n.widget == nil {
		return
	}
	switch t := n.widget.Type; {
	case t == "FileUploader":
		u.checkUploaderRefs(n, file)
	case timelineWidgets[t]:
		u.checkTimelineRefs(n, file)
	}
}

// checkUploaderRefs resolves a FileUploader's dataSource and operation.
func (u *unit) checkUploaderRefs(n *node, file string) {
	name := literalString(n.doc.Props["dataSource"])
	if name == "" {
		return
	}
	ptr := n.ptr + "/props/dataSource"
	g := &graph{page: n.owner.page, plugin: n.owner.plugin(), scope: n.scope}
	if n.scope == nil {
		return
	}
	id, ok := n.scope.ids["data"][name]
	if !ok {
		u.report(plxerr.UnresolvedReference, file, ptr, "no data source is named %q", name)
		return
	}
	u.graph.add(Edge{From: ownerID(n.owner), Kind: EdgeUsesDataSource, To: id, File: file, Path: ptr})
	op := literalString(n.doc.Props["operation"])
	if op == "" {
		return
	}
	if src, _ := u.operation(g, name+"."+op); src == nil {
		u.report(plxerr.UnresolvedReference, file, n.ptr+"/props/operation", "data source %q has no operation %q", name, op)
	}
}

// checkTimelineRefs resolves the timeline a node plays by name, and the
// track index a transition widget follows, on the node's page.
func (u *unit) checkTimelineRefs(n *node, file string) {
	pg := n.owner.page
	name := literalString(n.doc.Props["timeline"])
	if pg == nil || name == "" {
		return
	}
	ptr := n.ptr + "/props/timeline"
	for i := range pg.doc.Animations {
		tl := &pg.doc.Animations[i]
		if tl.Name != name {
			continue
		}
		u.graph.add(Edge{From: ownerID(n.owner), Kind: EdgeUsesTimeline, To: tl.ID, File: file, Path: ptr})
		var track int
		if raw, set := n.doc.Props["track"]; set && json.Unmarshal(raw, &track) == nil && track >= len(tl.Tracks) {
			u.report(plxerr.UnresolvedReference, file, n.ptr+"/props/track", "timeline %q has %d tracks, not a track %d", name, len(tl.Tracks), track)
		}
		return
	}
	u.report(plxerr.UnresolvedReference, file, ptr, "the page has no timeline named %q", name)
}
