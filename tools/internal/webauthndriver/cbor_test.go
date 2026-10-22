// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package webauthndriver

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"testing"
)

// decodeCBOR is the test's reader of what the encoder wrote: integers (as
// int64), byte strings, text strings and maps (as map[any]any), the types the
// server's decoder reads from an attestation object. It returns the item and
// the bytes it used.
func decodeCBOR(b []byte) (any, int, error) {
	if len(b) == 0 {
		return nil, 0, fmt.Errorf("empty input")
	}
	major, info := b[0]>>5, b[0]&0x1f
	n, used := uint64(info), 1
	switch {
	case info == 24:
		n, used = uint64(b[1]), 2
	case info == 25:
		n, used = uint64(binary.BigEndian.Uint16(b[1:])), 3
	case info == 26:
		n, used = uint64(binary.BigEndian.Uint32(b[1:])), 5
	case info == 27:
		n, used = binary.BigEndian.Uint64(b[1:]), 9
	case info > 27:
		return nil, 0, fmt.Errorf("unsupported additional information %d", info)
	}
	switch major {
	case majorUnsigned:
		return int64(n), used, nil
	case majorNegative:
		return -1 - int64(n), used, nil
	case majorBytes:
		return b[used : used+int(n)], used + int(n), nil
	case majorText:
		return string(b[used : used+int(n)]), used + int(n), nil
	case majorMap:
		m := map[any]any{}
		for range n {
			k, ku, err := decodeCBOR(b[used:])
			if err != nil {
				return nil, 0, err
			}
			v, vu, err := decodeCBOR(b[used+ku:])
			if err != nil {
				return nil, 0, err
			}
			m[k] = v
			used += ku + vu
		}
		return m, used, nil
	}
	return nil, 0, fmt.Errorf("unsupported major type %d", major)
}

// TestCBOREncoding_SEC103 checks the encoder against the examples of RFC 8949
// Appendix A, and against the test's own reader.
//
// Verifies: SEC-103.
func TestCBOREncoding_SEC103(t *testing.T) {
	tests := []struct {
		name string
		got  []byte
		want string
	}{
		{"zero", cborInt(0), "00"},
		{"23", cborInt(23), "17"},
		{"24", cborInt(24), "1818"},
		{"500", cborInt(500), "1901f4"},
		{"one million", cborInt(1000000), "1a000f4240"},
		{"2^32", cborInt(1 << 32), "1b0000000100000000"},
		{"minus one", cborInt(-1), "20"},
		{"minus 1000", cborInt(-1000), "3903e7"},
		{"text", cborText("IETF"), "6449455446"},
		{"bytes", cborBytes([]byte{1, 2, 3, 4}), "4401020304"},
		{"empty map", cborMap(), "a0"},
		{"map", cborMap([2][]byte{cborText("a"), cborInt(1)}, [2][]byte{cborText("b"), cborBytes([]byte{2})}), "a2616101616241" + "02"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hex.EncodeToString(tt.got); got != tt.want {
				t.Fatalf("encoded %s, want %s", got, tt.want)
			}
			if _, used, err := decodeCBOR(tt.got); err != nil || used != len(tt.got) {
				t.Fatalf("the reader used %d of %d bytes: %v", used, len(tt.got), err)
			}
		})
	}
	if !bytes.Equal(cborInt(-7), []byte{0x26}) {
		t.Fatalf("ES256 is %x, want 26", cborInt(-7))
	}
}
