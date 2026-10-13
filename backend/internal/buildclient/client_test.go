// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package buildclient

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/fnpb"
	"github.com/nightCode42/plux3/backend/internal/fnpb/fnpbconnect"
	"github.com/nightCode42/plux3/backend/internal/function"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/signing"
	"github.com/nightCode42/plux3/backend/internal/storage/objects"
)

// fakeRole is a BuilderService that answers what a test sets and keeps the
// requests it saw.
type fakeRole struct {
	fnpbconnect.UnimplementedBuilderServiceHandler
	mu   sync.Mutex
	resp *fnpb.BuildResponse
	err  error
	seen []*fnpb.BuildRequest
}

func (f *fakeRole) Build(_ context.Context, req *connect.Request[fnpb.BuildRequest]) (*connect.Response[fnpb.BuildResponse], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seen = append(f.seen, req.Msg)
	if f.err != nil {
		return nil, f.err
	}
	return connect.NewResponse(f.resp), nil
}

// rig is a client over a fake role and a store holding one source.
type rig struct {
	client *Client
	role   *fakeRole
	in     function.BuildInput
	source []byte
}

func newRig(t *testing.T) *rig {
	t.Helper()
	role := &fakeRole{}
	mux := http.NewServeMux()
	mux.Handle(fnpbconnect.NewBuilderServiceHandler(role))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	store, err := objects.NewFilesystem(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	source := []byte("gzip tar archive")
	sum := sha256.Sum256(source)
	key, err := objects.Key(objects.KindSource, hex.EncodeToString(sum[:]))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Put(t.Context(), key, source, "application/gzip"); err != nil {
		t.Fatal(err)
	}
	c, err := NewClient(Options{HTTP: srv.Client(), URL: srv.URL, Objects: store, MaxResponseBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	return &rig{client: c, role: role, source: source, in: function.BuildInput{
		BuildID: "b1", SourceObjectKey: key, SourceSHA256: hex.EncodeToString(sum[:]), Name: "bank.transferFee",
		Placement: function.PlacementServer, Timeout: 10 * time.Minute, MemoryBytes: 4 << 30, ModuleBytes: 16 << 20,
	}}
}

// Verifies: FN-016, FN-005, FN-031.
func TestBuildSendsTheStoredSourceAndConvertsTheResponse_FN_016(t *testing.T) {
	r := newRig(t)
	r.role.resp = &fnpb.BuildResponse{
		Module: []byte("wasm"), AbiVersion: 1, Placement: "server", Declaration: []byte(`{"kind":"function"}`),
		Functions: []*fnpb.BuiltFunction{{Name: "bank.transferFee", InputSchema: `{"a":1}`, OutputSchema: `{"b":2}`}},
		Provenance: &fnpb.BuildProvenance{
			SourceSha256: "s", SdkSha256: "sdk", Toolchain: "rustc 1.97.0", WasmOptVersion: "version_132",
			BuildTime: "2026-10-13T08:30:00Z", BuilderIdentity: "builder-1",
			Dependencies: []*fnpb.BuildDependency{{Name: "serde", Version: "1.0.229", Checksum: "c1"}},
		},
		TestResults: []*fnpb.BuildTestResult{{Name: "t", Passed: false, Message: "wrong"}},
	}
	out, err := r.client.Build(t.Context(), r.in)
	if err != nil {
		t.Fatal(err)
	}
	req := r.role.seen[0]
	if string(req.GetSource()) != string(r.source) || req.GetSourceSha256() != r.in.SourceSHA256 || req.GetName() != "bank.transferFee" ||
		req.GetPlacement() != "server" || req.GetLimits().GetTimeoutMs() != 600000 || req.GetLimits().GetMemoryBytes() != 4<<30 {
		t.Errorf("request = %v", req)
	}
	p := out.Provenance
	if string(out.Module) != "wasm" || out.ABIVersion != 1 || out.Placement != function.PlacementServer || string(out.Declaration) != `{"kind":"function"}` ||
		out.Functions[0].OutputSchema != `{"b":2}` || p.SDKSHA256 != "sdk" || p.WasmOptVersion != "version_132" || p.BuildTime.Year() != 2026 ||
		len(p.Dependencies) != 1 || len(out.TestResults) != 1 || out.TestResults[0].Passed {
		t.Errorf("output = %+v", out)
	}
}

// Verifies: FN-016.
func TestBuildReturnsTheDiagnosticsOfASourceThatDoesNotBuild_FN_016(t *testing.T) {
	r := newRig(t)
	raw, _ := json.Marshal(plxerr.Diagnostics{plxerr.NewDiagnostic(plxerr.FunctionBuildFailed, plxerr.Location{}, "error[E0308]")})
	r.role.resp = &fnpb.BuildResponse{DiagnosticsJson: raw}
	out, err := r.client.Build(t.Context(), r.in)
	if err != nil || len(out.Module) != 0 || !out.Diagnostics.HasErrors() || out.Diagnostics[0].Code != plxerr.FunctionBuildFailed {
		t.Errorf("output = %+v, %v", out, err)
	}
}

// Verifies: FN-016.
func TestBuildFailuresOfTheRoleAreErrorsTheJobRetries_FN_016(t *testing.T) {
	r := newRig(t)
	r.role.err = connect.NewError(connect.CodeFailedPrecondition, errors.New("sandbox"))
	if _, err := r.client.Build(t.Context(), r.in); err == nil {
		t.Error("a role failure was not an error")
	} else if _, domain := plxerr.CodeOf(err); domain {
		t.Errorf("a role failure must not carry a PLX code, which would fail the build at once: %v", err)
	}
	r.role.err = nil
	r.role.resp = &fnpb.BuildResponse{}
	if _, err := r.client.Build(t.Context(), r.in); err == nil {
		t.Error("a response with neither a module nor an error was accepted")
	}
	r.role.resp = &fnpb.BuildResponse{Module: []byte("w"), Provenance: &fnpb.BuildProvenance{BuildTime: "yesterday"}}
	if _, err := r.client.Build(t.Context(), r.in); err == nil {
		t.Error("a build time that is not RFC 3339 was accepted")
	}
}

// Verifies: FN-005.
func TestBuildRefusesASourceThatChangedInTheStore_FN_005(t *testing.T) {
	r := newRig(t)
	r.in.SourceSHA256 = hex.EncodeToString(make([]byte, 32))
	if _, err := r.client.Build(t.Context(), r.in); err == nil {
		t.Error("a source that does not match its recorded hash was sent")
	}
	if len(r.role.seen) != 0 {
		t.Error("the role received a source that failed the integrity check")
	}
	r.in.SourceObjectKey = "sources/missing"
	if _, err := r.client.Build(t.Context(), r.in); err == nil {
		t.Error("a missing source was accepted")
	}
}

// Verifies: FN-009.
func TestSignerSignsTheStatementWithTheFunctionArtifactKey_FN_009(t *testing.T) {
	backend, err := signing.NewFile(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewSigner(backend, DefaultKeyRef)
	if err != nil {
		t.Fatal(err)
	}
	statement := []byte(`{"moduleSha256":"ab"}`)
	signature, keyID, err := s.SignStatement(t.Context(), statement)
	if err != nil {
		t.Fatal(err)
	}
	pub, wantID, err := backend.PublicKey(t.Context(), "function-artifact")
	if err != nil || keyID != wantID || !ed25519.Verify(pub, statement, signature) {
		t.Errorf("the signature does not verify under the function-artifact key: %v", err)
	}
	if _, err := NewSigner(nil, "x"); err == nil {
		t.Error("a signer without a backend was accepted")
	}
}
