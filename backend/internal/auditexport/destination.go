// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package auditexport

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// The destination kinds. Only KindSyslog is part of the core; the others
// are enterprise sinks (GOV-030).
const (
	KindSyslog  = "syslog"
	KindCEF     = "cef"
	KindOTLP    = "otlp"
	KindWebhook = "webhook"
)

// namePattern is the form of a destination name; the database enforces it
// too.
var namePattern = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)

// maxNameLength bounds a destination name.
const maxNameLength = 64

// Warning names an unsafe setting of a destination. Each is reported to
// whoever reads the destination and in the log of every run.
const (
	// WarningPrivateAddress: the destination may be a private or loopback
	// address, which the egress guard otherwise refuses.
	WarningPrivateAddress = "private addresses are allowed for this destination; use this for development only"
)

// Destination is a registered SIEM destination and how far delivery has
// got.
type Destination struct {
	ID             string
	OrganizationID string
	Name           string
	Kind           string
	Address        string
	Enabled        bool
	AllowPrivate   bool
	CAPEM          string
	// CredentialsHint says what is stored sealed, never the value.
	CredentialsHint string
	// CursorSequence is the last entry the sink acknowledged and CursorHash
	// its hash.
	CursorSequence int64
	CursorHash     string
	ExportedCount  int64
	Failures       int32
	NextAttemptAt  time.Time
	LastError      string
	LastErrorAt    time.Time
	LastSuccessAt  time.Time
	Version        int64
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Warnings lists the unsafe settings of the destination.
func (d Destination) Warnings() []string {
	if d.AllowPrivate {
		return []string{WarningPrivateAddress}
	}
	return nil
}

// Credentials are the secrets of a destination. They are sealed before they
// are stored and never returned.
type Credentials struct {
	// ClientCertificatePEM and ClientKeyPEM are the client certificate and
	// key for mutual TLS; both or neither.
	ClientCertificatePEM string `json:"clientCertificate,omitempty"`
	ClientKeyPEM         string `json:"clientKey,omitempty"`
	// SigningSecret signs the requests of a webhook destination.
	SigningSecret string `json:"signingSecret,omitempty"`
}

// IsZero reports whether there are no credentials.
func (c Credentials) IsZero() bool { return c == Credentials{} }

// hint says which credentials are held, without any of their content.
func (c Credentials) hint() string {
	var parts []string
	if c.ClientCertificatePEM != "" {
		parts = append(parts, "client certificate")
	}
	if c.SigningSecret != "" {
		parts = append(parts, "signing secret")
	}
	return strings.Join(parts, ", ")
}

// check reports why the credentials are not usable.
func (c Credentials) check(kind string) error {
	if (c.ClientCertificatePEM == "") != (c.ClientKeyPEM == "") {
		return plxerr.New(plxerr.MissingProperty, "a client certificate needs its key, and a key its certificate")
	}
	if c.ClientCertificatePEM != "" {
		if _, err := tls.X509KeyPair([]byte(c.ClientCertificatePEM), []byte(c.ClientKeyPEM)); err != nil {
			return plxerr.New(plxerr.InvalidFormat, "the client certificate and key are not a valid pair")
		}
	}
	if c.SigningSecret != "" && kind != KindWebhook {
		return plxerr.New(plxerr.InvalidFormat, "only a webhook destination has a signing secret")
	}
	return nil
}

// NewDestination describes a destination to register.
type NewDestination struct {
	Name    string
	Kind    string
	Address string
	// AllowPrivate lets the destination be a private or loopback address.
	AllowPrivate bool
	// CAPEM pins the roots that verify the destination's certificate.
	CAPEM       string
	Credentials Credentials
	// StartAfterSequence is the last entry the destination already has; zero
	// delivers the chain from its first entry.
	StartAfterSequence int64
}

// Change replaces the settings of a destination; its name, kind and cursor
// stay.
type Change struct {
	ID           string
	Address      string
	Enabled      bool
	AllowPrivate bool
	CAPEM        string
	// Credentials replace the stored ones when any is set; the zero value
	// keeps them.
	Credentials Credentials
	// ClearCredentials removes the stored credentials.
	ClearCredentials bool
}

// checkName reports why a name is not valid.
func checkName(name string) error {
	if name == "" || len(name) > maxNameLength || !namePattern.MatchString(name) {
		return plxerr.New(plxerr.InvalidFormat,
			"a destination name is lower-case words joined by hyphens, starting with a letter, up to %d characters", maxNameLength)
	}
	return nil
}

// fromRow converts a stored destination.
func fromRow(row dbgen.AuditExportDestination) Destination {
	return Destination{
		ID: storage.ID(row.ID), OrganizationID: storage.ID(row.OrganizationID), Name: row.Name, Kind: row.Kind,
		Address: row.Address, Enabled: row.Enabled, AllowPrivate: row.AllowPrivate, CAPEM: row.CaPem,
		CredentialsHint: row.CredentialsHint, CursorSequence: row.CursorSequence, CursorHash: row.CursorHash,
		ExportedCount: row.ExportedCount, Failures: row.Failures, NextAttemptAt: storage.Time(row.NextAttemptAt),
		LastError: row.LastError, LastErrorAt: storage.Time(row.LastErrorAt), LastSuccessAt: storage.Time(row.LastSuccessAt),
		Version: row.Version, CreatedAt: storage.Time(row.CreatedAt), UpdatedAt: storage.Time(row.UpdatedAt),
	}
}

// bindingOf ties sealed credentials to their destination, so a copy moved to
// another row does not open.
func bindingOf(destinationID string) []byte {
	return []byte("audit-export-credentials:" + destinationID)
}

// marshalCredentials encodes credentials for sealing, in the form the
// struct tags of Credentials read back.
func marshalCredentials(c Credentials) ([]byte, error) {
	fields := map[string]string{}
	for key, value := range map[string]string{
		"clientCertificate": c.ClientCertificatePEM, "clientKey": c.ClientKeyPEM, "signingSecret": c.SigningSecret,
	} {
		if value != "" {
			fields[key] = value
		}
	}
	b, err := json.Marshal(fields)
	if err != nil {
		return nil, fmt.Errorf("auditexport: encode the credentials: %w", err)
	}
	return b, nil
}

// unmarshalCredentials decodes opened credentials.
func unmarshalCredentials(b []byte) (Credentials, error) {
	var c Credentials
	if err := json.Unmarshal(b, &c); err != nil {
		return Credentials{}, fmt.Errorf("auditexport: decode the credentials: %w", err)
	}
	return c, nil
}
