// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package buildclient

import (
	"context"
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
	"github.com/nightCode42/plux3/backend/internal/storage/objects"
)

// fakeEditorRole is a BuilderService answering the editor RPCs: a language
// session that streams the events a test sets, and a test run.
type fakeEditorRole struct {
	fnpbconnect.UnimplementedBuilderServiceHandler
	mu       sync.Mutex
	events   []*fnpb.LanguageOpenResponse
	openSeen *fnpb.LanguageOpenRequest
	sendSeen *fnpb.LanguageSendRequest
	test     *fnpb.TestResponse
	testSeen *fnpb.TestRequest
	err      error
}

func (f *fakeEditorRole) LanguageOpen(_ context.Context, req *connect.Request[fnpb.LanguageOpenRequest], stream *connect.ServerStream[fnpb.LanguageOpenResponse]) error {
	f.mu.Lock()
	f.openSeen = req.Msg
	events, err := f.events, f.err
	f.mu.Unlock()
	if err != nil {
		return err
	}
	for _, ev := range events {
		if err := stream.Send(ev); err != nil {
			return err
		}
	}
	return nil
}

func (f *fakeEditorRole) LanguageSend(_ context.Context, req *connect.Request[fnpb.LanguageSendRequest]) (*connect.Response[fnpb.LanguageSendResponse], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sendSeen = req.Msg
	if f.err != nil {
		return nil, f.err
	}
	return connect.NewResponse(&fnpb.LanguageSendResponse{}), nil
}

func (f *fakeEditorRole) Test(_ context.Context, req *connect.Request[fnpb.TestRequest]) (*connect.Response[fnpb.TestResponse], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.testSeen = req.Msg
	if f.err != nil {
		return nil, f.err
	}
	return connect.NewResponse(f.test), nil
}

// newEditorRig is a client over a fake editor role and a store holding the
// source of the default rig.
func newEditorRig(t *testing.T) (*Client, *fakeEditorRole, function.EditorInput) {
	t.Helper()
	base := newRig(t)
	role := &fakeEditorRole{}
	mux := http.NewServeMux()
	mux.Handle(fnpbconnect.NewBuilderServiceHandler(role))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c, err := NewClient(Options{HTTP: srv.Client(), URL: srv.URL, Objects: base.client.objects, MaxResponseBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	return c, role, function.EditorInput{
		SourceObjectKey: base.in.SourceObjectKey, SourceSHA256: base.in.SourceSHA256, Name: base.in.Name,
		SessionMinutes: 30, MemoryBytes: 4 << 30, Timeout: time.Minute,
	}
}

func TestOpenLanguagePassesTheSessionEventsOn_FN_033(t *testing.T) {
	c, role, in := newEditorRig(t)
	role.events = []*fnpb.LanguageOpenResponse{
		{Event: &fnpb.LanguageOpenResponse_Opened{Opened: &fnpb.LanguageOpened{SessionId: "s1", RootUri: "file:///src"}}},
		{Event: &fnpb.LanguageOpenResponse_Message{Message: []byte(`{"jsonrpc":"2.0"}`)}},
		{},
		{Event: &fnpb.LanguageOpenResponse_Closed{Closed: &fnpb.LanguageClosed{Reason: "timeout", Detail: "30 minutes"}}},
	}
	var got []function.EditorEvent
	if err := c.OpenLanguage(t.Context(), in, func(ev function.EditorEvent) error {
		got = append(got, ev)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("events = %d, want 3 (an empty event is skipped)", len(got))
	}
	if o := got[0].Opened; o == nil || o.SessionID != "s1" || o.RootURI != "file:///src" || o.Seconds != 1800 {
		t.Errorf("opened = %+v", got[0].Opened)
	}
	if string(got[1].Message) != `{"jsonrpc":"2.0"}` {
		t.Errorf("message = %q", got[1].Message)
	}
	if cl := got[2].Closed; cl == nil || cl.Reason != "timeout" || cl.Detail != "30 minutes" {
		t.Errorf("closed = %+v", got[2].Closed)
	}
	seen := role.openSeen
	if seen.GetSourceSha256() != in.SourceSHA256 || seen.GetName() != in.Name || seen.GetSessionMinutes() != 30 || seen.GetMemoryBytes() != 4<<30 || len(seen.GetSource()) == 0 {
		t.Errorf("request = %+v", seen)
	}
}

func TestOpenLanguageStopsWhenTheReceiverFails_FN_033(t *testing.T) {
	c, role, in := newEditorRig(t)
	role.events = []*fnpb.LanguageOpenResponse{
		{Event: &fnpb.LanguageOpenResponse_Message{Message: []byte("a")}},
		{Event: &fnpb.LanguageOpenResponse_Message{Message: []byte("b")}},
	}
	stop := errors.New("socket closed")
	calls := 0
	err := c.OpenLanguage(t.Context(), in, func(function.EditorEvent) error {
		calls++
		return stop
	})
	if !errors.Is(err, stop) || calls != 1 {
		t.Fatalf("err = %v after %d calls, want the receiver's error after 1", err, calls)
	}
}

func TestOpenLanguageReportsAFailureOfTheRole_FN_033(t *testing.T) {
	c, role, in := newEditorRig(t)
	role.err = connect.NewError(connect.CodeUnavailable, errors.New("no sandbox"))
	err := c.OpenLanguage(t.Context(), in, func(function.EditorEvent) error { return nil })
	if connect.CodeOf(err) != connect.CodeUnavailable {
		t.Fatalf("err = %v, want unavailable", err)
	}
}

func TestEditorRefusesASourceThatChangedInTheStore_FN_005(t *testing.T) {
	c, _, in := newEditorRig(t)
	in.SourceSHA256 = "00" + in.SourceSHA256[2:]
	if err := c.OpenLanguage(t.Context(), in, func(function.EditorEvent) error { return nil }); err == nil {
		t.Error("OpenLanguage accepted a source whose hash does not match")
	}
	if _, err := c.Test(t.Context(), in); err == nil {
		t.Error("Test accepted a source whose hash does not match")
	}
	key, err := objects.Key(objects.KindSource, "11"+in.SourceSHA256[2:])
	if err != nil {
		t.Fatal(err)
	}
	in.SourceObjectKey = key
	if _, err := c.Test(t.Context(), in); err == nil {
		t.Error("Test accepted a source missing from the store")
	}
}

func TestSendLanguageForwardsTheMessage_FN_033(t *testing.T) {
	c, role, _ := newEditorRig(t)
	if err := c.SendLanguage(t.Context(), "s1", 7, []byte("hover")); err != nil {
		t.Fatal(err)
	}
	if s := role.sendSeen; s.GetSessionId() != "s1" || s.GetSequence() != 7 || string(s.GetMessage()) != "hover" {
		t.Errorf("request = %+v", s)
	}
	role.err = connect.NewError(connect.CodeNotFound, errors.New("no session"))
	if err := c.SendLanguage(t.Context(), "s2", 1, nil); connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("err = %v, want not found", err)
	}
}

func TestEditorTestConvertsResultsAndDiagnostics_FN_034(t *testing.T) {
	c, role, in := newEditorRig(t)
	role.test = &fnpb.TestResponse{
		TestResults:     []*fnpb.BuildTestResult{{Name: "fee", Passed: true}, {Name: "limit", Message: "expected 5"}},
		DiagnosticsJson: []byte(`[]`),
	}
	got, err := c.Test(t.Context(), in)
	if err != nil {
		t.Fatal(err)
	}
	want := []function.TestResult{{Name: "fee", Passed: true}, {Name: "limit", Message: "expected 5"}}
	if len(got.Results) != 2 || got.Results[0] != want[0] || got.Results[1] != want[1] {
		t.Errorf("results = %+v", got.Results)
	}
	b := role.testSeen.GetBuild()
	if b.GetBuildId() != "editor-test" || b.GetLimits().GetTimeoutMs() != 60000 || b.GetLimits().GetMemoryBytes() != 4<<30 {
		t.Errorf("request = %+v", b)
	}

	role.test = &fnpb.TestResponse{DiagnosticsJson: []byte(`{`)}
	if _, err := c.Test(t.Context(), in); err == nil {
		t.Error("Test accepted malformed diagnostics")
	}
	in.Timeout = 0
	role.err = connect.NewError(connect.CodeInternal, errors.New("crashed"))
	if _, err := c.Test(t.Context(), in); connect.CodeOf(err) != connect.CodeInternal {
		t.Errorf("err = %v, want internal", err)
	}
}
