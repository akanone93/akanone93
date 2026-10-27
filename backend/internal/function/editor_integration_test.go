// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package function_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/cache"
	"github.com/nightCode42/plux3/backend/internal/function"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

// fakeEditor stands in for the builder role: a session opens, reports its
// events and stays until the caller's context ends or the test releases it.
type fakeEditor struct {
	mu      sync.Mutex // guards inputs, sent
	inputs  []function.EditorInput
	sent    []uint64
	opened  chan string
	release chan struct{}
	tests   function.EditorTests
}

func newFakeEditor() *fakeEditor {
	return &fakeEditor{opened: make(chan string, 8), release: make(chan struct{})}
}

func (f *fakeEditor) OpenLanguage(ctx context.Context, in function.EditorInput, send func(function.EditorEvent) error) error {
	f.mu.Lock()
	f.inputs = append(f.inputs, in)
	id := fmt.Sprintf("session-%d", len(f.inputs))
	f.mu.Unlock()
	if err := send(function.EditorEvent{Opened: &function.EditorOpened{SessionID: id, RootURI: "file:///workspace/functions/x/"}}); err != nil {
		return err
	}
	f.opened <- id
	select {
	case <-ctx.Done():
	case <-f.release:
		return send(function.EditorEvent{Closed: &function.EditorClosed{Reason: "exited"}})
	}
	return nil
}

func (f *fakeEditor) SendLanguage(_ context.Context, _ string, seq uint64, _ []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, seq)
	return nil
}

func (f *fakeEditor) Test(context.Context, function.EditorInput) (function.EditorTests, error) {
	return f.tests, nil
}

// editorWorld is a world whose service has a fake editor.
func editorWorld(t *testing.T, sessions int64) (*world, *fakeEditor, string) {
	t.Helper()
	w := newWorld(t, nil, func(s *limits.Set) {
		tight, err := s.Tighten(limits.LspSessionsPerOrganization, limits.ScopeOrganization, sessions)
		if err != nil {
			t.Fatal(err)
		}
		*s = tight
	})
	ed := newFakeEditor()
	w.svc = w.service(t, func(o *function.Options) { o.Editor = ed })
	src, err := w.svc.UploadSource(t.Context(), w.developer, w.app, w.envs["development"], fnName, strings.NewReader("source"))
	if err != nil {
		t.Fatal(err)
	}
	return w, ed, src.ID
}

// Verifies: FN-033.
func TestEditorSessionIsOwnedBoundedAndAudited_FN_033(t *testing.T) {
	t.Parallel()
	w, ed, source := editorWorld(t, 1)

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- w.svc.OpenEditor(ctx, w.developer, source, func(function.EditorEvent) error { return nil })
	}()
	var id string
	select {
	case id = <-ed.opened:
	case <-time.After(20 * time.Second):
		t.Fatal("the session did not open")
	}
	if got := w.auditCount(t, audit.FunctionEditorStarted); got != 1 {
		t.Errorf("start entries = %d; want 1", got)
	}
	in := ed.inputs[0]
	if int64(in.SessionMinutes) != limits.Defaults().Get(limits.LspSessionMinutes) || in.Name != fnName {
		t.Errorf("the builder was given %+v; want the app's lsp.sessionMinutes and the function's name", in)
	}

	// Only the person who opened it may send, and only with the permission.
	if err := w.svc.SendEditor(t.Context(), w.developer, id, 1, []byte("{}")); err != nil {
		t.Fatalf("the owner's message was refused: %v", err)
	}
	if err := w.svc.SendEditor(t.Context(), w.owner, id, 2, []byte("{}")); !isCode(err, plxerr.ResourceNotFound) {
		t.Errorf("another person's message: %v; want a not-found refusal that does not reveal the session", err)
	}
	if err := w.svc.SendEditor(t.Context(), w.viewer, id, 2, []byte("{}")); !isCode(err, plxerr.PermissionDenied) {
		t.Errorf("a viewer's message: %v; want permission denied", err)
	}

	// The organisation's cap holds, and a refused start leaves no entry.
	err := w.svc.OpenEditor(t.Context(), w.developer, source, func(function.EditorEvent) error { return nil })
	if !isCode(err, plxerr.LimitExceeded) {
		t.Errorf("a second session beyond lsp.sessionsPerOrganization: %v; want LIMIT_EXCEEDED", err)
	}
	if got := w.auditCount(t, audit.FunctionEditorStarted); got != 1 {
		t.Errorf("start entries after a refused start = %d; want 1", got)
	}

	cancel()
	if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
		t.Errorf("OpenEditor ended with %v", err)
	}
	if got := w.auditCount(t, audit.FunctionEditorStopped); got != 1 {
		t.Errorf("stop entries = %d; want 1, written although the caller's context ended", got)
	}
	if err := w.svc.SendEditor(t.Context(), w.developer, id, 3, []byte("{}")); !isCode(err, plxerr.ResourceNotFound) {
		t.Errorf("a message after the end: %v; want not found", err)
	}
	// The slot is free again.
	next := make(chan error, 1)
	go func() {
		next <- w.svc.OpenEditor(t.Context(), w.developer, source, func(function.EditorEvent) error { return nil })
	}()
	<-ed.opened
	close(ed.release)
	if err := <-next; err != nil {
		t.Errorf("a session after the slot was freed: %v", err)
	}
}

// brokenCache is a shared cache that cannot count.
type brokenCache struct{ cache.Cache }

func (brokenCache) SetNX(context.Context, string, []byte, time.Duration) (bool, error) {
	return false, errors.New("cache down")
}

// Verifies: FN-033.
func TestEditorCapIsSharedAcrossReplicas_FN_033(t *testing.T) {
	t.Parallel()
	w, _, source := editorWorld(t, 1)
	shared := cache.NewMemory(time.Now)
	t.Cleanup(func() { _ = shared.Close() })
	first := w.service(t, func(o *function.Options) { o.Editor = newFakeEditor(); o.Shared = shared })
	edB := newFakeEditor()
	second := w.service(t, func(o *function.Options) { o.Editor = edB; o.Shared = shared })

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- first.OpenEditor(ctx, w.developer, source, func(function.EditorEvent) error { return nil })
	}()
	// The first replica's session is open once its slot is taken; wait for the audit entry.
	deadline := time.Now().Add(20 * time.Second)
	for w.auditCount(t, audit.FunctionEditorStarted) < 1 {
		if time.Now().After(deadline) {
			t.Fatal("the session did not open")
		}
		time.Sleep(10 * time.Millisecond)
	}
	err := second.OpenEditor(t.Context(), w.developer, source, func(function.EditorEvent) error { return nil })
	if !isCode(err, plxerr.LimitExceeded) {
		t.Errorf("a second replica beyond lsp.sessionsPerOrganization: %v; want LIMIT_EXCEEDED", err)
	}
	cancel()
	<-done
	// The slot went back to the shared cache, so the other replica may start.
	next := make(chan error, 1)
	go func() {
		next <- second.OpenEditor(t.Context(), w.developer, source, func(function.EditorEvent) error { return nil })
	}()
	<-edB.opened
	close(edB.release)
	if err := <-next; err != nil {
		t.Errorf("the other replica after the slot was freed: %v", err)
	}

	broken := w.service(t, func(o *function.Options) { o.Editor = newFakeEditor(); o.Shared = brokenCache{shared} })
	if err := broken.OpenEditor(t.Context(), w.developer, source, func(function.EditorEvent) error { return nil }); err == nil || isCode(err, plxerr.LimitExceeded) {
		t.Errorf("a cache that cannot count: %v; want the start refused with the failure", err)
	}
}

// Verifies: FN-033.
func TestEditorRefusesWithoutPermissionOrBuilder_FN_033(t *testing.T) {
	t.Parallel()
	w, _, source := editorWorld(t, 2)
	if err := w.svc.OpenEditor(t.Context(), w.viewer, source, func(function.EditorEvent) error { return nil }); !isCode(err, plxerr.PermissionDenied) {
		t.Errorf("a viewer: %v; want permission denied", err)
	}
	bare := w.service(t, func(o *function.Options) { o.Editor = nil })
	if err := bare.OpenEditor(t.Context(), w.developer, source, func(function.EditorEvent) error { return nil }); !isCode(err, plxerr.FunctionBuildFailed) {
		t.Errorf("a server without a builder: %v; want a refusal", err)
	}
	if _, err := bare.RunEditorTests(t.Context(), w.viewer, source); !isCode(err, plxerr.PermissionDenied) {
		t.Errorf("a viewer running tests: %v; want permission denied, builder or not", err)
	}
}

// Verifies: FN-034.
func TestEditorRunsTheStoredTests_FN_034(t *testing.T) {
	t.Parallel()
	w, ed, source := editorWorld(t, 2)
	ed.tests = function.EditorTests{Results: []function.TestResult{{Name: "a", Passed: true}, {Name: "b", Message: "expected 2"}}}
	got, err := w.svc.RunEditorTests(t.Context(), w.developer, source)
	if err != nil || len(got.Results) != 2 || got.Results[1].Message != "expected 2" {
		t.Errorf("RunEditorTests = %+v, %v; want the builder's results", got, err)
	}
}

// isCode reports whether err carries the Plux code.
func isCode(err error, code plxerr.Code) bool {
	got, ok := plxerr.CodeOf(err)
	return ok && got == code
}
