// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package config

import (
	"crypto/ed25519"
	"encoding/base64"
	"slices"
	"strings"
	"testing"
)

// fnrunnerSource renders a valid process that runs only the fnrunner role
// (no database, storage, cache or signing section). Each key of set replaces
// the line that starts with it; an empty value removes the line.
func fnrunnerSource(set map[string]string) string {
	key := base64.StdEncoding.EncodeToString(make([]byte, ed25519.PublicKeySize))
	lines := [][2]string{
		{"listen", `":8445"`},
		{"internalTLS", "\n    caFile: /ca.crt\n    certFile: /r.crt\n    keyFile: /r.key"},
		{"apiURL", `"https://api.internal:8443"`},
		{"egressAddr", `"egress.internal:8444"`},
		{"artifactKeys", "\n    - keyID: k1\n      publicKey: " + key},
	}
	var b strings.Builder
	b.WriteString("server:\n  roles: [fnrunner]\nfnrunner:\n")
	for _, l := range lines {
		v, replaced := set[l[0]]
		if !replaced {
			v = l[1]
		}
		if v != "" {
			b.WriteString("  " + l[0] + ": " + v + "\n")
		}
	}
	for name, v := range set {
		if !slices.ContainsFunc(lines, func(l [2]string) bool { return l[0] == name }) {
			b.WriteString("  " + name + ": " + v + "\n")
		}
	}
	return b.String()
}

// Verifies: FN-013, FN-009, SEC-043.
func TestFnRunnerRoleIsValidated(t *testing.T) {
	t.Parallel()
	c := parse(t, fnrunnerSource(nil))
	f := c.FnRunner
	if !c.Has(RoleFnRunner) || !c.NoDataAccess() || c.Database.URL != "" {
		t.Errorf("roles = %v, database = %q", c.Server.Roles, c.Database.URL)
	}
	if f.MaxMemoryBytes != 512<<20 || f.HTTPResponseBytes != 1<<20 || f.MaxModules < 1 || f.MaxConcurrent < 1 || f.MaxModuleBytes < 1 {
		t.Errorf("fnrunner defaults = %+v", f)
	}
	key, err := f.ArtifactKeys[0].Decode()
	if err != nil || len(key) != ed25519.PublicKeySize {
		t.Errorf("Decode = %v, %v", key, err)
	}

	keyAs := func(v string) map[string]string {
		return map[string]string{"artifactKeys": "\n    - keyID: k1\n      publicKey: " + v}
	}
	mentions(t, []struct{ name, src, want string }{
		{"no listener", fnrunnerSource(map[string]string{"listen": ""}), "fnrunner.listen"},
		{"no mutual TLS", fnrunnerSource(map[string]string{"internalTLS": ""}), "fnrunner.internalTLS"},
		{"no API URL", fnrunnerSource(map[string]string{"apiURL": ""}), "fnrunner.apiURL"},
		{"relative API URL", fnrunnerSource(map[string]string{"apiURL": `"/api"`}), "is not an absolute URL"},
		{"no egress", fnrunnerSource(map[string]string{"egressAddr": ""}), "fnrunner.egressAddr"},
		{"no artifact keys", fnrunnerSource(map[string]string{"artifactKeys": ""}), "fnrunner.artifactKeys"},
		{"short key", fnrunnerSource(keyAs("AAAA")), "fnrunner.artifactKeys[0].publicKey"},
		{"not base64", fnrunnerSource(keyAs(`"***"`)), "fnrunner.artifactKeys[0].publicKey"},
		{"no key id", fnrunnerSource(map[string]string{"artifactKeys": "\n    - keyID: \"\"\n      publicKey: AAAA"}), "fnrunner.artifactKeys[0].keyID"},
		{"no module count", fnrunnerSource(map[string]string{"maxModules": "0"}), "fnrunner.maxModules"},
		{"no module bytes", fnrunnerSource(map[string]string{"maxModuleBytes": "0"}), "fnrunner.maxModuleBytes"},
		{"no concurrency", fnrunnerSource(map[string]string{"maxConcurrent": "0"}), "fnrunner.maxConcurrent"},
		{"no memory", fnrunnerSource(map[string]string{"maxMemoryBytes": "0"}), "fnrunner.maxMemoryBytes"},
		{"too much memory", fnrunnerSource(map[string]string{"maxMemoryBytes": "4294967296"}), "fnrunner.maxMemoryBytes"},
		{"no response bytes", fnrunnerSource(map[string]string{"httpResponseBytes": "0"}), "fnrunner.httpResponseBytes"},
	})
}

// Verifies: FN-013.
// A process that also runs a data role is not exempt from the database.
func TestNoDataAccessNeedsOnlyFnRunnerAndEgress(t *testing.T) {
	t.Parallel()
	for roles, want := range map[string]bool{
		"fnrunner": true, "egress": true, "fnrunner, egress": true,
		"api": false, "worker, fnrunner": false, "gateway, egress": false, "": false,
	} {
		var names []Role
		for _, r := range strings.Split(roles, ", ") {
			if r != "" {
				names = append(names, Role(r))
			}
		}
		c := &Config{Server: Server{Roles: names}}
		if got := c.NoDataAccess(); got != want {
			t.Errorf("NoDataAccess(%q) = %v, want %v", roles, got, want)
		}
	}
	src := strings.Replace(fnrunnerSource(nil), "[fnrunner]", "[api, fnrunner]", 1)
	if got := refuse(t, src); !strings.Contains(got, "database.url") {
		t.Errorf("api with fnrunner: %q does not require a database", got)
	}
}
