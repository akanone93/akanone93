// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// handshakeKey is the nonce of RFC 6455's handshake example, whose accept
// key the tests know.
var handshakeKey = base64.StdEncoding.EncodeToString([]byte("the sample nonce"))

// testAPI is the API under test with the gateway it trusts.
type testAPI struct {
	*httptest.Server
	gw  *testGateway
	api *api
}

// testServer starts the API over TLS, as httptest does, trusting a test
// gateway whose keys it publishes.
func testServer(t *testing.T, o Options) *testAPI {
	t.Helper()
	gw := newTestGateway(t)
	o.Gateway = gw.trust
	srv := httptest.NewUnstartedServer(nil)
	srv.StartTLS()
	o.PublicURL = srv.URL
	a, err := newAPI(o)
	if err != nil {
		srv.Close()
		t.Fatal(err)
	}
	srv.Config.Handler = a.handler()
	t.Cleanup(srv.Close)
	return &testAPI{Server: srv, gw: gw, api: a}
}

// testGateway stands in for the Plux server: it publishes the keys that
// verify assertions and mints assertions as the gateway does.
type testGateway struct {
	key   *ecdsa.PrivateKey
	kid   string
	trust GatewayTrust
	now   func() time.Time
}

const (
	testIssuer   = "https://plux.example"
	testAudience = "refapi-test"
)

func newTestGateway(t *testing.T) *testGateway {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	g := &testGateway{key: key, kid: "gw-1", now: time.Now}
	keys := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		k, err := publicJWK(&g.key.PublicKey, g.kid)
		if err != nil {
			t.Error(err)
		}
		_ = json.NewEncoder(w).Encode(jwkSet{Keys: []jwk{k}})
	}))
	t.Cleanup(keys.Close)
	g.trust = GatewayTrust{JWKSURL: keys.URL + "/jwks", Issuer: testIssuer, Audience: testAudience, Client: keys.Client()}
	return g
}

// who says what an assertion vouches for.
type who string

const (
	nobody who = ""
	// aDevice: a device, no user.
	aDevice who = "device"
	// aUser: a device and a verified user.
	aUser who = "user"
	// aConfirmedUser: a verified user who confirmed this transaction.
	aConfirmedUser who = "sca"
)

// tamper changes an assertion's claims or header before it is signed.
type tamper func(h *jwsHeader, c map[string]any)

// assertion mints the assertion the gateway adds to a request.
func (g *testGateway) assertion(t *testing.T, w who, method, uri, body string, tampers ...tamper) string {
	t.Helper()
	if w == nobody {
		return ""
	}
	var bodySum []byte
	if body != "" {
		sum := sha256.Sum256([]byte(body))
		bodySum = sum[:]
	}
	now := g.now()
	claims := map[string]any{
		"iss": testIssuer, "aud": testAudience, "sub": "device-1", "app": "app-1", "env": "env-1", "org": "org-1", "al": "AL0",
		"rh": requestHash(method, uri, bodySum), "iat": now.Unix(), "exp": now.Add(time.Minute).Unix(), "jti": fmt.Sprintf("jti-%d", now.UnixNano()),
	}
	if w == aUser || w == aConfirmedUser {
		claims["usr"] = map[string]any{"sub": demoSubject, "iss": "https://idp.example"}
	}
	if w == aConfirmedUser {
		claims["sca"] = map[string]any{"cid": "challenge-1", "sh": "summary-hash-1"}
	}
	h := jwsHeader{Typ: assertionType, Kid: g.kid}
	for _, tm := range tampers {
		tm(&h, claims)
	}
	token, err := signES256(g.key, h, claims)
	if err != nil {
		t.Fatal(err)
	}
	return token
}

// reply is a response read to its end.
type reply struct {
	status int
	header http.Header
	body   []byte
}

// json decodes the body into v.
func (r reply) json(t *testing.T, v any) {
	t.Helper()
	if err := json.Unmarshal(r.body, v); err != nil {
		t.Fatalf("the body is not JSON: %v\n%s", err, r.body)
	}
}

// errorCode is the code of the JSON error the body holds.
func (r reply) errorCode(t *testing.T) string {
	t.Helper()
	var e errorBody
	r.json(t, &e)
	return e.Error.Code
}

// call sends a request the way the gateway forwards it, carrying the
// assertion for w, and reads the whole response.
func call(t *testing.T, srv *testAPI, method, path string, w who, body string, headers ...string) reply {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, srv.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if a := srv.gw.assertion(t, w, method, path, body); a != "" {
		req.Header.Set(assertionHeader, a)
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	return send(t, srv, req)
}

// send performs req and reads the whole response.
func send(t *testing.T, srv *testAPI, req *http.Request) reply {
	t.Helper()
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return reply{status: res.StatusCode, header: res.Header, body: data}
}

// testWS is a WebSocket client, enough to test the server with.
type testWS struct {
	conn net.Conn
	br   *bufio.Reader
}

// dialWS connects to path and completes the handshake; it returns the
// handshake's response too, whose status is 101 when it worked.
func dialWS(t *testing.T, srv *testAPI, path string) (*testWS, *handshake) {
	t.Helper()
	pool := x509.NewCertPool()
	pool.AddCert(srv.Certificate())
	dialer := &tls.Dialer{Config: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}}
	conn, err := dialer.DialContext(t.Context(), "tcp", strings.TrimPrefix(srv.URL, "https://"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-WebSocket-Version", "13")
	req.Header.Set("Sec-WebSocket-Key", handshakeKey)
	if err := req.Write(conn); err != nil {
		t.Fatal(err)
	}
	br := bufio.NewReader(conn)
	res, err := http.ReadResponse(br, req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	return &testWS{conn: conn, br: br}, &handshake{StatusCode: res.StatusCode, Header: res.Header}
}

// handshake is what a WebSocket handshake answered with.
type handshake struct {
	StatusCode int
	Header     http.Header
}

// frameLength is the payload length of a frame as an int; the test client
// does not read frames longer than 2 GiB.
func frameLength(t *testing.T, length uint64) int {
	t.Helper()
	if length > math.MaxInt32 {
		t.Fatalf("a frame of %d bytes is too long for the test client", length)
		return 0
	}
	return int(length)
}

// maskedFrame encodes a client frame.
func maskedFrame(op byte, fin bool, payload []byte) []byte {
	b0 := op
	if fin {
		b0 |= 0x80
	}
	out := []byte{b0}
	switch n := len(payload); {
	case n < 126:
		out = append(out, 0x80|byte(n))
	case n <= 0xFFFF:
		out = binary.BigEndian.AppendUint16(append(out, 0x80|126), uint16(n))
	default:
		out = binary.BigEndian.AppendUint64(append(out, 0x80|127), uint64(n))
	}
	mask := [4]byte{0x12, 0x34, 0x56, 0x78}
	out = append(out, mask[:]...)
	for i, c := range payload {
		out = append(out, c^mask[i%4])
	}
	return out
}

// send writes a masked frame.
func (c *testWS) send(t *testing.T, op byte, fin bool, payload []byte) {
	t.Helper()
	if _, err := c.conn.Write(maskedFrame(op, fin, payload)); err != nil {
		t.Fatal(err)
	}
}

// recv reads one server frame.
func (c *testWS) recv(t *testing.T) (op byte, payload []byte) {
	t.Helper()
	if err := c.conn.SetReadDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatal(err)
	}
	return readServerFrame(t, c.br)
}

// readServerFrame reads an unmasked frame from r.
func readServerFrame(t *testing.T, r io.Reader) (op byte, payload []byte) {
	t.Helper()
	var head [2]byte
	if _, err := io.ReadFull(r, head[:]); err != nil {
		t.Fatalf("reading a frame: %v", err)
	}
	if head[1]&0x80 != 0 {
		t.Fatal("the server masked a frame")
	}
	n := int(head[1] & 0x7F)
	switch n {
	case 126:
		var b [2]byte
		if _, err := io.ReadFull(r, b[:]); err != nil {
			t.Fatal(err)
		}
		n = int(binary.BigEndian.Uint16(b[:]))
	case 127:
		var b [8]byte
		if _, err := io.ReadFull(r, b[:]); err != nil {
			t.Fatal(err)
		}
		n = frameLength(t, binary.BigEndian.Uint64(b[:]))
	}
	payload = make([]byte, n)
	if _, err := io.ReadFull(r, payload); err != nil {
		t.Fatal(err)
	}
	return head[0] & 0x0F, payload
}

// closeCode is the status code of a close frame's payload.
func closeCode(t *testing.T, payload []byte) uint16 {
	t.Helper()
	if len(payload) < 2 {
		t.Fatalf("a close frame without a code: %x", payload)
	}
	return binary.BigEndian.Uint16(payload)
}

// bodyOf is the JSON of v as a string.
func bodyOf(t *testing.T, v any) string {
	t.Helper()
	var b bytes.Buffer
	if err := json.NewEncoder(&b).Encode(v); err != nil {
		t.Fatal(err)
	}
	return b.String()
}
