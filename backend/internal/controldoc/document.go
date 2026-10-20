// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package controldoc

import (
	"cmp"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"time"
)

// Identifier is the file identifier of an encoded control document.
const Identifier = "PLXC"

// Scope is what a kill switch disables (REL-060).
type Scope string

// The scopes of a kill switch. The spellings are those of the API and the
// command line; the wire form is controlfbs.SwitchScope.
const (
	ScopePlugin      Scope = "plugin"
	ScopePage        Scope = "page"
	ScopeActionGraph Scope = "actionGraph"
	ScopeDataSource  Scope = "dataSource"
	ScopeFunction    Scope = "function"
)

// Scopes lists every scope in the order of the wire enumeration.
func Scopes() []Scope {
	return []Scope{ScopePlugin, ScopePage, ScopeActionGraph, ScopeDataSource, ScopeFunction}
}

var (
	// ErrInvalid means a document breaks the rules of its schema.
	ErrInvalid = errors.New("controldoc: invalid document")
	// ErrMalformed means bytes that are not a control document.
	ErrMalformed = errors.New("controldoc: malformed buffer")
)

// KillSwitch disables one thing. A targeting rule (E4, E5) is reserved in
// the wire format and not modelled here: the server never writes one.
type KillSwitch struct {
	Scope  Scope
	Target string
}

// MandatoryUpdate names the release every device must activate before it
// shows another Plux page (REL-070).
type MandatoryUpdate struct {
	// ReleaseID is the release's identifier, for reports.
	ReleaseID string
	// ReleaseSequence is the number the manifest calls releaseSequence.
	ReleaseSequence uint64
	// MessageKey is the localisation key of the update screen's message;
	// empty means the host's default.
	MessageKey string
}

// ForcedRollback names the earlier release a device must activate from
// its retained bundles.
type ForcedRollback struct {
	ReleaseID       string
	ReleaseSequence uint64
}

// Document is the control document of one app, environment and channel.
type Document struct {
	// Version increases by at least one with every document of a channel.
	Version     uint64
	AppID       string
	Environment string
	Channel     string
	// IssuedAt and ExpiresAt are whole seconds in UTC.
	IssuedAt, ExpiresAt time.Time
	KillSwitches        []KillSwitch
	MandatoryUpdate     *MandatoryUpdate
	ForcedRollback      *ForcedRollback
}

// MaxText bounds an identifier, a key and a kill switch target.
const MaxText = 128

var (
	// targetPattern is the shape of a target of any scope: a plugin key, a
	// document UUID or a dotted function name all fit.
	targetPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	// textPattern is the shape of the identifiers and keys of a document:
	// printable ASCII without spaces.
	textPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]*$`)
)

// Validate checks a document against the rules of the schema: a version, an
// app, environment and channel, an expiry after the issue time, known
// scopes, well-formed and unrepeated targets, and releases that name a
// sequence.
func (d Document) Validate() error {
	switch {
	case d.Version < 1:
		return fmt.Errorf("%w: version %d", ErrInvalid, d.Version)
	case !validText(d.AppID) || !validText(d.Environment) || !validText(d.Channel):
		return fmt.Errorf("%w: the app, environment and channel are required identifiers", ErrInvalid)
	case !d.ExpiresAt.After(d.IssuedAt):
		return fmt.Errorf("%w: the document expires at %s, not after it was issued at %s", ErrInvalid,
			d.ExpiresAt.UTC().Format(time.RFC3339), d.IssuedAt.UTC().Format(time.RFC3339))
	}
	seen := make(map[KillSwitch]bool, len(d.KillSwitches))
	for _, k := range d.KillSwitches {
		if err := k.validate(); err != nil {
			return err
		}
		if seen[k] {
			return fmt.Errorf("%w: kill switch %s %q is listed twice", ErrInvalid, k.Scope, k.Target)
		}
		seen[k] = true
	}
	return d.validateReleases()
}

// validateReleases checks the mandatory update and the forced rollback.
func (d Document) validateReleases() error {
	if m := d.MandatoryUpdate; m != nil {
		if err := validRelease("mandatory update", m.ReleaseID, m.ReleaseSequence); err != nil {
			return err
		}
		if m.MessageKey != "" && !validText(m.MessageKey) {
			return fmt.Errorf("%w: the mandatory update's message key is not a localisation key", ErrInvalid)
		}
	}
	if r := d.ForcedRollback; r != nil {
		return validRelease("forced rollback", r.ReleaseID, r.ReleaseSequence)
	}
	return nil
}

func (k KillSwitch) validate() error {
	if !slices.Contains(Scopes(), k.Scope) {
		return fmt.Errorf("%w: %q is not a kill switch scope", ErrInvalid, k.Scope)
	}
	if len(k.Target) > MaxText || !targetPattern.MatchString(k.Target) {
		return fmt.Errorf("%w: %q is not a target identifier", ErrInvalid, k.Target)
	}
	return nil
}

func validRelease(what, id string, sequence uint64) error {
	if !validText(id) || sequence < 1 {
		return fmt.Errorf("%w: the %s needs a release identifier and its sequence", ErrInvalid, what)
	}
	return nil
}

func validText(s string) bool { return len(s) <= MaxText && textPattern.MatchString(s) }

// ValidIdentifier reports whether s can be an identifier, key or message key
// of a document: up to MaxText printable ASCII characters without spaces.
func ValidIdentifier(s string) bool { return validText(s) }

// sortedSwitches returns the kill switches in the order the wire format
// promises: by scope in wire order, then by target.
func sortedSwitches(in []KillSwitch) []KillSwitch {
	out := slices.Clone(in)
	slices.SortFunc(out, func(a, b KillSwitch) int {
		if c := cmp.Compare(scopeIndex(a.Scope), scopeIndex(b.Scope)); c != 0 {
			return c
		}
		return cmp.Compare(a.Target, b.Target)
	})
	return out
}

func scopeIndex(s Scope) int { return slices.Index(Scopes(), s) }
