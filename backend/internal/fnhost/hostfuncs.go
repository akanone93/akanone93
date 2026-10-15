// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package fnhost

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/url"
	"slices"

	"github.com/tetratelabs/wazero/api"
)

// hostFunc implements one import of plux_v1. It reads its arguments from
// stack and leaves its result in stack[0].
type hostFunc func(ctx context.Context, st *callState, mod api.Module, stack []uint64)

// hostImpls maps each import to its implementation. The server-only ones are
// wrapped so that, even if a device module slipped past Compile, they answer
// "denied" (FN-052).
func hostImpls() map[string]hostFunc {
	return map[string]hostFunc{
		"log":             hostLog,
		"clock_now":       hostClockNow,
		"context":         hostContext,
		"env_get":         hostEnvGet,
		"kv_get":          hostKVGet,
		"kv_put":          hostKVPut,
		"kv_delete":       hostKVDelete,
		"http_request":    serverOnly(hostHTTPRequest),
		"secret_get":      serverOnly(hostSecretGet),
		"claims":          serverOnly(hostClaims),
		"invoke_function": serverOnly(hostInvokeFunction),
	}
}

// instantiatePluxV1 instantiates the plux_v1 host module once for the
// engine; its functions find the call's state in the context.
func (e *Engine) instantiatePluxV1(ctx context.Context) error {
	specs, impls := hostImports(), hostImpls()
	b := e.rt.NewHostModuleBuilder(hostModule)
	for _, name := range slices.Sorted(maps.Keys(specs)) {
		spec := specs[name]
		b.NewFunctionBuilder().
			WithGoModuleFunction(bind(impls[name]), spec.params, spec.results).
			Export(name)
	}
	if _, err := b.Instantiate(ctx); err != nil {
		return fmt.Errorf("instantiate: %w", err)
	}
	return nil
}

// bind adapts a hostFunc to wazero, finding the call's state in the context.
func bind(f hostFunc) api.GoModuleFunc {
	return func(ctx context.Context, mod api.Module, stack []uint64) {
		st, _ := ctx.Value(stateKey{}).(*callState)
		if st == nil {
			stack[0] = status(statusUnavailable)
			return
		}
		f(ctx, st, mod, stack)
	}
}

func serverOnly(f hostFunc) hostFunc {
	return func(ctx context.Context, st *callState, mod api.Module, stack []uint64) {
		if st.module.placement == PlacementDevice {
			stack[0] = status(st.deny())
			return
		}
		f(ctx, st, mod, stack)
	}
}

// status encodes a negative status as an i64 result slot.
func status(s int32) uint64 { return uint64(int64(s)) } //nolint:gosec // G115: sign extension is the encoding.

// result stores an i64 result: a packed buffer, or a negative status.
func result(stack []uint64, packed int64) { stack[0] = uint64(packed) } //nolint:gosec // G115: the bits are the packed value.

// reply answers with data, or with the status of err.
func (st *callState) reply(ctx context.Context, mod api.Module, stack []uint64, data []byte, err error) {
	if err != nil {
		stack[0] = status(statusOf(err))
		return
	}
	result(stack, st.putBuffer(ctx, mod, data))
}

func hostLog(ctx context.Context, st *callState, mod api.Module, stack []uint64) {
	level := LogLevel(min(api.DecodeU32(stack[0]), uint32(LogError)))
	st.budget.add(level, st.read(ctx, mod, stack[1], stack[2]))
}

func hostClockNow(_ context.Context, st *callState, _ api.Module, stack []uint64) {
	stack[0] = uint64(st.engine.clock().UnixNano())
}

func hostContext(ctx context.Context, st *callState, mod api.Module, stack []uint64) {
	result(stack, st.putBuffer(ctx, mod, st.ctxJSON))
}

func hostEnvGet(ctx context.Context, st *callState, mod api.Module, stack []uint64) {
	value, ok := st.call.Env[string(st.read(ctx, mod, stack[0], stack[1]))]
	if !ok {
		stack[0] = status(statusNotFound)
		return
	}
	st.reply(ctx, mod, stack, []byte(value), nil)
}

func hostKVGet(ctx context.Context, st *callState, mod api.Module, stack []uint64) {
	key := string(st.read(ctx, mod, stack[0], stack[1]))
	if !st.call.Grants.KV {
		stack[0] = status(st.deny())
		return
	}
	value, err := st.call.Host.KVGet(ctx, key)
	st.reply(ctx, mod, stack, value, err)
}

func hostKVPut(ctx context.Context, st *callState, mod api.Module, stack []uint64) {
	key := string(st.read(ctx, mod, stack[0], stack[1]))
	value := st.read(ctx, mod, stack[2], stack[3])
	if !st.call.Grants.KV {
		stack[0] = status(st.deny())
		return
	}
	stack[0] = statusFor(st.call.Host.KVPut(ctx, key, value))
}

func hostKVDelete(ctx context.Context, st *callState, mod api.Module, stack []uint64) {
	key := string(st.read(ctx, mod, stack[0], stack[1]))
	if !st.call.Grants.KV {
		stack[0] = status(st.deny())
		return
	}
	stack[0] = statusFor(st.call.Host.KVDelete(ctx, key))
}

// statusFor is the i32 result of a call that returns only a status: 0 for
// success or the status of a Host error.
func statusFor(err error) uint64 {
	if err == nil {
		return 0
	}
	return api.EncodeI32(statusOf(err))
}

func hostSecretGet(ctx context.Context, st *callState, mod api.Module, stack []uint64) {
	name := string(st.read(ctx, mod, stack[0], stack[1]))
	if !slices.Contains(st.call.Grants.Secrets, name) {
		stack[0] = status(st.deny())
		return
	}
	value, err := st.call.Host.Secret(ctx, name)
	st.reply(ctx, mod, stack, value, err)
}

func hostClaims(ctx context.Context, st *callState, mod api.Module, stack []uint64) {
	if !st.call.Grants.Claims {
		stack[0] = status(st.deny())
		return
	}
	if len(st.call.Claims) == 0 {
		stack[0] = status(statusNotFound)
		return
	}
	result(stack, st.putBuffer(ctx, mod, st.call.Claims))
}

func hostHTTPRequest(ctx context.Context, st *callState, mod api.Module, stack []uint64) {
	raw := st.read(ctx, mod, stack[0], stack[1])
	req, ok := parseHTTPRequest(raw)
	if !ok {
		stack[0] = status(statusInvalid)
		return
	}
	if s := st.httpAdmission(req); s != 0 {
		stack[0] = status(s)
		return
	}
	resp, err := st.call.Host.HTTP(ctx, req)
	if err != nil {
		stack[0] = status(statusOf(err))
		return
	}
	if resp.Headers == nil {
		resp.Headers = map[string]string{}
	}
	if resp.Body == nil {
		resp.Body = []byte{}
	}
	encoded, err := json.Marshal(resp)
	st.reply(ctx, mod, stack, encoded, err)
}

// httpAdmission decides whether a well-formed request may reach the Host: the
// capability must be granted (-2), the host must be on the allowlist (-4)
// and the call budget must not be spent (-3). It counts the call it admits.
func (st *callState) httpAdmission(req HTTPRequest) int32 {
	allow := st.call.Grants.HTTPDomains
	if len(allow) == 0 {
		return st.deny()
	}
	u, err := url.Parse(req.URL)
	if err != nil || !domainAllowed(allow, u.Hostname()) {
		return statusEgressDenied
	}
	if st.httpCalls >= st.call.Limits.HTTPCalls {
		return statusLimit
	}
	st.httpCalls++
	return 0
}

// parseHTTPRequest decodes and checks the request JSON: a method of letters
// (GET when absent) and an http or https URL with a host.
func parseHTTPRequest(raw []byte) (HTTPRequest, bool) {
	var req HTTPRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return req, false
	}
	if req.Method == "" {
		req.Method = "GET"
	}
	for _, c := range req.Method {
		if (c < 'A' || c > 'Z') && (c < 'a' || c > 'z') {
			return req, false
		}
	}
	u, err := url.Parse(req.URL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" {
		return req, false
	}
	return req, true
}

func hostInvokeFunction(ctx context.Context, st *callState, mod api.Module, stack []uint64) {
	name := string(st.read(ctx, mod, stack[0], stack[1]))
	input := st.read(ctx, mod, stack[2], stack[3])
	if !slices.Contains(st.call.Grants.Functions, name) {
		stack[0] = status(st.deny())
		return
	}
	if st.call.Depth+1 > st.call.Limits.CallDepth {
		stack[0] = status(statusLimit)
		return
	}
	out, err := st.call.Host.Invoke(ctx, name, input, st.call.Depth+1)
	if err != nil {
		stack[0] = status(statusOf(err))
		return
	}
	encoded, err := encodeOutcome(out)
	st.reply(ctx, mod, stack, encoded, err)
}

// encodeOutcome renders an Outcome as the `tag || JSON` of plux_invoke.
func encodeOutcome(out Outcome) ([]byte, error) {
	if out.Business != nil {
		body, err := json.Marshal(out.Business)
		if err != nil {
			return nil, fmt.Errorf("fnhost: encode business error: %w", err)
		}
		return append([]byte{tagBusinessError}, body...), nil
	}
	output := out.Output
	if len(output) == 0 {
		output = json.RawMessage("null")
	}
	return append([]byte{tagOutput}, output...), nil
}
