// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package devsession_test

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/protobuf/encoding/protodelim"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/device/devicetest"
	"github.com/nightCode42/plux3/backend/internal/devsession"
	"github.com/nightCode42/plux3/backend/internal/dpop"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// wait bounds every wait of these tests.
const wait = 10 * time.Second

// relay is a fixture with the device socket served over HTTP.
type relay struct {
	*fixture
	srv *httptest.Server
	mu  sync.Mutex
	// used are the proof identifiers seen, for the replay check.
	used map[string]bool
}

// newRelay serves the socket; tight tightens limits before anything runs.
func newRelay(t *testing.T, tight map[limits.Key]int64) *relay {
	t.Helper()
	return newRelayWith(t, tight, nil)
}

// newRelayWith is newRelay with the service's options changed by tune.
func newRelayWith(t *testing.T, tight map[limits.Key]int64, tune func(*devsession.Options)) *relay {
	t.Helper()
	r := &relay{fixture: newFixtureWith(t, 0, tune), used: map[string]bool{}}
	for k, v := range tight {
		r.tight[k] = v
	}
	mux := http.NewServeMux()
	mux.Handle("GET "+devsession.SocketPath, r.svc.SocketHandler(r.proof))
	r.srv = httptest.NewServer(mux)
	t.Cleanup(r.srv.Close)
	return r
}

// proof verifies a DPoP proof as the device authentication does: htm and
// htu are the request's own, and a proof is accepted once.
func (r *relay) proof(_ context.Context, _ http.ResponseWriter, req *http.Request) (string, error) {
	raw := req.Header.Get("DPoP")
	if raw == "" {
		return "", plxerr.New(plxerr.DPoPProofInvalid, "dpop proof failed check %q", "missing")
	}
	p, err := (&dpop.Verifier{}).Verify(raw, dpop.Expect{Method: req.Method, URL: "http://" + req.Host + req.URL.Path, Window: time.Minute})
	if err != nil {
		return "", err //nolint:wrapcheck // a test
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.used[p.JTI] {
		return "", plxerr.New(plxerr.DPoPReplay, "dpop proof was used before")
	}
	r.used[p.JTI] = true
	return p.JKT, nil
}

// paired is a device paired with a session, with its key.
type paired struct {
	devsession.Paired
	key    *ecdsa.PrivateKey
	caller devsession.Caller
}

// pairDevice pairs a device with a session, with a fresh token from the
// session's creator.
func (r *relay) pairDevice(t *testing.T, sessionID, registered string) paired {
	t.Helper()
	key, _ := devicetest.NewKey(t)
	jkt, err := dpop.Thumbprint(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	p, err := r.svc.GetPairing(context.Background(), r.Developer, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	c := r.device(registered, jkt)
	res, err := r.pair(c, p.Token)
	if err != nil {
		t.Fatal(err)
	}
	return paired{Paired: res, key: key, caller: c}
}

// signedProof is a proof of the socket's upgrade made with key.
func (r *relay) signedProof(t *testing.T, key *ecdsa.PrivateKey) string {
	t.Helper()
	return devicetest.SignProof(t, key, devicetest.Proof{Method: http.MethodGet, URL: r.srv.URL + devsession.SocketPath})
}

// refusal is the answer to a refused upgrade.
type refusal struct {
	status int
	code   string
}

// dialWith opens the socket with the headers; a refusal says how it was
// refused.
func (r *relay) dialWith(t *testing.T, h http.Header) (*websocket.Conn, refusal, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	c, res, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(r.srv.URL, "http")+devsession.SocketPath, &websocket.DialOptions{HTTPHeader: h})
	if c != nil {
		t.Cleanup(func() { _ = c.CloseNow() })
	}
	var out refusal
	if res != nil && res.Body != nil {
		defer func() { _ = res.Body.Close() }()
		var body struct{ Code string }
		_ = json.NewDecoder(res.Body).Decode(&body)
		out = refusal{status: res.StatusCode, code: body.Code}
	}
	return c, out, err //nolint:wrapcheck // a test
}

// headers are the upgrade's headers: a ticket and a fresh proof of key.
func (r *relay) headers(t *testing.T, ticket string, key *ecdsa.PrivateKey) http.Header {
	t.Helper()
	h := http.Header{}
	if ticket != "" {
		h.Set(devsession.TicketHeader, ticket)
	}
	if key != nil {
		h.Set("DPoP", r.signedProof(t, key))
	}
	return h
}

// connect opens the socket of a paired device and says hello.
func (r *relay) connect(t *testing.T, ticket string, key *ecdsa.PrivateKey, hello *pluxv1.DevDeviceHello) *client {
	t.Helper()
	c, _, err := r.dialWith(t, r.headers(t, ticket, key))
	if err != nil {
		t.Fatal(err)
	}
	k := &client{c: c}
	k.send(t, &pluxv1.DevDeviceFrame{Payload: &pluxv1.DevDeviceFrame_Hello{Hello: hello}})
	return k
}

// refused checks that an upgrade was refused with 401 and the code.
func refused(t *testing.T, res refusal, err error, want plxerr.Code, what string) {
	t.Helper()
	if err == nil || res.status != http.StatusUnauthorized || res.code != want.String() {
		t.Fatalf("%s: %+v, %v; want 401 with %s", what, res, err, want)
	}
}

// client is the device's end of a socket.
type client struct{ c *websocket.Conn }

// send writes a device frame.
func (k *client) send(t *testing.T, f *pluxv1.DevDeviceFrame) {
	t.Helper()
	data, err := protojson.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	if err := k.c.Write(ctx, websocket.MessageText, data); err != nil {
		t.Fatal(err)
	}
}

// event sends a device event with the device's own sequence.
func (k *client) event(t *testing.T, seq uint64, ev *pluxv1.DevEvent) {
	t.Helper()
	ev.Sequence = seq
	k.send(t, &pluxv1.DevDeviceFrame{Payload: &pluxv1.DevDeviceFrame_Event{Event: ev}})
}

// next reads a server frame.
func (k *client) next(t *testing.T) *pluxv1.DevServerFrame {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	_, data, err := k.c.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	f := &pluxv1.DevServerFrame{}
	if err := protojson.Unmarshal(data, f); err != nil {
		t.Fatal(err)
	}
	return f
}

// ackedUpTo reads frames until the events up to seq are acknowledged and
// returns the other frames read on the way.
func (k *client) ackedUpTo(t *testing.T, seq uint64) []*pluxv1.DevServerFrame {
	t.Helper()
	var other []*pluxv1.DevServerFrame
	for {
		f := k.next(t)
		if a := f.GetAck(); a != nil {
			if a.GetUpTo() >= seq {
				return other
			}
			continue
		}
		other = append(other, f)
	}
}

// logEvent is a device log line.
func logEvent(msg string) *pluxv1.DevEvent {
	return &pluxv1.DevEvent{Payload: &pluxv1.DevEvent_Log{Log: &pluxv1.LogEvent{Message: msg}}}
}

// watch runs a watcher of the session as the developer and returns its
// events as they come; send, when set, runs before each event is passed.
func (r *relay) watch(t *testing.T, sessionID string, after uint64, gate func()) <-chan *pluxv1.DevEvent {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	out := make(chan *pluxv1.DevEvent, 1024)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = r.svc.Watch(ctx, r.Developer, sessionID, nil, after, func(ev *pluxv1.DevEvent) error {
			if gate != nil {
				gate()
			}
			out <- ev
			return nil
		})
	}()
	t.Cleanup(func() { cancel(); <-done })
	return out
}

// receive reads n events from a watcher.
func receive(t *testing.T, events <-chan *pluxv1.DevEvent, n int) []*pluxv1.DevEvent {
	t.Helper()
	var out []*pluxv1.DevEvent
	timeout := time.After(wait)
	for len(out) < n {
		select {
		case ev := <-events:
			out = append(out, ev)
		case <-timeout:
			t.Fatalf("received %d events, want %d", len(out), n)
		}
	}
	return out
}

// Verifies: DEV-020, SEC-021.
// The socket is refused with 401 and the code before the upgrade without a
// ticket, with a proof that does not verify or was used before, with a key
// other than the one the device paired with, and with a spent ticket.
func TestSocket_Refusals_DEV_020_SEC_021(t *testing.T) {
	r := newRelay(t, nil)
	s, _ := r.open(t)
	d := r.pairDevice(t, s.ID, "device-1")

	c, res, err := r.dialWith(t, r.headers(t, "", d.key))
	if c != nil {
		t.Fatal("upgraded without a ticket")
	}
	refused(t, res, err, plxerr.SocketTicketInvalid, "without a ticket")

	h := http.Header{}
	h.Set(devsession.TicketHeader, d.Ticket)
	h.Set("DPoP", "not.a.proof")
	_, res, err = r.dialWith(t, h)
	refused(t, res, err, plxerr.DPoPProofInvalid, "a bad proof")

	other, _ := devicetest.NewKey(t)
	_, res, err = r.dialWith(t, r.headers(t, d.Ticket, other))
	refused(t, res, err, plxerr.SocketTicketInvalid, "another key")

	// None of the refusals spent the ticket.
	h = r.headers(t, d.Ticket, d.key)
	c, _, err = r.dialWith(t, h)
	if err != nil {
		t.Fatal(err)
	}
	_ = c.CloseNow()
	_, res, err = r.dialWith(t, h)
	refused(t, res, err, plxerr.DPoPReplay, "a replayed proof")
	_, res, err = r.dialWith(t, r.headers(t, d.Ticket, d.key))
	refused(t, res, err, plxerr.SocketTicketInvalid, "a spent ticket")
}

// Verifies: DEV-020.
// The first frame must be a hello; anything else closes the socket.
func TestSocket_HelloRequired_DEV_020(t *testing.T) {
	r := newRelay(t, nil)
	s, _ := r.open(t)
	d := r.pairDevice(t, s.ID, "device-1")
	c, _, err := r.dialWith(t, r.headers(t, d.Ticket, d.key))
	if err != nil {
		t.Fatal(err)
	}
	k := &client{c: c}
	k.event(t, 1, logEvent("too early"))
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	_, _, err = c.Read(ctx)
	if websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
		t.Fatalf("read after an event first: %v, want a policy-violation close", err)
	}
}

// Verifies: DEV-020, DEV-021.
// Two watchers receive the events of two devices in order, stamped with
// the session-wide sequence and the device; each event is acknowledged
// with the device's own sequence.
func TestRelay_FanOutInOrder_DEV_020(t *testing.T) {
	r := newRelay(t, nil)
	s, _ := r.open(t)
	a := r.pairDevice(t, s.ID, "device-a")
	b := r.pairDevice(t, s.ID, "device-b")
	first := r.watch(t, s.ID, 0, nil)
	second := r.watch(t, s.ID, 0, nil)
	ka := r.connect(t, a.Ticket, a.key, &pluxv1.DevDeviceHello{})
	kb := r.connect(t, b.Ticket, b.key, &pluxv1.DevDeviceHello{})
	ka.event(t, 1, logEvent("a1"))
	ka.ackedUpTo(t, 1)
	kb.event(t, 1, logEvent("b1"))
	kb.ackedUpTo(t, 1)
	ka.event(t, 2, logEvent("a2"))
	ka.ackedUpTo(t, 2)
	want := []struct {
		device, msg string
	}{{a.DeviceID, "a1"}, {b.DeviceID, "b1"}, {a.DeviceID, "a2"}}
	for _, w := range []<-chan *pluxv1.DevEvent{first, second} {
		got := receive(t, w, 3)
		for i, ev := range got {
			if ev.GetSequence() != uint64(i+1) || ev.GetDeviceId() != want[i].device || ev.GetLog().GetMessage() != want[i].msg {
				t.Errorf("event %d = %v, want sequence %d from %s", i, ev, i+1, want[i].device)
			}
		}
	}
	resumed := receive(t, r.watch(t, s.ID, 2, nil), 1)
	if resumed[0].GetSequence() != 3 {
		t.Errorf("resumed after 2 with %v", resumed[0])
	}
}

// Verifies: DEV-021.
// A watcher that does not keep up loses its oldest events and is told how
// many with a drop event; the device is never held up.
func TestRelay_SlowWatcherDrops_DEV_021(t *testing.T) {
	r := newRelay(t, map[limits.Key]int64{limits.DevtoolsStreamBufferEvents: 4})
	s, _ := r.open(t)
	d := r.pairDevice(t, s.ID, "device-1")
	release := make(chan struct{})
	var once sync.Once
	events := r.watch(t, s.ID, 0, func() { once.Do(func() { <-release }) })
	k := r.connect(t, d.Ticket, d.key, &pluxv1.DevDeviceHello{})
	const sent = 20
	for i := uint64(1); i <= sent; i++ {
		k.event(t, i, logEvent("e"))
	}
	k.ackedUpTo(t, sent)
	close(release)
	var relayed, dropped uint64
	timeout := time.After(wait)
	for relayed+dropped < sent {
		select {
		case ev := <-events:
			if drop := ev.GetDrop(); drop != nil {
				if drop.GetReason() != pluxv1.DevDropReason_DEV_DROP_REASON_BUFFER_FULL {
					t.Errorf("drop reason %v", drop.GetReason())
				}
				dropped += drop.GetDropped()
				continue
			}
			relayed++
		case <-timeout:
			t.Fatalf("relayed %d and dropped %d of %d", relayed, dropped, sent)
		}
	}
	if dropped == 0 {
		t.Error("a watcher that fell behind lost nothing")
	}
}

// Verifies: DEV-021.
// A device that reconnects with a renewed ticket gets the server frames it
// did not carry out, and the events it resends are acknowledged without
// being relayed twice.
func TestRelay_ReconnectResumes_DEV_021(t *testing.T) {
	r := newRelay(t, nil)
	ctx := context.Background()
	s, _ := r.open(t)
	d := r.pairDevice(t, s.ID, "device-1")
	events := r.watch(t, s.ID, 0, nil)
	k := r.connect(t, d.Ticket, d.key, &pluxv1.DevDeviceHello{})
	k.event(t, 1, logEvent("one"))
	k.event(t, 2, logEvent("two"))
	k.ackedUpTo(t, 2)
	var ids []string
	for range 2 {
		sent, err := r.svc.SendCommand(ctx, r.Developer, &pluxv1.DevSessionServiceSendCommandRequest{
			SessionId: s.ID, Command: &pluxv1.DevSessionServiceSendCommandRequest_CaptureScreenshot{CaptureScreenshot: &pluxv1.CaptureScreenshotCommand{}},
		})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, sent.CommandID)
	}
	for i := range 2 {
		if f := k.next(t); f.GetSequence() != uint64(i+1) || f.GetCommand().GetCommandId() != ids[i] {
			t.Fatalf("frame %d = %v", i, f)
		}
	}
	_ = k.c.Close(websocket.StatusNormalClosure, "")

	renewed, err := r.svc.RenewSocketTicket(ctx, d.caller, s.ID, d.DeviceID)
	if err != nil {
		t.Fatal(err)
	}
	k = r.connect(t, renewed.Ticket, d.key, &pluxv1.DevDeviceHello{LastServerSequence: 1, LastAcknowledgedEvent: 1})
	other := k.ackedUpTo(t, 2)
	if len(other) == 0 {
		other = append(other, k.next(t))
	}
	if f := other[0]; f.GetSequence() != 2 || f.GetCommand().GetCommandId() != ids[1] {
		t.Fatalf("resumed with %v, want the second command", f)
	}
	k.event(t, 2, logEvent("two again"))
	k.event(t, 3, logEvent("three"))
	k.ackedUpTo(t, 3)
	got := receive(t, events, 3)
	for i, msg := range []string{"one", "two", "three"} {
		if got[i].GetLog().GetMessage() != msg || got[i].GetSequence() != uint64(i+1) {
			t.Errorf("event %d = %v, want %q", i, got[i], msg)
		}
	}
	select {
	case ev := <-events:
		t.Errorf("a resent event was relayed again: %v", ev)
	case <-time.After(100 * time.Millisecond):
	}
}

// Verifies: DEV-020, DEV-021.
// A command reaches the device; past devSession.commandsPerMinute it is
// refused with PLX-9206, to a device of another session with PLX-9204,
// and from an observer without devsession.create. The audit names the
// command's kind, never its values.
func TestSendCommand_DEV_020(t *testing.T) {
	r := newRelay(t, map[limits.Key]int64{limits.DevSessionCommandsPerMinute: 2})
	ctx := context.Background()
	s, _ := r.open(t)
	d := r.pairDevice(t, s.ID, "device-1")
	k := r.connect(t, d.Ticket, d.key, &pluxv1.DevDeviceHello{})
	open := func(device string) *pluxv1.DevSessionServiceSendCommandRequest {
		return &pluxv1.DevSessionServiceSendCommandRequest{
			SessionId: s.ID, DeviceId: device,
			Command: &pluxv1.DevSessionServiceSendCommandRequest_OpenScreen{OpenScreen: &pluxv1.OpenScreenCommand{Route: "/private-route"}},
		}
	}
	_, err := r.svc.SendCommand(ctx, r.Viewer, open(d.DeviceID))
	wantCode(t, err, plxerr.PermissionDenied, "an observer")
	_, err = r.svc.SendCommand(ctx, r.Developer, open(r.NewID(t)))
	wantCode(t, err, plxerr.DeviceNotPaired, "a device of no session")
	sent, err := r.svc.SendCommand(ctx, r.Developer, open(d.DeviceID))
	if err != nil || len(sent.DeviceIDs) != 1 || sent.DeviceIDs[0] != d.DeviceID {
		t.Fatalf("sent = %+v: %v", sent, err)
	}
	f := k.next(t)
	if f.GetCommand().GetCommandId() != sent.CommandID || f.GetCommand().GetCommand().GetOpenScreen().GetRoute() != "/private-route" ||
		f.GetCommand().GetCommand().GetSessionId() != "" {
		t.Errorf("frame = %v", f)
	}
	if _, err := r.svc.SendCommand(ctx, r.Developer, open("")); err != nil {
		t.Fatal(err)
	}
	_, err = r.svc.SendCommand(ctx, r.Developer, open(""))
	wantCode(t, err, plxerr.DevCommandNotAllowed, "past the rate")
	rows := r.Audit(t, audit.DevSessionCommandSent)
	if len(rows) != 2 {
		t.Fatalf("audit entries = %+v", rows)
	}
	for _, row := range rows {
		if !strings.Contains(row.Detail, "open_screen") || strings.Contains(row.Detail, "private") {
			t.Errorf("audit detail %q", row.Detail)
		}
	}
}

// Verifies: FN-036, DEV-020.
// A server function's call event carries the log lines of its run, joined
// by trace ID, of the session's own app and environment only.
func TestRelay_MergesFunctionLogs_FN_036(t *testing.T) {
	r := newRelay(t, nil)
	s, _ := r.open(t)
	other := ""
	for key, id := range r.Envs {
		if key != "development" {
			other = id
		}
	}
	at := pgtype.Timestamptz{Time: r.Now, Valid: true}
	insert := func(env, trace string, messages ...string) {
		levels := make([]string, len(messages))
		times := make([]pgtype.Timestamptz, len(messages))
		for i := range messages {
			levels[i], times[i] = "info", at
		}
		err := r.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
			return dbgen.New(tx).InsertFunctionLogs(ctx, dbgen.InsertFunctionLogsParams{ //nolint:wrapcheck // a test
				OrganizationID: storage.MustUUID(r.Org), AppID: storage.MustUUID(r.App), EnvironmentID: storage.MustUUID(env),
				Function: "loans.score", Version: 1, InvocationID: storage.MustUUID(r.NewID(t)),
				Levels: levels, Messages: messages, TraceID: trace, Times: times,
			})
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	insert(r.Envs["development"], "trace-1", "scored", "done")
	insert(other, "trace-1", "another environment")
	insert(r.Envs["development"], "trace-2", "another call")
	d := r.pairDevice(t, s.ID, "device-1")
	events := r.watch(t, s.ID, 0, nil)
	k := r.connect(t, d.Ticket, d.key, &pluxv1.DevDeviceHello{})
	k.event(t, 1, &pluxv1.DevEvent{TraceId: "trace-1", Payload: &pluxv1.DevEvent_FunctionCall{FunctionCall: &pluxv1.FunctionCallEvent{
		Function: "loans.score", Version: 1, Placement: "server", StartUnixMicros: r.Now.UnixMicro(), DurationMicros: 1000,
	}}})
	got := receive(t, events, 1)[0].GetFunctionCall().GetLogs()
	if len(got) != 2 || got[0].GetMessage() != "scored" || got[1].GetMessage() != "done" || got[0].GetLevel() != pluxv1.DevLogLevel_DEV_LOG_LEVEL_INFO {
		t.Errorf("merged lines = %v", got)
	}
}

// Verifies: DEV-021.
// An exported trace is gzip over length-prefixed messages: the header,
// then the retained events in sequence order.
func TestExportTrace_RoundTrips_DEV_021(t *testing.T) {
	r := newRelay(t, nil)
	s, _ := r.open(t)
	d := r.pairDevice(t, s.ID, "device-1")
	k := r.connect(t, d.Ticket, d.key, &pluxv1.DevDeviceHello{})
	k.event(t, 1, logEvent("one"))
	k.event(t, 2, logEvent("two"))
	k.ackedUpTo(t, 2)
	trace, err := r.svc.ExportTrace(context.Background(), r.Viewer, s.ID, nil)
	if err != nil || trace.Events != 2 {
		t.Fatalf("trace = %d events: %v", trace.Events, err)
	}
	zr, err := gzip.NewReader(bytes.NewReader(trace.File))
	if err != nil {
		t.Fatal(err)
	}
	in := bufio.NewReader(zr)
	header := &pluxv1.TraceFileHeader{}
	if err := protodelim.UnmarshalFrom(in, header); err != nil {
		t.Fatal(err)
	}
	if header.GetFormatVersion() != devsession.TraceFormatVersion || header.GetSessionId() != s.ID || header.GetAppKey() == "" ||
		header.GetEnvironmentKey() != "development" {
		t.Errorf("header = %v", header)
	}
	for i, msg := range []string{"one", "two"} {
		ev := &pluxv1.DevEvent{}
		if err := protodelim.UnmarshalFrom(in, ev); err != nil {
			t.Fatal(err)
		}
		if ev.GetSequence() != uint64(i+1) || ev.GetLog().GetMessage() != msg || ev.GetDeviceId() != d.DeviceID {
			t.Errorf("event %d = %v", i, ev)
		}
	}
	if err := protodelim.UnmarshalFrom(in, &pluxv1.DevEvent{}); !errors.Is(err, io.EOF) {
		t.Errorf("after the events: %v, want the end", err)
	}
}

// Verifies: DEV-021, SEC-021.
// Only the device paired in the session, with the key it paired with,
// renews its socket ticket, and only while the session is live.
func TestRenewSocketTicket_DEV_021(t *testing.T) {
	r := newRelay(t, nil)
	ctx := context.Background()
	s, _ := r.open(t)
	d := r.pairDevice(t, s.ID, "device-1")
	e := r.pairDevice(t, s.ID, "device-2")
	_, err := r.svc.RenewSocketTicket(ctx, e.caller, s.ID, d.DeviceID)
	wantCode(t, err, plxerr.DeviceNotPaired, "another device")
	wrongKey := d.caller
	wrongKey.Thumbprint = e.caller.Thumbprint
	_, err = r.svc.RenewSocketTicket(ctx, wrongKey, s.ID, d.DeviceID)
	wantCode(t, err, plxerr.DeviceNotPaired, "another key")
	renewed, err := r.svc.RenewSocketTicket(ctx, d.caller, s.ID, d.DeviceID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.ConsumeSocketTicket(ctx, d.Ticket, d.caller.Thumbprint); err == nil {
		t.Error("the replaced ticket still works")
	}
	if b, err := r.svc.ConsumeSocketTicket(ctx, renewed.Ticket, d.caller.Thumbprint); err != nil || b.DeviceID != d.DeviceID {
		t.Errorf("renewed ticket: %+v, %v", b, err)
	}
	if _, err := r.svc.EndSession(ctx, r.Developer, s.ID); err != nil {
		t.Fatal(err)
	}
	_, err = r.svc.RenewSocketTicket(ctx, d.caller, s.ID, d.DeviceID)
	wantCode(t, err, plxerr.DevSessionExpired, "an ended session")
}

// Verifies: DEV-020.
// A socket that stays silent past the idle timeout is closed by the relay,
// which does not end the session.
func TestSocket_IdleTimeoutClosesSocket_DEV_020(t *testing.T) {
	r := newRelayWith(t, nil, func(o *devsession.Options) {
		o.PingInterval = 20 * time.Millisecond
		o.IdleTimeout = 60 * time.Millisecond
	})
	s, _ := r.open(t)
	d := r.pairDevice(t, s.ID, "device-1")
	k := r.connect(t, d.Ticket, d.key, &pluxv1.DevDeviceHello{})

	// The device reads nothing, so it answers no ping, and sends nothing.
	time.Sleep(300 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	var err error
	for err == nil {
		_, _, err = k.c.Read(ctx)
	}
	if got := websocket.CloseStatus(err); got != websocket.StatusGoingAway {
		t.Errorf("the idle socket closed with %v (%v), want going away", got, err)
	}
	live, err := r.svc.ListDevices(context.Background(), r.Developer, s.ID)
	if err != nil || len(live) != 1 {
		t.Errorf("the session after the idle close: %d devices, %v; want it kept", len(live), err)
	}
}

// Verifies: DEV-020, DEV-002.
// A session that is no longer live (past its idle time) is ended by the
// socket's keep-alive: the device is told PLX-9200 and the socket closes.
func TestSocket_KeepAliveEndsExpiredSession_DEV_020(t *testing.T) {
	var skew atomic.Int64
	r := newRelayWith(t, nil, func(o *devsession.Options) {
		o.PingInterval = 20 * time.Millisecond
		o.IdleTimeout = time.Minute
		clock := o.Now
		o.Now = func() time.Time { return clock().Add(time.Duration(skew.Load())) }
	})
	s, _ := r.open(t)
	d := r.pairDevice(t, s.ID, "device-1")
	k := r.connect(t, d.Ticket, d.key, &pluxv1.DevDeviceHello{})
	k.event(t, 1, logEvent("alive"))
	k.ackedUpTo(t, 1)

	skew.Store(int64(48 * time.Hour))
	for {
		f := k.next(t)
		if end := f.GetEnded(); end != nil {
			if end.GetCode() != plxerr.DevSessionExpired.String() {
				t.Errorf("ended with %q, want %q", end.GetCode(), plxerr.DevSessionExpired)
			}
			return
		}
	}
}
