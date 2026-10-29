// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package replay_test

import (
	"math"
	"strings"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/replay"
)

// Verifies: SCH-012, ANL-050.
// Sanitise keeps the structure of a replay and removes everything a replay
// may not hold: sensitive state entries, string values, long numbers, text
// inputs and what is inside them.
func TestSanitise(t *testing.T) {
	t.Parallel()
	in := []replay.Event{
		{Kind: replay.KindScreen, Route: "/loans", PluginKey: "loans", Name: "smuggled", Value: "x"},
		{Kind: replay.KindLayout, Nodes: []replay.Node{
			{Path: "0/1", Widget: "Column", Width: 100, Height: 200},
			{Path: "0/2", Widget: "TextField", Width: 100, Height: 40},
			{Path: "0/2/0", Widget: "Text", Width: 50, Height: 10},
			{Path: "0/3", Widget: "Text", Width: 50, Height: 10},
		}},
		{AtMs: 5, Kind: replay.KindTap, NodePath: "0/3", FX: 0.5, FY: 0.25, Value: "smuggled"},
		{AtMs: 6, Kind: replay.KindInput, NodePath: "0/2", Value: "hunter2", Name: "smuggled"},
		{AtMs: 7, Kind: replay.KindState, Name: "count", ValueType: "int", Value: "3"},
		{AtMs: 8, Kind: replay.KindState, Name: "label", ValueType: "string", Value: "Ada Lovelace"},
		{AtMs: 9, Kind: replay.KindState, Name: "cardNumber", ValueType: "int", Value: "4"},
		{AtMs: 10, Kind: replay.KindState, Name: "account", ValueType: "int", Value: "4111111111111111"},
		{AtMs: 11, Kind: replay.KindState, Name: "email", ValueType: "string", Value: "a@b.c"},
		{AtMs: 12, Kind: replay.KindState, Name: "ratio", ValueType: "double", Value: "0.5"},
		{AtMs: 13, Kind: replay.KindState, Name: "agreed", ValueType: "bool", Value: "true"},
		{AtMs: 14, Kind: replay.KindState, Name: "odd", ValueType: "bytes", Value: "xx"},
		{AtMs: 15, Kind: "screenshot", Route: "/x"},
		{AtMs: -1, Kind: replay.KindTap, NodePath: "0/3"},
		{AtMs: 16, Kind: replay.KindTap, NodePath: "0/3", FX: 2},
		{AtMs: 17, Kind: replay.KindTap, NodePath: strings.Repeat("a", 300)},
	}
	got, dropped := replay.Sanitise(in)
	if dropped != 7 || len(got) != 9 {
		t.Fatalf("Sanitise kept %d and dropped %d events: %+v", len(got), dropped, got)
	}
	if e := got[0]; e.Name != "" || e.Value != "" || e.Route != "/loans" {
		t.Errorf("screen: %+v", e)
	}
	nodes := got[1].Nodes
	if len(nodes) != 4 || nodes[0].Masked || !nodes[1].Masked || !nodes[2].Masked || nodes[3].Masked {
		t.Errorf("layout masks: %+v", nodes)
	}
	if e := got[2]; e.Value != "" || e.FX != 0.5 {
		t.Errorf("tap: %+v", e)
	}
	if e := got[3]; e.Value != "" || e.Name != "" || e.NodePath != "0/2" {
		t.Errorf("input must hold the node only: %+v", e)
	}
	want := map[string]string{"count": "3", "label": "", "account": "", "ratio": "0.5", "agreed": "true"}
	for _, e := range got[4:] {
		if v, ok := want[e.Name]; !ok || e.Value != v {
			t.Errorf("state %q kept %q", e.Name, e.Value)
		}
		delete(want, e.Name)
	}
	if len(want) != 0 {
		t.Errorf("state events missing: %v", want)
	}
}

// Verifies: ANL-050.
// A rectangle that is not finite or is absurdly large refuses the layout.
func TestSanitiseRejectsBadRectangles(t *testing.T) {
	t.Parallel()
	for _, n := range []replay.Node{
		{Path: "0", Widget: "Box", Width: math.NaN()},
		{Path: "0", Widget: "Box", X: math.Inf(1)},
		{Path: "0", Widget: "Box", Width: 1e9},
		{Path: "0", Widget: "Box", Width: -1},
		{Path: "0", Widget: "bad widget"},
	} {
		got, dropped := replay.Sanitise([]replay.Event{{Kind: replay.KindLayout, Nodes: []replay.Node{n}}})
		if len(got) != 0 || dropped != 1 {
			t.Errorf("%+v was kept", n)
		}
	}
}
