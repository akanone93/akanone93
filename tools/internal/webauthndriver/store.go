// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package webauthndriver

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
)

// ErrNoCredential is returned by [FileStore.Load] for a person who has not
// enrolled a security key.
var ErrNoCredential = errors.New("webauthndriver: no security key is enrolled for this person")

// actorName matches the names a store accepts; it keeps a name from reaching
// outside the store's directory.
var actorName = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// FileStore keeps the credentials of a run in a directory only the current
// user can read, one file per person. The two commands of a run (the token and
// the step-up) are separate processes, and the server refuses a signature
// counter that goes backwards, so the credential must outlive the process.
type FileStore struct {
	// Dir is the directory; it is created with mode 0700.
	Dir string
}

// storedCredential is the file form of a [Credential].
type storedCredential struct {
	ID      []byte `json:"id"`
	Key     []byte `json:"key"` // PKCS #8 DER
	Counter uint32 `json:"counter"`
}

// Load reads the credential of a person, or returns [ErrNoCredential].
func (s FileStore) Load(actor string) (Credential, error) {
	path, err := s.path(actor)
	if err != nil {
		return Credential{}, err
	}
	data, err := os.ReadFile(path) //nolint:gosec // a name checked by path inside the store's directory
	if errors.Is(err, fs.ErrNotExist) {
		return Credential{}, ErrNoCredential
	}
	if err != nil {
		return Credential{}, fmt.Errorf("webauthndriver.FileStore.Load: %w", err)
	}
	var sc storedCredential
	if err := json.Unmarshal(data, &sc); err != nil {
		return Credential{}, fmt.Errorf("webauthndriver.FileStore.Load: the credential file of %s is damaged", actor)
	}
	parsed, err := x509.ParsePKCS8PrivateKey(sc.Key)
	key, ok := parsed.(*ecdsa.PrivateKey)
	if err != nil || !ok || len(sc.ID) == 0 {
		return Credential{}, fmt.Errorf("webauthndriver.FileStore.Load: the credential file of %s is damaged", actor)
	}
	return Credential{ID: sc.ID, Key: key, Counter: sc.Counter}, nil
}

// Save writes the credential of a person, replacing the file atomically so
// that a crash leaves the previous counter rather than a torn file.
func (s FileStore) Save(actor string, c Credential) error {
	path, err := s.path(actor)
	if err != nil {
		return err
	}
	der, err := x509.MarshalPKCS8PrivateKey(c.Key)
	if err != nil {
		return fmt.Errorf("webauthndriver.FileStore.Save: %w", err)
	}
	data, err := json.Marshal(storedCredential{ID: c.ID, Key: der, Counter: c.Counter})
	if err != nil {
		return fmt.Errorf("webauthndriver.FileStore.Save: %w", err)
	}
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return fmt.Errorf("webauthndriver.FileStore.Save: %w", err)
	}
	tmp, err := os.CreateTemp(s.Dir, ".credential-*") // created with mode 0600
	if err != nil {
		return fmt.Errorf("webauthndriver.FileStore.Save: %w", err)
	}
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if err := errors.Join(werr, cerr); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("webauthndriver.FileStore.Save: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("webauthndriver.FileStore.Save: %w", err)
	}
	return nil
}

// path returns the file of a person, refusing a name that is not a plain one.
func (s FileStore) path(actor string) (string, error) {
	if s.Dir == "" {
		return "", errors.New("webauthndriver.FileStore: no directory is set")
	}
	if !actorName.MatchString(actor) {
		return "", fmt.Errorf("webauthndriver.FileStore: %q is not a plain person name", actor)
	}
	return filepath.Join(s.Dir, actor+".json"), nil
}
