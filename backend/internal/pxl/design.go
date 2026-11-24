// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package pxl

import (
	"encoding/json"
	"math"

	"github.com/nightCode42/plux3/backend/internal/pxl/decimal"
)

// designMoneyCurrency is the currency of a money value a design-time root
// does not give: any ISO 4217 code would do, as the value is a stand-in.
const designMoneyCurrency = "EUR"

// DesignFromJSON converts a design-time value into a value of type t: what
// Studio's canvas holds for a root, such as a page's state defaults or a
// data source's mock, which may lack fields or hold a value of another
// shape. What fits is kept; what is absent or does not fit reads as the
// type's empty value, or null where t is nullable. It never fails. Only the
// canvas preview uses it (STU-005, ADR-0112); the runtime and the compiler
// take values with FromJSON, which refuses what does not fit.
func DesignFromJSON(t *Type, v any) Value {
	if v == nil {
		return EmptyValue(t)
	}
	switch t.kind {
	case KindList:
		return designList(t, v)
	case KindMap:
		return designMap(t, v)
	case KindObject:
		return designObject(t, v)
	case KindInt:
		if i, ok := designInt(v); ok {
			return i
		}
	}
	if x, err := FromJSON(t, v); err == nil {
		return x
	}
	return EmptyValue(t)
}

// designList reads a JSON array as a list, each element leniently.
func designList(t *Type, v any) Value {
	arr, ok := v.([]any)
	if !ok {
		return EmptyValue(t)
	}
	out := make(List, len(arr))
	for i, e := range arr {
		out[i] = DesignFromJSON(t.elem, e)
	}
	return out
}

// designMap reads a JSON object as a map, each value leniently.
func designMap(t *Type, v any) Value {
	obj, ok := v.(map[string]any)
	if !ok {
		return EmptyValue(t)
	}
	out := make(Map, len(obj))
	for k, e := range obj {
		out[k] = DesignFromJSON(t.elem, e)
	}
	return out
}

// designObject reads a JSON object as an object: its declared fields, each
// leniently; fields it does not declare are left out.
func designObject(t *Type, v any) Value {
	obj, ok := v.(map[string]any)
	if !ok {
		return EmptyValue(t)
	}
	out := make(Map, len(t.named.FieldOrder))
	for _, f := range t.named.FieldOrder {
		out[f] = DesignFromJSON(t.named.Fields[f], obj[f])
	}
	return out
}

// designInt reads a whole number written as a double (3.0) as an int; any
// other value is left to FromJSON.
func designInt(v any) (Value, bool) {
	n, ok := v.(json.Number)
	if !ok {
		return nil, false
	}
	f, err := n.Float64()
	if err != nil || f != math.Trunc(f) || math.Abs(f) >= 1<<53 {
		return nil, false
	}
	return int64(f), true
}

// emptyDepth bounds the nesting of an empty object: a type that holds
// itself, which no value can satisfy, ends in null there.
const emptyDepth = 32

// EmptyValue is the value a design-time root reads where it gives none: null
// for a nullable type, else false, zero, the empty string, list or map, the
// first member of an enum, 1970-01-01, or an object of empty fields.
func EmptyValue(t *Type) Value { return emptyAt(t, 0) }

// emptyAt is EmptyValue at a depth of nested objects.
func emptyAt(t *Type, depth int) Value {
	if t.nullable || depth > emptyDepth {
		return nil
	}
	switch t.kind {
	case KindBool:
		return false
	case KindInt:
		return int64(0)
	case KindDouble:
		return float64(0)
	case KindString, KindAsset, KindRoute:
		return ""
	case KindDecimal:
		return decimal.FromInt64(0)
	case KindMoney:
		return Money{Amount: decimal.FromInt64(0), Currency: designMoneyCurrency}
	case KindDate:
		return Date(0)
	case KindDateTime:
		return DateTime{}
	case KindDuration:
		return Duration(0)
	case KindColor:
		return Color(0)
	case KindEnum:
		if len(t.named.Members) > 0 {
			return t.named.Members[0]
		}
	case KindList:
		return List{}
	case KindMap:
		return Map{}
	case KindObject:
		out := make(Map, len(t.named.FieldOrder))
		for _, f := range t.named.FieldOrder {
			out[f] = emptyAt(t.named.Fields[f], depth+1)
		}
		return out
	}
	return nil
}
