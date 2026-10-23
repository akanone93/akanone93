// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package analytics

import "errors"

// errIdentifier says an identifier is not a canonical UUID. It never quotes
// the value, which may be hostile.
var errIdentifier = errors.New("analytics: an identifier is not a UUID")

// uuidLen is the length of a canonical UUID, 8-4-4-4-12 hexadecimal digits.
const uuidLen = 36

// canonicalUUID returns s lower-cased when it is a canonical UUID. Every
// identifier is checked before it reaches a query, as a second line behind
// parameter binding.
func canonicalUUID(s string) (string, error) {
	if len(s) != uuidLen {
		return "", errIdentifier
	}
	out := []byte(s)
	for i, c := range out {
		switch {
		case i == 8 || i == 13 || i == 18 || i == 23:
			if c != '-' {
				return "", errIdentifier
			}
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f':
		case c >= 'A' && c <= 'F':
			out[i] = c + ('a' - 'A')
		default:
			return "", errIdentifier
		}
	}
	return string(out), nil
}
