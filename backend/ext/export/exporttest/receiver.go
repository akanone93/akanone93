// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

// Package exporttest is a syslog receiver for tests of the audit export: a
// TLS server with a throwaway certificate authority that reads RFC 5425
// octet-counted frames and can be told to fail. It is for tests only.
package exporttest

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"
)

// Receiver is a TLS syslog receiver listening on the loopback interface.
type Receiver struct {
	ln    net.Listener
	caPEM []byte

	mu          sync.Mutex
	messages    []string
	connections int
	handshakes  bool // refuse the TLS handshake
	resetAfter  int  // reset a connection after this many frames; 0 never
	resetsLeft  int  // connections to reset before reading normally
	wg          sync.WaitGroup
}

// NewReceiver starts a receiver that is stopped with the test.
func NewReceiver(t testing.TB) *Receiver {
	t.Helper()
	ca, caKey := authority(t)
	cert := serverCertificate(t, ca, caKey)
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12})
	if err != nil {
		t.Fatal(err)
	}
	r := &Receiver{ln: ln, caPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Raw})}
	r.wg.Add(1)
	go r.serve()
	t.Cleanup(func() {
		_ = ln.Close()
		r.wg.Wait()
	})
	return r
}

// Address is host:port of the receiver.
func (r *Receiver) Address() string { return r.ln.Addr().String() }

// CAPEM is the certificate authority that signed the receiver's
// certificate.
func (r *Receiver) CAPEM() string { return string(r.caPEM) }

// Messages returns every message received so far, in arrival order,
// including those of a batch that was later reset.
func (r *Receiver) Messages() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.messages...)
}

// Connections counts the connections accepted.
func (r *Receiver) Connections() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.connections
}

// RefuseHandshakes makes the receiver drop every connection before the TLS
// handshake, or stop doing so.
func (r *Receiver) RefuseHandshakes(refuse bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handshakes = refuse
}

// ResetAfterFrames makes the next connections reset after reading n frames,
// as a receiver that crashes does: the connection ends with a reset and no
// close_notify. times connections are reset, then the receiver reads
// normally again.
func (r *Receiver) ResetAfterFrames(n, times int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.resetAfter, r.resetsLeft = n, times
}

// serve accepts connections until the listener closes.
func (r *Receiver) serve() {
	defer r.wg.Done()
	for {
		conn, err := r.ln.Accept()
		if err != nil {
			return
		}
		r.wg.Add(1)
		go r.handle(conn)
	}
}

// handle reads one connection's frames.
func (r *Receiver) handle(conn net.Conn) {
	defer r.wg.Done()
	r.mu.Lock()
	r.connections++
	refuse := r.handshakes
	resetAfter := 0
	if r.resetsLeft > 0 {
		r.resetsLeft--
		resetAfter = r.resetAfter
	}
	r.mu.Unlock()
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
	tlsConn := conn.(*tls.Conn)
	if refuse {
		abort(tlsConn.NetConn())
		return
	}
	handshakeCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := tlsConn.HandshakeContext(handshakeCtx); err != nil {
		abort(tlsConn.NetConn())
		return
	}
	br := bufio.NewReader(tlsConn)
	for frames := 0; ; frames++ {
		if resetAfter > 0 && frames == resetAfter {
			abort(tlsConn.NetConn())
			return
		}
		message, err := readFrame(br)
		if err != nil {
			_ = tlsConn.Close()
			return
		}
		r.mu.Lock()
		r.messages = append(r.messages, message)
		r.mu.Unlock()
	}
}

// abort ends a connection with a reset.
func abort(c net.Conn) {
	if tcp, ok := c.(*net.TCPConn); ok {
		_ = tcp.SetLinger(0)
	}
	_ = c.Close()
}

// readFrame reads one octet-counted frame.
func readFrame(br *bufio.Reader) (string, error) {
	prefix, err := br.ReadString(' ')
	if err != nil {
		return "", fmt.Errorf("exporttest.readFrame: read the octet count: %w", err)
	}
	n, err := strconv.Atoi(prefix[:len(prefix)-1])
	if err != nil || n < 0 {
		return "", errors.New("exporttest: bad octet count")
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(br, buf); err != nil {
		return "", fmt.Errorf("exporttest.readFrame: read the message: %w", err)
	}
	return string(buf), nil
}

// authority makes a throwaway certificate authority.
func authority(t testing.TB) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "exporttest CA"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return ca, key
}

// serverCertificate issues a certificate for 127.0.0.1 and localhost.
func serverCertificate(t testing.TB, ca *x509.Certificate, caKey *ecdsa.PrivateKey) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "exporttest receiver"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1)}, DNSNames: []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca, &key.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}
