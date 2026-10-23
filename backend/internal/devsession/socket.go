// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package devsession

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

const (
	// SocketPath is where a paired device opens its socket (ADR-0087).
	SocketPath = "/dev/socket"
	// TicketHeader carries the socket ticket at the upgrade.
	TicketHeader = "Plux-Dev-Ticket"
	// touchEvery is how often a socket's events record the session's
	// activity at most, so that a busy device does not write the session
	// row with every event.
	touchEvery = 10 * time.Second
	// frameOverhead is the room the proto3 JSON frame around an event
	// takes beyond the event itself.
	frameOverhead = 4096
	// defaultPingInterval and defaultIdleTimeout apply when the options
	// leave them zero; they are the configuration's defaults.
	defaultPingInterval = 30 * time.Second
	defaultIdleTimeout  = 120 * time.Second
)

// ProofFunc verifies the DPoP proof of the socket's upgrade request, with
// the request's own method and URL as htm and htu, the server nonce and the
// replay check of a device call, and returns the thumbprint of its key
// (SEC-021).
type ProofFunc func(ctx context.Context, w http.ResponseWriter, r *http.Request) (string, error)

// SocketHandler serves the device socket at SocketPath (DEV-020, DEV-021,
// ADR-0087): the upgrade spends the ticket in the Plux-Dev-Ticket header
// with the key of the DPoP proof, and is refused with 401 and the Plux
// code otherwise, before anything is upgraded.
func (s *Service) SocketHandler(proof ProofFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.serveSocket(w, r, proof)
	})
}

// serveSocket admits a device and relays its socket until it closes.
func (s *Service) serveSocket(w http.ResponseWriter, r *http.Request, proof ProofFunc) {
	ctx := r.Context()
	b, lim, err := s.admit(ctx, w, r, proof)
	if err != nil {
		refuseSocket(w, err)
		return
	}
	room := s.hub.room(b.SessionID, &lim)
	sock := &socket{s: s, b: b, room: room, lim: lim}
	sock.seen.Store(time.Now().UnixNano())
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// permessage-deflate with context takeover: a dev bundle is sent
		// whole and differs little from the previous one, so compressing
		// against the last frames is what brings an edit within the
		// 500 ms of DEV-010 over Wi-Fi (docs/benchmarks/devloop.md). The
		// runtime's dart:io WebSocket offers it by default. Frames carry no
		// secret beside attacker-chosen text (ADR-0087), so compression
		// leaks nothing.
		CompressionMode: websocket.CompressionContextTakeover,
		OnPingReceived:  func(context.Context, []byte) bool { sock.touched(); return true },
		OnPongReceived:  func(context.Context, []byte) { sock.touched() },
	})
	if err != nil {
		return
	}
	c.SetReadLimit(2*lim.eventBytes + frameOverhead)
	sock.c = c
	sock.run(context.WithoutCancel(ctx))
}

// admit checks the ticket and the proof of an upgrade and returns the
// device's place in its session with the session's relay bounds.
func (s *Service) admit(ctx context.Context, w http.ResponseWriter, r *http.Request, proof ProofFunc) (DeviceBinding, relayLimits, error) {
	ticket := r.Header.Get(TicketHeader)
	if ticket == "" {
		return DeviceBinding{}, relayLimits{}, ticketInvalid()
	}
	jkt, err := proof(ctx, w, r)
	if err != nil {
		return DeviceBinding{}, relayLimits{}, err
	}
	b, err := s.ConsumeSocketTicket(ctx, ticket, jkt)
	if err != nil {
		return DeviceBinding{}, relayLimits{}, err
	}
	var lim relayLimits
	err = s.inTenant(ctx, storage.Tenant{OrganizationID: b.OrganizationID}, func(ctx context.Context, tx pgx.Tx) error {
		set, err := s.o.Limits(ctx, tx, b.OrganizationID, b.AppID)
		if err != nil {
			return fmt.Errorf("devsession: read the limits: %w", err)
		}
		lim = relayLimitsOf(set)
		return nil
	})
	return b, lim, err
}

// refuseSocket answers a refused upgrade: 401 with the Plux code and
// reason of a refusal, 500 without detail otherwise.
func refuseSocket(w http.ResponseWriter, err error) {
	body := map[string]string{"code": plxerr.InternalServerError.String(), "reason": "INTERNAL_SERVER_ERROR"}
	status := http.StatusInternalServerError
	var plx *plxerr.Error
	if errors.As(err, &plx) {
		status = http.StatusUnauthorized
		body = map[string]string{"code": plx.Code.String(), "reason": string(plx.Reason()), "message": plx.Message}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// socket is one open device socket.
type socket struct {
	s    *Service
	c    *websocket.Conn
	b    DeviceBinding
	room *room
	lim  relayLimits
	link *link
	// seen is when the socket last carried anything, pings included, in
	// Unix nanoseconds.
	seen atomic.Int64
	// touched is when the session's activity was last recorded.
	lastTouch time.Time
}

// touched records traffic on the socket.
func (k *socket) touched() { k.seen.Store(time.Now().UnixNano()) }

// run reads the hello, then relays until the socket closes, the session
// ends or the device goes silent for the idle timeout.
func (k *socket) run(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	defer func() { _ = k.c.CloseNow() }()
	hello, err := k.hello(ctx)
	if err != nil {
		_ = k.c.Close(websocket.StatusPolicyViolation, "the first frame must be a hello")
		return
	}
	k.link, err = k.room.attach(k.b.DeviceID)
	if err != nil {
		k.sendEnded(ctx, plxerr.DevSessionExpired)
		return
	}
	defer k.room.detach(k.b.DeviceID, k.link)
	k.room.hello(k.b.DeviceID, k.link, hello)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); defer cancel(); k.write(ctx) }()
	go func() { defer wg.Done(); k.keepAlive(ctx) }()
	k.read(ctx)
	// Once the session ended, the writer tells the device before it
	// closes the socket; otherwise everything stops now.
	if !k.room.isEnded() {
		cancel()
	}
	wg.Wait()
}

// hello reads the first frame, which must be a hello.
func (k *socket) hello(ctx context.Context) (*pluxv1.DevDeviceHello, error) {
	ctx, cancel := context.WithTimeout(ctx, k.s.idleTimeout())
	defer cancel()
	frame, err := k.frame(ctx)
	if err != nil {
		return nil, err
	}
	h := frame.GetHello()
	if h == nil {
		return nil, errors.New("the first frame must be a hello")
	}
	return h, nil
}

// frame reads one device frame.
func (k *socket) frame(ctx context.Context) (*pluxv1.DevDeviceFrame, error) {
	typ, data, err := k.c.Read(ctx)
	if err != nil {
		return nil, fmt.Errorf("devsession: read a frame: %w", err)
	}
	k.touched()
	if typ != websocket.MessageText {
		return nil, errors.New("frames are proto3 JSON text messages")
	}
	frame := &pluxv1.DevDeviceFrame{}
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(data, frame); err != nil {
		return nil, errors.New("the frame is not a device frame")
	}
	return frame, nil
}

// read relays the device's events until the socket fails.
func (k *socket) read(ctx context.Context) {
	for {
		frame, err := k.frame(ctx)
		if err != nil {
			if ctx.Err() == nil && websocket.CloseStatus(err) == -1 {
				_ = k.c.Close(websocket.StatusPolicyViolation, "the frame is not a device frame")
			}
			return
		}
		ev := frame.GetEvent()
		if ev == nil {
			continue
		}
		if err := k.event(ctx, ev); err != nil {
			return
		}
	}
}

// event validates, enriches and relays one device event (DEV-020).
func (k *socket) event(ctx context.Context, ev *pluxv1.DevEvent) error {
	deviceSeq := ev.GetSequence()
	if int64(proto.Size(ev)) > k.lim.eventBytes {
		return k.room.refuse(k.b.DeviceID, k.link, deviceSeq, pluxv1.DevDropReason_DEV_DROP_REASON_EVENT_TOO_LARGE)
	}
	if !k.room.duplicate(k.b.DeviceID, deviceSeq) {
		k.s.mergeFunctionLogs(ctx, k.b, ev, k.lim.eventBytes)
	}
	if err := k.room.publish(k.b.DeviceID, k.link, ev); err != nil {
		return err
	}
	return k.touch(ctx)
}

// touch records the session's activity at most every touchEvery. A
// session that is no longer live ends the relay; a failure to record is
// retried with the next event.
func (k *socket) touch(ctx context.Context) error {
	now := k.s.now()
	if !k.lastTouch.IsZero() && now.Sub(k.lastTouch) < touchEvery {
		return nil
	}
	err := k.s.Touch(ctx, k.b.OrganizationID, k.b.SessionID)
	if code, ok := plxerr.CodeOf(err); ok && code == plxerr.DevSessionExpired {
		k.s.hub.End(k.b.SessionID, plxerr.DevSessionExpired)
		return err
	}
	if err == nil {
		k.lastTouch = now
	}
	return nil
}

// write sends the frames the relay has for the device, and the end of the
// session when it ends.
func (k *socket) write(ctx context.Context) {
	for {
		frames, ended, replaced := k.room.pending(k.b.DeviceID, k.link)
		if replaced {
			_ = k.c.Close(websocket.StatusPolicyViolation, "a newer socket of the device took over")
			return
		}
		for _, f := range frames {
			if err := k.send(ctx, f); err != nil {
				return
			}
		}
		if ended != 0 {
			k.sendEnded(ctx, ended)
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-k.link.wake:
		}
	}
}

// sendEnded tells the device its session ended and closes the socket.
func (k *socket) sendEnded(ctx context.Context, code plxerr.Code) {
	_ = k.send(ctx, &pluxv1.DevServerFrame{Payload: &pluxv1.DevServerFrame_Ended{Ended: &pluxv1.DevSessionEnded{Code: code.String()}}})
	_ = k.c.Close(websocket.StatusNormalClosure, "the dev session ended")
}

// send writes one server frame.
func (k *socket) send(ctx context.Context, f *pluxv1.DevServerFrame) error {
	data, err := protojson.Marshal(f)
	if err != nil {
		return fmt.Errorf("devsession: encode a frame: %w", err)
	}
	if err := k.c.Write(ctx, websocket.MessageText, data); err != nil {
		return fmt.Errorf("devsession: write a frame: %w", err)
	}
	return nil
}

// keepAlive pings the device every ping interval, closes the socket after
// the idle timeout of silence, and ends the relay once the session is no
// longer live.
func (k *socket) keepAlive(ctx context.Context) {
	interval, idle := k.s.pingInterval(), k.s.idleTimeout()
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		if time.Since(time.Unix(0, k.seen.Load())) > idle {
			_ = k.c.Close(websocket.StatusGoingAway, "the socket was idle")
			return
		}
		live, err := k.s.live(ctx, k.b.OrganizationID, k.b.SessionID)
		if err == nil && !live {
			// The writer tells the device and closes the socket.
			k.s.hub.End(k.b.SessionID, plxerr.DevSessionExpired)
			continue
		}
		pctx, cancel := context.WithTimeout(ctx, interval)
		_ = k.c.Ping(pctx)
		cancel()
	}
}

// pingInterval is the configured ping interval, or the default.
func (s *Service) pingInterval() time.Duration {
	if s.o.PingInterval > 0 {
		return s.o.PingInterval
	}
	return defaultPingInterval
}

// idleTimeout is the configured idle timeout, or the default.
func (s *Service) idleTimeout() time.Duration {
	if s.o.IdleTimeout > 0 {
		return s.o.IdleTimeout
	}
	return defaultIdleTimeout
}
