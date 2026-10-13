// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"bytes"
	"testing"
)

// goldenArtifact is a fixed artefact for the statement tests.
func goldenArtifact() *Artifact {
	module := []byte("\x00asm\x01\x00\x00\x00")
	return &Artifact{
		Module:       module,
		ModuleSHA256: hexSum(module),
		ABIVersion:   1,
		Functions: []Function{
			{
				Name: "fee.compute", Index: 0, Placement: PlacementDevice,
				InputSchema: []byte(`{"type":"object"}`), OutputSchema: []byte(`{"type":"string"}`),
			},
			{
				Name: "loan.schedule", Index: 1, Placement: PlacementServer,
				InputSchema: []byte(`{"a":1}`), OutputSchema: []byte(`{"b":2}`),
			},
		},
		Provenance: Provenance{
			SourceSHA256: "11" + string(bytes.Repeat([]byte("a"), 62)),
			SDKSHA256:    "22" + string(bytes.Repeat([]byte("b"), 62)),
			Dependencies: []Dependency{
				{Name: "itoa", Version: "1.0.18", Checksum: "c1"},
				{Name: "serde", Version: "1.0.229", Checksum: "c2"},
			},
			Toolchain:       "rustc 1.97.0 (2d8144b78 2026-07-07)",
			WasmOpt:         "version_132",
			BuildTime:       "2026-10-13T08:30:00Z",
			BuilderIdentity: "builder-1",
		},
	}
}

// Verifies: FN-005.
func TestStatementIsCanonicalJSON_FN_005(t *testing.T) {
	got, err := goldenArtifact().Statement()
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "statement.json", append(got[:len(got):len(got)], '\n'))
	again, err := goldenArtifact().Statement()
	if err != nil || !bytes.Equal(got, again) {
		t.Error("the statement is not deterministic")
	}
	if bytes.ContainsAny(got, "\n\t") || bytes.Contains(got, []byte(`": `)) {
		t.Errorf("canonical JSON contains insignificant white space: %s", got)
	}
}

// Verifies: FN-005.
func TestStatementCoversTheSchemasAndTheModule_FN_005(t *testing.T) {
	base, _ := goldenArtifact().Statement()
	a := goldenArtifact()
	a.Functions[1].OutputSchema = []byte(`{"b":3}`)
	changed, _ := a.Statement()
	if bytes.Equal(base, changed) {
		t.Error("changing a schema did not change the statement")
	}
	a = goldenArtifact()
	a.ModuleSHA256 = hexSum([]byte("other"))
	changed, _ = a.Statement()
	if bytes.Equal(base, changed) {
		t.Error("changing the module did not change the statement")
	}
	a = goldenArtifact()
	a.Provenance.Dependencies = nil
	if s, err := a.Statement(); err != nil || !bytes.Contains(s, []byte(`"dependencies":[]`)) {
		t.Errorf("empty dependencies must be an array: %s, %v", s, err)
	}
}
