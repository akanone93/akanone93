// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package mf

import (
	"strconv"
	"strings"
)

// String returns MessageFormat source that parses to a message equal to m,
// up to the positions of its nodes. Literal text is quoted where the syntax
// needs it, so printing a parsed message never changes its meaning
// (I18N-001, I18N-002).
func (m *Message) String() string {
	var b strings.Builder
	printNodes(&b, m.Nodes, false)
	return b.String()
}

// printNodes writes nodes; inPlural is true inside a plural, where # is
// syntax.
func printNodes(b *strings.Builder, nodes []Node, inPlural bool) {
	for i := range nodes {
		n := &nodes[i]
		switch n.Kind {
		case KindText:
			printText(b, n.Text, inPlural)
		case KindPound:
			b.WriteByte('#')
		case KindArg:
			b.WriteString("{" + n.Name + "}")
		case KindNumber, KindDate, KindTime:
			b.WriteString("{" + n.Name + ", " + map[Kind]string{KindNumber: "number", KindDate: "date", KindTime: "time"}[n.Kind])
			if n.Style != "" {
				b.WriteString(", " + n.Style)
			}
			b.WriteByte('}')
		case KindPlural, KindSelect:
			kind := "select"
			if n.Kind == KindPlural {
				kind = "plural"
				if n.Ordinal {
					kind = "selectordinal"
				}
			}
			b.WriteString("{" + n.Name + ", " + kind + ",")
			if n.Offset != 0 {
				b.WriteString(" offset:" + strconv.FormatInt(n.Offset, 10))
			}
			for j := range n.Cases {
				c := &n.Cases[j]
				b.WriteString(" " + c.Selector + " {")
				printNodes(b, c.Body, inPlural || n.Kind == KindPlural)
				b.WriteByte('}')
			}
			b.WriteByte('}')
		}
	}
}

// printText writes literal text, quoting what the syntax reserves.
func printText(b *strings.Builder, s string, inPlural bool) {
	for _, r := range s {
		switch {
		case r == '\'':
			b.WriteString("''")
		case r == '{' || r == '}' || r == '#' && inPlural:
			b.WriteString("'" + string(r) + "'")
		default:
			b.WriteRune(r)
		}
	}
}
