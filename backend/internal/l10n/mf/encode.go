// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package mf

import (
	"encoding/binary"
	"errors"
	"fmt"
	"unicode/utf8"
)

// astVersion is the first byte of an encoded message.
const astVersion = 1

// ErrAST is wrapped by every error of Decode.
var ErrAST = errors.New("invalid message AST")

// Encode returns the compact binary form of the message: the version byte,
// then the nodes. A message is a count (uvarint) and that many nodes; a
// node is its tag, then by kind a string (uvarint length and UTF-8 bytes)
// for text; the name for an argument; the name and style for number, date
// and time; the name, the ordinal flag, the offset, and the cases for plural;
// the name and the cases for select. A case is its selector and a message.
// The encoding is deterministic (CMP-002).
func (m *Message) Encode() []byte {
	out := []byte{astVersion}
	return appendNodes(out, m.Nodes)
}

func appendString(out []byte, s string) []byte {
	out = binary.AppendUvarint(out, uint64(len(s)))
	return append(out, s...)
}

func appendNodes(out []byte, nodes []Node) []byte {
	out = binary.AppendUvarint(out, uint64(len(nodes)))
	for i := range nodes {
		n := &nodes[i]
		out = append(out, byte(n.Kind))
		switch n.Kind {
		case KindText:
			out = appendString(out, n.Text)
		case KindArg:
			out = appendString(out, n.Name)
		case KindNumber, KindDate, KindTime:
			out = appendString(out, n.Name)
			out = appendString(out, n.Style)
		case KindPlural:
			out = appendString(out, n.Name)
			if n.Ordinal {
				out = append(out, 1)
			} else {
				out = append(out, 0)
			}
			out = binary.AppendUvarint(out, uint64(n.Offset)) //nolint:gosec // G115: the parser rejects negative offsets.
			out = appendCases(out, n.Cases)
		case KindSelect:
			out = appendString(out, n.Name)
			out = appendCases(out, n.Cases)
		case KindPound:
		}
	}
	return out
}

func appendCases(out []byte, cases []Case) []byte {
	out = binary.AppendUvarint(out, uint64(len(cases)))
	for i := range cases {
		out = appendString(out, cases[i].Selector)
		out = appendNodes(out, cases[i].Body)
	}
	return out
}

type reader struct {
	b   []byte
	pos int
}

func (r *reader) uvarint() (uint64, error) {
	v, n := binary.Uvarint(r.b[r.pos:])
	if n <= 0 {
		return 0, fmt.Errorf("%w: bad varint at %d", ErrAST, r.pos)
	}
	r.pos += n
	return v, nil
}

func (r *reader) byte() (byte, error) {
	if r.pos >= len(r.b) {
		return 0, fmt.Errorf("%w: truncated", ErrAST)
	}
	c := r.b[r.pos]
	r.pos++
	return c, nil
}

func (r *reader) string() (string, error) {
	n, err := r.uvarint()
	if err != nil {
		return "", err
	}
	if n > uint64(len(r.b)-r.pos) {
		return "", fmt.Errorf("%w: truncated string", ErrAST)
	}
	s := string(r.b[r.pos : r.pos+int(n)])
	r.pos += int(n)
	if !utf8.ValidString(s) {
		return "", fmt.Errorf("%w: string is not UTF-8", ErrAST)
	}
	return s, nil
}

// Decode reads a message written by Encode; malformed input is an error,
// never a panic.
func Decode(b []byte) (*Message, error) {
	if len(b) == 0 || b[0] != astVersion {
		return nil, fmt.Errorf("%w: unknown version", ErrAST)
	}
	r := &reader{b: b, pos: 1}
	nodes, err := r.nodes(0)
	if err != nil {
		return nil, err
	}
	if r.pos != len(b) {
		return nil, fmt.Errorf("%w: trailing bytes", ErrAST)
	}
	return &Message{Nodes: nodes}, nil
}

func (r *reader) nodes(depth int) ([]Node, error) {
	if depth > maxDepth+1 {
		return nil, fmt.Errorf("%w: nested too deep", ErrAST)
	}
	count, err := r.uvarint()
	if err != nil {
		return nil, err
	}
	if count > uint64(len(r.b)-r.pos) { // every node is at least one byte
		return nil, fmt.Errorf("%w: truncated", ErrAST)
	}
	nodes := make([]Node, 0, count)
	for range count {
		tag, err := r.byte()
		if err != nil {
			return nil, err
		}
		n := Node{Kind: Kind(tag)}
		switch n.Kind {
		case KindText:
			n.Text, err = r.string()
		case KindArg:
			n.Name, err = r.string()
		case KindNumber, KindDate, KindTime:
			if n.Name, err = r.string(); err == nil {
				n.Style, err = r.string()
			}
		case KindPlural:
			err = r.plural(&n, depth)
		case KindSelect:
			if n.Name, err = r.string(); err == nil {
				n.Cases, err = r.cases(depth)
			}
		case KindPound:
		default:
			err = fmt.Errorf("%w: unknown node %d", ErrAST, tag)
		}
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, n)
	}
	return nodes, nil
}

func (r *reader) plural(n *Node, depth int) error {
	var err error
	if n.Name, err = r.string(); err != nil {
		return err
	}
	flag, err := r.byte()
	if err != nil {
		return err
	}
	n.Ordinal = flag == 1
	off, err := r.uvarint()
	if err != nil {
		return err
	}
	if off > 1<<31-1 {
		return fmt.Errorf("%w: offset out of range", ErrAST)
	}
	n.Offset = int64(off)
	n.Cases, err = r.cases(depth)
	return err
}

func (r *reader) cases(depth int) ([]Case, error) {
	count, err := r.uvarint()
	if err != nil {
		return nil, err
	}
	if count > uint64(len(r.b)-r.pos) {
		return nil, fmt.Errorf("%w: truncated", ErrAST)
	}
	cases := make([]Case, 0, count)
	for range count {
		sel, err := r.string()
		if err != nil {
			return nil, err
		}
		body, err := r.nodes(depth + 1)
		if err != nil {
			return nil, err
		}
		cases = append(cases, Case{Selector: sel, Body: body})
	}
	return cases, nil
}
