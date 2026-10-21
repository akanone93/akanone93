// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package walkthrough

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// maxFrame bounds one syslog frame the sink accepts.
const maxFrame = 1 << 20

// Sink is the syslog destination of step 6: a TLS server that reads RFC 5425
// octet-counted frames and keeps the messages. Its certificate is signed by a
// certificate authority made for the run; the audit export is configured with
// that authority's PEM and nothing else.
type Sink struct {
	ln    net.Listener
	caPEM []byte

	mu       sync.Mutex
	messages []string
	wg       sync.WaitGroup
}

// NewSink listens on addr (host:port; port 0 picks one) with a certificate
// valid for the given host names and addresses, which must include the one
// the server connects to.
func NewSink(addr string, hosts []string, now func() time.Time, random io.Reader) (*Sink, error) {
	cfg, caPEM, err := serverTLS(hosts, now, random)
	if err != nil {
		return nil, fmt.Errorf("walkthrough.NewSink: %w", err)
	}
	ln, err := tls.Listen("tcp", addr, cfg)
	if err != nil {
		return nil, fmt.Errorf("walkthrough.NewSink: %w", err)
	}
	s := &Sink{ln: ln, caPEM: caPEM}
	s.wg.Add(1)
	go s.serve()
	return s, nil
}

// Address returns the host:port the sink listens on.
func (s *Sink) Address() string { return s.ln.Addr().String() }

// CAPEM returns the certificate authority the sink's certificate chains to.
func (s *Sink) CAPEM() string { return string(s.caPEM) }

// Messages returns the messages received so far.
func (s *Sink) Messages() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.messages...)
}

// WaitFor blocks until a message containing want has arrived and returns it,
// or fails when ctx ends first.
func (s *Sink) WaitFor(ctx context.Context, want string) (string, error) {
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	for {
		for _, m := range s.Messages() {
			if strings.Contains(m, want) {
				return m, nil
			}
		}
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("walkthrough.Sink.WaitFor %q: %w (%d messages received)", want, ctx.Err(), len(s.Messages()))
		case <-tick.C:
		}
	}
}

// Close stops the sink and waits for its connections to end.
func (s *Sink) Close() error {
	err := s.ln.Close()
	s.wg.Wait()
	if err != nil {
		return fmt.Errorf("walkthrough.Sink.Close: %w", err)
	}
	return nil
}

// serve accepts connections until the listener closes.
func (s *Sink) serve() {
	defer s.wg.Done()
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.handle(conn)
		}()
	}
}

// handle reads the frames of one connection until it ends.
func (s *Sink) handle(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(time.Hour))
	br := bufio.NewReader(conn)
	for {
		msg, err := readFrame(br)
		if err != nil {
			return
		}
		s.mu.Lock()
		s.messages = append(s.messages, msg)
		s.mu.Unlock()
	}
}

// errFrame is returned for a frame that is not an octet-counted message.
var errFrame = errors.New("walkthrough: malformed syslog frame")

// readFrame reads one RFC 5425 octet-counted message: its length in decimal,
// a space and that many octets.
func readFrame(br *bufio.Reader) (string, error) {
	digits, err := br.ReadString(' ')
	if err != nil {
		return "", fmt.Errorf("walkthrough.readFrame: %w", err)
	}
	n, err := strconv.Atoi(strings.TrimSuffix(digits, " "))
	if err != nil || n < 1 || n > maxFrame {
		return "", errFrame
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(br, buf); err != nil {
		return "", fmt.Errorf("walkthrough.readFrame: %w", err)
	}
	return string(buf), nil
}
