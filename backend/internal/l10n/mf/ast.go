// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

// Package mf parses, checks, encodes and evaluates ICU MessageFormat, the
// syntax of Plux translations (ADR-0066, I18N-001, I18N-011).
//
// The compiler parses every message once, checks it against the key's
// declared arguments and the locale's plural categories, and stores the
// compact binary AST in the bundle; the device decodes and evaluates the AST
// and never parses message text. The Dart evaluator in
// packages/plux_flutter/lib/src/l10n/message.dart decodes the same encoding
// and gives the same output (QA-003).
package mf

// Kind is the kind of a node.
type Kind uint8

// The node kinds. The values are the tags of the binary encoding and never
// change.
const (
	// KindText is literal text.
	KindText Kind = 1
	// KindArg is a plain argument, {name}.
	KindArg Kind = 2
	// KindNumber is {name, number[, style]}.
	KindNumber Kind = 3
	// KindDate is {name, date[, style]}.
	KindDate Kind = 4
	// KindTime is {name, time[, style]}.
	KindTime Kind = 5
	// KindPlural is {name, plural, ...} or, with Ordinal, selectordinal.
	KindPlural Kind = 6
	// KindSelect is {name, select, ...}.
	KindSelect Kind = 7
	// KindPound is # inside a plural: the number less the offset.
	KindPound Kind = 8
)

// Node is one element of a message.
type Node struct {
	Kind Kind
	// Text is the literal of KindText.
	Text string
	// Name is the argument of every kind but KindText and KindPound.
	Name string
	// Style is the style of number, date and time arguments: "" for the
	// default, a named style, or "::" followed by a skeleton.
	Style string
	// Ordinal selects ordinal rules for KindPlural (selectordinal).
	Ordinal bool
	// Offset is the plural offset.
	Offset int64
	// Cases are the branches of KindPlural and KindSelect, in source order.
	Cases []Case
	// Pos is the byte offset of the node in the source; it is not encoded.
	Pos int
}

// Case is one branch: a selector ("other", "one", "=0", a select word) and
// its message.
type Case struct {
	Selector string
	Body     []Node
	// Pos is the byte offset of the selector in the source; it is not encoded.
	Pos int
}

// Message is a parsed message.
type Message struct {
	Nodes []Node
}
