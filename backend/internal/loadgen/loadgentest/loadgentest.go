// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Package loadgentest provides a fake device endpoint for the tests of the
// load generator. It speaks the device protocol (registration, token,
// manifest, signed control, telemetry, function invocation) and verifies
// every DPoP proof with the server's own verifier, so a generator that signs
// wrongly fails here as it would against the real server. It keeps no data
// beyond what a test asks about.
package loadgentest

import (
	"context"
	"crypto/rand"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/dpop"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
)

// Server is a fake device endpoint.
type Server struct {
	// URL is where devices reach it.
	URL string

	nonces   *dpop.Nonces
	verifier dpop.Verifier

	mu       sync.Mutex
	calls    map[string]int
	events   []string
	tokens   []string
	devices  int
	control  int64
	seenJTIs map[string]bool

	// FailTelemetry, when set, makes IngestEvents fail with that code.
	failTelemetry atomic.Int32
}

// New starts a fake endpoint that serves until the test ends.
func New(t testing.TB) *Server {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	nonces, err := dpop.NewNonces(key, 5*time.Minute, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{nonces: nonces, calls: map[string]int{}, seenJTIs: map[string]bool{}, control: 1}
	s.verifier = dpop.Verifier{Nonces: nonces}
	mux := http.NewServeMux()
	opts := connect.WithInterceptors(s)
	for _, h := range []func() (string, http.Handler){
		func() (string, http.Handler) { return pluxv1connect.NewDeviceServiceHandler(deviceHandler{s: s}, opts) },
		func() (string, http.Handler) { return pluxv1connect.NewTokenServiceHandler(tokenHandler{s: s}, opts) },
		func() (string, http.Handler) {
			return pluxv1connect.NewManifestServiceHandler(manifestHandler{s: s}, opts)
		},
		func() (string, http.Handler) {
			return pluxv1connect.NewTelemetryServiceHandler(telemetryHandler{s: s}, opts)
		},
		func() (string, http.Handler) {
			return pluxv1connect.NewFunctionServiceHandler(functionHandler{s: s}, opts)
		},
	} {
		mux.Handle(h())
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	s.URL = srv.URL
	return s
}

// SetControlVersion publishes a control document of the given version.
func (s *Server) SetControlVersion(v int64) {
	s.mu.Lock()
	s.control = v
	s.mu.Unlock()
}

// FailTelemetry makes IngestEvents answer with the given Connect code from
// now on; zero restores it.
func (s *Server) FailTelemetry(code connect.Code) { s.failTelemetry.Store(int32(code)) } //nolint:gosec // a Connect code is small

// Calls returns how many calls the procedure (for example
// "ManifestService/GetManifest") received.
func (s *Server) Calls(procedure string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls["/plux.v1."+procedure]
}

// Events returns the names of the telemetry events received, in order.
func (s *Server) Events() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.events...)
}

// PushTokens returns the push tokens devices registered.
func (s *Server) PushTokens() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.tokens...)
}

// WrapUnary verifies the DPoP proof of every call, which is what the
// server's authentication does for the device procedures, and answers with
// the current nonce.
func (s *Server) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, r connect.AnyRequest) (connect.AnyResponse, error) {
		procedure := r.Spec().Procedure
		if err := s.verify(r, procedure); err != nil {
			err.Meta().Set("DPoP-Nonce", s.nonces.Current())
			return nil, err
		}
		s.mu.Lock()
		s.calls[procedure]++
		s.mu.Unlock()
		res, err := next(ctx, r)
		if res != nil {
			res.Header().Set("DPoP-Nonce", s.nonces.Current())
		}
		return res, err
	}
}

// verify checks the proof and, for a call with an access token, that the
// token is the one the fake issued. It also refuses a proof used twice.
func (s *Server) verify(r connect.AnyRequest, procedure string) *connect.Error {
	token := strings.TrimPrefix(r.Header().Get("Authorization"), "DPoP ")
	if r.Header().Get("Authorization") == "" {
		token = ""
	}
	if token != "" && token != accessToken {
		return connect.NewError(connect.CodeUnauthenticated, errors.New("unknown access token"))
	}
	proof, err := s.verifier.Verify(r.Header().Get("DPoP"), dpop.Expect{
		Method: http.MethodPost, URL: s.URL + procedure, AccessToken: token, Window: time.Minute,
	})
	if err != nil {
		e := connect.NewError(connect.CodeUnauthenticated, err)
		e.Meta().Set("WWW-Authenticate", `DPoP error="use_dpop_nonce"`)
		return e
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.seenJTIs[proof.JTI] {
		return connect.NewError(connect.CodeUnauthenticated, errors.New("proof replayed"))
	}
	s.seenJTIs[proof.JTI] = true
	return nil
}

// WrapStreamingClient is part of the interceptor interface.
func (*Server) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

// WrapStreamingHandler is part of the interceptor interface.
func (*Server) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return next
}

// accessToken is the token the fake issues to every device.
const accessToken = "loadgentest-access-token"

type deviceHandler struct {
	pluxv1connect.UnimplementedDeviceServiceHandler
	s *Server
}

func (deviceHandler) CreateRegistrationChallenge(context.Context, *connect.Request[pluxv1.CreateRegistrationChallengeRequest]) (*connect.Response[pluxv1.CreateRegistrationChallengeResponse], error) {
	return connect.NewResponse(&pluxv1.CreateRegistrationChallengeResponse{Challenge: []byte("challenge")}), nil
}

func (h deviceHandler) RegisterAttestedDevice(_ context.Context, _ *connect.Request[pluxv1.RegisterAttestedDeviceRequest]) (*connect.Response[pluxv1.RegisterAttestedDeviceResponse], error) {
	h.s.mu.Lock()
	h.s.devices++
	id := "device-" + strconv.Itoa(h.s.devices)
	h.s.mu.Unlock()
	return connect.NewResponse(&pluxv1.RegisterAttestedDeviceResponse{Device: &pluxv1.Device{Id: id}}), nil
}

func (h deviceHandler) RegisterPushToken(_ context.Context, r *connect.Request[pluxv1.RegisterPushTokenRequest]) (*connect.Response[pluxv1.RegisterPushTokenResponse], error) {
	h.s.mu.Lock()
	h.s.tokens = append(h.s.tokens, r.Msg.GetToken())
	h.s.mu.Unlock()
	return connect.NewResponse(&pluxv1.RegisterPushTokenResponse{}), nil
}

type tokenHandler struct {
	pluxv1connect.UnimplementedTokenServiceHandler
	s *Server
}

func (tokenHandler) RefreshDeviceToken(context.Context, *connect.Request[pluxv1.RefreshDeviceTokenRequest]) (*connect.Response[pluxv1.RefreshDeviceTokenResponse], error) {
	return connect.NewResponse(&pluxv1.RefreshDeviceTokenResponse{AccessToken: accessToken, TokenType: "DPoP"}), nil
}

type manifestHandler struct {
	pluxv1connect.UnimplementedManifestServiceHandler
	s *Server
}

const manifestETag = `"manifest-1"`

func (manifestHandler) GetManifest(_ context.Context, r *connect.Request[pluxv1.GetManifestRequest]) (*connect.Response[pluxv1.GetManifestResponse], error) {
	if r.Msg.GetIfNoneMatch() == manifestETag {
		return connect.NewResponse(&pluxv1.GetManifestResponse{NotModified: true, Etag: manifestETag}), nil
	}
	return connect.NewResponse(&pluxv1.GetManifestResponse{Manifest: &pluxv1.Manifest{}, Etag: manifestETag}), nil
}

func (h manifestHandler) GetSignedControl(_ context.Context, r *connect.Request[pluxv1.GetSignedControlRequest]) (*connect.Response[pluxv1.GetSignedControlResponse], error) {
	h.s.mu.Lock()
	version := h.s.control
	h.s.mu.Unlock()
	etag := `"control-` + strconv.FormatInt(version, 10) + `"`
	if r.Msg.GetIfNoneMatch() == etag {
		return connect.NewResponse(&pluxv1.GetSignedControlResponse{NotModified: true, Etag: etag, Version: version}), nil
	}
	return connect.NewResponse(&pluxv1.GetSignedControlResponse{Etag: etag, Version: version, Signed: []byte("signed")}), nil
}

type telemetryHandler struct {
	pluxv1connect.UnimplementedTelemetryServiceHandler
	s *Server
}

func (h telemetryHandler) IngestEvents(_ context.Context, r *connect.Request[pluxv1.IngestEventsRequest]) (*connect.Response[pluxv1.IngestEventsResponse], error) {
	if code := connect.Code(h.s.failTelemetry.Load()); code != 0 {
		return nil, connect.NewError(code, errors.New("telemetry refused"))
	}
	h.s.mu.Lock()
	for _, e := range r.Msg.GetEvents() {
		h.s.events = append(h.s.events, e.GetName())
	}
	h.s.mu.Unlock()
	return connect.NewResponse(&pluxv1.IngestEventsResponse{Accepted: int32(len(r.Msg.GetEvents()))}), nil //nolint:gosec // a batch is small
}

type functionHandler struct {
	pluxv1connect.UnimplementedFunctionServiceHandler
	s *Server
}

func (functionHandler) Invoke(context.Context, *connect.Request[pluxv1.FunctionServiceInvokeRequest]) (*connect.Response[pluxv1.FunctionServiceInvokeResponse], error) {
	return connect.NewResponse(&pluxv1.FunctionServiceInvokeResponse{Result: &pluxv1.FunctionServiceInvokeResponse_Output{Output: []byte("{}")}}), nil
}
