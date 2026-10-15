// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/storage/storagetest"
)

// Verifies: SEC-123.
// The keys command refuses what it cannot run before it touches a
// database.
func TestKeysUsage_SEC_123(t *testing.T) {
	t.Parallel()
	for name, args := range map[string][]string{
		"no subcommand":      {"keys"},
		"unknown subcommand": {"keys", "export"},
		"import no role":     {"keys", "import", "-key-id", "K", "-team-id", "T", "-file", "k.p8"},
		"import no key ID":   {"keys", "import", "-role", "r", "-team-id", "T", "-file", "k.p8"},
		"import no team ID":  {"keys", "import", "-role", "r", "-key-id", "K", "-file", "k.p8"},
		"import no file":     {"keys", "import", "-role", "r", "-key-id", "K", "-team-id", "T"},
		"show no role":       {"keys", "show"},
		"show with a file":   {"keys", "show", "-role", "r", "-file", "k.p8"},
		"extra argument":     {"keys", "show", "-role", "r", "extra"},
		"unknown flag":       {"keys", "show", "-role", "r", "-nonsense"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			if code := run(context.Background(), args, &stdout, &stderr); code != exitUsage {
				t.Errorf("exit code = %d, want %d (stderr: %s)", code, exitUsage, stderr.String())
			}
		})
	}
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"help"}, &stdout, &stderr); code != exitOK || !strings.Contains(stdout.String(), "keys ") {
		t.Errorf("the help does not list the keys command: %q", stdout.String())
	}
}

// Verifies: SEC-123.
// A key file is imported once, shown by its metadata only, replaced by a
// second import, and a wrong file is refused without echoing its content.
func TestKeysImportAndShow_SEC_123(t *testing.T) {
	url := storagetest.SchemaURL(t)
	dir := t.TempDir()
	path := configFile(t, "server:\n  publicBaseURL: \"https://p.example\"\ndatabase:\n  url: \""+url+"\"\n"+
		"signing:\n  directory: \""+filepath.Join(dir, "keys")+"\"\n")
	ctx := context.Background()
	var stdout, stderr bytes.Buffer
	if code := run(ctx, []string{"migrate", "-config", path}, &stdout, &stderr); code != exitOK {
		t.Fatalf("migrate: %d; %s", code, stderr.String())
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	p8 := filepath.Join(dir, "AuthKey_ABC123DEFG.p8")
	if err := os.WriteFile(p8, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(pubDER)
	digest := hex.EncodeToString(sum[:])
	scalarBytes, err := key.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	secrets := []string{string(der), hex.EncodeToString(der), hex.EncodeToString(scalarBytes)}

	stdout.Reset()
	stderr.Reset()
	code := run(ctx, []string{"keys", "import", "-config", path, "-role", "apple-appattest-receipt", "-key-id", "ABC123DEFG", "-team-id", "TEAM123456", "-file", p8}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("keys import: %d; %s", code, stderr.String())
	}
	for _, want := range []string{"role: apple-appattest-receipt", "key ID: ABC123DEFG", "team ID: TEAM123456", "public key SHA-256: " + digest, "Delete " + p8} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("import output lacks %q:\n%s", want, stdout.String())
		}
	}
	for _, s := range secrets {
		if strings.Contains(stdout.String()+stderr.String(), s) {
			t.Error("the output carries key material")
		}
	}

	stdout.Reset()
	if code := run(ctx, []string{"keys", "show", "-config", path, "-role", "apple-appattest-receipt"}, &stdout, &stderr); code != exitOK {
		t.Fatalf("keys show: %d; %s", code, stderr.String())
	}
	for _, want := range []string{"key ID: ABC123DEFG", "team ID: TEAM123456", "public key SHA-256: " + digest} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("show output lacks %q:\n%s", want, stdout.String())
		}
	}

	stderr.Reset()
	if code := run(ctx, []string{"keys", "show", "-config", path, "-role", "no-such-role"}, &stdout, &stderr); code != exitFailed ||
		!strings.Contains(stderr.String(), "no key is imported") {
		t.Errorf("show of an unknown role: %d %q", code, stderr.String())
	}

	bad := filepath.Join(dir, "bad.p8")
	if err := os.WriteFile(bad, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("secret-not-a-key")}), 0o600); err != nil {
		t.Fatal(err)
	}
	big := filepath.Join(dir, "big.p8")
	if err := os.WriteFile(big, bytes.Repeat([]byte("A"), maxKeyFile+1), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, file := range map[string]string{"not a key": bad, "too large": big, "missing": filepath.Join(dir, "none.p8")} {
		stderr.Reset()
		args := []string{"keys", "import", "-config", path, "-role", "other-role", "-key-id", "K", "-team-id", "T", "-file", file}
		if code := run(ctx, args, &stdout, &stderr); code != exitFailed {
			t.Errorf("%s: exit %d (stderr: %s)", name, code, stderr.String())
		}
		if strings.Contains(stderr.String(), "c2VjcmV0") || strings.Contains(stderr.String(), "secret-not-a-key") {
			t.Errorf("%s: the error echoes the file: %s", name, stderr.String())
		}
	}
	stderr.Reset()
	if code := run(ctx, []string{"keys", "show", "-config", path, "-role", "other-role"}, &stdout, &stderr); code != exitFailed {
		t.Errorf("a refused import left a key: exit %d", code)
	}
}
