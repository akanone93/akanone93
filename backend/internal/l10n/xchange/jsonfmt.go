// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package xchange

import (
	"bytes"
	"encoding/json"
	"sort"

	"github.com/nightCode42/plux3/backend/internal/schema/jcs"
)

// The JSON format is the Plux translations document itself (translations.
// schema.json): messages and states by key identifier. It carries no source
// text, so an import of it cannot detect a changed source.

// jsonMaxDepth bounds JSON nesting: document, messages, string.
const jsonMaxDepth = 8

// exportJSON writes the translations document of a locale. Properties are
// in key order and the output is indented, like the files of a project.
func exportJSON(target Doc) ([]byte, error) {
	doc := map[string]any{
		"schemaVersion": exportSchemaVersion,
		"kind":          "translations",
		"locale":        target.Locale,
		"messages":      nonNil(target.Messages),
	}
	if target.ID != "" {
		doc["id"] = target.ID
	}
	states := map[string]string{}
	for id, s := range target.States {
		if _, translated := target.Messages[id]; translated && s != "" {
			states[id] = s
		}
	}
	if len(states) > 0 {
		doc["states"] = states
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return nil, err //nolint:wrapcheck // a map of strings always encodes
	}
	return b.Bytes(), nil
}

func nonNil(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}

// parseJSON reads a translations document. Members other than locale,
// messages and states are not read.
func parseJSON(data []byte) (importFile, error) {
	tree, err := jcs.Parse(data, jsonMaxDepth)
	if err != nil {
		return importFile{}, invalid("the file is not valid JSON: %s", clip(err.Error()))
	}
	obj, ok := tree.(map[string]any)
	if !ok {
		return importFile{}, invalid("a translations document is a JSON object")
	}
	if kind, present := obj["kind"]; present && kind != "translations" {
		return importFile{}, invalid("the document is not a translations document")
	}
	var out importFile
	if v, present := obj["locale"]; present {
		s, isString := v.(string)
		if !isString {
			return importFile{}, invalid("locale is not a string")
		}
		out.locale = s
	}
	messages, ok := obj["messages"].(map[string]any)
	if !ok {
		return importFile{}, invalid("the document has no messages object")
	}
	var states map[string]any
	if v, present := obj["states"]; present {
		if states, ok = v.(map[string]any); !ok {
			return importFile{}, invalid("states is not an object")
		}
	}
	out.entries, err = jsonEntries(messages, states)
	if err != nil {
		return importFile{}, err
	}
	return out, nil
}

// jsonEntries reads the messages, in id order, with their review states.
func jsonEntries(messages, states map[string]any) ([]entry, error) {
	ids := make([]string, 0, len(messages))
	for id := range messages {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var entries []entry
	for _, id := range ids {
		text, isString := messages[id].(string)
		if !isString {
			return nil, invalid("the message %s is not a string", clip(id))
		}
		e := entry{id: id, target: text, hasTarget: true}
		if v, present := states[id]; present {
			s, isString := v.(string)
			if !isString || !validState(s) {
				return nil, invalid("the state of %s is not a review state", clip(id))
			}
			e.state = s
		}
		entries = append(entries, e)
	}
	return entries, nil
}
