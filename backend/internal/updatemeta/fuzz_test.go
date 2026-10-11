// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package updatemeta_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/updatemeta"
)

// refusals are the errors a verifier may return; anything else escaping
// is a failure to classify malformed input.
var refusals = []error{
	updatemeta.ErrFormat, updatemeta.ErrThreshold, updatemeta.ErrExpired, updatemeta.ErrRollback,
	updatemeta.ErrMismatch, updatemeta.ErrDevelopmentKey, updatemeta.ErrAlgorithm,
}

func isRefusal(err error) bool {
	for _, r := range refusals {
		if errors.Is(err, r) {
			return true
		}
	}
	return false
}

// vectorDocuments returns the signed parts of the conformance vectors,
// valid and invalid, per role (schema/testdata/update/vectors.json).
func vectorDocuments(tb testing.TB) map[string][][]byte {
	tb.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "schema", "testdata", "update", "vectors.json"))
	if err != nil {
		tb.Fatal(err)
	}
	var file map[string]json.RawMessage
	if err := json.Unmarshal(raw, &file); err != nil {
		tb.Fatal(err)
	}
	out := map[string][][]byte{}
	for _, role := range []string{"root", "snapshot", "timestamp"} {
		var kinds map[string][]struct {
			Document json.RawMessage `json:"document"`
		}
		if err := json.Unmarshal(file[role], &kinds); err != nil {
			tb.Fatal(err)
		}
		for _, list := range kinds {
			for _, c := range list {
				out[role] = append(out[role], c.Document)
			}
		}
	}
	return out
}

// FuzzParse checks that parsing any bytes as a metadata file or as the
// signed part of a role never panics, and that every refusal is one of the
// package's errors.
//
// Verifies: QA-004, SEC-050, SEC-122.
func FuzzParse(f *testing.F) {
	docs := vectorDocuments(f)
	for _, list := range docs {
		for _, d := range list {
			f.Add(d)
			wrapped, err := json.Marshal(map[string]any{"signed": json.RawMessage(d), "signatures": []any{}})
			if err != nil {
				f.Fatal(err)
			}
			f.Add(wrapped)
		}
	}
	f.Add([]byte(`{"signed":{},"signatures":[{"keyid":"x","alg":"","sig":"="}]}`))
	f.Add([]byte(``))
	f.Fuzz(func(t *testing.T, data []byte) {
		check := func(name string, err error) {
			if err != nil && !isRefusal(err) {
				t.Fatalf("%s: the error is not a metadata refusal: %v", name, err)
			}
		}
		_, err := updatemeta.ParseDocument(data)
		check("document", err)
		_, err = updatemeta.ParseRoot(data)
		check("root", err)
		_, err = updatemeta.ParseSnapshot(data)
		check("snapshot", err)
		_, err = updatemeta.ParseTimestamp(data)
		check("timestamp", err)
		_, err = updatemeta.ParseTargets(data)
		check("targets", err)
		_, _, err = updatemeta.FirstRoot(data)
		check("first root", err)
	})
}

// FuzzChain checks that a verifier holding a trusted root never panics on
// any timestamp, snapshot, manifest or manifest signature, refuses with the
// package's errors, and accepts nothing whose versions differ from the
// chain it was seeded with.
//
// Verifies: QA-004, SEC-050.
func FuzzChain(f *testing.F) {
	on := newOnline(f)
	c := newChain(f, on, 4, 9)
	sigs, err := json.Marshal(c.sigs)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(c.timestamp, c.snapshot, c.manifest, sigs)
	f.Add(c.snapshot, c.timestamp, c.manifest, []byte(`[]`))
	f.Add([]byte(`{}`), []byte(`{}`), []byte(`{}`), []byte(`null`))
	v := updatemeta.Verifier{Root: c.root, Now: at.Add(time.Hour)}
	f.Fuzz(func(t *testing.T, timestamp, snapshot, manifest, signatures []byte) {
		var sigList []updatemeta.Signature
		if json.Unmarshal(signatures, &sigList) != nil {
			sigList = nil
		}
		floor, err := v.Chain(timestamp, snapshot, manifest, sigList)
		if err != nil {
			if !isRefusal(err) {
				t.Fatalf("the error is not a metadata refusal: %v", err)
			}
			return
		}
		if floor != (updatemeta.Floor{Timestamp: 9, Snapshot: 3, Targets: 4}) {
			t.Fatalf("accepted a chain of other versions: %+v", floor)
		}
	})
}
