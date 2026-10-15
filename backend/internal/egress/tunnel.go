// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package egress

import (
	"bufio"
	"errors"
	"net"
	"os"
	"sync/atomic"
	"time"
)

// copyBufferSize is the size of the buffer each direction copies through.
const copyBufferSize = 16 << 10

// How a tunnel ended, for the log.
const (
	endClosed   = "closed"
	endIdle     = "idle"
	endDuration = "duration"
	endBytes    = "bytes"
	endShutdown = "shutdown"
)

// spliceLimits bound one tunnel (FN-017).
type spliceLimits struct {
	// Idle ends a tunnel with no byte in either direction for this long.
	Idle time.Duration
	// Duration ends a tunnel that has been open this long.
	Duration time.Duration
	// MaxBytes ends a tunnel once one direction has carried this many.
	MaxBytes int64
}

// tunnel copies bytes between two connections until one of the limits is
// reached or either side closes. It is used once and is not reusable.
type tunnel struct {
	limits spliceLimits
	now    func() time.Time
	count  func(direction string, n int64)
	// last is the time of the latest byte in either direction, in Unix
	// nanoseconds.
	last atomic.Int64
}

// splice runs the tunnel and reports how it ended. Whichever direction
// finishes first closes both connections, so no half-open tunnel
// outlives its purpose; both connections are closed when it returns.
// Closing quit ends the tunnel at once.
func splice(quit <-chan struct{}, client, upstream net.Conn, t *tunnel) string {
	t.last.Store(t.now().UnixNano())
	ended := make(chan string, 2)
	go func() { ended <- t.copy(upstream, client, directionToUpstream) }()
	go func() { ended <- t.copy(client, upstream, directionFromUpstream) }()

	timer := time.NewTimer(t.limits.Duration)
	defer timer.Stop()
	var reason string
	select {
	case reason = <-ended:
		_ = client.Close()
		_ = upstream.Close()
		<-ended
	case <-timer.C:
		reason = endDuration
		_ = client.Close()
		_ = upstream.Close()
		<-ended
		<-ended
	case <-quit:
		reason = endShutdown
		_ = client.Close()
		_ = upstream.Close()
		<-ended
		<-ended
	}
	return reason
}

// copy moves bytes from src to dst until the stream ends, the tunnel has
// been idle too long, or the direction has carried the maximum. A read
// that times out while the other direction is active is retried: idleness
// is a property of the tunnel, not of one direction.
func (t *tunnel) copy(dst, src net.Conn, direction string) string {
	buf := make([]byte, copyBufferSize)
	var total int64
	for {
		_ = src.SetReadDeadline(t.now().Add(t.limits.Idle))
		n, err := src.Read(buf)
		if n > 0 {
			t.last.Store(t.now().UnixNano())
			written, end := t.forward(dst, buf[:n], t.limits.MaxBytes-total, direction)
			total += written
			if end != "" {
				return end
			}
		}
		if err == nil {
			continue
		}
		if errors.Is(err, os.ErrDeadlineExceeded) && t.idleFor() < t.limits.Idle {
			continue
		}
		if errors.Is(err, os.ErrDeadlineExceeded) {
			return endIdle
		}
		return endClosed
	}
}

// forward writes one chunk to dst, truncated to room bytes. It returns the
// bytes written and, when the tunnel must end, the reason; otherwise the
// reason is empty.
func (t *tunnel) forward(dst net.Conn, chunk []byte, room int64, direction string) (int64, string) {
	over := int64(len(chunk)) > room
	if over {
		chunk = chunk[:room]
	}
	var written int64
	if len(chunk) > 0 {
		_ = dst.SetWriteDeadline(t.now().Add(t.limits.Idle))
		n, err := dst.Write(chunk)
		written = int64(n)
		t.count(direction, written)
		if err != nil {
			return written, endClosed
		}
	}
	if over {
		return written, endBytes
	}
	return written, ""
}

// idleFor is the time since the latest byte in either direction.
func (t *tunnel) idleFor() time.Duration {
	return t.now().Sub(time.Unix(0, t.last.Load()))
}

// bufferedConn serves the bytes the HTTP server had already read from
// the connection before it was hijacked, then the connection itself.
type bufferedConn struct {
	net.Conn
	r *bufio.Reader
}

// Read returns buffered bytes first.
func (c *bufferedConn) Read(p []byte) (int, error) { return c.r.Read(p) } //nolint:wrapcheck // net.Conn contract: callers compare io.EOF and timeouts.
