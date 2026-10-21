// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package experiment

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/nightCode42/plux3/backend/internal/controldoc"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/jcs"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

// valueChecker checks the values of one flag against its type and, for a
// json flag, its JSON Schema, and writes them in canonical form.
type valueChecker struct {
	typ      controldoc.FlagType
	schema   *jsonschema.Schema
	depth    int
	maxBytes int
}

// newValueChecker prepares the checker of a flag. A json flag needs a JSON
// Schema, which must compile and which may not refer to other documents: the
// loader of the compiler refuses every scheme, so a schema cannot make the
// server fetch anything.
func newValueChecker(typ controldoc.FlagType, rawSchema json.RawMessage, lim limits.Set) (valueChecker, error) {
	c := valueChecker{typ: typ, depth: int(lim.Get(limits.DocumentJSONDepth)), maxBytes: int(lim.Get(limits.DocumentStringPropSize))}
	if !slices.Contains(controldoc.FlagTypes(), typ) {
		return c, plxerr.New(plxerr.UnknownFlagType, "%q is not a flag type; use bool, number, string or json", typ)
	}
	hasSchema := len(bytes.TrimSpace(rawSchema)) > 0 && string(bytes.TrimSpace(rawSchema)) != "null"
	switch {
	case typ != controldoc.FlagJSON && hasSchema:
		return c, plxerr.New(plxerr.InvalidFormat, "only a json flag has a schema")
	case typ == controldoc.FlagJSON && !hasSchema:
		return c, plxerr.New(plxerr.MissingProperty, "a json flag needs the JSON Schema its values satisfy")
	case typ != controldoc.FlagJSON:
		return c, nil
	}
	compiled, err := compileSchema(rawSchema)
	if err != nil {
		return c, plxerr.New(plxerr.InvalidStructure, "the flag's JSON Schema is not valid: %v", err)
	}
	c.schema = compiled
	return c, nil
}

// compileSchema compiles a flag's JSON Schema without access to any other
// document.
func compileSchema(raw json.RawMessage) (*jsonschema.Schema, error) {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	c := jsonschema.NewCompiler()
	c.DefaultDraft(jsonschema.Draft2020)
	c.UseLoader(jsonschema.SchemeURLLoader{})
	const location = "flag-schema.json"
	if err := c.AddResource(location, doc); err != nil {
		return nil, fmt.Errorf("add: %w", err)
	}
	compiled, err := c.Compile(location)
	if err != nil {
		return nil, fmt.Errorf("compile: %w", err)
	}
	return compiled, nil
}

// canonical checks one value of the flag, described by what in the refusal,
// and returns it as canonical JSON text.
func (c valueChecker) canonical(raw json.RawMessage, what string) (string, error) {
	tree, err := jcs.Parse(raw, c.depth)
	if err != nil {
		return "", plxerr.New(plxerr.InvalidJSON, "%s is not valid JSON: %v", what, err)
	}
	if err := c.matchesType(tree, what); err != nil {
		return "", err
	}
	if c.schema != nil {
		if err := c.schema.Validate(tree); err != nil {
			return "", plxerr.New(plxerr.InvalidStructure, "%s does not satisfy the flag's schema: %v", what, err)
		}
	}
	text, err := jcs.Marshal(tree)
	if err != nil {
		return "", plxerr.New(plxerr.InvalidJSON, "%s cannot be written canonically: %v", what, err)
	}
	if len(text) > c.maxBytes {
		return "", plxerr.New(plxerr.LimitExceeded, "%s is %d bytes; at most %d (document.stringPropSize)", what, len(text), c.maxBytes)
	}
	return string(text), nil
}

// matchesType checks the JSON type of a value against the flag's type.
func (c valueChecker) matchesType(tree any, what string) error {
	ok := false
	switch c.typ {
	case controldoc.FlagBool:
		_, ok = tree.(bool)
	case controldoc.FlagNumber:
		_, ok = tree.(json.Number)
	case controldoc.FlagString:
		_, ok = tree.(string)
	case controldoc.FlagJSON:
		ok = tree != nil
	}
	if !ok {
		return plxerr.New(plxerr.WrongJSONType, "%s must be a %s value", what, c.typ)
	}
	return nil
}
