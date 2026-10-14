// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/fnpb"
	"github.com/nightCode42/plux3/backend/internal/function"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// examplesDir is where the example functions of the reference apps and the
// artefacts built from them are.
var examplesDir = filepath.Join("..", "..", "..", "sdk", "fn", "examples")

// exampleFunctions stands in for the builder role in the reference stacks:
// it hands the worker the artefacts that the real builder role made of the
// example functions (sdk/fn/examples, committed under modules/*/dist and
// checked against a rebuild by TestExampleArtefactsAreWhatTheBuilderMakes_FN_019),
// so that nothing needs a Rust toolchain. Everything after the builder is the
// real path: the worker signs the statement, the service judges the output,
// stores the module and the declaration, the deploy makes a version, and the
// release pins it.
type exampleFunctions struct {
	builds map[string]function.BuildOutput
	links  map[string]function.LinkOutput
}

var (
	_ function.Builder = (*exampleFunctions)(nil)
	_ function.Linker  = (*exampleFunctions)(nil)
)

// exampleModules maps each function the reference apps call to the module
// crate its artefacts are filed under and the crate that implements it.
var exampleModules = map[string]struct{ module, crate string }{
	"bank.transferFee":       {module: "fee-module", crate: "fee"},
	"loan.calculateSchedule": {module: "loan-module", crate: "loan"},
}

// exampleRecord mirrors build.json of a dist directory (see distRecord in
// backend/internal/function).
type exampleRecord struct {
	ABIVersion  int32                 `json:"abiVersion"`
	Placement   function.Placement    `json:"placement"`
	Functions   []function.Export     `json:"functions"`
	Provenance  function.Provenance   `json:"provenance"`
	TestResults []function.TestResult `json:"testResults"`
}

// exampleLink mirrors link.json of a dist directory.
type exampleLink struct {
	ABIVersion int32             `json:"abiVersion"`
	Index      map[string]uint32 `json:"index"`
}

// loadExampleFunctions reads the committed artefacts.
func loadExampleFunctions(t *testing.T) *exampleFunctions {
	t.Helper()
	read := func(module, file string) []byte {
		data, err := os.ReadFile(filepath.Join(examplesDir, "modules", module, "dist", file))
		if err != nil {
			t.Fatalf("%v; the artefacts come from the test in backend/internal/function (PLUX_RUST_E2E=1 PLUX_UPDATE_DIST=1)", err)
		}
		return data
	}
	e := &exampleFunctions{builds: map[string]function.BuildOutput{}, links: map[string]function.LinkOutput{}}
	for name, m := range exampleModules {
		var rec exampleRecord
		if err := json.Unmarshal(read(m.module, "build.json"), &rec); err != nil {
			t.Fatalf("%s build.json: %v", m.module, err)
		}
		module := read(m.module, "module.wasm")
		e.builds[name] = function.BuildOutput{
			Module: module, ABIVersion: rec.ABIVersion, Placement: rec.Placement, Functions: rec.Functions,
			Provenance: rec.Provenance, Declaration: bytes.TrimSpace(read(m.module, "declaration.json")), TestResults: rec.TestResults,
		}
		if rec.Placement != function.PlacementDevice {
			continue
		}
		// The module that links a set of one function is the module built
		// for it.
		var link exampleLink
		if err := json.Unmarshal(read(m.module, "link.json"), &link); err != nil {
			t.Fatalf("%s link.json: %v", m.module, err)
		}
		e.links[name] = function.LinkOutput{Module: module, ABIVersion: link.ABIVersion, Index: link.Index}
	}
	return e
}

// Build implements function.Builder: the artefacts of the example the
// source is named for. A name that is no example fails the build, as a
// source that does not compile does.
func (e *exampleFunctions) Build(_ context.Context, in function.BuildInput) (function.BuildOutput, error) {
	out, ok := e.builds[in.Name]
	if !ok {
		return function.BuildOutput{}, plxerr.New(plxerr.FunctionBuildFailed, "no example function is named %s", in.Name)
	}
	return out, nil
}

// Link implements function.Linker for the set of device functions the
// reference apps have: one.
func (e *exampleFunctions) Link(_ context.Context, in function.LinkInput) (function.LinkOutput, error) {
	if len(in.Sources) != 1 {
		return function.LinkOutput{}, fmt.Errorf("the example artefacts link one device function, not %d", len(in.Sources))
	}
	out, ok := e.links[in.Sources[0].Name]
	if !ok {
		return function.LinkOutput{}, plxerr.New(plxerr.FunctionBuildFailed, "no example device function is named %s", in.Sources[0].Name)
	}
	return out, nil
}

// exampleArchive packs the sources of an example crate (not its vendored
// dependencies, which nothing here compiles) as the gzip-compressed tar
// archive that UploadSource takes.
func exampleArchive(t *testing.T, crate string) []byte {
	t.Helper()
	root := filepath.Join(examplesDir, crate)
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path) //nolint:gosec // G304: walked from the repository's example.
		if err != nil {
			return err
		}
		if err := tw.WriteHeader(&tar.Header{Name: filepath.ToSlash(rel), Mode: 0o644, Size: int64(len(data))}); err != nil {
			return err
		}
		_, err = tw.Write(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// deployFunction uploads the source archive of the function name to the
// environment, waits for the worker to build it, and deploys the version, as
// `plux fn deploy` does over FunctionAdminService (FN-006, FN-016).
func (s *stack) deployFunction(t *testing.T, environment, name string, archive []byte) *pluxv1.FunctionVersion {
	t.Helper()
	ctx, cancel := context.WithTimeout(s.ctx, 2*time.Minute)
	defer cancel()
	client := pluxv1connect.NewFunctionAdminServiceClient(&http.Client{}, s.server)
	authorise := func(h http.Header) { h.Set("Authorization", "Bearer "+s.token) }

	upload := client.UploadSource(ctx)
	authorise(upload.RequestHeader())
	if err := upload.Send(&pluxv1.UploadSourceRequest{AppId: s.app.ID, EnvironmentId: s.environmentID(t, environment), Name: name}); err != nil {
		t.Fatalf("upload the source of %s: %v", name, err)
	}
	if err := upload.Send(&pluxv1.UploadSourceRequest{Chunk: archive}); err != nil {
		t.Fatalf("upload the source of %s: %v", name, err)
	}
	source, err := upload.CloseAndReceive()
	if err != nil {
		t.Fatalf("upload the source of %s: %v", name, err)
	}
	request := connect.NewRequest(&pluxv1.BuildRequest{SourceId: source.Msg.GetSourceId()})
	authorise(request.Header())
	started, err := client.Build(ctx, request)
	if err != nil {
		t.Fatalf("build %s: %v", name, err)
	}
	id := started.Msg.GetBuild().GetId()
	var build *pluxv1.Build
	for build == nil || (build.GetState() != pluxv1.BuildState_BUILD_STATE_SUCCEEDED && build.GetState() != pluxv1.BuildState_BUILD_STATE_FAILED) {
		select {
		case <-ctx.Done():
			t.Fatalf("the build of %s did not end: %v (last state %v)", name, ctx.Err(), build.GetState())
		case <-time.After(100 * time.Millisecond):
		}
		poll := connect.NewRequest(&pluxv1.GetBuildRequest{BuildId: id})
		authorise(poll.Header())
		got, err := client.GetBuild(ctx, poll)
		if err != nil {
			t.Fatalf("read the build of %s: %v", name, err)
		}
		build = got.Msg.GetBuild()
	}
	if build.GetState() != pluxv1.BuildState_BUILD_STATE_SUCCEEDED {
		t.Fatalf("the build of %s failed: %v", name, build.GetDiagnostics())
	}
	for _, r := range build.GetTestResults() {
		if !r.GetPassed() {
			t.Errorf("test %q of %s did not pass: %s", r.GetName(), name, r.GetMessage())
		}
	}
	deploy := connect.NewRequest(&pluxv1.DeployRequest{BuildId: id})
	authorise(deploy.Header())
	deployed, err := client.Deploy(ctx, deploy)
	if err != nil {
		t.Fatalf("deploy %s: %v", name, err)
	}
	return deployed.Msg.GetVersion()
}

// deployExampleFunctions deploys the functions the project declares in its
// functions directory, each from the sources of its example crate, and
// returns the versions by name.
func (s *stack) deployExampleFunctions(t *testing.T, project, environment string) map[string]*pluxv1.FunctionVersion {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(project, "functions"))
	if err != nil {
		t.Fatal(err)
	}
	deployed := map[string]*pluxv1.FunctionVersion{}
	for _, entry := range entries {
		name := entry.Name()
		m, ok := exampleModules[name]
		if !ok {
			t.Fatalf("the project declares the function %s, which has no example", name)
		}
		deployed[name] = s.deployFunction(t, environment, name, exampleArchive(t, m.crate))
	}
	return deployed
}

// Verifies: FN-006, FN-009, FN-013, FN-016, FN-051, SEC-043, DX-004.
// The reference apps' functions go the whole way on the server short of a
// device: each app's functions are deployed through the worker (the builder
// replaced by the artefacts of the examples), the app is published, and the
// release pins the versions. Plux Express's device function is linked into
// the plugin's module. Plux Bank's server function, called on the fnrunner
// role (mutual TLS, a module fetched from the api role and verified against
// the worker's signature), prices a transfer from the tier claim the
// identity provider vouches for.
func TestReferenceAppsPublishWithTheirFunctions(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the CLI and the reference backend")
	}
	for _, app := range []referenceApp{
		{fixture: "plux_express", dir: "plux_express", addr: "127.0.0.1:0"},
		{fixture: "plux_bank", dir: "plux_bank", addr: "127.0.0.1:0"},
	} {
		t.Run(app.dir, func(t *testing.T) {
			run := startReferenceApp(t, app)
			st := run.st
			request := connect.NewRequest(&pluxv1.GetReleaseRequest{AppId: st.app.ID, Sequence: run.release})
			request.Header().Set("Authorization", "Bearer "+st.token)
			rel, err := pluxv1connect.NewReleaseServiceClient(&http.Client{}, st.server).GetRelease(st.ctx, request)
			if err != nil {
				t.Fatal(err)
			}
			var pinned []string
			for _, f := range rel.Msg.GetRelease().GetFunctions() {
				pinned = append(pinned, fmt.Sprintf("%s@%d/%s", f.GetName(), f.GetVersion(), f.GetPlacement()))
			}
			switch app.dir {
			case "plux_express":
				if !slices.Equal(pinned, []string{"loan.calculateSchedule@1/device"}) {
					t.Fatalf("the release pins %v", pinned)
				}
			case "plux_bank":
				if !slices.Equal(pinned, []string{"bank.transferFee@1/server"}) {
					t.Fatalf("the release pins %v", pinned)
				}
				run.checkTransferFee(t)
			}
		})
	}
}

// checkTransferFee calls the deployed bank.transferFee on the fnrunner role
// with the grant the api role would give, as the api role does for a device
// whose user token carries the tier.
func (r *referenceRun) checkTransferFee(t *testing.T) {
	t.Helper()
	st := r.st
	const name = "bank.transferFee"
	ctx := st.ctx
	deployed := r.functions[name]
	if deployed == nil || deployed.GetVersion() != 1 {
		t.Fatalf("the deployed versions are %v", r.functions)
	}
	module := deployed.GetModuleSha256()
	call := func(claims, amount string) *fnpb.FunctionRunnerServiceInvokeResponse {
		t.Helper()
		grant, _, err := st.grants.Issue(function.GrantClaims{
			OrganizationID: st.org, AppID: st.app.ID, EnvironmentID: st.environmentID(t, "staging"),
			Function: name, Version: 1, ModuleSHA256: module,
		}, 30*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		res, err := st.runner.Invoke(ctx, connect.NewRequest(&fnpb.FunctionRunnerServiceInvokeRequest{
			Function: name, Version: 1, ModuleSha256: module, Input: []byte(`{"amount":"` + amount + `"}`),
			Limits: &fnpb.InvocationLimits{
				TimeoutMs: 5000, MemoryBytes: 64 << 20, OutputBytes: 1 << 20, HttpCalls: 0, CallDepth: 1, LogBytes: 1 << 16, InputBytes: 1 << 20,
			},
			Capabilities: &fnpb.Capabilities{Claims: true},
			Claims:       []byte(claims), Grant: string(grant),
			App: st.app.ID, Environment: "staging", AssuranceLevel: "AL1",
		}))
		if err != nil {
			t.Fatalf("Invoke %s: %v", name, err)
		}
		return res.Msg
	}
	if out := call(`{"tier":"standard"}`, "12.50").GetOutput(); string(out) != `{"fee":"0.50"}` {
		t.Errorf("the fee of a standard customer's 12.50 is %s", out)
	}
	if out := call(`{"tier":"premium"}`, "12.50").GetOutput(); string(out) != `{"fee":"0.00"}` {
		t.Errorf("the fee of a premium customer's 12.50 is %s", out)
	}
	var business struct{ Code string }
	if err := json.Unmarshal(call(`{"tier":"gold"}`, "12.50").GetBusinessError(), &business); err != nil || business.Code != "TIER_UNKNOWN" {
		t.Errorf("an unknown tier gave the error %+v, %v", business, err)
	}
	// Without a grant the runner fetches nothing: the api role's host
	// service verifies it before it gives a module out (ADR-0064).
	_, err := st.runner.Invoke(ctx, connect.NewRequest(&fnpb.FunctionRunnerServiceInvokeRequest{
		Function: name, Version: 1, ModuleSha256: module, Input: []byte(`{"amount":"1"}`),
	}))
	var refused *connect.Error
	if !errors.As(err, &refused) {
		t.Errorf("a call without a grant ran: %v", err)
	}
}

// declaredLike reads a function document and drops what the declaration a
// build records leaves out: the description, which is for people.
func declaredLike(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	delete(doc, "description")
	delete(doc, "tests")
	return doc
}

// Verifies: FN-008, DX-004.
// What the worker records as the declaration of a deployed example is what
// the reference app's project declares for the function: the capabilities,
// the placement and the requirements of the call (claims, strong customer
// authentication, attestation). The artefacts cannot drift from the
// fixtures, whose publish and flows rely on them.
func TestExampleDeclarationsAreTheFixtures(t *testing.T) {
	t.Parallel()
	fixtures := map[string]string{"bank.transferFee": "plux_bank", "loan.calculateSchedule": "plux_express"}
	for name, fixture := range fixtures {
		doc, err := os.ReadFile(filepath.Join("..", "..", "..", "schema", "testdata", "documents", fixture, "functions", name, "function.json"))
		if err != nil {
			t.Fatal(err)
		}
		declaration, err := os.ReadFile(filepath.Join(examplesDir, "modules", exampleModules[name].module, "dist", "declaration.json"))
		if err != nil {
			t.Fatal(err)
		}
		want, got := declaredLike(t, doc), declaredLike(t, declaration)
		if !reflect.DeepEqual(want, got) {
			t.Errorf("%s: the artefact declares %v, the %s fixture %v", name, got, fixture, want)
		}
	}
}
