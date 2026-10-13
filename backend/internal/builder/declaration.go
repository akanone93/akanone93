// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"encoding/json"
	"io/fs"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema"
)

// DeclarationFile is the function document inside a source archive, beside
// the crate's Cargo.toml (plan P7 E5).
const DeclarationFile = "function.json"

// Declaration is the function document of an uploaded source, validated
// against schema/json/function.schema.json (FN-008, FN-012).
type Declaration struct {
	// Document is the canonical JSON (RFC 8785) of the migrated document; it
	// is what the server stores with the build (ADR-0063).
	Document []byte
	// Function is the decoded document.
	Function schema.FunctionDocument
}

// Placement returns the placement the document declares.
func (d *Declaration) Placement() Placement { return Placement(d.Function.Placement) }

// ReadDeclaration reads and validates function.json from the root of the
// source and checks that its name, and its placement when wantPlacement is not
// empty, are the ones the build was asked for (PLX-7205). Problems are
// returned as diagnostics located in function.json; a nil Declaration comes
// with at least one.
func ReadDeclaration(loader *schema.Loader, files fs.FS, wantName string, wantPlacement Placement) (*Declaration, plxerr.Diagnostics) {
	data, err := fs.ReadFile(files, DeclarationFile)
	if err != nil {
		return nil, plxerr.Diagnostics{plxerr.NewDiagnostic(plxerr.InvalidProjectLayout, plxerr.Location{File: DeclarationFile},
			"the source has no %s beside its Cargo.toml", DeclarationFile)}
	}
	src, diags := loader.ParseDocument(DeclarationFile, data, schema.KindFunction)
	if diags.HasErrors() || src == nil {
		return nil, diags
	}
	var doc schema.FunctionDocument
	if err := json.Unmarshal(src.Canonical, &doc); err != nil {
		return nil, append(diags, plxerr.NewDiagnostic(plxerr.InvalidStructure, plxerr.Location{File: DeclarationFile}, "decode: %v", err))
	}
	if mismatch := checkDeclared(doc, wantName, wantPlacement); mismatch != nil {
		return nil, append(diags, *mismatch)
	}
	return &Declaration{Document: src.Canonical, Function: doc}, diags
}

// checkDeclared compares the document with the build request (PLX-7205).
func checkDeclared(doc schema.FunctionDocument, name string, placement Placement) *plxerr.Diagnostic {
	switch {
	case doc.Name != name:
		d := plxerr.NewDiagnostic(plxerr.FunctionDeclarationMismatch, plxerr.Location{File: DeclarationFile, Path: "/name"},
			"the document declares the function %q; the build is for %q", doc.Name, name)
		return &d
	case placement != "" && Placement(doc.Placement) != placement:
		d := plxerr.NewDiagnostic(plxerr.FunctionDeclarationMismatch, plxerr.Location{File: DeclarationFile, Path: "/placement"},
			"the document declares placement %q; the build asked for %q", doc.Placement, placement)
		return &d
	}
	return nil
}

// Source returns the FunctionSource to build for the declaration over the
// crate files (FN-008).
func (d *Declaration) Source(files fs.FS) FunctionSource {
	return FunctionSource{Name: d.Function.Name, Placement: d.Placement(), Files: files}
}
