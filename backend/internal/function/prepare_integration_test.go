// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package function_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/fnpb"
	"github.com/nightCode42/plux3/backend/internal/fnpb/fnpbconnect"
	"github.com/nightCode42/plux3/backend/internal/function"
)

// fakePreparer records the versions the service asked the runner to
// prepare, and fails when err is set.
type fakePreparer struct {
	mu       sync.Mutex
	versions []function.Version
	orgs     []string
	deadline []time.Duration
	err      error
}

func (f *fakePreparer) Prepare(ctx context.Context, organizationID string, v function.Version) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.versions = append(f.versions, v)
	f.orgs = append(f.orgs, organizationID)
	if d, ok := ctx.Deadline(); ok {
		f.deadline = append(f.deadline, time.Until(d))
	} else {
		f.deadline = append(f.deadline, -1)
	}
	return f.err
}

func (f *fakePreparer) calls() []function.Version {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]function.Version(nil), f.versions...)
}

// prepareCounter counts preparations by result.
type prepareCounter struct {
	mu     sync.Mutex
	counts map[string]int
}

func (c *prepareCounter) Prepared(result string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.counts == nil {
		c.counts = map[string]int{}
	}
	c.counts[result]++
}

func (c *prepareCounter) count(result string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.counts[result]
}

// syncBuffer is a log destination safe for the service's use.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// Verifies: FN-023, FN-010.
// A deploy and a promotion of a server-placed version ask the runner to
// compile its module, once the version is stored, within a bounded time; a
// device-placed version is not prepared.
func TestDeployAndPromotePrepareTheModule_FN_023(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := newWorld(t, nil, nil)
	prep, counter := &fakePreparer{}, &prepareCounter{}
	w.svc = w.service(t, func(o *function.Options) { o.Preparer, o.PrepareMetrics = prep, counter })

	v1 := w.deployWith(t, "development", fnName, function.PlacementServer, `{"name":"`+fnName+`","placement":"server"}`, "")
	got := prep.calls()
	if len(got) != 1 || got[0].Name != fnName || got[0].Version != v1.Version || got[0].EnvironmentID != w.envs["development"] ||
		got[0].AppID != w.app || !bytes.Equal(got[0].ModuleSHA256, v1.ModuleSHA256) || prep.orgs[0] != w.org {
		t.Fatalf("after Deploy the runner was asked to prepare %+v (orgs %v), want version %+v", got, prep.orgs, v1)
	}
	if d := prep.deadline[0]; d <= 0 || d > function.PrepareTimeout {
		t.Errorf("the preparation ran with %v left, want a deadline within %v", d, function.PrepareTimeout)
	}

	if _, err := w.svc.Promote(ctx, w.owner, w.app, w.envs["development"], w.envs["production"], fnName, v1.Version, ""); err != nil {
		t.Fatalf("Promote: %v", err)
	}
	got = prep.calls()
	if len(got) != 2 || got[1].EnvironmentID != w.envs["production"] || got[1].Version != v1.Version {
		t.Errorf("after Promote the runner was asked to prepare %+v", got)
	}

	w.deployWith(t, "development", "loans.device", function.PlacementDevice, `{"name":"loans.device","placement":"device"}`, "")
	if n := len(prep.calls()); n != 2 {
		t.Errorf("a device-placed version was prepared: %d calls", n)
	}
	if n := counter.count(function.PrepareOK); n != 2 {
		t.Errorf("ok preparations counted = %d, want 2", n)
	}
}

// Verifies: FN-023.
// A runner that cannot prepare the module does not fail the deploy or the
// promotion: the failure is logged, without the grant, and counted.
func TestPrepareFailureNeverFailsADeploy_FN_023(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := newWorld(t, nil, nil)
	prep, counter, logs := &fakePreparer{err: errors.New("the runner is down")}, &prepareCounter{}, &syncBuffer{}
	w.svc = w.service(t, func(o *function.Options) {
		o.Preparer, o.PrepareMetrics = prep, counter
		o.Log = slog.New(slog.NewTextHandler(logs, nil))
	})

	v := w.deployWith(t, "development", fnName, function.PlacementServer, `{"name":"`+fnName+`","placement":"server"}`, "stable")
	if v.Version != 1 {
		t.Fatalf("the deploy returned %+v", v)
	}
	if _, err := w.svc.Promote(ctx, w.owner, w.app, w.envs["development"], w.envs["production"], fnName, v.Version, "stable"); err != nil {
		t.Fatalf("Promote with a failing runner: %v", err)
	}
	if n := counter.count(function.PrepareFailed); n != 2 {
		t.Errorf("failed preparations counted = %d, want 2", n)
	}
	if n := counter.count(function.PrepareOK); n != 0 {
		t.Errorf("ok preparations counted = %d, want 0", n)
	}
	out := logs.String()
	if strings.Count(out, "preparing a function module failed") != 2 || !strings.Contains(out, "the runner is down") {
		t.Errorf("the log = %q, want two warnings with the cause", out)
	}
	list, err := w.svc.ListVersions(ctx, w.owner, w.app, w.envs["production"], fnName, 0, 10)
	if err != nil || len(list) != 1 {
		t.Errorf("the promoted version was not stored: %+v, %v", list, err)
	}
}

// fakeRunnerClient records the Prepare requests of a ConnectPreparer.
type fakeRunnerClient struct {
	fnpbconnect.FunctionRunnerServiceClient
	reqs []*fnpb.PrepareRequest
	err  error
}

func (f *fakeRunnerClient) Prepare(_ context.Context, r *connect.Request[fnpb.PrepareRequest]) (*connect.Response[fnpb.PrepareResponse], error) {
	f.reqs = append(f.reqs, r.Msg)
	if f.err != nil {
		return nil, f.err
	}
	return connect.NewResponse(&fnpb.PrepareResponse{}), nil
}

// Verifies: FN-023, FN-012.
// The grant of a preparation reaches exactly the version's module, so the
// runner can fetch it and nothing else.
func TestConnectPreparerIssuesAModuleGrant_FN_023(t *testing.T) {
	t.Parallel()
	grants, err := function.NewGrants(bytes.Repeat([]byte{7}, 32), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &fakeRunnerClient{}
	v := function.Version{AppID: "app", EnvironmentID: "env", Name: fnName, Version: 4, ModuleSHA256: bytes.Repeat([]byte{0xab}, 32)}
	if err := (function.ConnectPreparer{Client: client, Grants: grants}).Prepare(context.Background(), "org", v); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if len(client.reqs) != 1 {
		t.Fatalf("requests = %d, want 1", len(client.reqs))
	}
	req := client.reqs[0]
	digest := strings.Repeat("ab", 32)
	if req.GetModuleSha256() != digest {
		t.Errorf("module_sha256 = %q, want %q", req.GetModuleSha256(), digest)
	}
	c, err := grants.Verify(function.Grant(req.GetGrant()))
	if err != nil {
		t.Fatalf("the grant does not verify: %v", err)
	}
	if c.OrganizationID != "org" || c.AppID != "app" || c.EnvironmentID != "env" || c.Function != fnName || c.Version != 4 ||
		c.ModuleSHA256 != digest || c.Depth != 0 {
		t.Errorf("claims = %+v", c)
	}

	client.err = connect.NewError(connect.CodeUnavailable, errors.New("down"))
	if err := (function.ConnectPreparer{Client: client, Grants: grants}).Prepare(context.Background(), "org", v); err == nil {
		t.Error("a refused Prepare returned no error")
	}
}
