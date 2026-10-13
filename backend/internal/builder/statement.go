// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"encoding/json"
	"fmt"

	"github.com/nightCode42/plux3/backend/internal/schema/jcs"
)

// statementDepth bounds the nesting the canonicaliser accepts for the statement.
const statementDepth = 16

// statement is the JSON the signature covers (ADR-0063): the module hash, the
// ABI version, the functions with the hashes of their schemas and the
// provenance (FN-005).
type statement struct {
	ABIVersion   int                 `json:"abiVersion"`
	Functions    []statementFunction `json:"functions"`
	ModuleSHA256 string              `json:"moduleSha256"`
	Provenance   statementProvenance `json:"provenance"`
}

// statementFunction is one function of the statement.
type statementFunction struct {
	Index              int    `json:"index"`
	InputSchemaSHA256  string `json:"inputSchemaSha256"`
	Name               string `json:"name"`
	OutputSchemaSHA256 string `json:"outputSchemaSha256"`
	Placement          string `json:"placement"`
}

// statementProvenance is the provenance of the statement.
type statementProvenance struct {
	BuildTime       string       `json:"buildTime"`
	BuilderIdentity string       `json:"builderIdentity"`
	Dependencies    []Dependency `json:"dependencies"`
	SDKSHA256       string       `json:"sdkSha256"`
	SourceSHA256    string       `json:"sourceSha256"`
	Toolchain       string       `json:"toolchain"`
	WasmOpt         string       `json:"wasmOpt"`
}

// Statement returns the artifact statement as canonical JSON (RFC 8785): the
// module SHA-256, the ABI version, the functions with the SHA-256 of their
// canonical schemas, and the provenance. `signing/` signs these bytes with the
// `function` key role; the builder holds no key (ADR-0063, L-4).
func (a *Artifact) Statement() ([]byte, error) {
	st := statement{
		ABIVersion:   a.ABIVersion,
		ModuleSHA256: a.ModuleSHA256,
		Functions:    make([]statementFunction, len(a.Functions)),
		Provenance: statementProvenance{
			BuildTime:       a.Provenance.BuildTime,
			BuilderIdentity: a.Provenance.BuilderIdentity,
			Dependencies:    a.Provenance.Dependencies,
			SDKSHA256:       a.Provenance.SDKSHA256,
			SourceSHA256:    a.Provenance.SourceSHA256,
			Toolchain:       a.Provenance.Toolchain,
			WasmOpt:         a.Provenance.WasmOpt,
		},
	}
	if st.Provenance.Dependencies == nil {
		st.Provenance.Dependencies = []Dependency{}
	}
	for i, f := range a.Functions {
		st.Functions[i] = statementFunction{
			Index:              f.Index,
			InputSchemaSHA256:  hexSum(f.InputSchema),
			Name:               f.Name,
			OutputSchemaSHA256: hexSum(f.OutputSchema),
			Placement:          string(f.Placement),
		}
	}
	raw, err := json.Marshal(st)
	if err != nil {
		return nil, fmt.Errorf("builder.Statement: %w", err)
	}
	canonical, err := jcs.Canonicalize(raw, statementDepth)
	if err != nil {
		return nil, fmt.Errorf("builder.Statement: %w", err)
	}
	return canonical, nil
}
