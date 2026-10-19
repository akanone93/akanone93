// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package export

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"os"
	"time"
)

// probeWait is how long the liveness check waits for the peer to say
// anything; a healthy syslog receiver says nothing.
const probeWait = time.Millisecond

// Stream is a TLS connection to a syslog receiver that is opened when a
// batch needs it and replaced when the peer has gone (RFC 5425). It is not
// safe for concurrent use.
//
// RFC 5425 has no application-level acknowledgement: a write that
// completes means the TLS layer accepted the bytes. Before every write the
// stream checks that the peer has not closed the connection, which shuts the
// window in which a batch written to a connection the peer already dropped
// would be taken for delivered. The window cannot be closed entirely; a
// destination that needs an acknowledgement per batch uses an HTTP kind.
type Stream struct {
	address      string
	dial         DialFunc
	tls          *tls.Config
	writeTimeout time.Duration
	conn         net.Conn
}

// NewStream returns a stream to address. The TLS configuration must verify
// the server: it is used as given, so its ServerName is the name the
// certificate must carry. writeTimeout bounds the handshake and each write.
func NewStream(address string, dial DialFunc, cfg *tls.Config, writeTimeout time.Duration) (*Stream, error) {
	if dial == nil || cfg == nil || cfg.InsecureSkipVerify {
		return nil, errors.New("export: a stream needs a dialer and a TLS configuration that verifies the server")
	}
	if writeTimeout <= 0 {
		return nil, errors.New("export: a stream needs a write timeout")
	}
	return &Stream{address: address, dial: dial, tls: cfg, writeTimeout: writeTimeout}, nil
}

// Send writes data to the receiver, connecting first when there is no
// usable connection. Any failure closes the connection, so the next call
// starts afresh; data is then to be sent again whole.
func (s *Stream) Send(ctx context.Context, data []byte) error {
	if s.conn != nil && !s.alive() {
		_ = s.Close()
	}
	if s.conn == nil {
		if err := s.connect(ctx); err != nil {
			return err
		}
	}
	stop := context.AfterFunc(ctx, func() { _ = s.conn.Close() })
	defer stop()
	if err := s.conn.SetWriteDeadline(deadline(ctx, s.writeTimeout)); err != nil {
		_ = s.Close()
		return fmt.Errorf("export: set the write deadline: %w", err)
	}
	if _, err := s.conn.Write(data); err != nil {
		_ = s.Close()
		return fmt.Errorf("export: write to %s: %w", s.address, err)
	}
	return nil
}

// Close closes the connection, if there is one.
func (s *Stream) Close() error {
	if s.conn == nil {
		return nil
	}
	err := s.conn.Close()
	s.conn = nil
	if err != nil {
		return fmt.Errorf("export: close the connection: %w", err)
	}
	return nil
}

// connect opens and handshakes a connection.
func (s *Stream) connect(ctx context.Context) error {
	raw, err := s.dial(ctx, "tcp", s.address)
	if err != nil {
		return fmt.Errorf("export: connect to %s: %w", s.address, err)
	}
	conn := tls.Client(raw, s.tls)
	hctx, cancel := context.WithTimeout(ctx, s.writeTimeout)
	defer cancel()
	if err := conn.HandshakeContext(hctx); err != nil {
		_ = raw.Close()
		return fmt.Errorf("export: TLS handshake with %s: %w", s.address, err)
	}
	s.conn = conn
	return nil
}

// alive reports whether the peer has left the connection open. A receiver
// that is listening sends nothing, so the read times out; an end of stream,
// an alert or any data means the connection is not to be written to.
func (s *Stream) alive() bool {
	if err := s.conn.SetReadDeadline(time.Now().Add(probeWait)); err != nil {
		return false
	}
	var one [1]byte
	_, err := s.conn.Read(one[:])
	_ = s.conn.SetReadDeadline(time.Time{})
	return errors.Is(err, os.ErrDeadlineExceeded)
}

// deadline is the earlier of the context's deadline and now plus timeout.
func deadline(ctx context.Context, timeout time.Duration) time.Time {
	d := time.Now().Add(timeout)
	if cd, ok := ctx.Deadline(); ok && cd.Before(d) {
		return cd
	}
	return d
}
