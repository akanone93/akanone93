// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package main

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/edition"
	"github.com/nightCode42/plux3/backend/edition/licensefile"
	"github.com/nightCode42/plux3/ee/backend/license"
)

var testNow = func() time.Time { return time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC) }

func do(args ...string) (int, string, string) {
	var out, errOut bytes.Buffer
	code := run(args, &out, &errOut, testNow)
	return code, out.String(), errOut.String()
}

func write(t *testing.T, path, content string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// signedLicence writes a licence that lists the given features, signed by a
// fresh key, and returns its path and the key's base64 public half.
func signedLicence(t *testing.T, dir string, features ...string) (string, string) {
	t.Helper()
	pub, priv, err := license.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := license.Sign(licensefile.Payload{
		LicenseID: "LIC-1", Organization: "Acme AG", Edition: "enterprise",
		IssuedAt: "2026-10-01T00:00:00Z", NotBefore: "2026-10-01T00:00:00Z", ExpiresAt: "2027-10-01T00:00:00Z",
		Features: features,
	}, priv)
	if err != nil {
		t.Fatal(err)
	}
	return write(t, filepath.Join(dir, "licence-"+strings.Join(features, "-")+".json"), string(raw)), base64.StdEncoding.EncodeToString(pub)
}

// imageTar writes a minimal OCI layout tar with one image and returns its path.
func imageTar(t *testing.T, dir string) string {
	t.Helper()
	put := func(files map[string][]byte, b []byte) (string, int) {
		sum := sha256.Sum256(b)
		h := hex.EncodeToString(sum[:])
		files["blobs/sha256/"+h] = b
		return "sha256:" + h, len(b)
	}
	files := map[string][]byte{}
	cfg, cfgN := put(files, []byte(`{}`))
	layer, layerN := put(files, []byte("layer"))
	m, mN := put(files, []byte(`{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json","config":{"digest":"`+cfg+`","size":`+strconv.Itoa(cfgN)+`},"layers":[{"digest":"`+layer+`","size":`+strconv.Itoa(layerN)+`}]}`))
	files["index.json"] = []byte(`{"schemaVersion":2,"manifests":[{"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":"` + m + `","size":` + strconv.Itoa(mN) + `,"annotations":{"org.opencontainers.image.ref.name":"example.test/plux/server:1"}}]}`)
	files["oci-layout"] = []byte(`{"imageLayoutVersion":"1.0.0"}`)
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for n, b := range files {
		_ = tw.WriteHeader(&tar.Header{Name: n, Size: int64(len(b)), Mode: 0o644, Typeflag: tar.TypeReg})
		_, _ = tw.Write(b)
	}
	_ = tw.Close()
	return write(t, filepath.Join(dir, "server.tar"), buf.String())
}

// env is a working directory with every input of the bundle command.
type env struct {
	dir, image, chart, docs, key, pubKey string
}

func newEnv(t *testing.T) env {
	t.Helper()
	dir := t.TempDir()
	e := env{dir: dir, image: imageTar(t, dir), chart: filepath.Join(dir, "plux"), docs: filepath.Join(dir, "docs"), key: filepath.Join(dir, "bundle.key")}
	write(t, filepath.Join(e.chart, "Chart.yaml"), "name: plux\n")
	write(t, filepath.Join(e.docs, "egress.md"), "# Egress\n")
	write(t, filepath.Join(e.docs, "air-gapped.md"), "# Guide\n")
	code, _, errOut := do("keygen", "--out", e.key)
	if code != exitOK {
		t.Fatalf("keygen: %s", errOut)
	}
	e.pubKey = e.key + ".pub"
	return e
}

func (e env) bundleArgs(out, licence, publicKey string) []string {
	return []string{
		"bundle", "--version", "0.0.1", "--images", e.image, "--chart", e.chart, "--docs", e.docs,
		"--key", e.key, "--licence", licence, "--public-key", publicKey, "--out", out,
	}
}

// TestBundleAndVerify_DEP_040 builds a bundle with the command line and
// verifies it, including a refusal with the wrong key.
//
// Verifies: DEP-040.
func TestBundleAndVerify_DEP_040(t *testing.T) {
	e := newEnv(t)
	licence, licKey := signedLicence(t, e.dir, string(edition.FeatureAirGap))
	out := filepath.Join(e.dir, "plux-airgap-0.0.1.tar")

	if code, _, errOut := do(e.bundleArgs(out, licence, licKey)...); code != exitOK {
		t.Fatalf("bundle: %d %s", code, errOut)
	}
	code, stdout, errOut := do("verify", out, "--key", e.pubKey)
	if code != exitOK {
		t.Fatalf("verify: %d %s", code, errOut)
	}
	if !strings.Contains(stdout, "image example.test/plux/server:1 sha256:") {
		t.Errorf("verify did not list the image:\n%s", stdout)
	}

	other := filepath.Join(e.dir, "other.key")
	if code, _, _ := do("keygen", "--out", other); code != exitOK {
		t.Fatal("keygen")
	}
	if code, _, errOut := do("verify", out, "--key", other+".pub"); code != exitFailed || !strings.Contains(errOut, "not signed by a trusted key") {
		t.Errorf("verify with the wrong key = %d %q", code, errOut)
	}
	if code, _, errOut := do("verify", out); code != exitFailed || !strings.Contains(errOut, "trusts no bundle key") {
		t.Errorf("verify with no key = %d %q", code, errOut)
	}
}

// TestVerifyRefusesAlteredBundle_DEP_040 flips one byte of a bundle file and
// expects a non-zero exit.
//
// Verifies: DEP-040.
func TestVerifyRefusesAlteredBundle_DEP_040(t *testing.T) {
	e := newEnv(t)
	licence, licKey := signedLicence(t, e.dir, string(edition.FeatureAirGap))
	out := filepath.Join(e.dir, "b.tar")
	if code, _, errOut := do(e.bundleArgs(out, licence, licKey)...); code != exitOK {
		t.Fatalf("bundle: %s", errOut)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	i := bytes.Index(raw, []byte("# Guide"))
	raw[i+2] ^= 1
	write(t, out, string(raw))
	if code, _, errOut := do("verify", out, "--key", e.pubKey); code != exitFailed || !strings.Contains(errOut, "altered") {
		t.Errorf("verify = %d %q", code, errOut)
	}
}

// TestBundleNeedsAirGapLicence_DEP_040 refuses to build without a licence
// that lists the air-gap feature, and never creates the output.
//
// Verifies: DEP-040, GOV-030.
func TestBundleNeedsAirGapLicence_DEP_040(t *testing.T) {
	e := newEnv(t)
	noFeature, licKey := signedLicence(t, e.dir, "saml")
	out := filepath.Join(e.dir, "refused.tar")

	for name, licence := range map[string]string{"other feature": noFeature, "no file": filepath.Join(e.dir, "absent.json")} {
		code, _, errOut := do(e.bundleArgs(out, licence, licKey)...)
		if code != exitFailed || !strings.Contains(errOut, "does not allow") {
			t.Errorf("%s: bundle = %d %q", name, code, errOut)
		}
	}
	if _, err := os.Stat(out); err == nil {
		t.Error("a refused build left a file")
	}
}

// TestUsage_DEP_040 covers the usage paths.
//
// Verifies: DEP-040.
func TestUsage_DEP_040(t *testing.T) {
	for _, args := range [][]string{{}, {"nope"}, {"bundle"}, {"verify"}, {"keygen"}} {
		if code, _, _ := do(args...); code != exitUsage {
			t.Errorf("%v = %d, want usage", args, code)
		}
	}
	if code, _, _ := do("help"); code != exitOK {
		t.Error("help failed")
	}
}
