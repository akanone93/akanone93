// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package importer

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// Import kinds Studio's data source designer offers (L-12).
const (
	KindOpenAPI = "openapi"
	KindGraphQL = "graphql"
)

// ImportJSON runs one import for Studio's data source designer, which calls
// it in the validator WebAssembly module (L-12) on text the user provided; it
// never reads the network. For KindOpenAPI the first source is the document;
// for KindGraphQL the first is the schema and the others are operation
// documents. The answer is JSON: {"fragment": {...}, "diagnostics": [...]}.
func ImportJSON(kind string, sources []Source) ([]byte, error) {
	if len(sources) == 0 {
		return nil, fmt.Errorf("importer.ImportJSON: no source")
	}
	var r Result
	switch kind {
	case KindOpenAPI:
		r = ImportOpenAPI(context.Background(), sources[0].File, sources[0].Data)
	case KindGraphQL:
		r = ImportGraphQL(sources[0], sources[1:])
	default:
		return nil, fmt.Errorf("importer.ImportJSON: unknown kind %q", kind)
	}
	diags := r.Diagnostics
	if diags == nil {
		diags = plxerr.Diagnostics{}
	}
	frag := r.Fragment
	if frag == nil {
		frag = map[string]any{}
	}
	out, err := json.Marshal(map[string]any{"fragment": frag, "diagnostics": diags})
	if err != nil {
		return nil, fmt.Errorf("importer.ImportJSON: %w", err)
	}
	return out, nil
}
