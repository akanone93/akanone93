// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package renderworker_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/compiler"
	"github.com/nightCode42/plux3/backend/internal/renderpb"
	"github.com/nightCode42/plux3/backend/internal/renderpb/renderpbconnect"
	"github.com/nightCode42/plux3/backend/internal/renderworker"
	"github.com/nightCode42/plux3/backend/internal/schema"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

const pluginFile = "plugins/welcome/plugin.json"

// callingFunctions is the starter with its plugin granting a server function
// and a device function, so that it compiles only against a function
// catalogue that knows both (FN-006, PLX-7031).
func callingFunctions(t *testing.T) fstest.MapFS {
	t.Helper()
	files := project(t)
	var doc map[string]any
	if err := json.Unmarshal(files[pluginFile].Data, &doc); err != nil {
		t.Fatal(err)
	}
	doc["capabilities"] = map[string]any{"functions": []any{
		map[string]any{"id": "01e0c450-6c00-7000-8000-0000000fff01", "function": "demo.quote"},
		map[string]any{"id": "01e0c450-6c00-7000-8000-0000000fff02", "function": "demo.format", "alias": "stable"},
	}}
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	files[pluginFile] = &fstest.MapFile{Data: data}
	return files
}

func demoCatalogue() *compiler.FunctionSet {
	yes := true
	return compiler.NewFunctionSet([]compiler.FunctionEntry{
		{Name: "demo.quote", Info: compiler.FunctionInfo{
			Placement: schema.PlacementServer, Version: 3, InputSchema: []byte(`{"type":"object"}`), OutputSchema: []byte(`{"type":"number"}`),
			Capabilities: &schema.FunctionCapabilities{HTTP: []string{"api.example.org"}, KV: &yes}, RequiresAttestation: true,
		}},
		{Name: "demo.format", Alias: "stable", Info: compiler.FunctionInfo{
			Placement: schema.PlacementDevice, Version: 2, Limits: &schema.FunctionLimits{TimeoutMs: new(int64)}, RequiresSCA: true,
		}},
	}, true)
}

func renderRequest(t *testing.T, c renderpbconnect.RenderWorkerServiceClient, req *renderpb.RenderRequest, after fstest.MapFS) ([]*renderpb.RenderResponse, error) {
	t.Helper()
	var err error
	if req.After, err = renderworker.Pack(after, testLimits()); err != nil {
		t.Fatal(err)
	}
	req.Devices = []string{smallest}
	stream, err := c.Render(context.Background(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	var out []*renderpb.RenderResponse
	for stream.Receive() {
		out = append(out, stream.Msg())
	}
	return out, stream.Err()
}

// Verifies: STU-100, FN-006.
// A project whose plugin calls a server and a device function renders when the
// request carries the function catalogue of the environment, and is refused
// as not compiling without it, as it was before the catalogue was sent.
func TestRenderCompilesAgainstTheFunctionCatalogue_STU_100(t *testing.T) {
	t.Parallel()
	c := startWorker(t, &fakeFlutter{})
	files := callingFunctions(t)

	if _, err := renderRequest(t, c, &renderpb.RenderRequest{JobId: "j1"}, files); connect.CodeOf(err) != connect.CodeFailedPrecondition || !strings.Contains(err.Error(), "does not compile") {
		t.Fatalf("without a catalogue: %v", err)
	}

	decls, err := renderworker.EncodeCatalogue(demoCatalogue())
	if err != nil {
		t.Fatal(err)
	}
	screens, err := renderRequest(t, c, &renderpb.RenderRequest{JobId: "j2", Functions: decls, DeviceFunctions: true}, files)
	if err != nil || len(screens) != 5 {
		t.Fatalf("with the catalogue: %d screens, %v", len(screens), err)
	}

	// deviceFunctions is a security setting of the environment (FN-055): off,
	// the project's device function is refused as the release build would.
	if _, err := renderRequest(t, c, &renderpb.RenderRequest{JobId: "j3", Functions: decls}, files); connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Errorf("with device functions off: %v", err)
	}
}

// Verifies: STU-100, FN-006.
// The declarations survive the trip: what the catalogue answers on the server
// is what the compiler asks on the worker.
func TestCatalogueRoundTrips_STU_100(t *testing.T) {
	t.Parallel()
	want := demoCatalogue()
	decls, err := renderworker.EncodeCatalogue(want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := renderworker.DecodeCatalogue(decls, true)
	if err != nil {
		t.Fatal(err)
	}
	lister, ok := got.(compiler.FunctionLister)
	if !ok {
		t.Fatal("the decoded catalogue cannot list its answers")
	}
	if !reflect.DeepEqual(lister.FunctionEntries(), want.FunctionEntries()) {
		t.Errorf("entries = %+v\nwant %+v", lister.FunctionEntries(), want.FunctionEntries())
	}
	if !got.DeviceFunctionsEnabled() {
		t.Error("deviceFunctions was lost")
	}
	if none, err := renderworker.EncodeCatalogue(opaque{}); err != nil || none != nil {
		t.Errorf("a catalogue that cannot list = %v, %v", none, err)
	}
}

type opaque struct{}

func (opaque) Function(string, string) (compiler.FunctionInfo, bool) {
	return compiler.FunctionInfo{}, false
}
func (opaque) DeviceFunctionsEnabled() bool { return false }

// Verifies: STU-100, LIM-001, SEC-052.
// A catalogue is bounded by app.functions and by its size, and a declaration
// from the network is validated before the compiler sees it.
func TestCatalogueIsBoundedAndValidated_STU_100(t *testing.T) {
	t.Parallel()
	def, _ := limits.Lookup(limits.AppFunctions)
	var many []compiler.FunctionEntry
	for i := range def.Max + 1 {
		many = append(many, compiler.FunctionEntry{Name: fmt.Sprintf("demo.f%d", i), Info: compiler.FunctionInfo{Placement: schema.PlacementServer}})
	}
	if _, err := renderworker.EncodeCatalogue(compiler.NewFunctionSet(many, false)); !errors.Is(err, renderworker.ErrCatalogue) {
		t.Errorf("too many functions sent: %v", err)
	}
	huge := compiler.NewFunctionSet([]compiler.FunctionEntry{{Name: "demo.big", Info: compiler.FunctionInfo{
		Placement: schema.PlacementServer, InputSchema: []byte(`"` + strings.Repeat("x", renderworker.CatalogueAllowance) + `"`),
	}}}, false)
	if _, err := renderworker.EncodeCatalogue(huge); !errors.Is(err, renderworker.ErrCatalogue) {
		t.Errorf("an oversized catalogue sent: %v", err)
	}

	for name, decls := range map[string][]*renderpb.FunctionDeclaration{
		"no name":            {{Placement: "server"}},
		"placement":          {{Name: "demo.x", Placement: "kernel"}},
		"schema not JSON":    {{Name: "demo.x", Placement: "server", InputSchema: []byte("{")}},
		"capabilities":       {{Name: "demo.x", Placement: "server", CapabilitiesJson: []byte("[1]")}},
		"limits":             {{Name: "demo.x", Placement: "server", LimitsJson: []byte(`{"timeoutMs":"slow"}`)}},
		"too big":            {{Name: "demo.x", Placement: "server", InputSchema: []byte(strings.Repeat(" ", renderworker.CatalogueAllowance+1))}},
		"too many functions": manyDecls(def.Max + 1),
	} {
		if _, err := renderworker.DecodeCatalogue(decls, false); !errors.Is(err, renderworker.ErrCatalogue) {
			t.Errorf("%s: %v", name, err)
		}
	}

	// The worker answers a bad catalogue as an invalid argument.
	c := startWorker(t, &fakeFlutter{})
	req := &renderpb.RenderRequest{JobId: "j", Functions: []*renderpb.FunctionDeclaration{{Name: "demo.x", Placement: "kernel"}}}
	if _, err := renderRequest(t, c, req, project(t)); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("a bad declaration: %v", err)
	}
}

func manyDecls(n int64) []*renderpb.FunctionDeclaration {
	out := make([]*renderpb.FunctionDeclaration, 0, n)
	for i := range n {
		out = append(out, &renderpb.FunctionDeclaration{Name: fmt.Sprintf("demo.f%d", i), Placement: "server"})
	}
	return out
}
