// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package licensefile

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

// Errors a licence file can fail with. They are matched with errors.Is; the
// message names the reason without quoting the file's content.
var (
	// ErrFormat is a file that is not a well-formed licence.
	ErrFormat = errors.New("licence file is malformed")
	// ErrSignature is a file no trusted key signed, or whose signed part was
	// changed after signing.
	ErrSignature = errors.New("licence file has no valid signature from a trusted key")
	// ErrLimitKey is a limit that the limits registry does not know
	// (LIM-001).
	ErrLimitKey = errors.New("licence file names a limit the registry does not know")
)

// Edition is the only licence edition there is.
const Edition = "enterprise"

// DefaultGraceDays is the grace period of a licence that names none
// (GOV-030).
const DefaultGraceDays = 30

// maxGraceDays bounds the grace period a licence may grant.
const maxGraceDays = 365

// Payload is the signed part of a licence file (GOV-030, ADR-0084).
type Payload struct {
	LicenseID    string `json:"licenseId"`
	Organization string `json:"organization"`
	// Edition is "enterprise".
	Edition string `json:"edition"`
	// IssuedAt, NotBefore and ExpiresAt are RFC 3339 instants.
	IssuedAt  string `json:"issuedAt"`
	NotBefore string `json:"notBefore"`
	ExpiresAt string `json:"expiresAt"`
	// GraceDays is the number of days after ExpiresAt in which enterprise
	// features keep working with warnings; nil means DefaultGraceDays.
	GraceDays *int `json:"graceDays,omitempty"`
	// Features names the enterprise features the licence enables.
	Features []string `json:"features"`
	// Limits caps registry limits (LIM-001) for this installation; the keys
	// must exist in schema/limits.json.
	Limits map[string]int64 `json:"limits"`
	// InstallationID, when set, binds the licence to one installation.
	InstallationID string `json:"installationId,omitempty"`
}

// Grace returns the grace period in days.
func (p Payload) Grace() int {
	if p.GraceDays == nil {
		return DefaultGraceDays
	}
	return *p.GraceDays
}

// Window returns the instants the payload names.
func (p Payload) Window() (issued, notBefore, expires time.Time, err error) {
	if issued, err = time.Parse(time.RFC3339, p.IssuedAt); err != nil {
		return time.Time{}, time.Time{}, time.Time{}, fmt.Errorf("%w: issuedAt: %w", ErrFormat, err)
	}
	if notBefore, err = time.Parse(time.RFC3339, p.NotBefore); err != nil {
		return time.Time{}, time.Time{}, time.Time{}, fmt.Errorf("%w: notBefore: %w", ErrFormat, err)
	}
	if expires, err = time.Parse(time.RFC3339, p.ExpiresAt); err != nil {
		return time.Time{}, time.Time{}, time.Time{}, fmt.Errorf("%w: expiresAt: %w", ErrFormat, err)
	}
	return issued, notBefore, expires, nil
}

var (
	featurePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)
	idPattern      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
)

// Validate checks the payload's shape: names, dates, grace and limits.
func (p Payload) Validate() error {
	switch {
	case !idPattern.MatchString(p.LicenseID):
		return fmt.Errorf("%w: licenseId", ErrFormat)
	case p.Organization == "" || len(p.Organization) > 256:
		return fmt.Errorf("%w: organization", ErrFormat)
	case p.Edition != Edition:
		return fmt.Errorf("%w: edition must be %q", ErrFormat, Edition)
	case p.InstallationID != "" && !idPattern.MatchString(p.InstallationID):
		return fmt.Errorf("%w: installationId", ErrFormat)
	}
	issued, notBefore, expires, err := p.Window()
	if err != nil {
		return err
	}
	if !expires.After(notBefore) || notBefore.Before(issued) {
		return fmt.Errorf("%w: the dates must satisfy issuedAt <= notBefore < expiresAt", ErrFormat)
	}
	if g := p.Grace(); g < 0 || g > maxGraceDays {
		return fmt.Errorf("%w: graceDays must be between 0 and %d", ErrFormat, maxGraceDays)
	}
	seen := map[string]bool{}
	for _, f := range p.Features {
		if !featurePattern.MatchString(f) || seen[f] {
			return fmt.Errorf("%w: feature %q is malformed or repeated", ErrFormat, f)
		}
		seen[f] = true
	}
	for k, v := range p.Limits {
		def, ok := limits.Lookup(limits.Key(k))
		if !ok {
			return fmt.Errorf("%w: %q", ErrLimitKey, k)
		}
		if v <= 0 || v > def.Max {
			return fmt.Errorf("%w: limit %q must be between 1 and its maximum %d", ErrFormat, k, def.Max)
		}
	}
	return nil
}

// decodeStrict decodes JSON, refusing fields the type does not have and
// trailing data.
func decodeStrict(raw []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("%w: %w", ErrFormat, err)
	}
	if dec.More() {
		return fmt.Errorf("%w: trailing data", ErrFormat)
	}
	return nil
}
