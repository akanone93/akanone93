// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package export

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"time"
)

// Record is one audit entry as a sink receives it.
//
// EntryHash is the hex SHA-256 of the fields in this order, each written as
// its length in decimal, a colon and the value: ID, OrganizationID, Sequence
// in decimal, At as RFC 3339 in UTC with nanoseconds, ActorKind, ActorID,
// ActorDisplay, Action, TargetKind, TargetID, SourceIP, UserAgent,
// RequestID, BeforeHash, AfterHash, PreviousHash and, only when it is not
// empty, Detail. A SIEM that recomputes it, and checks that PreviousHash is
// the previous record's EntryHash, verifies the chain.
type Record struct {
	ID             string
	OrganizationID string
	// Sequence is the record's position in the organisation's chain, from 1.
	Sequence     int64
	At           time.Time
	ActorKind    string
	ActorID      string
	ActorDisplay string
	Action       string
	TargetKind   string
	TargetID     string
	SourceIP     string
	UserAgent    string
	RequestID    string
	BeforeHash   string
	AfterHash    string
	Detail       string
	// PreviousHash is the previous record's EntryHash, empty for the first.
	PreviousHash string
	EntryHash    string
}

// Sink delivers batches of records to one destination.
//
// Deliver returns nil only when the destination has acknowledged every
// record of the batch; the core then moves the destination's cursor past
// it. Any error leaves the cursor where it was and the whole batch is
// delivered again, so a sink may see a record twice and must not drop one.
// Records arrive in chain order. A sink that holds a connection also
// implements io.Closer, and the core closes it when a run is over.
type Sink interface {
	// Name identifies the kind of sink, such as "syslog".
	Name() string
	// Deliver sends the batch and waits for the destination's
	// acknowledgement.
	Deliver(ctx context.Context, records []Record) error
}

// DialFunc opens a connection the way net.Dialer.DialContext does.
type DialFunc func(ctx context.Context, network, address string) (net.Conn, error)

// Destination describes where a sink delivers.
type Destination struct {
	ID             string
	OrganizationID string
	Name           string
	// Kind is the sink kind the destination was registered with.
	Kind string
	// Address is host:port for the stream kinds and a URL for the HTTP kinds.
	Address string
}

// Config is what the core gives a Provider to build a sink.
type Config struct {
	Destination Destination
	// Hostname identifies this server in the messages it sends.
	Hostname string
	// Secret is the destination's signing secret, empty when it has none.
	Secret []byte
	// TLS verifies the destination's certificate and may present a client
	// certificate. For the stream kinds its ServerName is the address's
	// host.
	TLS *tls.Config
	// Dial opens connections and refuses addresses the egress policy
	// forbids. A sink must open every connection through it.
	Dial DialFunc
	// HTTP sends requests the same way, for the HTTP kinds.
	HTTP *http.Client
	// Now is the clock.
	Now func() time.Time
}

// Provider builds sinks for the kinds it names. The enterprise edition
// registers one; the core knows only the syslog kind.
type Provider interface {
	// Kinds lists the destination kinds the provider serves.
	Kinds() []string
	// Validate says why a destination's address or signing secret is not
	// valid for its kind; it opens no connection. The secret is empty when
	// the destination has none.
	Validate(d Destination, secret []byte) error
	// New builds a sink for a destination of one of those kinds, or says why
	// the destination is not valid.
	New(ctx context.Context, c Config) (Sink, error)
}
