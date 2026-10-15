// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/nightCode42/plux3/backend/internal/fnpb"
	"github.com/nightCode42/plux3/backend/internal/fnpb/fnpbconnect"
	"github.com/nightCode42/plux3/backend/internal/function"
)

// lockedBuffer is a log sink safe for concurrent use.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// Verifies: FN-016, SEC-043, L-4.
// A process that runs only the builder role starts without a database,
// serves BuilderService over mutual TLS, warns about a build without a
// sandbox when it starts, and drains on shutdown.
func TestBuilderRoleRunsAlone_FN_016(t *testing.T) {
	t.Parallel()
	certFile, keyFile, pool := identity(t)
	addr := freeAddr(t)
	cfg := testConfig(t, "server:\n  roles: [builder]\n  shutdownGrace: 5s\nbuilder:\n  listen: \""+addr+"\"\n  sandbox: none\n"+
		"  sdkDir: /opt/plux/sdk/plux-fn\n  wasmOpt: [bun, /opt/binaryen/wasm-opt]\n"+
		"  internalTLS:\n    caFile: "+certFile+"\n    certFile: "+certFile+"\n    keyFile: "+keyFile+"\n")
	if !cfg.NoDataAccess() || cfg.Database.URL != "" {
		t.Fatalf("configuration = %+v", cfg.Server.Roles)
	}
	var logs lockedBuffer
	log := slog.New(slog.NewTextHandler(&logs, nil))
	built, err := Build(context.Background(), cfg, log, "test")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer built.Close()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- built.Server.Run(ctx) }()

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		t.Fatal(err)
	}
	hc := &http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{RootCAs: pool, Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS13}, ForceAttemptHTTP2: true,
	}, Timeout: 10 * time.Second}
	client := fnpbconnect.NewBuilderServiceClient(hc, "https://"+addr)
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, err = client.Build(context.Background(), connect.NewRequest(&fnpb.BuildRequest{}))
		if connect.CodeOf(err) == connect.CodeInvalidArgument {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the builder listener never answered: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !strings.Contains(logs.String(), "builder.sandbox is none") {
		t.Errorf("the start did not warn about the missing sandbox; log:\n%s", logs.String())
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the process did not drain")
	}
}

// fakeRunner is the function service as the build worker sees it.
type fakeRunner struct {
	runErr   error
	failed   []int
	failErr  error
	ranCount int
}

func (f *fakeRunner) RunBuild(context.Context, function.BuildJob) error {
	f.ranCount++
	return f.runErr
}

func (f *fakeRunner) FailBuild(_ context.Context, _ function.BuildJob, attempts int) error {
	f.failed = append(f.failed, attempts)
	return f.failErr
}

// job is a River job at an attempt.
// maxAttempts is how often a build job is tried (JobQueue.insert).
const maxAttempts = 5

func job(attempt int) *river.Job[function.BuildJob] {
	return &river.Job[function.BuildJob]{JobRow: &rivertype.JobRow{Attempt: attempt, MaxAttempts: maxAttempts}, Args: function.BuildJob{BuildID: "b1"}}
}

// Verifies: FN-016.
// A failure before the last attempt is retried and leaves the build alone;
// on the last attempt the build is failed so none stays running.
func TestBuildWorkerFailsTheBuildAfterTheLastAttempt_FN_016(t *testing.T) {
	t.Parallel()
	boom := errors.New("builder unreachable")
	r := &fakeRunner{runErr: boom}
	w := &functionBuildWorker{runner: r, log: discard()}
	for attempt := 1; attempt < maxAttempts; attempt++ {
		if err := w.Work(context.Background(), job(attempt)); !errors.Is(err, boom) {
			t.Fatalf("attempt %d: %v", attempt, err)
		}
	}
	if len(r.failed) != 0 {
		t.Fatalf("the build was failed before the last attempt: %v", r.failed)
	}
	if err := w.Work(context.Background(), job(maxAttempts)); !errors.Is(err, boom) {
		t.Errorf("the last attempt returns the failure: %v", err)
	}
	if len(r.failed) != 1 || r.failed[0] != 5 {
		t.Errorf("failed with attempts %v; want [5]", r.failed)
	}
	ok := &fakeRunner{}
	if err := (&functionBuildWorker{runner: ok, log: discard()}).Work(context.Background(), job(maxAttempts)); err != nil || len(ok.failed) != 0 {
		t.Errorf("a build that succeeded on its last attempt: %v, failed %v", err, ok.failed)
	}
	failing := &fakeRunner{runErr: boom, failErr: errors.New("database down")}
	if err := (&functionBuildWorker{runner: failing, log: discard()}).Work(context.Background(), job(maxAttempts)); err == nil || !strings.Contains(err.Error(), "database down") {
		t.Errorf("a failure to fail the build must be reported with the first: %v", err)
	}
}
