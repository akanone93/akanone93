// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/function"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
)

// functionFakes stand in for the builder role, the worker's signer and the
// job queue: they build a module named after the function, sign its
// statement with its hash, and keep the jobs for the test to run.
type functionFakes struct {
	mu   sync.Mutex
	jobs []function.BuildJob
	// hold keeps language sessions open and echoing; without it a session
	// opens and closes at once, which is all the permission matrix needs.
	hold     bool
	sessions int
	received []string
	outbox   chan []byte
}

// OpenLanguage is the fake language server: it announces a session and, when
// held, answers every message the client sends with an echo.
func (f *functionFakes) OpenLanguage(ctx context.Context, in function.EditorInput, send func(function.EditorEvent) error) error {
	f.mu.Lock()
	f.sessions++
	id := fmt.Sprintf("fake-session-%d", f.sessions)
	hold := f.hold
	if f.outbox == nil {
		f.outbox = make(chan []byte, 8)
	}
	f.mu.Unlock()
	opened := &function.EditorOpened{SessionID: id, RootURI: "file:///workspace/functions/" + in.Name + "/", Seconds: 60}
	if err := send(function.EditorEvent{Opened: opened}); err != nil {
		return err
	}
	if !hold {
		return send(function.EditorEvent{Closed: &function.EditorClosed{Reason: "exited"}})
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case m := <-f.outbox:
			if m == nil {
				return send(function.EditorEvent{Closed: &function.EditorClosed{Reason: "exited", Detail: "bye"}})
			}
			if err := send(function.EditorEvent{Message: m}); err != nil {
				return err
			}
		}
	}
}

// SendLanguage keeps the message and has the session echo it.
func (f *functionFakes) SendLanguage(_ context.Context, _ string, _ uint64, message []byte) error {
	f.mu.Lock()
	f.received = append(f.received, string(message))
	out := f.outbox
	f.mu.Unlock()
	out <- append([]byte("echo:"), message...)
	return nil
}

// Test reports no stored cases.
func (*functionFakes) Test(context.Context, function.EditorInput) (function.EditorTests, error) {
	return function.EditorTests{}, nil
}

// Enqueue keeps a build job.
func (f *functionFakes) Enqueue(_ context.Context, _ pgx.Tx, job function.BuildJob) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.jobs = append(f.jobs, job)
	return nil
}

// last returns the newest job.
func (f *functionFakes) last() function.BuildJob {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.jobs[len(f.jobs)-1]
}

// Build returns a module that exports the function.
func (*functionFakes) Build(_ context.Context, in function.BuildInput) (function.BuildOutput, error) {
	return function.BuildOutput{
		Module: []byte("\x00asm " + in.Name), ABIVersion: 1, Placement: function.PlacementServer,
		Functions:   []function.Export{{Name: in.Name, InputSchema: `{"type":"object"}`, OutputSchema: `{"type":"string"}`}},
		Provenance:  function.Provenance{Toolchain: "1.97.0", BuilderIdentity: "builder-test"},
		TestResults: []function.TestResult{{Name: "adds", Passed: true}},
	}, nil
}

// SignStatement signs with the hash of the statement.
func (*functionFakes) SignStatement(_ context.Context, statement []byte) ([]byte, string, error) {
	sum := sha256.Sum256(statement)
	return sum[:], "function-artifact-test", nil
}

const apiFunction = "payments.charge"

// upload sends a source archive as c.
func (w *world) uploadSource(t testing.TB, c caller, appID, envID, name, archive string) (*pluxv1.UploadSourceResponse, error) {
	t.Helper()
	stream := w.functionAdmin.UploadSource(context.Background())
	withHeaders(stream.RequestHeader(), c)
	half := len(archive) / 2
	for i, m := range []*pluxv1.UploadSourceRequest{
		{AppId: appID, EnvironmentId: envID, Name: name},
		{Chunk: []byte(archive[:half])},
		{Chunk: []byte(archive[half:])},
	} {
		if err := stream.Send(m); err != nil {
			if errors.Is(err, io.EOF) && i > 0 {
				break
			}
			t.Fatal(err)
		}
	}
	res, err := stream.CloseAndReceive()
	if err != nil {
		return nil, err //nolint:wrapcheck // the code is what the test reads
	}
	return res.Msg, nil
}

// builtBy uploads, builds and runs a build as c and returns its identifier.
func (w *world) builtBy(t testing.TB, c caller, appID, envID, name string) string {
	t.Helper()
	ctx := context.Background()
	src, err := w.uploadSource(t, c, appID, envID, name, "archive of "+name)
	if err != nil {
		t.Fatalf("UploadSource: %v", err)
	}
	b := must(w.functionAdmin.Build(ctx, req(c, &pluxv1.BuildRequest{SourceId: src.GetSourceId()})))(t).GetBuild()
	if b.GetState() != pluxv1.BuildState_BUILD_STATE_QUEUED {
		t.Fatalf("a new build is %v", b.GetState())
	}
	if err := w.functions.RunBuild(ctx, w.fn.last()); err != nil {
		t.Fatalf("RunBuild: %v", err)
	}
	return b.GetId()
}

// Verifies: FN-006, FN-009, FN-016, FN-031, FN-007.
// The handlers carry a source from upload to a deployed, aliased version
// and translate the domain's refusals: a viewer reads but does not deploy,
// a developer does not deploy to production, an unknown build is not found.
func TestFunctionAdminService_FN_006(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newMatrixFixture(t)
	w, owner, dev, viewer := f.w, f.owner(), f.callers["developer"], f.callers["viewer"]

	if _, err := w.uploadSource(t, viewer, f.app, f.dev, apiFunction, "x"); codeOf(err) != connect.CodePermissionDenied {
		t.Errorf("a viewer uploading: %v", err)
	}
	id := w.builtBy(t, dev, f.app, f.dev, apiFunction)
	b := must(w.functionAdmin.GetBuild(ctx, req(viewer, &pluxv1.GetBuildRequest{BuildId: id})))(t).GetBuild()
	if b.GetState() != pluxv1.BuildState_BUILD_STATE_SUCCEEDED || len(b.GetTestResults()) != 1 || !b.GetTestResults()[0].GetPassed() {
		t.Fatalf("build = %v", b)
	}
	if _, err := w.functionAdmin.Deploy(ctx, req(viewer, &pluxv1.DeployRequest{BuildId: id})); codeOf(err) != connect.CodePermissionDenied {
		t.Errorf("a viewer deploying: %v", err)
	}
	v := must(w.functionAdmin.Deploy(ctx, req(dev, &pluxv1.DeployRequest{BuildId: id, Alias: "stable"})))(t).GetVersion()
	if v.GetVersion() != 1 || v.GetPlacement() != "server" || len(v.GetModuleSha256()) != 64 || !v.GetTestsPassed() || v.GetProvenance().GetBuilderIdentity() != "builder-test" {
		t.Errorf("version = %v", v)
	}
	again := must(w.functionAdmin.Deploy(ctx, req(dev, &pluxv1.DeployRequest{BuildId: id})))(t).GetVersion()
	if again.GetVersion() != 2 {
		t.Errorf("the second deploy is version %d", again.GetVersion())
	}
	aliases := must(w.functionAdmin.SetAlias(ctx, req(dev, &pluxv1.SetAliasRequest{
		AppId: f.app, EnvironmentId: f.dev, Name: apiFunction, Alias: &pluxv1.Alias{Name: "prod", Version: 2},
	})))(t).GetAliases()
	if len(aliases) != 2 || aliases[0].GetName() != "prod" || aliases[0].GetVersion() != 2 {
		t.Errorf("aliases = %v", aliases)
	}
	got := must(w.functionAdmin.GetFunction(ctx, req(viewer, &pluxv1.GetFunctionRequest{AppId: f.app, EnvironmentId: f.dev, Name: apiFunction})))(t)
	if got.GetLatest().GetVersion() != 2 || len(got.GetAliases()) != 2 {
		t.Errorf("GetFunction = %v", got)
	}
	list := must(w.functionAdmin.ListFunctions(ctx, req(viewer, &pluxv1.ListFunctionsRequest{AppId: f.app, EnvironmentId: f.dev})))(t)
	if len(list.GetFunctions()) != 1 || list.GetFunctions()[0].GetVersion() != 2 {
		t.Errorf("ListFunctions = %v", list)
	}
	first := must(w.functionAdmin.ListVersions(ctx, req(viewer, &pluxv1.ListVersionsRequest{
		AppId: f.app, EnvironmentId: f.dev, Name: apiFunction, Page: &pluxv1.Page{PageSize: 1},
	})))(t)
	if len(first.GetVersions()) != 1 || first.GetVersions()[0].GetVersion() != 2 || first.GetPage().GetNextPageToken() == "" {
		t.Fatalf("first page = %v", first)
	}
	next := must(w.functionAdmin.ListVersions(ctx, req(viewer, &pluxv1.ListVersionsRequest{
		AppId: f.app, EnvironmentId: f.dev, Name: apiFunction, Page: &pluxv1.Page{PageSize: 1, PageToken: first.GetPage().GetNextPageToken()},
	})))(t)
	if len(next.GetVersions()) != 1 || next.GetVersions()[0].GetVersion() != 1 {
		t.Errorf("second page = %v", next)
	}

	if _, err := w.functionAdmin.GetFunction(ctx, req(viewer, &pluxv1.GetFunctionRequest{AppId: f.app, EnvironmentId: f.dev, Name: "no.such"})); codeOf(err) != connect.CodeNotFound {
		t.Errorf("an unknown function: %v", err)
	}
	if _, err := w.functionAdmin.Deploy(ctx, req(owner, &pluxv1.DeployRequest{BuildId: unknownID})); codeOf(err) != connect.CodeNotFound {
		t.Errorf("an unknown build: %v", err)
	}
	prod := w.builtBy(t, dev, f.app, f.prod, apiFunction)
	if _, err := w.functionAdmin.Deploy(ctx, req(dev, &pluxv1.DeployRequest{BuildId: prod})); codeOf(err) != connect.CodePermissionDenied {
		t.Errorf("a developer deploying to production: %v", err)
	}
	if _, err := w.functionAdmin.Deploy(ctx, req(owner, &pluxv1.DeployRequest{BuildId: prod})); err != nil {
		t.Errorf("the owner deploying to production: %v", err)
	}
	w.functions.Record(ctx, function.LogMeta{OrganizationID: f.org, AppID: f.app, EnvironmentID: f.dev, Function: apiFunction, Version: 2},
		[]function.LogLine{{Level: 1, Message: "first", Time: time.Now().Add(-time.Second)}, {Level: 3, Message: "second", Time: time.Now()}})
	w.functions.Observe(ctx, function.Observation{OrganizationID: f.org, AppID: f.app, EnvironmentID: f.dev, Function: apiFunction, Version: 2, Result: "ok", Duration: 3 * time.Millisecond})
	w.functions.Observe(ctx, function.Observation{OrganizationID: f.org, AppID: f.app, EnvironmentID: f.dev, Function: apiFunction, Version: 2, Result: "function_timeout"})
	logs := must(w.functionAdmin.ListLogs(ctx, req(viewer, &pluxv1.ListLogsRequest{AppId: f.app, EnvironmentId: f.dev, Name: apiFunction, Version: 2, Page: &pluxv1.Page{PageSize: 1}})))(t)
	if len(logs.GetLines()) != 1 || logs.GetLines()[0].GetMessage() != "second" || logs.GetLines()[0].GetLevel() != "error" || logs.GetPage().GetNextPageToken() == "" {
		t.Fatalf("ListLogs first page = %v", logs)
	}
	more := must(w.functionAdmin.ListLogs(ctx, req(viewer, &pluxv1.ListLogsRequest{
		AppId: f.app, EnvironmentId: f.dev, Name: apiFunction, Version: 2, Page: &pluxv1.Page{PageSize: 1, PageToken: logs.GetPage().GetNextPageToken()},
	})))(t)
	if len(more.GetLines()) != 1 || more.GetLines()[0].GetMessage() != "first" {
		t.Errorf("ListLogs second page = %v", more)
	}
	metrics := must(w.functionAdmin.GetMetrics(ctx, req(viewer, &pluxv1.GetMetricsRequest{AppId: f.app, EnvironmentId: f.dev, Name: apiFunction, WindowSeconds: 600})))(t)
	if metrics.GetInvocations() != 2 || metrics.GetErrorsByKind()["function_timeout"] != 1 || metrics.GetLatencyP99Micros() == 0 {
		t.Errorf("GetMetrics = %v", metrics)
	}
	if _, err := w.uploadSource(t, dev, f.app, f.dev, apiFunction, strings.Repeat("x", 4)); err != nil {
		t.Errorf("a second upload: %v", err)
	}
}

// freshSource uploads a source as the owner and returns its identifier.
func (f *matrixFixture) freshSource(t *testing.T) string {
	t.Helper()
	res, err := f.w.uploadSource(t, f.owner(), f.app, f.dev, apiFunction, "archive "+f.name())
	return must(connect.NewResponse(res), err)(t).GetSourceId()
}

// freshFunctionBuild builds a function in an environment as the owner and
// returns the build's identifier.
func (f *matrixFixture) freshFunctionBuild(t *testing.T, envID string) string {
	t.Helper()
	return f.w.builtBy(t, f.owner(), f.app, envID, apiFunction)
}

// Verifies: FN-033.
// The language-server handlers translate a session: Open streams the opened
// event, then the server's messages, then the close; Send is accepted for the
// person who opened the session and refused for anyone else without revealing
// it; an unknown source is not found, and a session cannot be sent to once over.
func TestLanguageServerService_FN_033(t *testing.T) {
	t.Parallel()
	f := newMatrixFixture(t)
	w, dev, owner, viewer := f.w, f.callers["developer"], f.owner(), f.callers["viewer"]
	w.fn.mu.Lock()
	w.fn.hold = true
	w.fn.mu.Unlock()
	src := f.freshSource(t)

	if err := drain(w.languageServer.Open(context.Background(), req(dev, &pluxv1.LanguageServerServiceOpenRequest{SourceId: "no-such-source"}))); codeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("a malformed source identifier: %v; want invalid argument", err)
	}
	stream, err := w.languageServer.Open(context.Background(), req(dev, &pluxv1.LanguageServerServiceOpenRequest{SourceId: src}))
	if err != nil {
		t.Fatal(err)
	}
	if !stream.Receive() {
		t.Fatalf("no opened event: %v", stream.Err())
	}
	opened := stream.Msg().GetOpened()
	if opened.GetSessionId() == "" || opened.GetSessionSeconds() != 60 || !strings.HasSuffix(opened.GetRootUri(), "/"+apiFunction+"/") {
		t.Fatalf("opened = %v", stream.Msg())
	}
	send := func(c caller, seq uint64) error {
		_, err := w.languageServer.Send(context.Background(), req(c, &pluxv1.LanguageServerServiceSendRequest{
			SessionId: opened.GetSessionId(), Sequence: seq, Message: []byte(`{"jsonrpc":"2.0"}`),
		}))
		return err
	}
	if err := send(dev, 1); err != nil {
		t.Fatalf("the owner's message: %v", err)
	}
	if !stream.Receive() || string(stream.Msg().GetMessage()) != `echo:{"jsonrpc":"2.0"}` {
		t.Fatalf("message = %v, %v", stream.Msg(), stream.Err())
	}
	if codeOf(send(owner, 2)) != connect.CodeNotFound {
		t.Errorf("another person's message: %v; want not found", send(owner, 2))
	}
	if codeOf(send(viewer, 2)) != connect.CodePermissionDenied {
		t.Errorf("a viewer's message: %v; want permission denied", send(viewer, 2))
	}

	w.fn.outbox <- nil
	if !stream.Receive() || stream.Msg().GetClosed().GetReason() != "exited" || stream.Msg().GetClosed().GetDetail() != "bye" {
		t.Fatalf("closed = %v, %v", stream.Msg(), stream.Err())
	}
	for stream.Receive() {
	}
	if stream.Err() != nil {
		t.Errorf("the stream ended with %v", stream.Err())
	}
	if codeOf(send(dev, 3)) != connect.CodeNotFound {
		t.Errorf("a message after the end: %v; want not found", send(dev, 3))
	}
}
