// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	"encoding/json"
	"regexp"
	"slices"
	"strings"

	"github.com/nightCode42/plux3/backend/internal/pxl"
)

// fnKind is the kind of a function type: what the compiler can say about
// a JSON Schema derived from Rust types (FN-004).
type fnKind uint8

// Function type kinds. fnAny is what the compiler cannot map; a check
// against it always passes.
const (
	fnAny fnKind = iota
	fnBool
	fnInt
	fnDouble
	fnString
	fnEnum
	fnList
	fnMap
	fnObject
)

// fnType is a JSON Schema reduced to the types of the compiler's type
// system (SCH-010): object, array, string, number, integer, boolean, enum
// and nullable. Anything else is fnAny (ACT-001).
type fnType struct {
	kind     fnKind
	nullable bool
	elem     *fnType
	members  []string
	fields   map[string]*fnType
	required map[string]bool
	// closed is set when the schema forbids properties it does not list.
	closed bool
}

// jsonSchema is the part of a JSON Schema the mapping reads.
type jsonSchema struct {
	Ref        string                     `json:"$ref"`
	Type       json.RawMessage            `json:"type"`
	Enum       []json.RawMessage          `json:"enum"`
	Const      json.RawMessage            `json:"const"`
	Properties map[string]json.RawMessage `json:"properties"`
	Required   []string                   `json:"required"`
	Items      json.RawMessage            `json:"items"`
	Additional json.RawMessage            `json:"additionalProperties"`
	AnyOf      []json.RawMessage          `json:"anyOf"`
	OneOf      []json.RawMessage          `json:"oneOf"`
	Defs       map[string]json.RawMessage `json:"$defs"`
	Definition map[string]json.RawMessage `json:"definitions"`
}

// maxSchemaDepth bounds the mapping of recursive schemas.
const maxSchemaDepth = 16

// parseFnSchema maps a JSON Schema document to a function type. An empty
// or unreadable schema maps to fnAny.
func parseFnSchema(raw json.RawMessage) *fnType {
	if len(raw) == 0 {
		return &fnType{kind: fnAny}
	}
	var root jsonSchema
	if json.Unmarshal(raw, &root) != nil {
		return &fnType{kind: fnAny}
	}
	defs := map[string]json.RawMessage{}
	for k, v := range root.Definition {
		defs["#/definitions/"+k] = v
	}
	for k, v := range root.Defs {
		defs["#/$defs/"+k] = v
	}
	return mapSchema(raw, defs, 0)
}

// mapSchema maps one schema node.
func mapSchema(raw json.RawMessage, defs map[string]json.RawMessage, depth int) *fnType {
	unmapped := &fnType{kind: fnAny}
	var s jsonSchema
	if depth > maxSchemaDepth || json.Unmarshal(raw, &s) != nil {
		return unmapped
	}
	if s.Ref != "" {
		target, ok := defs[s.Ref]
		if !ok {
			return unmapped
		}
		return mapSchema(target, defs, depth+1)
	}
	if alts := append(slices.Clone(s.AnyOf), s.OneOf...); len(alts) > 0 {
		return mapAlternatives(alts, defs, depth)
	}
	types, nullable := schemaTypes(s.Type)
	t := mapByType(&s, types, defs, depth)
	if len(s.Enum) > 0 {
		t = mapEnum(s.Enum, t)
	}
	t.nullable = t.nullable || nullable
	return t
}

// mapAlternatives maps anyOf and oneOf: a schema and null is that schema
// made nullable; any other union is not mapped.
func mapAlternatives(alts []json.RawMessage, defs map[string]json.RawMessage, depth int) *fnType {
	var nonNull []json.RawMessage
	nullable := false
	for _, a := range alts {
		var s jsonSchema
		if json.Unmarshal(a, &s) == nil && string(s.Type) == `"null"` {
			nullable = true
			continue
		}
		nonNull = append(nonNull, a)
	}
	if len(nonNull) != 1 {
		return &fnType{kind: fnAny, nullable: nullable}
	}
	t := mapSchema(nonNull[0], defs, depth+1)
	t.nullable = t.nullable || nullable
	return t
}

// schemaTypes reads "type": a name or a list of names. nullable is set
// when "null" is among them; types are the others.
func schemaTypes(raw json.RawMessage) (types []string, nullable bool) {
	var one string
	var many []string
	switch {
	case json.Unmarshal(raw, &one) == nil && one != "":
		many = []string{one}
	case json.Unmarshal(raw, &many) != nil:
		return nil, false
	}
	for _, n := range many {
		if n == "null" {
			nullable = true
		} else {
			types = append(types, n)
		}
	}
	return types, nullable
}

// mapByType maps the structural keywords of a schema of one type.
func mapByType(s *jsonSchema, types []string, defs map[string]json.RawMessage, depth int) *fnType {
	if len(types) == 0 && len(s.Properties) > 0 {
		types = []string{"object"}
	}
	if len(types) != 1 {
		return &fnType{kind: fnAny}
	}
	switch types[0] {
	case "boolean":
		return &fnType{kind: fnBool}
	case "integer":
		return &fnType{kind: fnInt}
	case "number":
		return &fnType{kind: fnDouble}
	case "string":
		return &fnType{kind: fnString}
	case "array":
		if len(s.Items) == 0 {
			return &fnType{kind: fnList, elem: &fnType{kind: fnAny}}
		}
		return &fnType{kind: fnList, elem: mapSchema(s.Items, defs, depth+1)}
	case "object":
		return mapObject(s, defs, depth)
	}
	return &fnType{kind: fnAny}
}

// mapObject maps an object schema: named properties, or a map of one
// value type when only additionalProperties is given.
func mapObject(s *jsonSchema, defs map[string]json.RawMessage, depth int) *fnType {
	if len(s.Properties) == 0 && len(s.Additional) > 0 && string(s.Additional) != "true" && string(s.Additional) != "false" {
		return &fnType{kind: fnMap, elem: mapSchema(s.Additional, defs, depth+1)}
	}
	t := &fnType{kind: fnObject, fields: map[string]*fnType{}, required: map[string]bool{}, closed: string(s.Additional) == "false"}
	for name, raw := range s.Properties {
		t.fields[name] = mapSchema(raw, defs, depth+1)
	}
	for _, name := range s.Required {
		t.required[name] = true
	}
	return t
}

// mapEnum maps an enumeration of strings. A mixed or non-string
// enumeration keeps the type of the schema around it.
func mapEnum(values []json.RawMessage, around *fnType) *fnType {
	members := make([]string, 0, len(values))
	nullable := false
	for _, v := range values {
		var m string
		switch {
		case string(v) == "null":
			nullable = true
		case json.Unmarshal(v, &m) == nil:
			members = append(members, m)
		default:
			return around
		}
	}
	if len(members) == 0 {
		return around
	}
	return &fnType{kind: fnEnum, members: members, nullable: nullable}
}

// identPattern is the form of a field or enum member the compiler can
// name in a type.
var identPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*$`)

// pxlKeywords are the identifiers PXL reserves.
var pxlKeywords = map[string]bool{"true": true, "false": true, "null": true, "in": true}

// nameable reports whether s can be a field or enum member of a type.
func nameable(s string) bool { return identPattern.MatchString(s) && !pxlKeywords[s] }

// typeExpr writes the type as a type expression (SCH-010), declaring the
// object and enum types it needs in synth under names derived from name.
// It returns "" for a type the compiler cannot express.
func (t *fnType) typeExpr(name string, synth map[string]pxl.TypeSpec) string {
	base := ""
	switch t.kind {
	case fnBool:
		base = "bool"
	case fnInt:
		base = "int"
	case fnDouble:
		base = "double"
	case fnString:
		base = "string"
	case fnEnum:
		base = t.enumExpr(name, synth)
	case fnList:
		if e := t.elem.typeExpr(name+"Item", synth); e != "" {
			base = "list<" + e + ">"
		}
	case fnMap:
		if e := t.elem.typeExpr(name+"Value", synth); e != "" {
			base = "map<string," + e + ">"
		}
	case fnObject:
		base = t.objectExpr(name, synth)
	}
	if base != "" && t.nullable && !strings.HasSuffix(base, "?") {
		base += "?"
	}
	return base
}

// enumExpr declares an enumeration, or falls back to string when a member
// cannot be named.
func (t *fnType) enumExpr(name string, synth map[string]pxl.TypeSpec) string {
	if !slices.ContainsFunc(t.members, func(m string) bool { return !nameable(m) }) {
		synth[name] = pxl.TypeSpec{Enum: slices.Clone(t.members)}
		return name
	}
	return "string"
}

// objectExpr declares an object type of the fields that can be named.
func (t *fnType) objectExpr(name string, synth map[string]pxl.TypeSpec) string {
	var fields [][2]string
	for _, f := range sortedKeys(t.fields) {
		if !nameable(f) {
			continue
		}
		ft := t.fields[f]
		expr := ft.typeExpr(name+upperFirst(f), synth)
		if expr != "" && !t.required[f] && !strings.HasSuffix(expr, "?") {
			expr += "?"
		}
		fields = append(fields, [2]string{f, expr})
	}
	synth[name] = objectType(fields)
	return name
}

// String names the type for a diagnostic.
func (t *fnType) String() string {
	var s string
	switch t.kind {
	case fnBool:
		s = "bool"
	case fnInt:
		s = "int"
	case fnDouble:
		s = "number"
	case fnString:
		s = "string"
	case fnEnum:
		s = "one of " + strings.Join(t.members, ", ")
	case fnList:
		s = "list<" + t.elem.String() + ">"
	case fnMap:
		s = "map<string," + t.elem.String() + ">"
	case fnObject:
		s = "object"
	default:
		s = "any"
	}
	if t.nullable {
		s += "?"
	}
	return s
}

// accepts reports whether a value of the PXL type from can be passed
// where the function type t is expected. Kinds whose JSON form the
// compiler does not model (decimal, money, dates, colours) are accepted.
func (t *fnType) accepts(from *pxl.Type) bool {
	if t.kind == fnAny {
		return true
	}
	switch {
	case from.Kind() == pxl.KindNull:
		return t.nullable
	case from.Kind() == pxl.KindNever:
		return true
	case from.Nullable() && !t.nullable:
		return false
	}
	switch from.Kind() {
	case pxl.KindBool:
		return t.kind == fnBool
	case pxl.KindInt:
		return t.kind == fnInt || t.kind == fnDouble
	case pxl.KindDouble:
		return t.kind == fnDouble
	case pxl.KindString:
		return t.kind == fnString
	case pxl.KindEnum:
		return t.acceptsEnum(from)
	case pxl.KindList:
		return t.kind == fnList && t.elem.accepts(from.Elem())
	case pxl.KindMap:
		return t.kind == fnMap && t.elem.accepts(from.Elem()) || t.kind == fnObject && len(t.fields) == 0 && !t.closed
	case pxl.KindObject:
		return t.acceptsObject(from)
	}
	return true
}

// acceptsEnum checks an enum value: its members must all be accepted.
func (t *fnType) acceptsEnum(from *pxl.Type) bool {
	switch t.kind {
	case fnString:
		return true
	case fnEnum:
		return from.Named() == nil || !slices.ContainsFunc(from.Named().Members, func(m string) bool { return !slices.Contains(t.members, m) })
	}
	return false
}

// acceptsObject checks an object value against an object or map type.
func (t *fnType) acceptsObject(from *pxl.Type) bool {
	n := from.Named()
	if n == nil {
		return true
	}
	switch t.kind {
	case fnMap:
		for _, f := range sortedKeys(n.Fields) {
			if !t.elem.accepts(n.Fields[f]) {
				return false
			}
		}
		return true
	case fnObject:
		for _, f := range sortedKeys(t.required) {
			if _, ok := n.Fields[f]; !ok {
				return false
			}
		}
		for _, f := range sortedKeys(n.Fields) {
			ft, known := t.fields[f]
			switch {
			case !known:
				if t.closed {
					return false
				}
			case !ft.accepts(n.Fields[f]):
				return false
			}
		}
		return true
	}
	return false
}
