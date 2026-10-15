// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package fnhost_test

import (
	"context"
	"encoding/binary"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/fnhost"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// schemaLimits are tight limits for reading schemas.
var schemaLimits = fnhost.Limits{Timeout: 5 * time.Second, MemoryBytes: 16 << 20, OutputBytes: 1 << 16, LogBytes: 1 << 10}

// section encodes a WebAssembly section.
func section(id byte, payload []byte) []byte {
	return append(append([]byte{id}, uleb(len(payload))...), payload...)
}

// uleb encodes an unsigned LEB128 integer.
func uleb(v int) []byte {
	var out []byte
	for {
		b := byte(v & 0x7f)
		v >>= 7
		if v == 0 {
			return append(out, b)
		}
		out = append(out, b|0x80)
	}
}

// vec encodes a vector of already encoded items.
func vec(items ...[]byte) []byte {
	out := uleb(len(items))
	for _, it := range items {
		out = append(out, it...)
	}
	return out
}

// name encodes a name.
func name(s string) []byte { return append(uleb(len(s)), s...) }

// schemaModule assembles a minimal function module that exports plux_schemas
// when withSchemas is set. Its linear memory starts with a table of one packed
// (ptr << 32) | len per document, followed by the documents, so
// plux_schemas(i) loads entry i.
func schemaModule(docs []string, withSchemas bool) []byte {
	table := make([]byte, 8*len(docs))
	var texts []byte
	for i, d := range docs {
		ptr := uint64(len(table) + len(texts))
		binary.LittleEndian.PutUint64(table[8*i:], ptr<<32|uint64(len(d)))
		texts = append(texts, d...)
	}
	const (
		i32 = 0x7f
		i64 = 0x7e
	)
	fn := func(params []byte, results []byte) []byte {
		return append(append([]byte{0x60}, vec(bytesOf(params)...)...), vec(bytesOf(results)...)...)
	}
	types := vec(fn(nil, []byte{i32}), fn([]byte{i32}, []byte{i32}), fn([]byte{i32, i32, i32}, []byte{i64}), fn([]byte{i32}, []byte{i64}))
	funcs := []byte{0, 1, 2}
	exports := [][]byte{
		append(name("memory"), 0x02, 0x00),
		append(name("plux_abi_version"), 0x00, 0x00),
		append(name("plux_alloc"), 0x00, 0x01),
		append(name("plux_invoke"), 0x00, 0x02),
	}
	body := func(code ...byte) []byte { return append(uleb(len(code)+1), append([]byte{0x00}, code...)...) }
	codes := [][]byte{
		body(0x41, 0x01, 0x0b),       // plux_abi_version: i32.const 1
		body(0x41, 0x80, 0x20, 0x0b), // plux_alloc: i32.const 4096
		body(0x42, 0x00, 0x0b),       // plux_invoke: i64.const 0
	}
	if withSchemas {
		funcs = append(funcs, 3)
		exports = append(exports, append(name("plux_schemas"), 0x00, 0x03))
		// local.get 0; i32.const 3; i32.shl; i64.load align=3 offset=0
		codes = append(codes, body(0x20, 0x00, 0x41, 0x03, 0x74, 0x29, 0x03, 0x00, 0x0b))
	}
	data := append([]byte{0x00, 0x41, 0x00, 0x0b}, append(uleb(len(table)+len(texts)), append(table, texts...)...)...)
	mod := []byte("\x00asm\x01\x00\x00\x00")
	mod = append(mod, section(1, types)...)
	mod = append(mod, section(3, vec(bytesOf(funcs)...))...)
	mod = append(mod, section(5, vec([]byte{0x00, 0x01}))...)
	mod = append(mod, section(7, vec(exports...))...)
	mod = append(mod, section(10, vec(codes...))...)
	return append(mod, section(11, vec(data))...)
}

// bytesOf splits b into one-byte items.
func bytesOf(b []byte) [][]byte {
	out := make([][]byte, len(b))
	for i := range b {
		out[i] = b[i : i+1]
	}
	return out
}

// Verifies: FN-004.
func TestReadSchemasReturnsTheSchemasOfEveryFunction_FN_004(t *testing.T) {
	e := newEngine(t)
	docs := []string{
		`{"input":{"type":"object","properties":{"n":{"type":"integer"}}},"output":{"type":"string"}}`,
		`{"output":{"type":"object"},"input":{"type":"object"}}`,
	}
	got, err := e.ReadSchemas(context.Background(), schemaModule(docs, true), 2, schemaLimits)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || string(got[0].Output) != `{"type":"string"}` || string(got[1].Input) != `{"type":"object"}` {
		t.Errorf("schemas = %s", got)
	}
}

// Verifies: FN-004.
func TestReadSchemasRefusesBadDocuments_FN_004(t *testing.T) {
	e := newEngine(t)
	for name, doc := range map[string]string{
		"not JSON":        `{"input":`,
		"not an object":   `[1]`,
		"missing output":  `{"input":{}}`,
		"extra member":    `{"input":{},"output":{},"x":1}`,
		"input is true":   `{"input":true,"output":{}}`,
		"output is array": `{"input":{},"output":[]}`,
		"output is null":  `{"input":{},"output":null}`,
	} {
		_, err := e.ReadSchemas(context.Background(), schemaModule([]string{doc}, true), 1, schemaLimits)
		if err == nil {
			t.Errorf("%s: no error", name)
			continue
		}
		wantCode(t, err, plxerr.FunctionSchemaDerivationFailed)
	}
}

// Verifies: FN-004.
func TestReadSchemasRefusesAModuleWithoutThem_FN_004(t *testing.T) {
	e := newEngine(t)
	_, err := e.ReadSchemas(context.Background(), schemaModule(nil, false), 1, schemaLimits)
	wantCode(t, err, plxerr.FunctionSchemaDerivationFailed)
	if err == nil || !strings.Contains(err.Error(), "plux_schemas") {
		t.Errorf("the message should name the export: %v", err)
	}
	_, err = e.ReadSchemas(context.Background(), []byte("not wasm"), 1, schemaLimits)
	wantCode(t, err, plxerr.FunctionSchemaDerivationFailed)
}

// Verifies: FN-004, FN-011.
func TestReadSchemasHonoursItsLimits_FN_004(t *testing.T) {
	e := newEngine(t)
	doc := `{"input":{"description":"` + strings.Repeat("x", 200) + `"},"output":{}}`
	small := schemaLimits
	small.OutputBytes = 64
	_, err := e.ReadSchemas(context.Background(), schemaModule([]string{doc}, true), 1, small)
	wantCode(t, err, plxerr.FunctionSchemaDerivationFailed)
	// An index beyond the table reads memory that is not a schema document.
	_, err = e.ReadSchemas(context.Background(), schemaModule([]string{`{"input":{},"output":{}}`}, true), 4, schemaLimits)
	wantCode(t, err, plxerr.FunctionSchemaDerivationFailed)
	_, err = e.ReadSchemas(context.Background(), schemaModule([]string{`{"input":{},"output":{}}`}, true), 0, schemaLimits)
	wantCode(t, err, plxerr.FunctionSchemaDerivationFailed)
	noTime := schemaLimits
	noTime.Timeout = 0
	_, err = e.ReadSchemas(context.Background(), schemaModule([]string{`{"input":{},"output":{}}`}, true), 1, noTime)
	if err == nil || errors.Is(err, context.Canceled) {
		t.Errorf("a zero timeout must be refused: %v", err)
	}
}
