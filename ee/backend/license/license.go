// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

// Package license enforces the Plux enterprise licence (GOV-030, ADR-0084).
//
// It reads the licence file named by PLUX_LICENSE_FILE, verifies it offline
// against the public keys compiled into the build, and turns the result into
// an edition.Status that the core asks per request. It re-verifies every
// hour, so a replaced file, an expiry or the end of a grace period take
// effect without a restart. Nothing here calls out of the process.
//
// The states are:
//
//   - absent: no file; enterprise features are off and the core runs fully;
//   - valid: signed by a trusted key, inside [notBefore, expiresAt);
//   - grace: past expiresAt but inside graceDays; features keep working and
//     the process warns;
//   - expired: past the grace period; features are read-only, existing data
//     stays readable and exportable and nothing is deleted;
//   - invalid: unreadable, malformed, unsigned or signed by an unknown key,
//     not yet valid, bound to another installation, or naming a limit the
//     registry does not know; features are off.
//
// No state refuses device traffic or turns a security capability off
// (GOV-032): the package only answers questions about enterprise features.
package license

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"math"
	"os"
	"sync"
	"time"

	"github.com/nightCode42/plux3/backend/edition"
	"github.com/nightCode42/plux3/backend/edition/licensefile"
)

// DefaultInterval is how often a provider re-verifies the licence.
const DefaultInterval = time.Hour

// day is the length of a licence day.
const day = 24 * time.Hour

// Verifier opens providers. It implements edition.Verifier.
type Verifier struct {
	// Keys are the public keys a licence may be signed with; use
	// licensefile.TrustedKeys in a binary.
	Keys licensefile.KeySet
	// Now is the clock; nil is time.Now.
	Now func() time.Time
	// Interval is the re-verification period; zero is DefaultInterval.
	Interval time.Duration
	// ReadFile reads the licence file; nil is os.ReadFile.
	ReadFile func(path string) ([]byte, error)
}

var _ edition.Verifier = Verifier{}

// Provider is a licence that re-verifies itself. It implements
// edition.Provider and is safe for concurrent use.
type Provider struct {
	v    Verifier
	set  edition.Settings
	log  *slog.Logger
	mu   sync.RWMutex
	last edition.Status
}

var _ edition.Provider = (*Provider)(nil)

// Open verifies the licence now and starts re-verifying it every Interval
// until ctx ends. An absent or invalid licence is not an error: the provider
// reports it and the core runs with enterprise features off.
func (v Verifier) Open(ctx context.Context, s edition.Settings) (edition.Provider, error) {
	log := s.Log
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	p := &Provider{v: v, set: s, log: log}
	p.Check(ctx)
	interval := v.Interval
	if interval <= 0 {
		interval = DefaultInterval
	}
	go p.run(ctx, interval)
	return p, nil
}

// run re-verifies until ctx ends.
func (p *Provider) run(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			p.Check(ctx)
		}
	}
}

// Status returns the result of the latest verification.
func (p *Provider) Status() edition.Status {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.last
}

// Check reads and verifies the licence file now, stores the result and logs
// a warning while the licence needs attention.
func (p *Provider) Check(ctx context.Context) {
	st := p.v.Evaluate(p.set.File, p.set.InstallationID)
	p.mu.Lock()
	changed := p.last.State != st.State
	p.last = st
	p.mu.Unlock()
	switch st.State {
	case edition.StateGrace, edition.StateExpired, edition.StateInvalid:
		p.log.WarnContext(ctx, "licence: "+st.Message,
			slog.String("state", string(st.State)), slog.Int("daysRemaining", st.DaysRemaining))
	case edition.StateValid:
		if changed {
			p.log.InfoContext(ctx, "licence: "+st.Message, slog.String("licenseId", st.LicenseID))
		}
	case edition.StateAbsent:
		if changed {
			p.log.InfoContext(ctx, "licence: "+st.Message)
		}
	}
}

// Evaluate reads the file at path and returns what the licence allows now.
// An empty path or a missing file is absent. Evaluate never returns an error:
// every failure is a state.
func (v Verifier) Evaluate(path, installationID string) edition.Status {
	if path == "" {
		return edition.Status{State: edition.StateAbsent, Message: "no licence file is configured: enterprise features are off"}
	}
	read := v.ReadFile
	if read == nil {
		read = os.ReadFile
	}
	raw, err := read(path)
	if errors.Is(err, fs.ErrNotExist) {
		return edition.Status{State: edition.StateAbsent, Message: "the licence file does not exist: enterprise features are off"}
	}
	if err != nil {
		return invalid("the licence file cannot be read")
	}
	return v.EvaluateBytes(raw, installationID)
}

// EvaluateBytes verifies a licence file's content and returns what it allows
// now.
func (v Verifier) EvaluateBytes(raw []byte, installationID string) edition.Status {
	now := time.Now
	if v.Now != nil {
		now = v.Now
	}
	f, err := licensefile.Parse(raw)
	switch {
	case errors.Is(err, licensefile.ErrLimitKey):
		return invalid("the licence names a limit the limits registry does not know")
	case err != nil:
		return invalid("the licence file is malformed")
	}
	if err := f.Verify(v.Keys); err != nil {
		return invalid("the licence is not signed by a trusted key")
	}
	p := f.Payload
	if p.InstallationID != "" && p.InstallationID != installationID {
		return invalid("the licence is bound to another installation")
	}
	_, notBefore, expires, err := p.Window()
	if err != nil {
		return invalid("the licence file is malformed")
	}
	at := now()
	st := edition.Status{
		LicenseID: p.LicenseID, Organization: p.Organization,
		ExpiresAt: expires, GraceEndsAt: expires.AddDate(0, 0, p.Grace()),
		Limits: p.Limits,
	}
	for _, name := range p.Features {
		st.Features = append(st.Features, edition.Feature(name))
	}
	st.DaysRemaining = int(math.Floor(float64(expires.Sub(at)) / float64(day)))
	switch {
	case at.Before(notBefore):
		return invalid(fmt.Sprintf("the licence is not valid before %s", notBefore.UTC().Format(time.DateOnly)))
	case at.Before(expires):
		st.State = edition.StateValid
		st.Message = fmt.Sprintf("the licence for %s is valid for %d more days", p.Organization, st.DaysRemaining)
	case at.Before(st.GraceEndsAt):
		st.State = edition.StateGrace
		st.Message = fmt.Sprintf("the licence for %s expired on %s; enterprise features stop accepting changes on %s",
			p.Organization, expires.UTC().Format(time.DateOnly), st.GraceEndsAt.UTC().Format(time.DateOnly))
	default:
		st.State = edition.StateExpired
		st.Message = fmt.Sprintf("the licence for %s expired on %s and its grace period has ended: enterprise features are read-only",
			p.Organization, expires.UTC().Format(time.DateOnly))
	}
	return st
}

// invalid is the status of a file that must not be trusted. It carries no
// content of the file: the message is one of a fixed set.
func invalid(message string) edition.Status {
	return edition.Status{State: edition.StateInvalid, Message: message + ": enterprise features are off"}
}
