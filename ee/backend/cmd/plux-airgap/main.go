// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

// Command plux-airgap builds and verifies the air-gapped installation bundle
// (DEP-040, ADR-0083): images as one OCI layout, the packaged chart, the
// operations documents and the egress list, under a signed manifest.
//
// It is a separate enterprise tool, like plux-license, so that the core
// `plux` CLI stays free of enterprise code. It runs offline. Building needs a
// licence that lists the air-gap feature (GOV-030); verifying needs none,
// because an operator must be able to check a bundle before anything else is
// installed and a security check is never gated by edition (GOV-032).
package main

import (
	"crypto/ed25519"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/nightCode42/plux3/backend/edition"
	"github.com/nightCode42/plux3/backend/edition/licensefile"
	"github.com/nightCode42/plux3/ee/backend/airgap"
	"github.com/nightCode42/plux3/ee/backend/license"
)

const name = "plux-airgap"

const usage = `Usage: plux-airgap <command> [flags]

Commands:
  keygen --out <file>
        Create a bundle-signing key; the private key goes to <file> (mode 0600), the public key to <file>.pub
  bundle --version <v> --images <oci-tar> [<oci-tar>...] --chart <dir> --key <file> --licence <file> [--docs <dir>] [--out <file>] [--public-key <b64>]
        Build a signed bundle (default out: plux-airgap-<v>.tar, default docs: docs/operations). --images may repeat.
  verify <bundle> [--key <file.pub>]
        Check the signature and every file; print the images. Exit status 1 on any failure.
  help  Show this help

Everything runs offline. The private key never leaves the machine that holds it.
`

// Exit codes: 0 success, 1 the command failed, 2 usage.
const (
	exitOK     = 0
	exitFailed = 1
	exitUsage  = 2
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, time.Now))
}

// run executes a command and returns the exit code.
func run(args []string, stdout, stderr io.Writer, now func() time.Time) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, usage)
		return exitUsage
	}
	switch args[0] {
	case "keygen":
		return keygen(args[1:], stdout, stderr)
	case "bundle":
		return bundle(args[1:], stdout, stderr, now)
	case "verify":
		return verify(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		_, _ = fmt.Fprint(stdout, usage)
		return exitOK
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown command %q\n\n%s", name, args[0], usage)
	return exitUsage
}

func fail(stderr io.Writer, err error) int {
	_, _ = fmt.Fprintf(stderr, "%s: %s\n", name, err)
	return exitFailed
}

// stringList is a repeatable string flag.
type stringList []string

func (s *stringList) String() string     { return fmt.Sprint([]string(*s)) }
func (s *stringList) Set(v string) error { *s = append(*s, v); return nil }

// parseInterspersed parses flags that may follow positional arguments and
// returns the positional ones.
func parseInterspersed(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err //nolint:wrapcheck // the flag set already printed the problem
		}
		if fs.NArg() == 0 {
			return positional, nil
		}
		positional = append(positional, fs.Arg(0))
		args = fs.Args()[1:]
	}
}

// keygen creates a bundle-signing key pair.
func keygen(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet(name+" keygen", flag.ContinueOnError)
	fs.SetOutput(stderr)
	out := fs.String("out", "", "file for the private key (must not exist)")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 || *out == "" {
		_, _ = fmt.Fprint(stderr, "usage: plux-airgap keygen --out <file>\n")
		return exitUsage
	}
	pub, priv, err := license.GenerateKey(nil)
	if err != nil {
		return fail(stderr, err)
	}
	if err := airgap.WriteKeyPair(*out, pub, priv); err != nil {
		return fail(stderr, err)
	}
	_, _ = fmt.Fprintf(stdout, "key ID:     %s\nprivate key written to %s; keep it offline\npublic key written to %s.pub; hand it to whoever verifies the bundle\n",
		licensefile.KeyID(pub), *out, *out)
	return exitOK
}

// bundleFlags are the parsed flags of the bundle command.
type bundleFlags struct {
	version, chart, docs, out, keyPath, licencePath string
	images, publicKeys                              stringList
}

// parseBundle parses the bundle command line. A bare argument after --images
// is one more image archive, so `--images a.tar b.tar` works.
func parseBundle(args []string, stderr io.Writer) (bundleFlags, bool) {
	var f bundleFlags
	fs := flag.NewFlagSet(name+" bundle", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&f.version, "version", "", "the Plux version the bundle installs")
	fs.Var(&f.images, "images", "an OCI image layout as a tar archive (repeatable)")
	fs.StringVar(&f.chart, "chart", "", "the Helm chart directory")
	fs.StringVar(&f.docs, "docs", "docs/operations", "the operations documents directory")
	fs.StringVar(&f.out, "out", "", "the bundle file (default plux-airgap-<version>.tar)")
	fs.StringVar(&f.keyPath, "key", "", "the private signing key file")
	fs.StringVar(&f.licencePath, "licence", "", "a licence file that lists the air-gap feature")
	fs.Var(&f.publicKeys, "public-key", "a base64 public key to trust for the licence besides the compiled-in ones (repeatable)")
	rest, err := parseInterspersed(fs, args)
	if err != nil || f.version == "" || len(f.images) == 0 || f.chart == "" || f.keyPath == "" || f.licencePath == "" {
		_, _ = fmt.Fprint(stderr, "usage: plux-airgap bundle --version <v> --images <oci-tar>... --chart <dir> --key <file> --licence <file> [--docs <dir>] [--out <file>]\n")
		return f, false
	}
	f.images = append(f.images, rest...)
	if f.out == "" {
		f.out = airgap.Name(f.version)
	}
	return f, true
}

// checkLicence refuses a licence that does not list the air-gap feature.
func checkLicence(f bundleFlags, now func() time.Time) error {
	var extra []ed25519.PublicKey
	for _, k := range f.publicKeys {
		p, err := licensefile.ParsePublicKey(k)
		if err != nil {
			return err //nolint:wrapcheck // the message names the problem
		}
		extra = append(extra, p)
	}
	st := license.Verifier{Keys: licensefile.TrustedKeysWith(extra...), Now: now}.Evaluate(f.licencePath, "")
	if !st.Allows(edition.FeatureAirGap, true) {
		return fmt.Errorf("the licence does not allow building an air-gapped bundle (state %s): %s", st.State, st.Message)
	}
	return nil
}

// bundle builds a signed bundle.
func bundle(args []string, stdout, stderr io.Writer, now func() time.Time) int {
	f, ok := parseBundle(args, stderr)
	if !ok {
		return exitUsage
	}
	if err := checkLicence(f, now); err != nil {
		return fail(stderr, err)
	}
	key, err := airgap.ReadPrivateKey(f.keyPath)
	if err != nil {
		return fail(stderr, err)
	}
	m, err := writeBundle(f, key)
	if err != nil {
		return fail(stderr, err)
	}
	_, _ = fmt.Fprintf(stdout, "bundle %s: %d files, %d images\n", f.out, len(m.Files), len(m.Images))
	return exitOK
}

// writeBundle builds the bundle in a temporary file beside the target and
// renames it, so an interrupted run leaves no half bundle behind.
func writeBundle(f bundleFlags, key ed25519.PrivateKey) (airgap.Manifest, error) {
	tmp, err := os.CreateTemp(filepath.Dir(f.out), ".plux-airgap-*")
	if err != nil {
		return airgap.Manifest{}, fmt.Errorf("create the bundle: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	m, err := airgap.Build(airgap.Options{
		Version: f.version, ImageArchives: f.images, ChartDir: f.chart, DocsDir: f.docs, Key: key,
		TempDir: filepath.Dir(f.out),
	}, tmp)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return airgap.Manifest{}, err //nolint:wrapcheck // the airgap package wraps it
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil { //nolint:gosec // a bundle is not secret
		return airgap.Manifest{}, fmt.Errorf("create the bundle: %w", err)
	}
	if err := os.Rename(tmp.Name(), f.out); err != nil {
		return airgap.Manifest{}, fmt.Errorf("create the bundle: %w", err)
	}
	return m, nil
}

// verify checks a bundle and prints its images.
func verify(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet(name+" verify", flag.ContinueOnError)
	fs.SetOutput(stderr)
	keyPath := fs.String("key", "", "a file with the base64 public key the bundle must be signed with (default: the keys compiled in)")
	rest, err := parseInterspersed(fs, args)
	if err != nil || len(rest) != 1 {
		_, _ = fmt.Fprint(stderr, "usage: plux-airgap verify <bundle> [--key <file.pub>]\n")
		return exitUsage
	}
	keys, err := trustedKeys(*keyPath)
	if err != nil {
		return fail(stderr, err)
	}
	f, err := os.Open(rest[0]) //nolint:gosec // G304: the operator names the bundle.
	if err != nil {
		return fail(stderr, fmt.Errorf("open the bundle: %w", err))
	}
	defer func() { _ = f.Close() }()
	m, err := airgap.Verify(f, keys)
	if err != nil {
		return fail(stderr, err)
	}
	printVerified(stdout, m)
	return exitOK
}

// trustedKeys returns the key in the file, or the compiled-in keys when no
// file is named. An empty set is an error: nothing could verify.
func trustedKeys(keyPath string) (licensefile.KeySet, error) {
	if keyPath != "" {
		pub, err := airgap.ReadPublicKey(keyPath)
		if err != nil {
			return licensefile.KeySet{}, err //nolint:wrapcheck // the airgap package wraps it
		}
		return licensefile.NewKeySet(pub), nil
	}
	keys := licensefile.TrustedKeys()
	if keys.Len() == 0 {
		return keys, errors.New("this build trusts no bundle key; pass --key <file.pub>")
	}
	return keys, nil
}

// printVerified prints the summary and one line per image.
func printVerified(w io.Writer, m airgap.Manifest) {
	_, _ = fmt.Fprintf(w, "verified: Plux %s, %d files\n", m.Version, len(m.Files))
	for _, img := range m.Images {
		ref := img.Ref
		if ref == "" {
			ref = "-"
		}
		_, _ = fmt.Fprintf(w, "image %s %s\n", ref, img.Digest)
	}
}
