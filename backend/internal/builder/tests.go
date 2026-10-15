// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nightCode42/plux3/backend/internal/fnhost"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema"
	"github.com/nightCode42/plux3/backend/internal/schema/jcs"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

// testJSONDepth bounds the nesting of a test case's expected output.
const testJSONDepth = 128

// TestResult is the outcome of one stored test case (FN-031).
type TestResult struct {
	// Name is the case's name in the function document.
	Name string
	// Passed is true when the function returned what the case expects.
	Passed bool
	// Message says why a case failed; it is empty for a case that passed.
	Message string
}

// RunTests runs the stored test cases of the declaration against the module
// on the function host, as a server function, each in a fresh instance with
// the case's claims (FN-031, SEC-026). Capabilities are the declared ones, but
// the host serves only an in-memory key-value store, so a case never reaches
// the network or a secret. The module holds the one function the build
// produced, at index 0. The limits are the registry's, tightened by the
// document's own (FN-011). A case that fails, or that the host could not run,
// is a failed TestResult, never an error; an error is a failure to set the run
// up.
func RunTests(ctx context.Context, engine *fnhost.Engine, module []byte, decl *Declaration, set limits.Set) ([]TestResult, error) {
	cases := decl.Function.Tests
	if len(cases) == 0 {
		return []TestResult{}, nil
	}
	if most := set.Get(limits.FunctionTestCases); int64(len(cases)) > most {
		return nil, plxerr.New(plxerr.FunctionBuildLimitExceeded, "the document has %d test cases; function.testCases = %d", len(cases), most)
	}
	m, err := engine.Compile(ctx, module, fnhost.PlacementServer)
	if err != nil {
		return nil, fmt.Errorf("builder.RunTests: %w", err)
	}
	defer func() { _ = m.Close(context.WithoutCancel(ctx)) }()
	lim, grants := testLimits(set, decl), testGrants(decl)
	results := make([]TestResult, len(cases))
	for i, c := range cases {
		call := fnhost.Call{
			Input: []byte(c.Input), Limits: lim, Grants: grants, Claims: c.Claims,
			Context: fnhost.ContextInfo{App: "test", Environment: "test", AssuranceLevel: string(decl.Function.Assurance)},
			Host:    &testHost{kv: map[string][]byte{}},
		}
		out, _, err := engine.Invoke(ctx, m, call)
		results[i] = judgeCase(c, out, err)
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("builder.RunTests: %w", err)
		}
	}
	return results, nil
}

// judgeCase compares what the function did with what the case expects.
func judgeCase(c schema.FunctionTestCase, out fnhost.Outcome, err error) TestResult {
	fail := func(format string, args ...any) TestResult {
		return TestResult{Name: c.Name, Message: fmt.Sprintf(format, args...)}
	}
	if err != nil {
		var pe *plxerr.Error
		if errors.As(err, &pe) {
			return fail("the invocation failed with %s: %s", pe.Code, pe.Error())
		}
		return fail("the invocation failed: %v", err)
	}
	switch {
	case c.Error != nil && out.Business == nil:
		return fail("expected the business error %q; the function returned an output", c.Error.Code)
	case c.Error != nil && out.Business.Code != c.Error.Code:
		return fail("expected the business error %q; got %q", c.Error.Code, out.Business.Code)
	case c.Error != nil:
		return TestResult{Name: c.Name, Passed: true}
	case out.Business != nil:
		return fail("expected an output; the function returned the business error %q", out.Business.Code)
	}
	want, werr := jcs.Canonicalize(c.Output, testJSONDepth)
	got, gerr := jcs.Canonicalize(out.Output, testJSONDepth)
	if werr != nil || gerr != nil {
		return fail("the expected or the returned output is not canonicalisable JSON")
	}
	if !bytes.Equal(want, got) {
		return fail("expected the output %s; got %s", clip(want), clip(got))
	}
	return TestResult{Name: c.Name, Passed: true}
}

// clipBytes bounds the part of an output a failure message quotes.
const clipBytes = 512

// clip shortens a JSON text for a message.
func clip(b []byte) string {
	if len(b) > clipBytes {
		return string(bytes.ToValidUTF8(b[:clipBytes], nil)) + "…"
	}
	return string(b)
}

// testLimits are the registry limits of a server invocation, tightened by the
// document's (FN-011, LIM-002).
func testLimits(set limits.Set, decl *Declaration) fnhost.Limits {
	lim := fnhost.Limits{
		Timeout:     time.Duration(set.Get(limits.FunctionTimeout)) * time.Millisecond,
		MemoryBytes: uint64(set.Get(limits.FunctionMemory)), //nolint:gosec // G115: registry values are non-negative.
		InputBytes:  int(set.Get(limits.FunctionInputSize)),
		OutputBytes: int(set.Get(limits.FunctionOutputSize)),
		LogBytes:    int(set.Get(limits.FunctionLogBytes)),
		HTTPCalls:   int(set.Get(limits.FunctionHTTPCalls)),
		CallDepth:   int(set.Get(limits.FunctionCallDepth)),
	}
	own := decl.Function.Limits
	if own == nil {
		return lim
	}
	if v := own.TimeoutMs; v != nil && time.Duration(*v)*time.Millisecond < lim.Timeout {
		lim.Timeout = time.Duration(*v) * time.Millisecond
	}
	if v := own.MemoryBytes; v != nil && *v > 0 && uint64(*v) < lim.MemoryBytes { //nolint:gosec // G115: checked positive.
		lim.MemoryBytes = uint64(*v) //nolint:gosec // G115: checked positive.
	}
	if v := own.OutputBytes; v != nil && int(*v) < lim.OutputBytes {
		lim.OutputBytes = int(*v)
	}
	if v := own.HTTPCalls; v != nil && int(*v) < lim.HTTPCalls {
		lim.HTTPCalls = int(*v)
	}
	return lim
}

// testGrants are the capabilities the document declares (FN-012).
func testGrants(decl *Declaration) fnhost.Grants {
	c := decl.Function.Capabilities
	if c == nil {
		return fnhost.Grants{}
	}
	return fnhost.Grants{
		HTTPDomains: c.HTTP, KV: c.KV != nil && *c.KV, Secrets: c.Secrets, Functions: c.Functions,
		Claims: c.Claims != nil && *c.Claims,
	}
}

// testHost serves one test case: an in-memory key-value namespace and nothing
// else. It is used by one invocation on one goroutine.
type testHost struct{ kv map[string][]byte }

func (h *testHost) KVGet(_ context.Context, key string) ([]byte, error) {
	v, ok := h.kv[key]
	if !ok {
		return nil, fnhost.ErrNotFound
	}
	return v, nil
}

func (h *testHost) KVPut(_ context.Context, key string, value []byte) error {
	h.kv[key] = bytes.Clone(value)
	return nil
}

func (h *testHost) KVDelete(_ context.Context, key string) error {
	if _, ok := h.kv[key]; !ok {
		return fnhost.ErrNotFound
	}
	delete(h.kv, key)
	return nil
}

func (*testHost) HTTP(context.Context, fnhost.HTTPRequest) (fnhost.HTTPResponse, error) {
	return fnhost.HTTPResponse{}, fnhost.ErrUnavailable
}

func (*testHost) Secret(context.Context, string) ([]byte, error) { return nil, fnhost.ErrUnavailable }

func (*testHost) Invoke(context.Context, string, []byte, int) (fnhost.Outcome, error) {
	return fnhost.Outcome{}, fnhost.ErrUnavailable
}
