// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

// Package pseudo generates the pseudo-locales of a default-locale message
// (I18N-002, ADR-0068): en-XA, accented and about 35% longer, which shows
// clipped layouts and hard-coded text; and ar-XB, wrapped in right-to-left
// controls, which shows layouts that do not mirror. Only the literal text of
// the parsed message is transformed; arguments, plural and select syntax and
// styles are untouched. The output is a function of the input.
package pseudo

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/nightCode42/plux3/backend/internal/l10n/mf"
)

// The tags of the pseudo-locales.
const (
	Accented = "en-XA"
	RTL      = "ar-XB"
)

// Tags lists the pseudo-locales, in the order they are generated.
func Tags() []string { return []string{Accented, RTL} }

// IsTag reports whether tag is a pseudo-locale.
func IsTag(tag string) bool { return tag == Accented || tag == RTL }

// Right-to-left override and pop directional formatting.
const (
	rlo = "‮"
	pdf = "‬"
)

// accents maps ASCII letters to accented Latin ones.
var accents = map[rune]rune{
	'a': 'á', 'b': 'ƀ', 'c': 'ç', 'd': 'ð', 'e': 'é', 'f': 'ƒ', 'g': 'ĝ', 'h': 'ĥ', 'i': 'í', 'j': 'ĵ',
	'k': 'ķ', 'l': 'ł', 'm': 'ḿ', 'n': 'ñ', 'o': 'ó', 'p': 'þ', 'q': 'ǫ', 'r': 'ŕ', 's': 'š', 't': 'ţ',
	'u': 'ú', 'v': 'ṽ', 'w': 'ŵ', 'x': 'ẋ', 'y': 'ý', 'z': 'ž',
	'A': 'Á', 'B': 'Ɓ', 'C': 'Ç', 'D': 'Ð', 'E': 'É', 'F': 'Ƒ', 'G': 'Ĝ', 'H': 'Ĥ', 'I': 'Í', 'J': 'Ĵ',
	'K': 'Ķ', 'L': 'Ł', 'M': 'Ḿ', 'N': 'Ñ', 'O': 'Ó', 'P': 'Þ', 'Q': 'Ǫ', 'R': 'Ŕ', 'S': 'Š', 'T': 'Ţ',
	'U': 'Ú', 'V': 'Ṽ', 'W': 'Ŵ', 'X': 'Ẋ', 'Y': 'Ý', 'Z': 'Ž',
}

// Message returns the message text of the pseudo-locale tag for the
// default-locale message text, or an error if text is not valid
// MessageFormat.
func Message(tag, text string) (string, error) {
	m, err := mf.Parse(text)
	if err != nil {
		return "", err
	}
	switch tag {
	case Accented:
		letters := countText(m.Nodes)
		accent(m.Nodes)
		pad := strings.Repeat("~", (letters*35+99)/100)
		m.Nodes = wrap(m.Nodes, "[", pad+"]")
	case RTL:
		m.Nodes = wrap(m.Nodes, rlo, pdf)
	default:
		return "", fmt.Errorf("pseudo: %q is not a pseudo-locale", tag)
	}
	return m.String(), nil
}

// Messages returns the pseudo-locale tag's version of every message, by
// key; a message that does not parse is left out.
func Messages(tag string, messages map[string]string) map[string]string {
	out := make(map[string]string, len(messages))
	for k, text := range messages {
		if s, err := Message(tag, text); err == nil {
			out[k] = s
		}
	}
	return out
}

// wrap adds literal text before and after the nodes.
func wrap(nodes []mf.Node, before, after string) []mf.Node {
	out := make([]mf.Node, 0, len(nodes)+2)
	out = append(out, mf.Node{Kind: mf.KindText, Text: before})
	out = append(out, nodes...)
	return append(out, mf.Node{Kind: mf.KindText, Text: after})
}

// countText counts the runes of literal text, in every branch.
func countText(nodes []mf.Node) int {
	n := 0
	for i := range nodes {
		if nodes[i].Kind == mf.KindText {
			n += utf8.RuneCountInString(nodes[i].Text)
		}
		for _, c := range nodes[i].Cases {
			n += countText(c.Body)
		}
	}
	return n
}

// accent replaces the letters of literal text in place.
func accent(nodes []mf.Node) {
	for i := range nodes {
		n := &nodes[i]
		if n.Kind == mf.KindText {
			n.Text = strings.Map(func(r rune) rune {
				if a, ok := accents[r]; ok {
					return a
				}
				return r
			}, n.Text)
		}
		for j := range n.Cases {
			accent(n.Cases[j].Body)
		}
	}
}
