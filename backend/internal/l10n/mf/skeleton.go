// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package mf

import (
	"fmt"
	"strings"
)

// numberSkeleton is the subset of ICU number skeletons Plux supports:
// percent, compact-short, currency/XXX, precision-integer, a fraction stem
// (.00, .##, .0#) and group-off.
type numberSkeleton struct {
	percent, compact, noGroup bool
	currency                  string
	// fractionSet is true when min and max were given.
	fractionSet bool
	min, max    int
}

// parseNumberSkeleton parses the tokens of a "::" skeleton.
func parseNumberSkeleton(s string) (numberSkeleton, error) {
	var sk numberSkeleton
	for _, tok := range strings.Fields(s) {
		switch {
		case tok == "percent":
			sk.percent = true
		case tok == "compact-short":
			sk.compact = true
		case tok == "group-off":
			sk.noGroup = true
		case tok == "precision-integer":
			sk.fractionSet, sk.min, sk.max = true, 0, 0
		case strings.HasPrefix(tok, "currency/"):
			code := tok[len("currency/"):]
			if len(code) != 3 || code != strings.ToUpper(code) || strings.Trim(code, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") != "" {
				return sk, fmt.Errorf("%q is not currency/ and an ISO 4217 code", tok)
			}
			sk.currency = code
		case strings.HasPrefix(tok, ".") && len(tok) > 1:
			zeros := strings.TrimLeft(tok[1:], "0")
			hashes := strings.Trim(zeros, "#")
			if hashes != "" || len(tok) > 21 {
				return sk, fmt.Errorf("%q is not a fraction stem such as .00 or .0#", tok)
			}
			sk.fractionSet, sk.min, sk.max = true, len(tok)-1-len(zeros), len(tok)-1
		default:
			return sk, fmt.Errorf("unsupported number skeleton token %q", tok)
		}
	}
	if sk.compact && (sk.percent || sk.currency != "" || sk.fractionSet) {
		return sk, fmt.Errorf("compact-short cannot combine with other number skeleton tokens")
	}
	if sk.percent && sk.currency != "" {
		return sk, fmt.Errorf("percent and currency cannot combine")
	}
	return sk, nil
}
