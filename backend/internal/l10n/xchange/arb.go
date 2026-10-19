// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package xchange

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"

	"github.com/nightCode42/plux3/backend/internal/schema/jcs"
)

// ARB (Application Resource Bundle): "@@locale" names the locale, each key
// maps to its text, and "@<key>" holds its metadata. Plux adds x-plux-id (the
// key identifier), x-plux-state (the review state) and x-plux-source (the
// source text the translation was made from) to the standard description
// and placeholders, so that an export round-trips.

// arbMaxDepth bounds JSON nesting: a file is an object of objects of
// objects (placeholders).
const arbMaxDepth = 8

// exportARB writes an ARB file. Keys without a translation carry an empty
// text, as the Flutter tooling expects every key in every file.
func exportARB(keys []Key, source, target Doc) ([]byte, error) {
	var w orderedWriter
	w.field("@@locale", target.Locale)
	for _, k := range keys {
		w.field(k.Key, target.Messages[k.ID])
		meta := map[string]any{
			"x-plux-id":     k.ID,
			"x-plux-state":  target.State(k.ID),
			"x-plux-source": source.Messages[k.ID],
		}
		if k.Description != "" {
			meta["description"] = k.Description
		}
		if k.MaxLength > 0 {
			meta["x-plux-maxLength"] = k.MaxLength
		}
		if len(k.Args) > 0 {
			ph := map[string]any{}
			for _, a := range k.Args {
				ph[a.Name] = map[string]any{"type": arbType(a.Type)}
			}
			meta["placeholders"] = ph
		}
		w.field("@"+k.Key, meta)
	}
	return w.bytes()
}

// arbType maps a Plux type to an ARB placeholder type.
func arbType(t string) string {
	switch strings.TrimSuffix(t, "?") {
	case "string":
		return "String"
	case "int":
		return "int"
	case "double", "decimal":
		return "double"
	case "datetime", "date":
		return "DateTime"
	}
	return "Object"
}

// orderedWriter writes a JSON object whose members keep the order they were
// added in; encoding/json would sort them, and ARB tools expect a key before
// its metadata.
type orderedWriter struct {
	b   bytes.Buffer
	n   int
	err error
}

func (w *orderedWriter) field(name string, v any) {
	if w.err != nil {
		return
	}
	if w.n == 0 {
		w.b.WriteString("{\n")
	} else {
		w.b.WriteString(",\n")
	}
	w.n++
	w.b.WriteString("  ")
	w.value(name, 2)
	w.b.WriteString(": ")
	w.value(v, 2)
}

// value writes a JSON value with the indentation of its nesting.
func (w *orderedWriter) value(v any, indent int) {
	var raw bytes.Buffer
	enc := json.NewEncoder(&raw)
	enc.SetEscapeHTML(false)
	enc.SetIndent(strings.Repeat(" ", indent), "  ")
	if err := enc.Encode(v); err != nil {
		w.err = err
		return
	}
	w.b.Write(bytes.TrimSuffix(raw.Bytes(), []byte("\n")))
}

func (w *orderedWriter) bytes() ([]byte, error) {
	if w.err != nil {
		return nil, w.err
	}
	if w.n == 0 {
		return []byte("{}\n"), nil
	}
	w.b.WriteString("\n}\n")
	return w.b.Bytes(), nil
}

// parseARB reads an ARB file.
func parseARB(data []byte) (importFile, error) {
	tree, err := jcs.Parse(data, arbMaxDepth)
	if err != nil {
		return importFile{}, invalid("the file is not valid JSON: %s", clip(err.Error()))
	}
	obj, ok := tree.(map[string]any)
	if !ok {
		return importFile{}, invalid("an ARB file is a JSON object")
	}
	var out importFile
	if v, present := obj["@@locale"]; present {
		s, isString := v.(string)
		if !isString {
			return importFile{}, invalid("@@locale is not a string")
		}
		out.locale = s
	}
	names := make([]string, 0, len(obj))
	for name := range obj {
		if !strings.HasPrefix(name, "@") {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		text, isString := obj[name].(string)
		if !isString {
			return importFile{}, invalid("the message %s is not a string", clip(name))
		}
		e := entry{key: name, target: text, hasTarget: true}
		if raw, present := obj["@"+name]; present {
			meta, isObject := raw.(map[string]any)
			if !isObject {
				return importFile{}, invalid("the metadata of %s is not an object", clip(name))
			}
			if err := arbMeta(&e, meta); err != nil {
				return importFile{}, err
			}
		}
		out.entries = append(out.entries, e)
	}
	return out, nil
}

// arbMeta reads the Plux members of a key's metadata.
func arbMeta(e *entry, meta map[string]any) error {
	str := func(member string) (string, bool, error) {
		v, present := meta[member]
		if !present {
			return "", false, nil
		}
		s, ok := v.(string)
		if !ok {
			return "", false, invalid("%s of %s is not a string", member, clip(e.key))
		}
		return s, true, nil
	}
	var err error
	if e.id, _, err = str("x-plux-id"); err != nil {
		return err
	}
	if e.source, e.hasSource, err = str("x-plux-source"); err != nil {
		return err
	}
	state, present, err := str("x-plux-state")
	if err != nil {
		return err
	}
	if present && state != "" {
		if !validState(state) {
			return invalid("x-plux-state of %s is %q, not a review state", clip(e.key), clip(state))
		}
		e.state = state
	}
	return nil
}
