// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	"strconv"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// Verifies: DB-009.
// A kvGet of a literal key reads the type the plugin's kvSet steps of
// the key write, nullable, wherever they are (build log row 134): a
// literal's type, or an expression's; a key written with two types, or
// none, is read untyped.
func TestKVGetIsTypedFromKVSet(t *testing.T) {
	t.Parallel()
	const page = "plugins/todo/pages/list.page.json"
	const read = "/root/slots/body/children/2/events/onPressed/steps/1/input/message"
	for _, c := range []struct {
		name, writes, src string
		code              plxerr.Code
	}{
		{"literal", `"draft"`, `steps.g.output ?? ""`, 0},
		{"expression", `{"$expr": "\"Rank \" + string(3)"}`, `steps.g.output ?? ""`, 0},
		{"int", `3`, `string((steps.g.output ?? 0) + 1)`, 0},
		{"read as another type", `3`, `steps.g.output ?? ""`, plxerr.PXLTypeMismatch},
		{"two types", `"draft"`, `string(steps.g.output)`, plxerr.PXLUnknownField},
		{"no kvSet", ``, `string(steps.g.output)`, plxerr.PXLUnknownField},
	} {
		m := project(t, dbDir)
		edit(t, m, page, func(doc map[string]any) {
			if c.writes != "" {
				// The write is on another button, typed before or after the read.
				at(t, doc, "root/slots/body/children/1/events")["onPressed"] = raw(t, `{"steps": [
					{"id": "s", "action": "kvSet", "input": {"key": "draft", "value": `+c.writes+`}}]}`)
			}
			if c.name == "two types" {
				at(t, doc, "root/slots/body/children/3/events")["onPressed"] = raw(t, `{"steps": [
					{"id": "s2", "action": "kvSet", "input": {"key": "draft", "value": true}}]}`)
			}
			at(t, doc, "root/slots/body/children/2/events")["onPressed"] = raw(t, `{"steps": [
				{"id": "g", "action": "kvGet", "input": {"key": "draft"}, "next": "say"},
				{"id": "say", "action": "showSnackbar", "input": {"message": {"$expr": `+strconv.Quote(c.src)+`}}}]}`)
		})
		res := compileFS(m)
		if c.code != 0 {
			wantDiag(t, res, c.code, page, read)
			continue
		}
		for _, d := range res.Diagnostics {
			if d.Severity == plxerr.SeverityError {
				t.Errorf("%s:\n%s", c.name, list(res.Diagnostics))
				break
			}
		}
	}
}
