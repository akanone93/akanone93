// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package receipt

import (
	"errors"
)

// maxDepth bounds the nesting the BER decoder follows. A receipt nests
// about ten levels; the margin leaves room without letting an attacker
// exhaust the stack.
const maxDepth = 24

// tagOctetString is the universal tag of OCTET STRING, the one string type
// BER may split into constructed chunks that CMS uses.
const tagOctetString = 4

// element is one decoded BER element: either primitive, with content, or
// constructed, with children.
type element struct {
	class       byte
	constructed bool
	tag         uint32
	content     []byte
	children    []element
}

// normaliseBER rewrites a BER encoding as DER-style definite-length
// encoding: minimal lengths, no indefinite lengths, and constructed OCTET
// STRINGs flattened into primitive ones. It does not reorder SET members.
// The input must be one complete element within maxLen bytes.
func normaliseBER(in []byte, maxLen int64) ([]byte, error) {
	if len(in) == 0 || int64(len(in)) > maxLen {
		return nil, errors.New("the encoding has an invalid size")
	}
	el, rest, err := parseElement(in, 0)
	if err != nil {
		return nil, err
	}
	if len(rest) != 0 {
		return nil, errors.New("the encoding has trailing data")
	}
	return appendDER(make([]byte, 0, len(in)), el), nil
}

// parseElement decodes one element from the front of b and returns the
// rest.
func parseElement(b []byte, depth int) (element, []byte, error) {
	if depth > maxDepth {
		return element{}, nil, errors.New("the encoding nests too deeply")
	}
	el, b, err := readIdentifier(b)
	if err != nil {
		return element{}, nil, err
	}
	length, indefinite, b, err := readLength(b)
	if err != nil {
		return element{}, nil, err
	}
	if !el.constructed {
		if indefinite || length > uint64(len(b)) {
			return element{}, nil, errors.New("a primitive element has an invalid length")
		}
		el.content = b[:length]
		return el, b[length:], nil
	}
	if indefinite {
		b, err = parseIndefinite(&el, b, depth)
	} else {
		b, err = parseDefinite(&el, b, length, depth)
	}
	if err != nil {
		return element{}, nil, err
	}
	if el.class == 0 && el.tag == tagOctetString {
		if err := flattenOctets(&el); err != nil {
			return element{}, nil, err
		}
	}
	return el, b, nil
}

// parseDefinite reads the children of a constructed element whose length
// is known.
func parseDefinite(el *element, b []byte, length uint64, depth int) ([]byte, error) {
	if length > uint64(len(b)) {
		return nil, errors.New("a constructed element is longer than its input")
	}
	body, rest := b[:length], b[length:]
	for len(body) > 0 {
		child, r, err := parseElement(body, depth+1)
		if err != nil {
			return nil, err
		}
		el.children = append(el.children, child)
		body = r
	}
	return rest, nil
}

// parseIndefinite reads the children of a constructed element up to its
// end-of-contents marker.
func parseIndefinite(el *element, b []byte, depth int) ([]byte, error) {
	for {
		if len(b) >= 2 && b[0] == 0 && b[1] == 0 {
			return b[2:], nil
		}
		if len(b) == 0 {
			return nil, errors.New("an indefinite-length element is not terminated")
		}
		child, r, err := parseElement(b, depth+1)
		if err != nil {
			return nil, err
		}
		el.children = append(el.children, child)
		b = r
	}
}

// flattenOctets turns a constructed OCTET STRING into a primitive one by
// concatenating its chunks. Nested chunks were flattened already.
func flattenOctets(el *element) error {
	var out []byte
	for _, c := range el.children {
		if c.class != 0 || c.tag != tagOctetString || c.constructed {
			return errors.New("a constructed OCTET STRING holds something else")
		}
		out = append(out, c.content...)
	}
	el.constructed, el.children, el.content = false, nil, out
	return nil
}

// readIdentifier decodes the identifier octets of an element.
func readIdentifier(b []byte) (element, []byte, error) {
	if len(b) == 0 {
		return element{}, nil, errors.New("the encoding ends inside an element")
	}
	first := b[0]
	el := element{class: first >> 6, constructed: first&0x20 != 0, tag: uint32(first & 0x1f)}
	b = b[1:]
	if el.tag == 0x1f {
		el.tag = 0
		for i := 0; ; i++ {
			if len(b) == 0 || i >= 4 {
				return element{}, nil, errors.New("a tag number is malformed")
			}
			c := b[0]
			b = b[1:]
			if i == 0 && c == 0x80 {
				return element{}, nil, errors.New("a tag number is not minimal")
			}
			el.tag = el.tag<<7 | uint32(c&0x7f)
			if c&0x80 == 0 {
				break
			}
		}
		if el.tag < 0x1f {
			return element{}, nil, errors.New("a tag number is not minimal")
		}
	}
	if el.class == 0 && el.tag == 0 {
		return element{}, nil, errors.New("an end-of-contents marker is out of place")
	}
	return el, b, nil
}

// readLength decodes the length octets of an element. indefinite is true
// for the 0x80 form.
func readLength(b []byte) (length uint64, indefinite bool, rest []byte, err error) {
	if len(b) == 0 {
		return 0, false, nil, errors.New("the encoding ends inside a length")
	}
	c := b[0]
	b = b[1:]
	switch {
	case c < 0x80:
		return uint64(c), false, b, nil
	case c == 0x80:
		return 0, true, b, nil
	case c == 0xff:
		return 0, false, nil, errors.New("a length uses the reserved form")
	}
	n := int(c & 0x7f)
	if n > 4 || len(b) < n {
		return 0, false, nil, errors.New("a length is malformed")
	}
	for _, d := range b[:n] {
		length = length<<8 | uint64(d)
	}
	return length, false, b[n:], nil
}

// appendDER appends the definite-length encoding of an element.
func appendDER(dst []byte, el element) []byte {
	body := el.content
	if el.constructed {
		body = nil
		for _, c := range el.children {
			body = appendDER(body, c)
		}
	}
	dst = appendIdentifier(dst, el)
	dst = appendLength(dst, len(body))
	return append(dst, body...)
}

// appendIdentifier appends the identifier octets of an element.
func appendIdentifier(dst []byte, el element) []byte {
	first := el.class << 6
	if el.constructed {
		first |= 0x20
	}
	if el.tag < 0x1f {
		return append(dst, first|byte(el.tag))
	}
	dst = append(dst, first|0x1f)
	var groups [5]byte
	n := 0
	for t := el.tag; t > 0; t >>= 7 {
		groups[n] = byte(t & 0x7f)
		n++
	}
	for i := n - 1; i >= 0; i-- {
		c := groups[i]
		if i > 0 {
			c |= 0x80
		}
		dst = append(dst, c)
	}
	return dst
}

// appendLength appends the minimal definite length.
func appendLength(dst []byte, n int) []byte {
	if n < 0x80 {
		return append(dst, byte(n&0x7f))
	}
	var tmp [8]byte
	i := len(tmp)
	for v := uint64(n); v > 0; v >>= 8 {
		i--
		tmp[i] = byte(v)
	}
	dst = append(dst, 0x80|byte(len(tmp)-i))
	return append(dst, tmp[i:]...)
}
