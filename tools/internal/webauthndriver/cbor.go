// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package webauthndriver

import "encoding/binary"

// This file is the CBOR (RFC 8949) an authenticator needs to write: unsigned
// and negative integers, byte and text strings, and maps. Every function
// returns a complete encoded data item, so items compose by concatenation.

// CBOR major types.
const (
	majorUnsigned = 0
	majorNegative = 1
	majorBytes    = 2
	majorText     = 3
	majorMap      = 5
)

// cborHead encodes a major type and its argument in the shortest form.
func cborHead(major byte, n uint64) []byte {
	m := major << 5
	switch {
	case n < 24:
		return []byte{m | byte(n)}
	case n <= 0xff:
		return []byte{m | 24, byte(n)}
	case n <= 0xffff:
		return binary.BigEndian.AppendUint16([]byte{m | 25}, uint16(n))
	case n <= 0xffffffff:
		return binary.BigEndian.AppendUint32([]byte{m | 26}, uint32(n))
	default:
		return binary.BigEndian.AppendUint64([]byte{m | 27}, n)
	}
}

// cborInt encodes a signed integer.
func cborInt(n int64) []byte {
	if n >= 0 {
		return cborHead(majorUnsigned, uint64(n))
	}
	return cborHead(majorNegative, uint64(-1-n))
}

// cborBytes encodes a byte string.
func cborBytes(b []byte) []byte { return append(cborHead(majorBytes, uint64(len(b))), b...) }

// cborText encodes a text string.
func cborText(s string) []byte { return append(cborHead(majorText, uint64(len(s))), s...) }

// cborMap encodes a map from already encoded keys and values, in the order
// given; pairs[i][0] is a key and pairs[i][1] its value.
func cborMap(pairs ...[2][]byte) []byte {
	out := cborHead(majorMap, uint64(len(pairs)))
	for _, p := range pairs {
		out = append(append(out, p[0]...), p[1]...)
	}
	return out
}
