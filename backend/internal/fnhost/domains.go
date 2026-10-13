// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package fnhost

import "strings"

// domainAllowed reports whether host matches an entry of the allowlist
// (FN-012). An entry is an exact host name or `*.example.com`, which matches
// any host with one label or more before `.example.com` but not
// `example.com` itself. Matching ignores case and a trailing dot; the port
// is not part of host.
func domainAllowed(allow []string, host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if host == "" {
		return false
	}
	for _, entry := range allow {
		entry = strings.ToLower(strings.TrimSuffix(entry, "."))
		if suffix, wild := strings.CutPrefix(entry, "*"); wild {
			if strings.HasPrefix(suffix, ".") && len(host) > len(suffix) && strings.HasSuffix(host, suffix) {
				return true
			}
			continue
		}
		if entry == host {
			return true
		}
	}
	return false
}
