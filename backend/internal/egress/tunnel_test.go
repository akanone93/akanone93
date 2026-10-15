// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package egress

import (
	"bufio"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

// pipes returns the two ends a tunnel joins and the two ends a test
// drives: client <-> (proxy) <-> upstream.
func pipes() (client, proxyClient, proxyUpstream, upstream net.Conn) {
	client, proxyClient = net.Pipe()
	proxyUpstream, upstream = net.Pipe()
	return client, proxyClient, proxyUpstream, upstream
}

// runSplice starts a tunnel and returns a function that waits for its
// reason.
func runSplice(proxyClient, proxyUpstream net.Conn, lim spliceLimits, quit <-chan struct{}) (wait func() string, bytesCounted func(string) int64) {
	var mu sync.Mutex
	counted := map[string]int64{}
	t := &tunnel{limits: lim, now: time.Now, count: func(d string, n int64) {
		mu.Lock()
		defer mu.Unlock()
		counted[d] += n
	}}
	done := make(chan string, 1)
	go func() { done <- splice(quit, proxyClient, proxyUpstream, t) }()
	return func() string { return <-done }, func(d string) int64 {
		mu.Lock()
		defer mu.Unlock()
		return counted[d]
	}
}

// expectClosed reads until the connection ends and fails if it does not
// end within the time.
func expectClosed(t *testing.T, c net.Conn, within time.Duration) []byte {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(within))
	data, err := io.ReadAll(c)
	var netErr net.Error
	if err != nil && errors.As(err, &netErr) && netErr.Timeout() {
		t.Fatalf("the tunnel stayed open for %v", within)
	}
	return data
}

// Verifies: FN-017.
func TestSpliceCopiesBothWays_FN_017(t *testing.T) {
	t.Parallel()
	client, pc, pu, upstream := pipes()
	wait, counted := runSplice(pc, pu, spliceLimits{Idle: time.Second, Duration: 10 * time.Second, MaxBytes: 1 << 20}, nil)

	go func() { _, _ = client.Write([]byte("ping")) }()
	got := make([]byte, 4)
	if _, err := io.ReadFull(upstream, got); err != nil || string(got) != "ping" {
		t.Fatalf("to upstream: %q, %v", got, err)
	}
	go func() { _, _ = upstream.Write([]byte("pong!")) }()
	got = make([]byte, 5)
	if _, err := io.ReadFull(client, got); err != nil || string(got) != "pong!" {
		t.Fatalf("from upstream: %q, %v", got, err)
	}
	_ = client.Close()
	if reason := wait(); reason != endClosed {
		t.Errorf("reason = %q", reason)
	}
	expectClosed(t, upstream, time.Second)
	if counted(directionToUpstream) != 4 || counted(directionFromUpstream) != 5 {
		t.Errorf("counted %d up, %d down", counted(directionToUpstream), counted(directionFromUpstream))
	}
}

// Verifies: FN-017.
func TestSpliceStopsAtTheByteLimit_FN_017(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		toUp    bool
		payload int
	}{{"from upstream", false, 1000}, {"to upstream", true, 1000}} {
		t.Run(tc.name, func(t *testing.T) {
			client, pc, pu, upstream := pipes()
			wait, counted := runSplice(pc, pu, spliceLimits{Idle: time.Second, Duration: 10 * time.Second, MaxBytes: 100}, nil)
			src, sink := upstream, client
			dir := directionFromUpstream
			if tc.toUp {
				src, sink, dir = client, upstream, directionToUpstream
			}
			go func() { _, _ = src.Write(make([]byte, tc.payload)) }()
			data := expectClosed(t, sink, 5*time.Second)
			if len(data) != 100 {
				t.Errorf("carried %d bytes, want exactly the limit of 100", len(data))
			}
			if reason := wait(); reason != endBytes {
				t.Errorf("reason = %q", reason)
			}
			if counted(dir) != 100 {
				t.Errorf("counted %d", counted(dir))
			}
		})
	}
}

// Verifies: FN-017.
func TestSpliceIsIdleAfterTheTimeout_FN_017(t *testing.T) {
	t.Parallel()
	client, pc, pu, upstream := pipes()
	start := time.Now()
	wait, _ := runSplice(pc, pu, spliceLimits{Idle: 150 * time.Millisecond, Duration: 10 * time.Second, MaxBytes: 1 << 20}, nil)
	expectClosed(t, client, 5*time.Second)
	expectClosed(t, upstream, 5*time.Second)
	if reason := wait(); reason != endIdle {
		t.Errorf("reason = %q", reason)
	}
	if elapsed := time.Since(start); elapsed < 150*time.Millisecond {
		t.Errorf("closed after %v, before the idle timeout", elapsed)
	}
}

// Verifies: FN-017.
// Idleness belongs to the tunnel: traffic in one direction keeps it open
// while the other direction is silent.
func TestSpliceStaysOpenWhileOneDirectionIsActive_FN_017(t *testing.T) {
	t.Parallel()
	client, pc, pu, upstream := pipes()
	wait, _ := runSplice(pc, pu, spliceLimits{Idle: 200 * time.Millisecond, Duration: 10 * time.Second, MaxBytes: 1 << 20}, nil)
	go func() { _, _ = io.Copy(io.Discard, upstream) }()
	for range 8 {
		if _, err := client.Write([]byte("x")); err != nil {
			t.Fatalf("the tunnel closed while active: %v", err)
		}
		time.Sleep(60 * time.Millisecond)
	}
	_ = client.Close()
	if reason := wait(); reason != endClosed {
		t.Errorf("reason = %q", reason)
	}
}

// Verifies: FN-017.
func TestSpliceEndsAtTheMaximumDuration_FN_017(t *testing.T) {
	t.Parallel()
	client, pc, pu, upstream := pipes()
	wait, _ := runSplice(pc, pu, spliceLimits{Idle: time.Second, Duration: 250 * time.Millisecond, MaxBytes: 1 << 20}, nil)
	go func() { _, _ = io.Copy(io.Discard, upstream) }()
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		for {
			select {
			case <-stop:
				return
			case <-time.After(20 * time.Millisecond):
				if _, err := client.Write([]byte("x")); err != nil {
					return
				}
			}
		}
	}()
	if reason := wait(); reason != endDuration {
		t.Errorf("reason = %q, want %q: an active tunnel still has a maximum age", reason, endDuration)
	}
}

// Verifies: FN-017.
func TestSpliceEndsOnShutdown_FN_017(t *testing.T) {
	t.Parallel()
	client, pc, pu, upstream := pipes()
	quit := make(chan struct{})
	wait, _ := runSplice(pc, pu, spliceLimits{Idle: time.Minute, Duration: time.Minute, MaxBytes: 1 << 20}, quit)
	close(quit)
	if reason := wait(); reason != endShutdown {
		t.Errorf("reason = %q", reason)
	}
	expectClosed(t, client, time.Second)
	expectClosed(t, upstream, time.Second)
}

// Verifies: FN-017.
func TestBufferedConnServesBufferedBytesFirst_FN_017(t *testing.T) {
	t.Parallel()
	a, b := net.Pipe()
	defer func() { _ = a.Close() }()
	go func() { _, _ = b.Write([]byte("early-bytes")); _ = b.Close() }()
	c := &bufferedConn{Conn: a, r: newReader(a)}
	data, err := io.ReadAll(c)
	if err != nil || string(data) != "early-bytes" {
		t.Errorf("read %q, %v", data, err)
	}
}

func newReader(c net.Conn) *bufio.Reader { return bufio.NewReader(c) }
