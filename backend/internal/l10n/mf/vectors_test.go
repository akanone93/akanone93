// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package mf

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/l10n"
)

// update fills the golden ast and any empty want of the vectors.
var update = flag.Bool("update", false, "rewrite the golden ast (and empty want) of schema/testdata/l10n/messages.json")

var vectorPath = filepath.Join("..", "..", "..", "..", "schema", "testdata", "l10n", "messages.json")

type vectorFile struct {
	Description string        `json:"description"`
	Cases       []*vectorCase `json:"cases"`
}

// vectorCase is one message. A case with Error has no Want: the parser must
// fail at the offset. Args are strings, booleans, numbers, {"decimal": "1.0"}
// for a number whose fraction digits count, and {"date": "2026-03-14T15:09:26"}.
type vectorCase struct {
	Name    string         `json:"name"`
	Locale  string         `json:"locale,omitempty"`
	Message string         `json:"message"`
	Args    map[string]any `json:"args,omitempty"`
	Want    *string        `json:"want,omitempty"`
	Error   *vectorError   `json:"error,omitempty"`
	AST     string         `json:"ast,omitempty"`
	// Invalid marks a case whose evaluation reports an error beside its text.
	Invalid bool `json:"invalid,omitempty"`
}

type vectorError struct {
	Offset int `json:"offset"`
}

func convertArgs(t *testing.T, in map[string]any) map[string]any {
	t.Helper()
	out := map[string]any{}
	for k, v := range in {
		switch x := v.(type) {
		case json.Number:
			d, err := l10n.ParseDecimal(x.String())
			if err != nil {
				t.Fatal(err)
			}
			out[k] = d
		case map[string]any:
			if s, ok := x["decimal"].(string); ok {
				d, err := l10n.ParseDecimal(s)
				if err != nil {
					t.Fatal(err)
				}
				out[k] = d
				continue
			}
			s, _ := x["date"].(string)
			ts, err := time.Parse("2006-01-02T15:04:05", s)
			if err != nil {
				t.Fatalf("date %q: %v", s, err)
			}
			out[k] = l10n.GregorianDate(ts.Year(), int(ts.Month()), ts.Day(), ts.Hour(), ts.Minute(), ts.Second())
		default:
			out[k] = v
		}
	}
	return out
}

// TestVectors runs schema/testdata/l10n/messages.json: parse, the golden
// binary AST, the decoded AST, and the formatted text. The Dart evaluator
// decodes the same goldens and must give the same text.
//
// Verifies: I18N-001, I18N-011, QA-003.
func TestVectors(t *testing.T) {
	data, err := os.ReadFile(vectorPath)
	if err != nil {
		t.Fatal(err)
	}
	var f vectorFile
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&f); err != nil {
		t.Fatal(err)
	}
	if len(f.Cases) < 60 {
		t.Fatalf("%d cases, want at least 60", len(f.Cases))
	}
	names := map[string]bool{}
	for _, c := range f.Cases {
		if names[c.Name] {
			t.Errorf("duplicate case %q", c.Name)
		}
		names[c.Name] = true
		m, err := Parse(c.Message)
		if c.Error != nil {
			var se *SyntaxError
			if !errors.As(err, &se) || se.Offset != c.Error.Offset {
				t.Errorf("%s: %q: error %v, want a syntax error at %d", c.Name, c.Message, err, c.Error.Offset)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %q: %v", c.Name, c.Message, err)
			continue
		}
		enc := base64.StdEncoding.EncodeToString(m.Encode())
		if *update {
			c.AST = enc
		} else if c.AST != enc {
			t.Errorf("%s: golden ast differs: got %s, want %s", c.Name, enc, c.AST)
		}
		back, err := Decode(m.Encode())
		if err != nil {
			t.Errorf("%s: decode: %v", c.Name, err)
			continue
		}
		loc, err := l10n.Load(c.Locale)
		if err != nil {
			t.Fatalf("%s: %v", c.Name, err)
		}
		got, err := back.Format(loc, convertArgs(t, c.Args))
		if (err != nil) != c.Invalid {
			t.Errorf("%s: format error %v, want an error: %v", c.Name, err, c.Invalid)
		}
		if *update && c.Want == nil {
			c.Want = &got
		}
		if c.Want == nil || got != *c.Want {
			t.Errorf("%s: %q = %q, want %v", c.Name, c.Message, got, c.Want)
		}
	}
	if *update {
		var b bytes.Buffer
		enc := json.NewEncoder(&b)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		if err := enc.Encode(&f); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(vectorPath, b.Bytes(), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}
