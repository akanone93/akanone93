// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
)

// fakeConfigAdmin keeps configuration values in memory.
type fakeConfigAdmin struct {
	pluxv1connect.UnimplementedSecurityAdminServiceHandler
	mu     sync.Mutex
	values map[string]*pluxv1.ConfigValue
	last   *pluxv1.SetConfigValueRequest
}

func (f *fakeConfigAdmin) SetConfigValue(_ context.Context, r *connect.Request[pluxv1.SetConfigValueRequest]) (*connect.Response[pluxv1.SetConfigValueResponse], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.last = r.Msg
	v := r.Msg.GetValue()
	stored := &pluxv1.ConfigValue{Key: v.GetKey(), Tier: v.GetTier(), Type: v.GetType(), Sensitive: v.GetSensitive(), Strict: v.GetStrict(), Version: 1}
	if old, ok := f.values[v.GetKey()]; ok {
		stored.Version = old.GetVersion() + 1
	}
	stored.Value = v.GetValue()
	f.values[v.GetKey()] = stored
	out := stored
	if out.GetSensitive() {
		out = &pluxv1.ConfigValue{Key: stored.GetKey(), Tier: stored.GetTier(), Type: stored.GetType(), Sensitive: true, Version: stored.GetVersion()}
	}
	return connect.NewResponse(&pluxv1.SetConfigValueResponse{Value: out}), nil
}

func (f *fakeConfigAdmin) GetConfigValue(_ context.Context, r *connect.Request[pluxv1.GetConfigValueRequest]) (*connect.Response[pluxv1.GetConfigValueResponse], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.values[r.Msg.GetKey()]
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, nil)
	}
	return connect.NewResponse(&pluxv1.GetConfigValueResponse{Value: hide(v)}), nil
}

func (f *fakeConfigAdmin) ListConfigValues(context.Context, *connect.Request[pluxv1.ListConfigValuesRequest]) (*connect.Response[pluxv1.ListConfigValuesResponse], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := &pluxv1.ListConfigValuesResponse{Epoch: 7}
	for _, key := range []string{"feeRate", "pool"} {
		if v, ok := f.values[key]; ok {
			out.Values = append(out.Values, hide(v))
		}
	}
	return connect.NewResponse(out), nil
}

func (f *fakeConfigAdmin) DeleteConfigValue(_ context.Context, r *connect.Request[pluxv1.DeleteConfigValueRequest]) (*connect.Response[pluxv1.DeleteConfigValueResponse], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.values, r.Msg.GetKey())
	return connect.NewResponse(&pluxv1.DeleteConfigValueResponse{}), nil
}

// hide drops the content of a sensitive value, as the server does.
func hide(v *pluxv1.ConfigValue) *pluxv1.ConfigValue {
	if !v.GetSensitive() {
		return v
	}
	return &pluxv1.ConfigValue{Key: v.GetKey(), Tier: v.GetTier(), Type: v.GetType(), Sensitive: true, Version: v.GetVersion()}
}

// Verifies: DAT-090.
// plux config set, get, list and delete against a fake server: a sensitive
// value is given on standard input and never printed.
func TestConfigCommands(t *testing.T) { //nolint:paralleltest // the environment is process-wide
	t.Setenv(tokenEnv, "plux_pat_cfg")
	t.Setenv(serverEnv, "")
	t.Setenv(orgEnv, "")
	f := &fakeConfigAdmin{values: map[string]*pluxv1.ConfigValue{}}
	mux := http.NewServeMux()
	mux.Handle(pluxv1connect.NewAppServiceHandler(fakeAppEnvs{}))
	mux.Handle(pluxv1connect.NewSecurityAdminServiceHandler(f))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	base := []string{"--server", srv.URL, "--app", "a1", "--env", "production"}
	run := func(stdin string, sub string, args ...string) (int, string, string) {
		var out, errOut bytes.Buffer
		e := newEnv(&out, &errOut)
		e.creds = credentials{dir: t.TempDir()}
		e.stdin = strings.NewReader(stdin)
		code := e.config(append(append([]string{sub}, base...), args...))
		return code, out.String(), errOut.String()
	}

	if code, out, stderr := run("", "set", "--tier", "live", "--type", "decimal", "--strict", "--value", "0.015", "--expected-version", "0", "feeRate"); code != exitOK || out != "feeRate is now at version 1\n" {
		t.Fatalf("set: %d %q %s", code, out, stderr)
	}
	if f.last.GetExpectedVersion() != 0 || f.last.ExpectedVersion == nil || !f.last.GetValue().GetStrict() || f.last.GetEnvironmentId() != "e-prod" {
		t.Errorf("request = %+v", f.last)
	}
	if code, out, stderr := run("NL91ABNA0417164300\n", "set", "pool", "--tier", "server-bound", "--sensitive", "--value-stdin"); code != exitOK || strings.Contains(out, "NL91") {
		t.Fatalf("set sensitive: %d %q %s", code, out, stderr)
	}
	if f.last.ExpectedVersion != nil || f.last.GetValue().GetValue() != "NL91ABNA0417164300" {
		t.Errorf("sensitive request = %+v", f.last)
	}
	if code, _, stderr := run("", "set", "pool2", "--tier", "server-bound", "--sensitive", "--value", "x"); code != exitUsage || !strings.Contains(stderr, "--value-stdin") {
		t.Errorf("a sensitive value on the command line: %d %s", code, stderr)
	}
	if code, _, _ := run("", "set", "x", "--tier", "bundled", "--value", "x"); code != exitUsage {
		t.Errorf("an unknown tier: %d", code)
	}

	if code, out, _ := run("", "get", "feeRate"); code != exitOK || out != "feeRate = 0.015 (live, decimal, version 1)\n" {
		t.Errorf("get: %d %q", code, out)
	}
	if code, out, _ := run("", "get", "pool"); code != exitOK || out != "pool = (sealed) (server-bound, string, version 1)\n" {
		t.Errorf("get sensitive: %d %q", code, out)
	}
	if code, out, _ := run("", "get", "pool", "--json"); code != exitOK || strings.Contains(out, "NL91") || strings.Contains(out, `"value"`) {
		t.Errorf("get sensitive --json: %d %s", code, out)
	}
	const wantList = `KEY      TIER          TYPE     FLAGS      VERSION  VALUE
feeRate  live          decimal  strict     1        0.015
pool     server-bound  string   sensitive  1        (sealed)
`
	if code, out, _ := run("", "list"); code != exitOK || out != wantList {
		t.Errorf("list: %d\n%s", code, out)
	}
	if code, out, _ := run("", "delete", "feeRate"); code != exitOK || out != "deleted feeRate\n" {
		t.Errorf("delete: %d %q", code, out)
	}
	if code, _, _ := run("", "get", "feeRate"); code == exitOK {
		t.Error("a deleted value was found")
	}
	if code, _, _ := run("", "get"); code != exitUsage {
		t.Errorf("get with no key: %d", code)
	}
}
