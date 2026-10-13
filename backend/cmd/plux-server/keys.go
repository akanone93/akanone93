// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/nightCode42/plux3/backend/internal/config"
	"github.com/nightCode42/plux3/backend/internal/server"
	"github.com/nightCode42/plux3/backend/internal/signing"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

// maxKeyFile bounds a key file read from disk. A PKCS#8 P-256 key is
// about 140 bytes; the bound only stops a wrong file from being read whole.
const maxKeyFile = 64 << 10

// keysUsage is the usage of the keys command.
const keysUsage = `usage: keys import -role <role> -key-id <id> -team-id <id> -file <path.p8> [-config <path>]
       keys show -role <role> [-config <path>]
`

// keysCommand runs the administration of imported signing keys (SEC-123,
// ADR-0065): `import` stores an issuer's key, such as Apple's .p8, sealed
// by the configured signing backend, and `show` prints what is stored
// about it. Neither prints key material.
func keysCommand(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || (args[0] != "import" && args[0] != "show") {
		_, _ = fmt.Fprint(stderr, name+": "+keysUsage)
		return exitUsage
	}
	fs := flag.NewFlagSet(name+" keys "+args[0], flag.ContinueOnError)
	fs.SetOutput(stderr)
	path := fs.String("config", "plux-server.yaml", "configuration file")
	role := fs.String("role", "", "the role the key serves, such as apple-appattest-receipt")
	keyID := fs.String("key-id", "", "the issuer's key identifier (import only)")
	teamID := fs.String("team-id", "", "the issuer's Team ID (import only)")
	file := fs.String("file", "", "the private key file, PKCS#8 (import only)")
	if err := fs.Parse(args[1:]); err != nil || fs.NArg() != 0 || *role == "" ||
		(args[0] == "import" && (*keyID == "" || *teamID == "" || *file == "")) ||
		(args[0] == "show" && (*keyID != "" || *teamID != "" || *file != "")) {
		_, _ = fmt.Fprint(stderr, name+": "+keysUsage)
		return exitUsage
	}
	cfg, err := config.Load(*path, os.LookupEnv)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %s:\n%s\n", name, *path, err)
		return exitUsage
	}
	fail := func(err error) int {
		_, _ = fmt.Fprintf(stderr, "%s keys: %s\n", name, err)
		return exitFailed
	}
	db, err := storage.Open(ctx, storage.Options{URL: cfg.Database.URL.Value(), MaxConnections: 2, Log: logger(cfg, stderr)})
	if err != nil {
		return fail(err)
	}
	defer db.Close()
	if args[0] == "show" {
		return showImportedKey(ctx, db, *role, stdout, fail)
	}
	return importKey(ctx, cfg, db, importRequest{role: *role, meta: signing.ImportedKeyMeta{KeyID: *keyID, TeamID: *teamID}, file: *file}, stdout, fail)
}

// importRequest is what `keys import` was asked to store.
type importRequest struct {
	role string
	meta signing.ImportedKeyMeta
	file string
}

// importKey reads the key file, stores it sealed and reports what it
// stored. The backend given to the importer wraps data keys and does
// nothing else.
func importKey(ctx context.Context, cfg *config.Config, db *storage.DB, req importRequest, stdout io.Writer, fail func(error) int) int {
	pkcs8, err := readKeyFile(req.file)
	if err != nil {
		return fail(err)
	}
	backend, err := server.BuildSigning(cfg)
	if err != nil {
		return fail(err)
	}
	imported, err := signing.NewImported(signing.ImportedOptions{
		Crypter: signing.CrypterOnly(backend), Store: signing.NewDBImportedKeyStore(db), Rand: rand.Reader, Now: time.Now,
	})
	if err != nil {
		return fail(err)
	}
	if err := imported.Import(ctx, req.role, pkcs8, req.meta); err != nil {
		return fail(err)
	}
	info, err := imported.Info(ctx, req.role)
	if err != nil {
		return fail(err)
	}
	printImportedKey(stdout, info)
	_, _ = fmt.Fprintf(stdout, "Delete %s now: the server keeps the key only as an encrypted envelope.\n", req.file)
	return exitOK
}

// showImportedKey prints the metadata of a stored key.
func showImportedKey(ctx context.Context, db *storage.DB, role string, stdout io.Writer, fail func(error) int) int {
	// Metadata is read without the wrapping key, so no Crypter is given
	// and nothing can open the key.
	imported, err := signing.NewImported(signing.ImportedOptions{
		Crypter: noCrypter{}, Store: signing.NewDBImportedKeyStore(db), Rand: rand.Reader, Now: time.Now,
	})
	if err != nil {
		return fail(err)
	}
	info, err := imported.Info(ctx, role)
	if err != nil {
		if errors.Is(err, signing.ErrNoKey) {
			return fail(fmt.Errorf("no key is imported for the role %q", role))
		}
		return fail(err)
	}
	printImportedKey(stdout, info)
	return exitOK
}

// noCrypter refuses to wrap or unwrap. It stands in where only metadata
// is read.
type noCrypter struct{}

// Wrap refuses.
func (noCrypter) Wrap(context.Context, []byte) ([]byte, string, error) {
	return nil, "", errors.New("this command does not wrap keys")
}

// Unwrap refuses.
func (noCrypter) Unwrap(context.Context, []byte, string) ([]byte, error) {
	return nil, errors.New("this command does not open keys")
}

// printImportedKey writes one line per field. The public key is shown by
// its digest, which is what an operator compares with the issuer's.
func printImportedKey(stdout io.Writer, info signing.ImportedKeyInfo) {
	_, _ = fmt.Fprintf(stdout, "role: %s\nkey ID: %s\nteam ID: %s\npublic key SHA-256: %s\nimported at: %s\n",
		info.Role, info.KeyID, info.TeamID, hex.EncodeToString(info.PublicKeySHA256[:]), info.CreatedAt.UTC().Format(time.RFC3339))
}

// readKeyFile reads a key file of at most maxKeyFile bytes.
func readKeyFile(path string) ([]byte, error) {
	f, err := os.Open(path) //nolint:gosec // the operator names the file
	if err != nil {
		return nil, fmt.Errorf("open the key file: %w", err)
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, maxKeyFile+1))
	if err != nil {
		return nil, fmt.Errorf("read the key file: %w", err)
	}
	if len(data) > maxKeyFile {
		clear(data)
		return nil, fmt.Errorf("the key file is larger than %d bytes", maxKeyFile)
	}
	return data, nil
}
