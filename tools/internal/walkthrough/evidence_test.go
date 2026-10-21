// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package walkthrough

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sampleStatement is a statement over the evidence of a store.
func sampleStatement(items []Item) Statement {
	return Statement{
		StartedAt: "2026-01-01T00:00:00Z", FinishedAt: "2026-01-01T00:10:00Z", Passed: true,
		Steps: []StepRecord{{ID: "01-a", Title: "a", Verifies: []string{"GOV-006"}, Status: StatusPassed, Evidence: items}},
	}
}

// Evidence files are written once, below plain names, and hashed.
func TestStoreAddsHashedFilesOnce(t *testing.T) {
	t.Parallel()
	s, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	item, err := s.Add("01-a", "one.txt", []byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	const helloSHA = "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if item.SHA256 != helloSHA || item.Size != 5 {
		t.Errorf("item = %+v", item)
	}
	got, err := os.ReadFile(filepath.Join(s.Dir(), "01-a", "one.txt"))
	if err != nil || string(got) != "hello" {
		t.Errorf("file = %q, %v", got, err)
	}
	if _, err := s.Add("01-a", "one.txt", []byte("again")); err == nil {
		t.Error("a name written twice must be refused: evidence is never overwritten")
	}
	for _, bad := range [][2]string{{"..", "x"}, {"a", "../x"}, {"a/b", "x"}, {"a", `b\c`}, {"", "x"}, {"a", ""}, {"a", "."}} {
		if _, err := s.Add(bad[0], bad[1], nil); err == nil {
			t.Errorf("Add(%q, %q) must be refused", bad[0], bad[1])
		}
	}
	if len(s.Items()) != 1 {
		t.Errorf("items = %v", s.Items())
	}
}

// A sealed statement verifies with the key it was signed with, and with no
// other: not a different key, not a swapped payload, not a changed type.
func TestSealAndVerify(t *testing.T) {
	t.Parallel()
	pub, priv, err := GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Seal(sampleStatement([]Item{}), priv)
	if err != nil {
		t.Fatal(err)
	}
	st, err := Verify(b, pub)
	if err != nil || st.Schema != StatementSchema || !st.Passed || b.KeyID != KeyID(pub) {
		t.Fatalf("Verify = %+v, %v", st, err)
	}

	otherPub, otherPriv, _ := GenerateKey(rand.Reader)
	forged, _ := Seal(sampleStatement(nil), otherPriv)
	tampered := b
	tampered.Payload = bytes.Replace(b.Payload, []byte(`"passed":true`), []byte(`"passed":false`), 1)
	retyped := b
	retyped.PayloadType = "text/plain"
	badSig := b
	badSig.Signature = append([]byte(nil), b.Signature...)
	badSig.Signature[0] ^= 0xff
	for name, tc := range map[string]struct {
		bundle  Bundle
		trusted ed25519.PublicKey
	}{
		"another key trusted":    {b, otherPub},
		"signed with another":    {forged, pub},
		"payload changed":        {tampered, pub},
		"payload type changed":   {retyped, pub},
		"signature changed":      {badSig, pub},
		"no trusted key":         {b, nil},
		"embedded key swapped":   {Bundle{PayloadType: b.PayloadType, Payload: forged.Payload, PublicKey: otherPub, Signature: forged.Signature}, pub},
		"signature over nothing": {Bundle{PayloadType: b.PayloadType, PublicKey: pub}, pub},
	} {
		if _, err := Verify(tc.bundle, tc.trusted); !errors.Is(err, ErrBadBundle) {
			t.Errorf("%s: err = %v, want ErrBadBundle", name, err)
		}
	}
}

// A signed statement that is not valid JSON, or of another schema, is refused
// after the signature is checked.
func TestVerifyRefusesAMalformedStatement(t *testing.T) {
	t.Parallel()
	pub, priv, _ := GenerateKey(rand.Reader)
	for name, payload := range map[string][]byte{"not JSON": []byte("{"), "wrong schema": []byte(`{"schema":"other/1"}`)} {
		b := Bundle{PayloadType: payloadType, Payload: payload, PublicKey: pub}
		b.Signature = ed25519.Sign(priv, pae(payloadType, payload))
		if _, err := Verify(b, pub); !errors.Is(err, ErrBadBundle) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

// Sealing the same statement with the same key gives the same bundle, so the
// evidence is reproducible from its inputs.
func TestSealIsDeterministic(t *testing.T) {
	t.Parallel()
	_, priv, _ := GenerateKey(rand.Reader)
	a, _ := Seal(sampleStatement(nil), priv)
	b, _ := Seal(sampleStatement(nil), priv)
	if !bytes.Equal(a.Payload, b.Payload) || !bytes.Equal(a.Signature, b.Signature) {
		t.Error("equal statements must seal equally")
	}
}

// A verified statement vouches for the files beside it: a changed, deleted or
// path-escaping entry is reported.
func TestCheckFiles(t *testing.T) {
	t.Parallel()
	s, _ := NewStore(t.TempDir())
	keep, _ := s.Add("01-a", "keep.txt", []byte("keep"))
	edit, _ := s.Add("01-a", "edit.txt", []byte("edit"))
	gone, _ := s.Add("01-a", "gone.txt", []byte("gone"))
	st := sampleStatement([]Item{keep, edit, gone})
	if err := CheckFiles(st, s.Dir()); err != nil {
		t.Fatalf("intact evidence: %v", err)
	}
	if err := os.WriteFile(filepath.Join(s.Dir(), "01-a", "edit.txt"), []byte("EDIT"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(s.Dir(), "01-a", "gone.txt")); err != nil {
		t.Fatal(err)
	}
	st.Steps[0].Evidence = append(st.Steps[0].Evidence, Item{Step: "01-a", Name: "../../etc/passwd", SHA256: "00"})
	err := CheckFiles(st, s.Dir())
	if err == nil {
		t.Fatal("changed evidence must be reported")
	}
	for _, want := range []string{"01-a/edit.txt", "01-a/gone.txt", "passwd"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %s", err, want)
		}
	}
	if strings.Contains(err.Error(), "keep.txt") {
		t.Errorf("error %q names an intact file", err)
	}
}
