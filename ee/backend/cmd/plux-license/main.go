// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

// Command plux-license is the maintainer's offline tool for Plux licence
// files (GOV-030, ADR-0084): it creates a signing key, signs a payload and
// verifies a file. It is run on the machine that holds the private key, and
// it is part of no image.
package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/nightCode42/plux3/backend/edition"
	"github.com/nightCode42/plux3/backend/edition/licensefile"
	"github.com/nightCode42/plux3/ee/backend/license"
)

const name = "plux-license"

const usage = `Usage: plux-license <command> [flags]

Commands:
  keygen --out <file>                     Create a signing key; the private key goes to <file> (mode 0600), the public key and its ID are printed
  sign <payload.json> --key <file> [--out <file>]
                                          Sign a payload with the private key; the licence goes to --out or stdout
  verify <licence> [--public-key <b64>]   Verify a licence against the compiled-in keys and any --public-key given, and print its state
  help                                    Show this help

The private key never leaves the machine that created it. Keep it offline.
`

// Exit codes: 0 success, 1 the command failed or the licence did not
// verify, 2 usage.
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
	case "sign":
		return sign(args[1:], stdout, stderr)
	case "verify":
		return verify(args[1:], stdout, stderr, now)
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

// keygen creates a key pair and writes the private key, base64-encoded, to
// a new file readable by its owner only.
func keygen(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet(name+" keygen", flag.ContinueOnError)
	fs.SetOutput(stderr)
	out := fs.String("out", "", "file for the private key (must not exist)")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 || *out == "" {
		_, _ = fmt.Fprint(stderr, "usage: plux-license keygen --out <file>\n")
		return exitUsage
	}
	pub, priv, err := license.GenerateKey(nil)
	if err != nil {
		return fail(stderr, err)
	}
	f, err := os.OpenFile(*out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fail(stderr, fmt.Errorf("create the key file: %w", err))
	}
	_, werr := f.WriteString(base64.StdEncoding.EncodeToString(priv) + "\n")
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		return fail(stderr, fmt.Errorf("write the key file: %w", werr))
	}
	_, _ = fmt.Fprintf(stdout, "key ID:     %s\npublic key: %s\nprivate key written to %s; keep it offline\n",
		licensefile.KeyID(pub), base64.StdEncoding.EncodeToString(pub), *out)
	return exitOK
}

// readKey reads a private key file written by keygen.
func readKey(path string) (ed25519.PrivateKey, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // G304: the maintainer names the key file.
	if err != nil {
		return nil, fmt.Errorf("read the key file: %w", err)
	}
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(b) != ed25519.PrivateKeySize {
		return nil, errors.New("the key file is not a plux-license private key")
	}
	return ed25519.PrivateKey(b), nil
}

// sign signs a payload file.
func sign(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet(name+" sign", flag.ContinueOnError)
	fs.SetOutput(stderr)
	keyPath := fs.String("key", "", "private key file")
	out := fs.String("out", "", "file for the licence (default stdout)")
	// Flags may follow the payload path, so parse in two passes.
	var positional []string
	rest := args
	for {
		if err := fs.Parse(rest); err != nil {
			return exitUsage
		}
		if fs.NArg() == 0 {
			break
		}
		positional = append(positional, fs.Arg(0))
		rest = fs.Args()[1:]
	}
	if len(positional) != 1 || *keyPath == "" {
		_, _ = fmt.Fprint(stderr, "usage: plux-license sign <payload.json> --key <file> [--out <file>]\n")
		return exitUsage
	}
	raw, err := os.ReadFile(positional[0])
	if err != nil {
		return fail(stderr, fmt.Errorf("read the payload: %w", err))
	}
	var p licensefile.Payload
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return fail(stderr, fmt.Errorf("the payload is not a licence payload: %w", err))
	}
	key, err := readKey(*keyPath)
	if err != nil {
		return fail(stderr, err)
	}
	file, err := license.Sign(p, key)
	if err != nil {
		return fail(stderr, err)
	}
	if *out == "" {
		_, _ = stdout.Write(append(file, '\n'))
		return exitOK
	}
	if err := os.WriteFile(*out, append(file, '\n'), 0o644); err != nil { //nolint:gosec // a licence file is not secret
		return fail(stderr, fmt.Errorf("write the licence: %w", err))
	}
	_, _ = fmt.Fprintf(stdout, "licence for %s written to %s\n", p.Organization, *out)
	return exitOK
}

// stringList is a repeatable string flag.
type stringList []string

func (s *stringList) String() string     { return strings.Join(*s, ",") }
func (s *stringList) Set(v string) error { *s = append(*s, v); return nil }

// verify checks a licence file and prints its state.
func verify(args []string, stdout, stderr io.Writer, now func() time.Time) int {
	fs := flag.NewFlagSet(name+" verify", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var extra stringList
	fs.Var(&extra, "public-key", "a base64 public key to trust besides the compiled-in ones (repeatable)")
	install := fs.String("installation-id", "", "the installation the licence must be bound to, when it is bound")
	var positional []string
	rest := args
	for {
		if err := fs.Parse(rest); err != nil {
			return exitUsage
		}
		if fs.NArg() == 0 {
			break
		}
		positional = append(positional, fs.Arg(0))
		rest = fs.Args()[1:]
	}
	if len(positional) != 1 {
		_, _ = fmt.Fprint(stderr, "usage: plux-license verify <licence> [--public-key <b64>]\n")
		return exitUsage
	}
	keys, err := trusted(extra)
	if err != nil {
		return fail(stderr, err)
	}
	st := license.Verifier{Keys: keys, Now: now}.Evaluate(positional[0], *install)
	_, _ = fmt.Fprintf(stdout, "state: %s\n%s\n", st.State, st.Message)
	if st.State == edition.StateValid || st.State == edition.StateGrace || st.State == edition.StateExpired {
		return exitOK
	}
	return exitFailed
}

// trusted returns the compiled-in keys and the extra ones.
func trusted(extra []string) (licensefile.KeySet, error) {
	var public []ed25519.PublicKey
	for _, e := range extra {
		p, err := licensefile.ParsePublicKey(e)
		if err != nil {
			return licensefile.KeySet{}, err //nolint:wrapcheck // the message names the problem
		}
		public = append(public, p)
	}
	return licensefile.TrustedKeysWith(public...), nil
}
