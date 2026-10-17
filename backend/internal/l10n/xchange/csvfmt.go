// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package xchange

import (
	"bytes"
	"encoding/csv"
	"errors"
	"io"
	"strconv"
	"strings"
)

// CSV (RFC 4180) with a header row and the columns keyId, key, source,
// target, state, description and maxLength. On import the columns are found
// by name, in any order; keyId or key, and target, are required.
//
// A spreadsheet runs a cell that begins with =, +, - or @ as a formula. An
// export therefore protects the text columns: a cell that begins with one of
// those characters, a tab, a carriage return or an apostrophe gets one
// apostrophe in front, and an import takes one away from a cell that begins
// with an apostrophe followed by such a character. The two are exact
// inverses, so no text is changed by a round trip.

var csvColumns = []string{"keyId", "key", "source", "target", "state", "description", "maxLength"}

// exportCSV writes a CSV file.
func exportCSV(keys []Key, source, target Doc) ([]byte, error) {
	var b bytes.Buffer
	w := csv.NewWriter(&b)
	if err := w.Write(csvColumns); err != nil {
		return nil, err //nolint:wrapcheck // a bytes.Buffer does not fail
	}
	for _, k := range keys {
		maxLength := ""
		if k.MaxLength > 0 {
			maxLength = strconv.Itoa(k.MaxLength)
		}
		row := []string{
			k.ID, k.Key,
			protectCell(source.Messages[k.ID]),
			protectCell(target.Messages[k.ID]),
			target.State(k.ID),
			protectCell(k.Description),
			maxLength,
		}
		if err := w.Write(row); err != nil {
			return nil, err //nolint:wrapcheck // a bytes.Buffer does not fail
		}
	}
	w.Flush()
	return b.Bytes(), w.Error() //nolint:wrapcheck // a bytes.Buffer does not fail
}

// formulaStart reports whether a spreadsheet would read a cell starting
// with r as a formula, or the protecting apostrophe itself.
func formulaStart(r byte) bool {
	switch r {
	case '=', '+', '-', '@', '\t', '\r', '\'':
		return true
	}
	return false
}

// protectCell prefixes a cell a spreadsheet could run.
func protectCell(s string) string {
	if s != "" && formulaStart(s[0]) {
		return "'" + s
	}
	return s
}

// unprotectCell reverses protectCell.
func unprotectCell(s string) string {
	if len(s) > 1 && s[0] == '\'' && formulaStart(s[1]) {
		return s[1:]
	}
	return s
}

// parseCSV reads a CSV file.
func parseCSV(data []byte) (importFile, error) {
	data = bytes.TrimPrefix(data, []byte("\xEF\xBB\xBF"))
	r := csv.NewReader(bytes.NewReader(data))
	r.FieldsPerRecord = -1
	r.ReuseRecord = true
	header, err := r.Read()
	if errors.Is(err, io.EOF) {
		return importFile{}, invalid("the CSV file is empty")
	}
	if err != nil {
		return importFile{}, invalid("the file is not valid CSV: %s", clip(err.Error()))
	}
	col := map[string]int{}
	for i, name := range header {
		name = strings.TrimSpace(name)
		if _, dup := col[name]; dup {
			return importFile{}, invalid("the CSV header names column %s twice", clip(name))
		}
		col[name] = i
	}
	_, hasID := col["keyId"]
	_, hasKey := col["key"]
	_, hasTarget := col["target"]
	if !hasTarget || (!hasID && !hasKey) {
		return importFile{}, invalid("the CSV header needs the columns target and keyId or key")
	}
	cell := func(row []string, name string) (string, bool) {
		i, ok := col[name]
		if !ok || i >= len(row) {
			return "", false
		}
		return row[i], true
	}
	var out importFile
	for {
		row, err := r.Read()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return importFile{}, invalid("the file is not valid CSV: %s", clip(err.Error()))
		}
		e := entry{}
		e.id, _ = cell(row, "keyId")
		e.key, _ = cell(row, "key")
		if e.id == "" && e.key == "" {
			continue
		}
		var present bool
		if e.source, present = cell(row, "source"); present {
			e.source, e.hasSource = unprotectCell(e.source), true
		}
		if e.target, present = cell(row, "target"); present {
			e.target, e.hasTarget = unprotectCell(e.target), true
		}
		if s, _ := cell(row, "state"); s != "" {
			if !validState(s) {
				return importFile{}, invalid("the state %q of %s is not a review state", clip(s), clip(firstNonEmpty(e.key, e.id)))
			}
			e.state = s
		}
		out.entries = append(out.entries, e)
	}
}
