// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package fnhost_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"maps"
	"reflect"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/fnhost"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// vectorFile is schema/testdata/functions/abi-v1.json.
type vectorFile struct {
	Version  int                        `json:"version"`
	Defaults map[string]json.RawMessage `json:"defaults"`
	Modules  map[string]struct {
		File string `json:"file"`
		Hex  string `json:"hex"`
	} `json:"modules"`
	Cases []map[string]json.RawMessage `json:"cases"`
}

// vectorCase is a case after the defaults were applied.
type vectorCase struct {
	Name      string `json:"name"`
	Module    string `json:"module"`
	Placement string `json:"placement"`
	Index     uint32 `json:"index"`
	Input     string `json:"input"`
	Depth     int    `json:"depth"`
	Limits    struct {
		TimeoutMs   int64  `json:"timeoutMs"`
		MemoryBytes uint64 `json:"memoryBytes"`
		InputBytes  int    `json:"inputBytes"`
		OutputBytes int    `json:"outputBytes"`
		LogBytes    int    `json:"logBytes"`
		HTTPCalls   int    `json:"httpCalls"`
		CallDepth   int    `json:"callDepth"`
	} `json:"limits"`
	Context fnhost.ContextInfo `json:"context"`
	Env     map[string]string  `json:"env"`
	Grants  struct {
		HTTPDomains []string `json:"httpDomains"`
		KV          bool     `json:"kv"`
		Secrets     []string `json:"secrets"`
		Functions   []string `json:"functions"`
		Claims      bool     `json:"claims"`
	} `json:"grants"`
	Host struct {
		KV      map[string]string `json:"kv"`
		Secrets map[string]string `json:"secrets"`
		Claims  json.RawMessage   `json:"claims"`
		HTTP    []struct {
			URL     string            `json:"url"`
			Status  int               `json:"status"`
			Headers map[string]string `json:"headers"`
			Body    string            `json:"body"`
		} `json:"http"`
		Invoke map[string]outcomeSpec `json:"invoke"`
		Fail   map[string]string      `json:"fail"`
	} `json:"host"`
	Expect struct {
		Tag          *int            `json:"tag"`
		Output       json.RawMessage `json:"output"`
		OutputText   *string         `json:"outputText"`
		OutputLength *int            `json:"outputLength"`
		Business     json.RawMessage `json:"business"`
		ErrorCode    *int            `json:"errorCode"`
		CompileError *int            `json:"compileError"`
		Logs         *[]struct {
			Level   int    `json:"level"`
			Message string `json:"message"`
		} `json:"logs"`
		LogBytes     *int   `json:"logBytes"`
		HTTPCalls    *int   `json:"httpCalls"`
		HostCalls    *int   `json:"hostCalls"`
		InvokeDepths *[]int `json:"invokeDepths"`
	} `json:"expect"`
}

// mergedKeys are the case keys that overlay the defaults key by key.
var mergedKeys = []string{"limits", "context", "env", "grants", "host"}

// resolve overlays a case on the defaults.
func resolve(t *testing.T, defaults, raw map[string]json.RawMessage) vectorCase {
	t.Helper()
	merged := maps.Clone(defaults)
	for k, v := range raw {
		merged[k] = v
	}
	for _, k := range mergedKeys {
		var base, over map[string]json.RawMessage
		if err := json.Unmarshal(defaults[k], &base); err != nil {
			t.Fatalf("defaults.%s: %v", k, err)
		}
		if v, ok := raw[k]; ok {
			if err := json.Unmarshal(v, &over); err != nil {
				t.Fatalf("case.%s: %v", k, err)
			}
		}
		maps.Copy(base, over)
		b, _ := json.Marshal(base)
		merged[k] = b
	}
	b, _ := json.Marshal(merged)
	var c vectorCase
	if err := json.Unmarshal(b, &c); err != nil {
		t.Fatalf("case: %v", err)
	}
	return c
}

// moduleBytes returns the bytes of a vector module.
func moduleBytes(t *testing.T, vf vectorFile, name string) []byte {
	t.Helper()
	m, ok := vf.Modules[name]
	if !ok {
		t.Fatalf("unknown module %q", name)
	}
	if m.File != "" {
		return fixture(t, m.File)
	}
	b, err := hex.DecodeString(m.Hex)
	if err != nil {
		t.Fatalf("module %q: %v", name, err)
	}
	return b
}

// hostFor builds the fake host of a case.
func (c *vectorCase) hostFor() *fakeHost {
	h := newFakeHost()
	for k, v := range c.Host.KV {
		h.kv[k] = []byte(v)
	}
	maps.Copy(h.secrets, c.Host.Secrets)
	for _, r := range c.Host.HTTP {
		headers := r.Headers
		if headers == nil {
			headers = map[string]string{}
		}
		h.http[r.URL] = fnhost.HTTPResponse{Status: r.Status, Headers: headers, Body: []byte(r.Body)}
	}
	for name, o := range c.Host.Invoke {
		h.invoke[name] = fnhost.Outcome{Output: o.Output, Business: o.Business}
	}
	for capability, name := range c.Host.Fail {
		h.fail[capability] = failure(name)
	}
	return h
}

// call builds the Call of a case.
func (c *vectorCase) call(h fnhost.Host, logs *[]fnhost.LogRecord) fnhost.Call {
	return fnhost.Call{
		Index: c.Index,
		Input: []byte(c.Input),
		Limits: fnhost.Limits{
			Timeout:     time.Duration(c.Limits.TimeoutMs) * time.Millisecond,
			MemoryBytes: c.Limits.MemoryBytes, InputBytes: c.Limits.InputBytes,
			OutputBytes: c.Limits.OutputBytes, LogBytes: c.Limits.LogBytes,
			HTTPCalls: c.Limits.HTTPCalls, CallDepth: c.Limits.CallDepth,
		},
		Grants: fnhost.Grants{
			HTTPDomains: c.Grants.HTTPDomains, KV: c.Grants.KV, Secrets: c.Grants.Secrets,
			Functions: c.Grants.Functions, Claims: c.Grants.Claims,
		},
		Context: c.Context,
		Env:     c.Env,
		Claims:  nonNull(c.Host.Claims),
		Depth:   c.Depth,
		Host:    h,
		Log:     func(r fnhost.LogRecord) { *logs = append(*logs, r) },
	}
}

func nonNull(raw json.RawMessage) json.RawMessage {
	if string(raw) == "null" {
		return nil
	}
	return raw
}

// jsonEqual compares two JSON documents as values.
func jsonEqual(t *testing.T, got, want []byte) bool {
	t.Helper()
	decode := func(b []byte) (any, error) {
		dec := json.NewDecoder(bytes.NewReader(b))
		dec.UseNumber()
		var v any
		err := dec.Decode(&v)
		return v, err
	}
	g, err := decode(got)
	if err != nil {
		return false
	}
	w, err := decode(want)
	if err != nil {
		t.Fatalf("expectation is not JSON: %v", err)
	}
	return reflect.DeepEqual(g, w)
}

func errorNumber(err error) int {
	var pe *plxerr.Error
	if errors.As(err, &pe) {
		return int(pe.Code)
	}
	return 0
}

// Verifies: FN-010, FN-011, FN-012, FN-015, FN-052.
func TestABIVectors_FN_015(t *testing.T) {
	var vf vectorFile
	if err := json.Unmarshal(fixture(t, "abi-v1.json"), &vf); err != nil {
		t.Fatal(err)
	}
	if vf.Version != 1 || len(vf.Cases) == 0 {
		t.Fatalf("unexpected vector file: version %d, %d cases", vf.Version, len(vf.Cases))
	}
	e := newEngine(t)
	for _, raw := range vf.Cases {
		c := resolve(t, vf.Defaults, raw)
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			runVector(t, e, vf, &c)
		})
	}
}

func runVector(t *testing.T, e *fnhost.Engine, vf vectorFile, c *vectorCase) {
	t.Helper()
	ctx := context.Background()
	m, err := e.Compile(ctx, moduleBytes(t, vf, c.Module), fnhost.Placement(c.Placement))
	if c.Expect.CompileError != nil {
		if got := errorNumber(err); got != *c.Expect.CompileError {
			t.Fatalf("Compile error = %v (code %d), want PLX-%d", err, got, *c.Expect.CompileError)
		}
		return
	}
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	t.Cleanup(func() { _ = m.Close(ctx) })
	host := c.hostFor()
	var logs []fnhost.LogRecord
	out, stats, err := e.Invoke(ctx, m, c.call(host, &logs))
	checkOutcome(t, c, out, err)
	checkObservations(t, c, host, stats, logs)
}

func checkOutcome(t *testing.T, c *vectorCase, out fnhost.Outcome, err error) {
	t.Helper()
	exp := &c.Expect
	if exp.ErrorCode != nil {
		if got := errorNumber(err); got != *exp.ErrorCode {
			t.Fatalf("error = %v (code %d), want PLX-%d", err, got, *exp.ErrorCode)
		}
		return
	}
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if exp.Tag != nil && *exp.Tag == 1 {
		got, _ := json.Marshal(out.Business)
		if out.Business == nil || !jsonEqual(t, got, exp.Business) {
			t.Fatalf("business error = %s, want %s", got, exp.Business)
		}
		return
	}
	if out.Business != nil {
		t.Fatalf("unexpected business error %+v", out.Business)
	}
	if exp.Output != nil && !jsonEqual(t, out.Output, exp.Output) {
		t.Fatalf("output = %s, want %s", out.Output, exp.Output)
	}
	if exp.OutputText != nil && string(out.Output) != *exp.OutputText {
		t.Fatalf("output = %q, want %q", out.Output, *exp.OutputText)
	}
	if exp.OutputLength != nil && len(out.Output) != *exp.OutputLength {
		t.Fatalf("output has %d bytes, want %d", len(out.Output), *exp.OutputLength)
	}
}

func checkObservations(t *testing.T, c *vectorCase, h *fakeHost, stats fnhost.Stats, logs []fnhost.LogRecord) {
	t.Helper()
	exp := &c.Expect
	if exp.Logs != nil {
		if len(logs) != len(*exp.Logs) {
			t.Fatalf("logs = %v, want %v", logs, *exp.Logs)
		}
		for i, want := range *exp.Logs {
			if int(logs[i].Level) != want.Level || logs[i].Message != want.Message {
				t.Fatalf("log %d = %v, want %v", i, logs[i], want)
			}
		}
	}
	if exp.LogBytes != nil && stats.LogBytes != *exp.LogBytes {
		t.Errorf("LogBytes = %d, want %d", stats.LogBytes, *exp.LogBytes)
	}
	if exp.HTTPCalls != nil && stats.HTTPCalls != *exp.HTTPCalls {
		t.Errorf("HTTPCalls = %d, want %d", stats.HTTPCalls, *exp.HTTPCalls)
	}
	if exp.HostCalls != nil && h.calls != *exp.HostCalls {
		t.Errorf("host calls = %d, want %d", h.calls, *exp.HostCalls)
	}
	if exp.InvokeDepths != nil && !reflect.DeepEqual(h.invokeDepths, *exp.InvokeDepths) {
		t.Errorf("nested depths = %v, want %v", h.invokeDepths, *exp.InvokeDepths)
	}
}
