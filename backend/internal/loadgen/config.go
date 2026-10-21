// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package loadgen

import (
	"errors"
	"fmt"
	"net/http"
	"time"
)

// Pacing is how often a device does each thing. The defaults are those of
// the runtime (SYN-060: control every five minutes in the foreground) and
// of a typical session: a manifest check on each start or resume, a
// telemetry batch each minute, a function call now and then.
type Pacing struct {
	Manifest  time.Duration
	Control   time.Duration
	Telemetry time.Duration
	Function  time.Duration
	// Jitter is the fraction of an interval by which each wait varies
	// either way, so that devices do not stay in step; 0 to 1.
	Jitter float64
}

// DefaultPacing is the pacing of a device in the foreground.
func DefaultPacing() Pacing {
	return Pacing{
		Manifest: 10 * time.Minute, Control: 5 * time.Minute,
		Telemetry: time.Minute, Function: 5 * time.Minute, Jitter: 0.2,
	}
}

// Scaled returns the pacing run factor times faster: a factor of 60 turns
// a five-minute interval into five seconds. The request rates of a scaled
// run are the real rates times the factor, so a report divides them by it.
func (p Pacing) Scaled(factor float64) Pacing {
	if factor <= 0 {
		factor = 1
	}
	div := func(d time.Duration) time.Duration { return time.Duration(float64(d) / factor) }
	return Pacing{Manifest: div(p.Manifest), Control: div(p.Control), Telemetry: div(p.Telemetry), Function: div(p.Function), Jitter: p.Jitter}
}

// validate checks that every interval is positive and the jitter a
// fraction. A zero Function interval turns function calls off.
func (p Pacing) validate() error {
	for name, d := range map[string]time.Duration{"manifest": p.Manifest, "control": p.Control, "telemetry": p.Telemetry} {
		if d <= 0 {
			return fmt.Errorf("the %s interval must be positive", name)
		}
	}
	if p.Function < 0 {
		return errors.New("the function interval must not be negative")
	}
	if p.Jitter < 0 || p.Jitter > 1 {
		return errors.New("the jitter must be between 0 and 1")
	}
	return nil
}

// Function is the server function a device calls now and then.
type Function struct {
	Name    string
	Version uint32
	// Input is the UTF-8 JSON input the function's schema accepts.
	Input []byte
}

// Applied is a device's report that it applied a control document.
type Applied struct {
	DeviceID string
	Version  int64
	// At is when the device saw the document, before it reported it.
	At time.Time
}

// Config describes one run.
type Config struct {
	// BaseURL is where devices reach the server, exactly as the server's
	// publicBaseURL names it: the DPoP proofs bind to it.
	BaseURL     string
	AppID       string
	Environment string
	// Channel is the channel the devices follow; empty is the default.
	Channel string
	Devices int
	// Duration is how long the fleet runs once registered.
	Duration time.Duration
	// Concurrency bounds the registrations in flight; zero means 32.
	Concurrency int
	Pacing      Pacing
	// Scale is the factor Pacing was scaled by, reported so that rates can
	// be brought back to real time; one when unset.
	Scale float64
	// Function, when set and its pacing interval positive, is called by
	// every device.
	Function *Function
	// PushTokens gives every device a push token and registers it, as a
	// host app with silent push does (SYN-060); Nudge then stands for the
	// push arriving.
	PushTokens bool
	// ForwardedFor gives each device its own source address in
	// X-Forwarded-For, for a server that trusts its proxy and rate-limits
	// per address (SRV-065).
	ForwardedFor bool
	// Seed makes the jitter and the first offsets repeatable.
	Seed uint64
	// HTTP is the client for every call; nil uses a default one.
	HTTP *http.Client
	// Now supplies the time; nil is the wall clock.
	Now func() time.Time
	// OnApplied, when set, is called as a device reports control_applied.
	// It runs on the device's goroutine and must not block.
	OnApplied func(Applied)
}

// withDefaults returns the configuration with its unset fields filled in,
// or an error naming what is wrong.
func (c Config) withDefaults() (Config, error) {
	switch {
	case c.BaseURL == "" || c.AppID == "":
		return c, errors.New("a base URL and an app are required")
	case c.Devices <= 0:
		return c, errors.New("at least one device is required")
	case c.Duration <= 0:
		return c, errors.New("the duration must be positive")
	}
	if c.Environment == "" {
		c.Environment = "staging"
	}
	if c.Concurrency <= 0 {
		c.Concurrency = 32
	}
	if c.Scale <= 0 {
		c.Scale = 1
	}
	if c.Pacing == (Pacing{}) {
		c.Pacing = DefaultPacing()
	}
	if c.HTTP == nil {
		c.HTTP = &http.Client{Timeout: 30 * time.Second}
	}
	if c.Now == nil {
		c.Now = time.Now
	}
	if err := c.Pacing.validate(); err != nil {
		return c, fmt.Errorf("loadgen.Config: %w", err)
	}
	return c, nil
}
