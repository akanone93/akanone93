// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package export

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"time"
)

// Stream sends batches to a syslog receiver over TLS (RFC 5425). Every
// batch has a connection of its own and is acknowledged by the way the
// connection ends, which is the strongest acknowledgement a syslog receiver
// can give: RFC 5425 has none per message. Send writes the batch, tells the
// receiver that nothing more is coming (TLS close_notify) and waits for the
// receiver to finish reading and close. A receiver that dies, resets the
// connection, refuses the handshake or does not close within the timeout
// fails the batch, which the caller then sends again whole. A batch is
// therefore never taken for delivered because the TLS layer accepted its
// bytes; it can still arrive twice. It is safe for concurrent use.
type Stream struct {
	address string
	dial    DialFunc
	tls     *tls.Config
	timeout time.Duration
}

// NewStream returns a stream to address. The TLS configuration must verify
// the server: it is used as given, so its ServerName is the name the
// certificate must carry. timeout bounds the handshake, the write and the
// wait for the receiver to close.
func NewStream(address string, dial DialFunc, cfg *tls.Config, timeout time.Duration) (*Stream, error) {
	if dial == nil || cfg == nil || cfg.InsecureSkipVerify {
		return nil, errors.New("export: a stream needs a dialer and a TLS configuration that verifies the server")
	}
	if timeout <= 0 {
		return nil, errors.New("export: a stream needs a timeout")
	}
	return &Stream{address: address, dial: dial, tls: cfg, timeout: timeout}, nil
}

// Send delivers data to the receiver and returns once the receiver has read
// it all and closed the connection.
func (s *Stream) Send(ctx context.Context, data []byte) error {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	raw, err := s.dial(ctx, "tcp", s.address)
	if err != nil {
		return fmt.Errorf("export: connect to %s: %w", s.address, err)
	}
	defer func() { _ = raw.Close() }()
	stop := context.AfterFunc(ctx, func() { _ = raw.Close() })
	defer stop()
	if d, ok := ctx.Deadline(); ok {
		if err := raw.SetDeadline(d); err != nil {
			return fmt.Errorf("export: set the deadline: %w", err)
		}
	}
	conn := tls.Client(raw, s.tls)
	if err := conn.HandshakeContext(ctx); err != nil {
		return fmt.Errorf("export: TLS handshake with %s: %w", s.address, err)
	}
	if _, err := conn.Write(data); err != nil {
		return fmt.Errorf("export: write to %s: %w", s.address, err)
	}
	if err := conn.CloseWrite(); err != nil {
		return fmt.Errorf("export: finish the write to %s: %w", s.address, err)
	}
	if _, err := io.Copy(io.Discard, conn); err != nil {
		return fmt.Errorf("export: wait for %s to close: %w", s.address, err)
	}
	return nil
}
