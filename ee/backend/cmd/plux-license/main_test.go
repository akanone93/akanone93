// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const payloadJSON = `{
  "licenseId": "LIC-77", "organization": "Acme AG", "edition": "enterprise",
  "issuedAt": "2026-10-01T00:00:00Z", "notBefore": "2026-10-01T00:00:00Z", "expiresAt": "2027-10-01T00:00:00Z",
  "features": ["saml", "scim"], "limits": {"page.nodes": 500}
}`

var testNow = func() time.Time { return time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC) }

func do(args ...string) (int, string, string) {
	var out, errOut bytes.Buffer
	code := run(args, &out, &errOut, testNow)
	return code, out.String(), errOut.String()
}

func field(out, prefix string) string {
	for _, line := range strings.Split(out, "\n") {
		if v, ok := strings.CutPrefix(line, prefix); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// Verifies: GOV-030.
// keygen, sign and verify round-trip offline; the private key file is
// readable by its owner only and is never overwritten.
func TestKeygenSignVerify_GOV_030(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "signing.key")
	code, out, stderr := do("keygen", "--out", keyFile)
	if code != exitOK {
		t.Fatalf("keygen: %d %s", code, stderr)
	}
	pub := field(out, "public key:")
	if pub == "" || strings.Contains(out, string(mustRead(t, keyFile))) {
		t.Fatalf("keygen output: %q", out)
	}
	if info, err := os.Stat(keyFile); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("key file mode: %v %v", info, err)
	}
	if code, _, _ := do("keygen", "--out", keyFile); code != exitFailed {
		t.Errorf("keygen overwrote an existing key: %d", code)
	}

	payload := filepath.Join(dir, "payload.json")
	if err := os.WriteFile(payload, []byte(payloadJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	licence := filepath.Join(dir, "license.json")
	if code, _, stderr := do("sign", payload, "--key", keyFile, "--out", licence); code != exitOK {
		t.Fatalf("sign: %d %s", code, stderr)
	}
	code, out, _ = do("verify", licence, "--public-key", pub)
	if code != exitOK || !strings.HasPrefix(out, "state: valid") {
		t.Errorf("verify: %d %q", code, out)
	}
	// Without the key a build trusts nothing: the production slot is empty.
	if code, out, _ := do("verify", licence); code != exitFailed || !strings.HasPrefix(out, "state: invalid") {
		t.Errorf("verify without a key: %d %q", code, out)
	}
	if code, out, _ := do("verify", filepath.Join(dir, "none")); code != exitFailed || !strings.HasPrefix(out, "state: absent") {
		t.Errorf("verify a missing file: %d %q", code, out)
	}

	// Signing to stdout, with the flags after the payload.
	code, out, _ = do("sign", payload, "--key", keyFile)
	if code != exitOK || !strings.Contains(out, `"signatures"`) {
		t.Errorf("sign to stdout: %d %q", code, out)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return bytes.TrimSpace(b)
}

// Verifies: GOV-030.
func TestSignRefusals_GOV_030(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "k")
	if code, _, _ := do("keygen", "--out", keyFile); code != exitOK {
		t.Fatal("keygen")
	}
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	badKey := write("badkey", "not a key")
	unknownField := write("unknown.json", `{"surprise": 1}`)
	unknownLimit := write("limit.json", strings.Replace(payloadJSON, "page.nodes", "no.such", 1))
	notJSON := write("notjson", "x")
	good := write("good.json", payloadJSON)

	for name, args := range map[string][]string{
		"no payload":     {"sign", "--key", keyFile},
		"no key":         {"sign", good},
		"missing":        {"sign", filepath.Join(dir, "none"), "--key", keyFile},
		"not json":       {"sign", notJSON, "--key", keyFile},
		"unknown field":  {"sign", unknownField, "--key", keyFile},
		"unknown limit":  {"sign", unknownLimit, "--key", keyFile},
		"bad key":        {"sign", good, "--key", badKey},
		"missing key":    {"sign", good, "--key", filepath.Join(dir, "none")},
		"unwritable out": {"sign", good, "--key", keyFile, "--out", filepath.Join(dir, "no", "dir", "x")},
		"bad flag":       {"sign", "--nope"},
	} {
		if code, _, _ := do(args...); code == exitOK {
			t.Errorf("%s: exit 0", name)
		}
	}
}

// Verifies: GOV-030.
func TestUsage_GOV_030(t *testing.T) {
	t.Parallel()
	if code, _, stderr := do(); code != exitUsage || !strings.Contains(stderr, "Usage") {
		t.Errorf("no args: %d", code)
	}
	if code, out, _ := do("help"); code != exitOK || !strings.Contains(out, "keygen") {
		t.Errorf("help: %d", code)
	}
	if code, _, _ := do("bogus"); code != exitUsage {
		t.Errorf("bogus: %d", code)
	}
	if code, _, _ := do("keygen"); code != exitUsage {
		t.Errorf("keygen without --out: %d", code)
	}
	if code, _, _ := do("verify"); code != exitUsage {
		t.Errorf("verify without a file: %d", code)
	}
	if code, _, _ := do("verify", "x", "--public-key", "!!"); code != exitFailed {
		t.Errorf("verify with a bad key: %d", code)
	}
	if code, _, _ := do("verify", "--nope"); code != exitUsage {
		t.Errorf("verify with a bad flag: %d", code)
	}
	if code, _, _ := do("keygen", "--out", filepath.Join(t.TempDir(), "no", "dir")); code != exitFailed {
		t.Errorf("keygen into a missing directory: %d", code)
	}
	var s stringList
	_ = s.Set("a")
	_ = s.Set("b")
	if s.String() != "a,b" {
		t.Errorf("stringList = %q", s.String())
	}
}
