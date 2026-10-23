// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package airgap

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/nightCode42/plux3/backend/edition/licensefile"
)

// WriteKeyPair writes a new signing key pair: the private key, base64-encoded,
// to path (mode 0600, which must not exist) and the public key to path+".pub".
// A private key written by plux-license keygen is read the same way.
func WriteKeyPair(path string, pub ed25519.PublicKey, priv ed25519.PrivateKey) error {
	if err := writeNew(path, 0o600, base64.StdEncoding.EncodeToString(priv)+"\n"); err != nil {
		return err
	}
	return writeNew(path+".pub", 0o644, base64.StdEncoding.EncodeToString(pub)+"\n")
}

// writeNew creates a file that must not exist yet.
func writeNew(path string, mode os.FileMode, content string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode) //nolint:gosec // G304: the operator names the key file.
	if err != nil {
		return fmt.Errorf("airgap.WriteKeyPair: create %s: %w", path, err)
	}
	_, werr := f.WriteString(content)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		return fmt.Errorf("airgap.WriteKeyPair: write %s: %w", path, werr)
	}
	return nil
}

// ReadPrivateKey reads a private key file written by WriteKeyPair (or by
// plux-license keygen).
func ReadPrivateKey(path string) (ed25519.PrivateKey, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // G304: the operator names the key file.
	if err != nil {
		return nil, fmt.Errorf("airgap.ReadPrivateKey: %w", err)
	}
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(b) != ed25519.PrivateKeySize {
		return nil, errors.New("airgap.ReadPrivateKey: the file is not an Ed25519 private key in base64")
	}
	return ed25519.PrivateKey(b), nil
}

// ReadPublicKey reads a file holding one base64 public key.
func ReadPublicKey(path string) (ed25519.PublicKey, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // G304: the operator names the key file.
	if err != nil {
		return nil, fmt.Errorf("airgap.ReadPublicKey: %w", err)
	}
	pub, err := licensefile.ParsePublicKey(strings.TrimSpace(string(raw)))
	if err != nil {
		return nil, fmt.Errorf("airgap.ReadPublicKey: %w", err)
	}
	return pub, nil
}
