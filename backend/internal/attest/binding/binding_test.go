// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package binding

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

type vector struct {
	Name         string `json:"name"`
	ChallengeHex string `json:"challengeHex"`
	JKT          string `json:"jkt"`
	KAJKT        string `json:"kaJkt"`
	SCAJKT       string `json:"scaJkt"`
	HashHex      string `json:"hashHex"`
	RequestHash  string `json:"requestHash"`
}

// Verifies: SEC-004.
// The binding reproduces the vectors shared with the Dart runtime.
func TestSharedVectors(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../../../schema/testdata/attest/binding.json")
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Vectors []vector `json:"vectors"`
	}
	if err := json.Unmarshal(raw, &file); err != nil || len(file.Vectors) == 0 {
		t.Fatalf("vectors: %v", err)
	}
	for _, v := range file.Vectors {
		challenge, err := hex.DecodeString(v.ChallengeHex)
		if err != nil {
			t.Fatal(err)
		}
		got := Hash(challenge, v.JKT, v.KAJKT, v.SCAJKT)
		if hex.EncodeToString(got[:]) != v.HashHex {
			t.Errorf("%s: Hash = %x, want %s", v.Name, got, v.HashHex)
		}
		if rh := RequestHash(challenge, v.JKT, v.KAJKT, v.SCAJKT); rh != v.RequestHash {
			t.Errorf("%s: RequestHash = %q, want %q", v.Name, rh, v.RequestHash)
		}
	}
}

// Verifies: SEC-004.
// Changing the challenge or any one thumbprint changes the binding, and a
// thumbprint cannot move from one field to its neighbour.
func TestEveryInputIsCovered(t *testing.T) {
	t.Parallel()
	base := Hash([]byte("c"), "d", "k", "s")
	for name, other := range map[string][32]byte{
		"challenge":     Hash([]byte("c2"), "d", "k", "s"),
		"dpop":          Hash([]byte("c"), "d2", "k", "s"),
		"agreement key": Hash([]byte("c"), "d", "k2", "s"),
		"SCA key":       Hash([]byte("c"), "d", "k", "s2"),
		"no SCA key":    Hash([]byte("c"), "d", "k", ""),
		"shifted":       Hash([]byte("c"), "dk", "", "s"),
	} {
		if other == base {
			t.Errorf("changing the %s does not change the binding", name)
		}
	}
}

// Verifies: FN-020, SEC-004.
// The invocation binding reproduces the vectors shared with the Dart
// runtime, and every one of its inputs changes it.
func TestInvocationVectors_FN_020(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../../../schema/testdata/attest/invocation_binding.json")
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Vectors []struct {
			Name        string `json:"name"`
			Function    string `json:"function"`
			Version     uint32 `json:"version"`
			Input       string `json:"input"`
			JTI         string `json:"jti"`
			HashHex     string `json:"hashHex"`
			RequestHash string `json:"requestHash"`
		} `json:"vectors"`
	}
	if err := json.Unmarshal(raw, &file); err != nil || len(file.Vectors) == 0 {
		t.Fatalf("vectors: %v", err)
	}
	for _, v := range file.Vectors {
		got := InvocationHash(v.Function, v.Version, []byte(v.Input), v.JTI)
		if hex.EncodeToString(got[:]) != v.HashHex {
			t.Errorf("%s: InvocationHash = %x, want %s", v.Name, got, v.HashHex)
		}
		if rh := InvocationRequestHash(v.Function, v.Version, []byte(v.Input), v.JTI); rh != v.RequestHash {
			t.Errorf("%s: InvocationRequestHash = %q, want %q", v.Name, rh, v.RequestHash)
		}
	}
	base := InvocationHash("a.b", 1, []byte("x"), "j")
	for name, other := range map[string][32]byte{
		"function": InvocationHash("a.c", 1, []byte("x"), "j"),
		"version":  InvocationHash("a.b", 2, []byte("x"), "j"),
		"input":    InvocationHash("a.b", 1, []byte("y"), "j"),
		"jti":      InvocationHash("a.b", 1, []byte("x"), "k"),
	} {
		if other == base {
			t.Errorf("changing the %s did not change the binding", name)
		}
	}
}
