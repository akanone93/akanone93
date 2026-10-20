// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package auth

import (
	"context"
	"time"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// StepUpWindow is how recent a user-verified WebAuthn assertion must be for
// an action that needs a step-up (SEC-103).
const StepUpWindow = 5 * time.Minute

// stepUp is what an identity carries of its last step-up: whether there was
// one and how old it was when the credential was authenticated. Carrying the
// age rather than the instant keeps every check free of the wall clock: the
// authenticator, which has the injected clock, measures it once.
type stepUp struct {
	done bool
	age  time.Duration
}

// WithStepUp returns the identity as having completed a step-up age ago,
// measured when its credential was authenticated. Only an authenticator that
// verified the evidence calls it; tests call it to stand for one.
func (id Identity) WithStepUp(age time.Duration) Identity {
	id.stepUp = stepUp{done: true, age: max(age, 0)}
	return id
}

// StepUpError returns nil when the identity may do what needs a step-up no
// older than maxAge, and otherwise the refusal: PLX-6200 when it has none,
// PLX-6201 when the last one is too old (SEC-103).
//
// Machine identities hold no authenticator, so the rule cannot reach them: a
// CI token and the server itself keep their existing rules, which are the
// scopes a workload identity was granted and the audit log (SRV-064).
func (id Identity) StepUpError(maxAge time.Duration) error {
	if id.Kind == KindCI || id.Kind == KindSystem {
		return nil
	}
	switch {
	case !id.stepUp.done:
		return plxerr.New(plxerr.StepUpRequired, "this action needs a step-up with a security key")
	case id.stepUp.age > maxAge:
		return plxerr.New(plxerr.StepUpExpired, "the last step-up is more than %d minutes old", int(maxAge.Minutes()))
	}
	return nil
}

// identityKey carries the caller's identity in a context.
type identityKey struct{}

// WithIdentity returns a context carrying an authenticated identity.
func WithIdentity(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, identityKey{}, id)
}

// IdentityFrom returns the authenticated caller, when there is one.
func IdentityFrom(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(identityKey{}).(Identity)
	return id, ok
}

// RequireStepUp refuses the call in ctx unless its caller completed a
// step-up no longer than maxAge ago: PLX-6200 when there is none, PLX-6201
// when it is too old, and PLX-8012 when the call has no caller (SEC-103).
// Permissions the catalogue marks stepUp are checked centrally by
// Principal.Authorize; RequireStepUp is for a call that needs the proof for a
// reason a permission does not carry.
func RequireStepUp(ctx context.Context, maxAge time.Duration) error {
	id, ok := IdentityFrom(ctx)
	if !ok {
		return plxerr.New(plxerr.AuthenticationRequired, "this call needs a session or an access token")
	}
	return id.StepUpError(maxAge)
}
