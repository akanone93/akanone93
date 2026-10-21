// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package walkthrough

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// StatementSchema names the version of the signed statement.
const StatementSchema = "plux.walkthrough.evidence/1"

// payloadType is the DSSE payload type of the signed statement.
const payloadType = "application/vnd.plux.walkthrough-evidence+json"

// Item is one evidence file: which step wrote it, its name and its hash.
type Item struct {
	Step   string `json:"step"`
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Size   int    `json:"size"`
}

// Store writes the evidence files of a run below one directory, one
// sub-directory per step, and remembers what it wrote. It is safe for
// concurrent use.
type Store struct {
	dir string

	mu    sync.Mutex
	items []Item
}

// NewStore returns a store that writes below dir, which it creates.
func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("walkthrough.NewStore: %w", err)
	}
	return &Store{dir: dir}, nil
}

// Dir returns the directory the store writes below.
func (s *Store) Dir() string { return s.dir }

// Add writes data as the evidence file name of a step. The step and the name
// must be plain names; a name written twice is an error, because evidence is
// never overwritten.
func (s *Store) Add(step, name string, data []byte) (Item, error) {
	if !plainName(step) || !plainName(name) {
		return Item{}, fmt.Errorf("walkthrough.Store.Add: %q/%q is not a plain file name", step, name)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	dir := filepath.Join(s.dir, step)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return Item{}, fmt.Errorf("walkthrough.Store.Add: %w", err)
	}
	file, err := os.OpenFile(filepath.Join(dir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640) //nolint:gosec // the path is two plain names below the store's directory
	if err != nil {
		return Item{}, fmt.Errorf("walkthrough.Store.Add: %w", err)
	}
	_, werr := file.Write(data)
	if cerr := file.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		return Item{}, fmt.Errorf("walkthrough.Store.Add: %w", werr)
	}
	sum := sha256.Sum256(data)
	item := Item{Step: step, Name: name, SHA256: hex.EncodeToString(sum[:]), Size: len(data)}
	s.items = append(s.items, item)
	return item, nil
}

// Items returns the evidence written so far, in the order it was written.
func (s *Store) Items() []Item {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Item(nil), s.items...)
}

// plainName reports whether s is a non-empty file name with no separator and
// not one of the two directory names.
func plainName(s string) bool {
	return s != "" && s != "." && s != ".." && !strings.ContainsAny(s, "/\\\x00")
}

// Status is the outcome of a step.
type Status string

// The outcomes of a step.
const (
	StatusPassed  Status = "passed"
	StatusFailed  Status = "failed"
	StatusSkipped Status = "skipped"
)

// StepRecord is what the statement says about one step.
type StepRecord struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Verifies []string `json:"verifies"`
	Status   Status   `json:"status"`
	Error    string   `json:"error,omitempty"`
	Evidence []Item   `json:"evidence"`
}

// Statement is the signed claim: which steps ran, how they ended and the
// hash of every evidence file and of the transcript. It holds times in
// RFC 3339, hashes and counts, never a secret.
type Statement struct {
	Schema           string       `json:"schema"`
	StartedAt        string       `json:"startedAt"`
	FinishedAt       string       `json:"finishedAt"`
	Passed           bool         `json:"passed"`
	TranscriptSHA256 string       `json:"transcriptSha256"`
	Steps            []StepRecord `json:"steps"`
}

// Bundle is the DSSE-style envelope of a signed statement. The payload is
// the statement's JSON; the signature covers its pre-authentication encoding.
type Bundle struct {
	PayloadType string `json:"payloadType"`
	Payload     []byte `json:"payload"`
	KeyID       string `json:"keyId"`
	PublicKey   []byte `json:"publicKey"`
	Signature   []byte `json:"signature"`
}

// GenerateKey makes the signing key of one run from r (crypto/rand.Reader in
// production). The private half is never written anywhere.
func GenerateKey(r io.Reader) (ed25519.PublicKey, ed25519.PrivateKey, error) {
	pub, priv, err := ed25519.GenerateKey(r)
	if err != nil {
		return nil, nil, fmt.Errorf("walkthrough.GenerateKey: %w", err)
	}
	return pub, priv, nil
}

// KeyID returns the identifier of a public key: the first eight bytes of its
// SHA-256, in hexadecimal.
func KeyID(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	return hex.EncodeToString(sum[:8])
}

// pae is the DSSE pre-authentication encoding of a payload, which binds the
// payload type to the signature.
func pae(typ string, payload []byte) []byte {
	return fmt.Appendf(nil, "DSSEv1 %d %s %d %s", len(typ), typ, len(payload), payload)
}

// Seal signs a statement with the run's key. Steps and their evidence keep
// the order the statement gives them, so equal statements sign equally.
func Seal(st Statement, priv ed25519.PrivateKey) (Bundle, error) {
	st.Schema = StatementSchema
	payload, err := json.Marshal(st)
	if err != nil {
		return Bundle{}, fmt.Errorf("walkthrough.Seal: %w", err)
	}
	pub, ok := priv.Public().(ed25519.PublicKey)
	if !ok {
		return Bundle{}, errors.New("walkthrough.Seal: the key is not an Ed25519 key")
	}
	return Bundle{
		PayloadType: payloadType, Payload: payload, KeyID: KeyID(pub), PublicKey: pub,
		Signature: ed25519.Sign(priv, pae(payloadType, payload)),
	}, nil
}

// ErrBadBundle is returned when a bundle's signature, key or payload type is
// wrong; nothing of its payload has been read then.
var ErrBadBundle = errors.New("walkthrough: the evidence bundle is not valid")

// Verify checks a bundle against the public key the verifier trusts and only
// then parses the statement. The embedded key is never trusted by itself: a
// bundle signed with another key is refused.
func Verify(b Bundle, trusted ed25519.PublicKey) (Statement, error) {
	if b.PayloadType != payloadType || len(trusted) != ed25519.PublicKeySize || !ed25519.PublicKey(b.PublicKey).Equal(trusted) {
		return Statement{}, fmt.Errorf("%w: wrong payload type or key", ErrBadBundle)
	}
	if !ed25519.Verify(trusted, pae(b.PayloadType, b.Payload), b.Signature) {
		return Statement{}, fmt.Errorf("%w: the signature does not match", ErrBadBundle)
	}
	var st Statement
	if err := json.Unmarshal(b.Payload, &st); err != nil {
		return Statement{}, fmt.Errorf("%w: %w", ErrBadBundle, err)
	}
	if st.Schema != StatementSchema {
		return Statement{}, fmt.Errorf("%w: schema %q", ErrBadBundle, st.Schema)
	}
	return st, nil
}

// CheckFiles re-hashes every evidence file the statement lists below dir and
// reports each that is missing or changed, so that a verified statement also
// vouches for the files beside it.
func CheckFiles(st Statement, dir string) error {
	var bad []string
	for _, step := range st.Steps {
		for _, it := range step.Evidence {
			if !intact(dir, it) {
				bad = append(bad, it.Step+"/"+it.Name)
			}
		}
	}
	if len(bad) > 0 {
		sort.Strings(bad)
		return fmt.Errorf("walkthrough.CheckFiles: missing or changed: %s", strings.Join(bad, ", "))
	}
	return nil
}

// intact reports whether the evidence file of an item exists below dir with
// the hash the statement records.
func intact(dir string, it Item) bool {
	if !plainName(it.Step) || !plainName(it.Name) {
		return false
	}
	data, err := os.ReadFile(filepath.Join(dir, it.Step, it.Name)) //nolint:gosec // both parts were checked as plain names
	if err != nil {
		return false
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]) == it.SHA256
}
