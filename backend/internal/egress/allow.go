// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package egress

import (
	"net/netip"
	"strings"
)

// maxHostLength and maxLabelLength are the DNS limits on a name.
const (
	maxHostLength  = 253
	maxLabelLength = 63
)

// maxPatterns bounds the allowlist a caller may send with one tunnel.
const maxPatterns = 256

// normalizeHost lower-cases a host name and drops one trailing dot. It
// reports false for anything that is not a plain ASCII DNS name: an
// empty name, non-ASCII input (an IDNA name must arrive already encoded
// as xn--), a label that is empty, longer than 63 bytes or edged by a hyphen, or a
// character outside letters, digits, hyphen and underscore.
func normalizeHost(host string) (string, bool) {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if host == "" || len(host) > maxHostLength {
		return "", false
	}
	for label := range strings.SplitSeq(host, ".") {
		if label == "" || len(label) > maxLabelLength || label[0] == '-' || label[len(label)-1] == '-' {
			return "", false
		}
		for i := range len(label) {
			if !isHostByte(label[i]) {
				return "", false
			}
		}
	}
	return host, true
}

// isHostByte reports whether b may appear in a label.
func isHostByte(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b == '-' || b == '_'
}

// looksLikeAddress reports whether a normalised host is an IP literal in
// any spelling a resolver or a socket library might accept: a parsed
// address, or a name whose last label is numeric or a hexadecimal number
// (2130706433, 0x7f.1, 127.1). No top-level domain looks like that, so
// refusing them costs nothing.
func looksLikeAddress(host string) bool {
	if _, err := netip.ParseAddr(host); err == nil {
		return true
	}
	last := host[strings.LastIndexByte(host, '.')+1:]
	if strings.HasPrefix(last, "0x") {
		return true
	}
	for i := range len(last) {
		if last[i] < '0' || last[i] > '9' {
			return false
		}
	}
	return true
}

// parseAllowlist splits the Plux-Egress-Allow header values into
// normalised patterns. A pattern that is not an exact name or
// "*.name" is dropped, so a malformed entry never widens the list; the
// caller refuses a request whose list ends up empty.
func parseAllowlist(values []string) []string {
	var out []string
	for _, value := range values {
		for raw := range strings.SplitSeq(value, ",") {
			if len(out) >= maxPatterns {
				return out
			}
			if p, ok := normalizePattern(strings.TrimSpace(raw)); ok {
				out = append(out, p)
			}
		}
	}
	return out
}

// normalizePattern validates one allowlist entry: a host name, or a
// host name preceded by "*." to match any number of labels beneath it.
func normalizePattern(raw string) (string, bool) {
	wild := strings.HasPrefix(raw, "*.")
	name, ok := normalizeHost(strings.TrimPrefix(raw, "*."))
	if !ok || looksLikeAddress(name) {
		return "", false
	}
	if wild {
		return "*." + name, true
	}
	return name, true
}

// matchAllowlist reports whether a normalised host matches one of the
// normalised patterns. "*.example.com" matches one or more labels under
// example.com and not example.com itself.
func matchAllowlist(patterns []string, host string) bool {
	for _, p := range patterns {
		if suffix, wild := strings.CutPrefix(p, "*"); wild {
			if len(host) > len(suffix) && strings.HasSuffix(host, suffix) {
				return true
			}
			continue
		}
		if p == host {
			return true
		}
	}
	return false
}
