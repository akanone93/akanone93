// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package builderrole

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/builder"
	"github.com/nightCode42/plux3/backend/internal/fnpb"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// linkSource is a link source for the archive.
func linkSource(name string, archive []byte) *fnpb.LinkSource {
	sum := sha256.Sum256(archive)
	return &fnpb.LinkSource{Name: name, Source: archive, SourceSha256: hex.EncodeToString(sum[:])}
}

// Verifies: FN-051.
func TestLinkRefusesAWrongRequest_FN_051(t *testing.T) {
	s := newService(t, builder.NoSandbox{}, false, nil, nil)
	good := archive(t, map[string]string{"Cargo.toml": "[package]"})
	wrongSum := linkSource("bank.a", good)
	wrongSum.SourceSha256 = strings.Repeat("0", 64)
	noName := linkSource("", good)
	empty := linkSource("bank.a", good)
	empty.Source = nil
	cases := map[string]*fnpb.LinkRequest{
		"no sources": {},
		"wrong hash": {Sources: []*fnpb.LinkSource{wrongSum}},
		"no name":    {Sources: []*fnpb.LinkSource{noName}},
		"no archive": {Sources: []*fnpb.LinkSource{empty}},
		"twice":      {Sources: []*fnpb.LinkSource{linkSource("bank.a", good), linkSource("bank.a", good)}},
	}
	for name, req := range cases {
		if _, err := s.Link(t.Context(), connect.NewRequest(req)); codeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("%s: error = %v", name, err)
		}
	}
}

// Verifies: FN-051, FN-008.
func TestLinkReportsSourcesThatCannotLinkAsDiagnostics_FN_051(t *testing.T) {
	s := newService(t, builder.NoSandbox{}, false, nil, nil)
	// The document declares the server placement, which a device module
	// refuses (PLX-7205).
	server := archive(t, map[string]string{
		"Cargo.toml": "[package]", "function.json": strings.Replace(declaration, "bank.transferFee", "bank.a", 1),
	})
	resp, err := s.Link(t.Context(), connect.NewRequest(&fnpb.LinkRequest{Sources: []*fnpb.LinkSource{linkSource("bank.a", server)}}))
	if err != nil {
		t.Fatal(err)
	}
	var ds plxerr.Diagnostics
	if err := json.Unmarshal(resp.Msg.GetDiagnosticsJson(), &ds); err != nil {
		t.Fatal(err)
	}
	if len(resp.Msg.GetModule()) != 0 || !ds.HasErrors() || ds[0].Code != plxerr.FunctionDeclarationMismatch {
		t.Errorf("module %d bytes, diagnostics %v; want %s", len(resp.Msg.GetModule()), ds, plxerr.FunctionDeclarationMismatch)
	}
}

// Verifies: FN-051.
func TestLinkResponseCarriesTheModuleTheIndexAndTheProvenance_FN_051(t *testing.T) {
	a := &builder.Artifact{
		Module: []byte("wasm"), ABIVersion: 1,
		Functions: []builder.Function{{Name: "bank.a", Index: 0}, {Name: "bank.b", Index: 1}},
		Provenance: builder.Provenance{
			SourceSHA256: "x", SDKSHA256: "sdk", Toolchain: "rustc 1.97.0", WasmOpt: "version_132", BuildTime: "2026-10-13T08:30:00Z",
			BuilderIdentity: "builder-1", Dependencies: []builder.Dependency{{Name: "serde", Version: "1.0.229", Checksum: "c1"}},
		},
	}
	r := linkResponseOf(a)
	fns := r.GetFunctions()
	if string(r.GetModule()) != "wasm" || r.GetAbiVersion() != 1 || len(fns) != 2 || fns[1].GetName() != "bank.b" || fns[1].GetIndex() != 1 ||
		r.GetProvenance().GetSourceSha256() != "x" || len(r.GetProvenance().GetDependencies()) != 1 {
		t.Errorf("response = %v", r)
	}
}
