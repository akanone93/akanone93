// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	"testing"

	"github.com/nightCode42/plux3/backend/internal/bundle"
	"github.com/nightCode42/plux3/backend/internal/bundle/fbs"
)

// Verifies: A11Y-006, A11Y-001, I18N-005.
// The semantics fields focusOrder, group, headingLevel and decorative, and a
// node's direction, reach the bundle; unset fields keep their defaults.
func TestSemanticsAndDirectionAreEncoded_A11Y_006(t *testing.T) {
	t.Parallel()
	kids := "[" +
		text("", `"semantics": {"focusOrder": 2.5, "headingLevel": 3, "group": true}`) + "," +
		text("", `"semantics": {"decorative": true}, "direction": "rtl"`) + "," +
		text("", `"direction": "ltr"`) + "," +
		text("", `"direction": "inherit", "semantics": {"label": "x"}`) + "]"
	res := compileFS(a11yProject(t, "standard", "#000000", "#FFFFFF", kids))
	byLo := map[uint64]*fbs.Node{}
	for _, b := range readAll(t, res) {
		for i := range b.Sections {
			if b.Sections[i].Kind != bundle.SectionPage {
				continue
			}
			pg := fbs.GetRootAsPage(b.Sections[i].Data, 0)
			for j := range pg.NodesLength() {
				n := new(fbs.Node)
				pg.Nodes(n, j)
				var u fbs.Uuid
				n.Id(&u)
				if u.Hi() == 0x01a0c4506c007fff && u.Lo()>>4 == 0x800000000000000 && u.Lo()&0xf >= 1 && u.Lo()&0xf <= 4 {
					byLo[u.Lo()&0xf] = n
				}
			}
		}
	}
	if len(byLo) != 4 {
		t.Fatalf("found %d nodes, want 4; diagnostics:\n%s", len(byLo), list(res.Diagnostics))
	}
	s1 := byLo[1].Semantics(nil)
	if s1 == nil || s1.FocusOrder() != 2.5 || s1.HeadingLevel() != 3 || !s1.Group() || s1.Decorative() {
		t.Errorf("node 1 semantics wrong")
	}
	s2 := byLo[2].Semantics(nil)
	if s2 == nil || !s2.Decorative() || s2.FocusOrder() != -1 || s2.HeadingLevel() != 0 {
		t.Errorf("node 2 semantics wrong")
	}
	for lo, want := range map[uint64]fbs.TextDirectionOverride{
		1: fbs.TextDirectionOverrideInherit, 2: fbs.TextDirectionOverrideRtl,
		3: fbs.TextDirectionOverrideLtr, 4: fbs.TextDirectionOverrideInherit,
	} {
		if got := byLo[lo].Direction(); got != want {
			t.Errorf("node %d direction %v, want %v", lo, got, want)
		}
	}
}

// Verifies: A11Y-007.
// PXL's device root carries the platform accessibility settings.
func TestDeviceRootHasAccessibilitySettings_A11Y_007(t *testing.T) {
	t.Parallel()
	dev := baseTypes()["PluxDevice"].Fields
	for name, typ := range map[string]string{
		"reduceMotion": "bool", "boldText": "bool", "highContrast": "bool",
		"screenReader": "bool", "textDirection": "PluxTextDirection",
	} {
		if dev[name] != typ {
			t.Errorf("device.%s is %q, want %q", name, dev[name], typ)
		}
	}
	if got := baseTypes()["PluxTextDirection"].Enum; len(got) != 2 || got[0] != "ltr" || got[1] != "rtl" {
		t.Errorf("PluxTextDirection = %v", got)
	}
}
