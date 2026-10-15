// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/device"
	"github.com/nightCode42/plux3/backend/internal/function"
	"github.com/nightCode42/plux3/backend/internal/gateway"
	"github.com/nightCode42/plux3/backend/internal/httpx"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// GatewayService serves the unary form of the secure gateway
// (SEC-030–SEC-032).
type GatewayService struct {
	pluxv1connect.UnimplementedGatewayServiceHandler
	h *Handlers
}

var _ pluxv1connect.GatewayServiceHandler = GatewayService{}

// GatewayService returns the GatewayService handler.
func (h *Handlers) GatewayService() GatewayService { return GatewayService{h: h} }

// Call forwards one request to a registered upstream (SEC-030). Every
// check the gateway makes happens before the upstream hears of it.
func (s GatewayService) Call(ctx context.Context, req *connect.Request[pluxv1.CallRequest]) (*connect.Response[pluxv1.CallResponse], error) {
	d, err := deviceOf(ctx)
	if err != nil {
		return nil, err
	}
	m := req.Msg
	header := http.Header{}
	for name, value := range m.GetHeaders() {
		header.Add(name, value)
	}
	version, err := configVersion(req.Header())
	if err != nil {
		return nil, err
	}
	p, err := s.h.Gateway.Begin(ctx, d, gateway.Request{
		Upstream: m.GetUpstream(), Operation: m.GetOperation(), Plugin: m.GetPlugin(), Method: m.GetMethod(), Target: m.GetPath(),
		Header: header, UserToken: req.Header().Get(headerUserToken), ChallengeID: m.GetTransactionChallengeId(), Signature: m.GetTransactionSignature(),
		ConfigVersion: version,
	})
	if err != nil {
		return nil, withEpoch(ctx, s.h.Gateway, d, err)
	}
	var body []byte
	if len(m.GetBody()) > 0 || p.NeedsBody() {
		body = m.GetBody()
		if body == nil {
			body = []byte{}
		}
	}
	epoch, hasEpoch := p.ConfigEpoch()
	refuse := func(err error) error {
		if hasEpoch {
			return &withHeader{err: err, header: epochHeader(epoch)}
		}
		return err
	}
	plan, err := p.Finish(ctx, body)
	if err != nil {
		return nil, refuse(err)
	}
	res, err := s.h.Gateway.Do(ctx, plan)
	if err != nil {
		return nil, refuse(err)
	}
	headers := make(map[string]string, len(res.Header))
	for name, values := range res.Header {
		headers[name] = strings.Join(values, ", ")
	}
	out := connect.NewResponse(&pluxv1.CallResponse{Status: int32(res.Status), Headers: headers, Body: res.Body}) //nolint:gosec // an HTTP status
	if hasEpoch {
		out.Header().Set(HeaderConfigEpoch, strconv.FormatInt(epoch, 10))
	}
	return out, nil
}

// ScaService serves the challenges of strong customer authentication
// (SEC-027).
type ScaService struct {
	pluxv1connect.UnimplementedScaServiceHandler
	h *Handlers
}

var _ pluxv1connect.ScaServiceHandler = ScaService{}

// ScaService returns the ScaService handler.
func (h *Handlers) ScaService() ScaService { return ScaService{h: h} }

// CreateTransactionChallenge returns a single-use challenge bound to the
// summary the user is shown (SEC-027, SEC-028).
func (s ScaService) CreateTransactionChallenge(ctx context.Context, req *connect.Request[pluxv1.CreateTransactionChallengeRequest]) (*connect.Response[pluxv1.CreateTransactionChallengeResponse], error) {
	d, err := deviceOf(ctx)
	if err != nil {
		return nil, err
	}
	m := req.Msg
	if m.GetDeviceId() != "" && m.GetDeviceId() != d.DeviceID {
		return nil, plxerr.New(plxerr.PermissionDenied, "a device asks for its own challenges only")
	}
	if m.GetFunction() != "" {
		return s.functionChallenge(ctx, d, m)
	}
	c, err := s.h.Gateway.CreateChallenge(ctx, d, m.GetDataSource(), m.GetOperation(), m.GetTransactionSummaryJcs())
	if err != nil {
		return nil, withEpoch(ctx, s.h.Gateway, d, err)
	}
	out := &pluxv1.CreateTransactionChallengeResponse{ChallengeId: c.ID, Challenge: c.Bytes, ExpiresAt: ts(c.ExpiresAt), TransactionSummaryJcs: c.Summary}
	for _, v := range c.Displayed {
		out.Displayed = append(out.Displayed, &pluxv1.DisplayedValue{Label: v.Label, Value: v.Value})
	}
	return connect.NewResponse(out), nil
}

// functionChallenge issues the challenge of a function invocation, a
// transaction whose operation is the function and its version and whose
// bound body is the input (FN-020, SEC-027, SEC-028).
func (s ScaService) functionChallenge(ctx context.Context, d device.Identity, m *pluxv1.CreateTransactionChallengeRequest) (*connect.Response[pluxv1.CreateTransactionChallengeResponse], error) {
	if s.h.FunctionInvoker == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("this server runs no server functions"))
	}
	if m.GetDataSource() != "" || m.GetOperation() != "" {
		return nil, plxerr.New(plxerr.InvalidFormat, "a challenge is for a function or for an upstream's operation, not both")
	}
	if m.GetFunctionVersion() == 0 || m.GetFunctionVersion() > math.MaxInt32 {
		return nil, plxerr.New(plxerr.FunctionNotFound, "no such function version")
	}
	c, err := s.h.FunctionInvoker.CreateChallenge(ctx, d, function.ChallengeRequest{
		Function: m.GetFunction(), Version: int32(m.GetFunctionVersion()), //nolint:gosec // bounded above
		InputSHA256: m.GetInputSha256(), Summary: m.GetTransactionSummaryJcs(),
	})
	if err != nil {
		return nil, err //nolint:wrapcheck // a domain error
	}
	return connect.NewResponse(&pluxv1.CreateTransactionChallengeResponse{
		ChallengeId: c.ID, Challenge: c.Bytes, ExpiresAt: ts(c.ExpiresAt), TransactionSummaryJcs: c.Summary,
	}), nil
}

// The request headers the streaming route reads its typed fields from,
// the equivalents of the fields of CallRequest. They are removed before
// the gateway sees the request's headers.
const (
	// GatewayRoutePrefix is where the streaming route is mounted:
	// /gateway/v1/{upstream}/{path...}.
	GatewayRoutePrefix = "/gateway/v1/"

	headerOperation   = "Plux-Operation"
	headerPlugin      = "Plux-Plugin"
	headerUserToken   = "Plux-User-Token" //nolint:gosec // G101: a header name, not a credential
	headerChallengeID = "Plux-Challenge-Id"
	headerSignature   = "Plux-Transaction-Signature"

	// headerConfigVersion carries the live-configuration version the
	// device used (DAT-096); HeaderConfigEpoch carries the environment's
	// configuration epoch on every response (DAT-095).
	headerConfigVersion = "Plux-Config-Version"
	HeaderConfigEpoch   = "Plux-Config-Epoch"
)

// configVersion reads Plux-Config-Version; absent means the device holds
// no live configuration.
func configVersion(h http.Header) (*int64, error) {
	v := h.Get(headerConfigVersion)
	if v == "" {
		return nil, nil //nolint:nilnil // absent is a valid answer
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 0 {
		return nil, plxerr.New(plxerr.InvalidFormat, "%s is a non-negative integer", headerConfigVersion)
	}
	return &n, nil
}

// epochHeader is the header every gateway response carries.
func epochHeader(epoch int64) http.Header {
	return http.Header{HeaderConfigEpoch: {strconv.FormatInt(epoch, 10)}}
}

// withEpoch attaches the configuration epoch to a refusal, when the
// gateway can tell it (DAT-095).
func withEpoch(ctx context.Context, g *gateway.Gateway, d device.Identity, err error) error {
	if epoch, ok := g.ConfigEpoch(ctx, d); ok {
		return &withHeader{err: err, header: epochHeader(epoch)}
	}
	return err
}

// GatewayRoute serves the HTTP form of the gateway, for bodies that must
// stream, WebSocket and server-sent events: a device calls
// /gateway/v1/{upstream}/{path} with the method it wants, its DPoP-bound
// access token and proof, and the typed fields as Plux- headers. It
// authenticates like any device call, runs the same checks as Call, and
// streams both bodies without buffering them; only a transaction's body is
// read, bounded by gateway.requestBytes (SEC-030, SEC-031).
type GatewayRoute struct {
	Auth    DeviceAuth
	Gateway *gateway.Gateway
}

// ServeHTTP handles one request.
func (g GatewayRoute) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	d, err := g.Auth.AuthenticateRequest(r.Context(), w, r)
	if err != nil {
		writeGatewayError(w, err)
		return
	}
	upstream, target, ok := splitGatewayPath(r)
	if !ok {
		writeGatewayError(w, plxerr.New(plxerr.GatewayUpstreamUnknown, "the path names no upstream"))
		return
	}
	signature, err := decodeSignature(r.Header.Get(headerSignature))
	if err != nil {
		writeGatewayError(w, err)
		return
	}
	version, err := configVersion(r.Header)
	if err != nil {
		writeGatewayError(w, err)
		return
	}
	req := gateway.Request{
		Upstream: upstream, Operation: r.Header.Get(headerOperation), Plugin: r.Header.Get(headerPlugin),
		Method: r.Method, Target: target, UserToken: r.Header.Get(headerUserToken),
		ChallengeID: r.Header.Get(headerChallengeID), Signature: signature, Header: withoutTypedFields(r.Header),
		ConfigVersion: version,
	}
	p, err := g.Gateway.Begin(r.Context(), d, req)
	if err != nil {
		if epoch, ok := g.Gateway.ConfigEpoch(r.Context(), d); ok {
			w.Header().Set(HeaderConfigEpoch, strconv.FormatInt(epoch, 10))
		}
		writeGatewayError(w, err)
		return
	}
	if epoch, ok := p.ConfigEpoch(); ok {
		w.Header().Set(HeaderConfigEpoch, strconv.FormatInt(epoch, 10))
	}
	var body []byte
	if p.NeedsBody() {
		if body, err = httpx.ReadAll(r.Body, p.MaxRequest()); err != nil {
			writeGatewayError(w, plxerr.New(plxerr.LimitExceeded, "the request body exceeds gateway.requestBytes = %d", p.MaxRequest()))
			return
		}
		if body == nil {
			body = []byte{}
		}
	}
	plan, err := p.Finish(r.Context(), body)
	if err != nil {
		writeGatewayError(w, err)
		return
	}
	g.Gateway.Proxy(w, r, plan, writeGatewayError)
}

// splitGatewayPath cuts /gateway/v1/{upstream}/{path} into the upstream
// and the path with its query, as the client sent them.
func splitGatewayPath(r *http.Request) (upstream, target string, ok bool) {
	rest := strings.TrimPrefix(r.URL.EscapedPath(), GatewayRoutePrefix)
	if rest == r.URL.EscapedPath() {
		return "", "", false
	}
	upstream, path, _ := strings.Cut(rest, "/")
	if upstream == "" {
		return "", "", false
	}
	target = "/" + path
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}
	return upstream, target, true
}

// decodeSignature reads the signature header, unpadded base64url.
func decodeSignature(v string) ([]byte, error) {
	if v == "" {
		return nil, nil
	}
	b, err := base64.RawURLEncoding.DecodeString(v)
	if err != nil {
		return nil, plxerr.New(plxerr.SCASignatureInvalid, "the signature is not base64url")
	}
	return b, nil
}

// withoutTypedFields removes the headers that stand for typed fields, so
// that the gateway sees only what the device meant for the upstream.
func withoutTypedFields(h http.Header) http.Header {
	out := h.Clone()
	for _, name := range []string{headerOperation, headerPlugin, headerUserToken, headerChallengeID, headerSignature, headerConfigVersion} {
		out.Del(name)
	}
	return out
}

// gatewayStatus maps a refusal to the HTTP status the streaming route
// answers with.
func gatewayStatus(err error) int {
	var plx *plxerr.Error
	if !errors.As(err, &plx) {
		return http.StatusInternalServerError
	}
	switch ConnectCode(plx.Code) {
	case connect.CodeUnauthenticated:
		return http.StatusUnauthorized
	case connect.CodePermissionDenied:
		return http.StatusForbidden
	case connect.CodeNotFound:
		return http.StatusNotFound
	case connect.CodeInvalidArgument, connect.CodeFailedPrecondition:
		return http.StatusBadRequest
	case connect.CodeResourceExhausted:
		if plx.Code == plxerr.GatewayRateLimited || plx.Code == plxerr.RateLimited {
			return http.StatusTooManyRequests
		}
		return http.StatusRequestEntityTooLarge
	case connect.CodeUnavailable:
		return http.StatusBadGateway
	}
	return http.StatusInternalServerError
}

// writeGatewayError answers a refusal as JSON with the Plux code and
// reason, never more than the refusal says.
func writeGatewayError(w http.ResponseWriter, err error) {
	var plx *plxerr.Error
	if !errors.As(err, &plx) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"code": plxerr.InternalServerError.String(), "reason": "INTERNAL_SERVER_ERROR"})
		return
	}
	status := gatewayStatus(err)
	if status == http.StatusTooManyRequests {
		w.Header().Set("Retry-After", "60")
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"code": plx.Code.String(), "reason": string(plx.Reason()), "message": plx.Message})
}
