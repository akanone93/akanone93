// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package identity

import (
	"context"
	"errors"
	"time"
)

// Errors an extension returns, wrapped with %w. The core maps them to the
// error codes of the API (Appendix F) and never shows the wrapped detail to
// the caller.
var (
	// ErrInvalidResponse reports a response that failed signature, audience,
	// recipient, destination, time or structure checks (PLX-6210).
	ErrInvalidResponse = errors.New("identity: the response is not valid")
	// ErrReplayed reports an assertion that was already accepted once
	// (PLX-6211).
	ErrReplayed = errors.New("identity: the assertion was already used")
)

// Identity is a person as an external identity provider vouches for them.
// The core links it to an account, applies the provider's group mapping and
// opens a session; nothing in it is trusted beyond what the protocol
// verified.
type Identity struct {
	// Issuer is the provider's entity identifier; with Subject it is the
	// key of the link to an account.
	Issuer string
	// Subject is the provider's stable identifier of the person: the SAML
	// NameID. A transient identifier is refused by the implementation.
	Subject string
	// Email is the address the provider asserts. Providers are trusted for
	// the addresses of their own people, so EmailVerified is true for SAML.
	Email         string
	EmailVerified bool
	// DisplayName is the person's name, or "".
	DisplayName string
	// Groups are the values of the provider's group attribute.
	Groups []string
	// Attributes are the remaining asserted attributes by name.
	Attributes map[string][]string
	// SessionIndex is the provider's session identifier, for single logout.
	SessionIndex string
	// AuthnContext is the authentication context class the provider used,
	// the SAML counterpart of an ID token's acr claim.
	AuthnContext string
}

// ReplayCache remembers identifiers that were accepted once. The core backs
// it with the shared cache (Valkey, or memory on a single node).
type ReplayCache interface {
	// Seen records key for ttl and reports whether it had been recorded
	// before. An error means the cache could not answer, which an
	// implementation must treat as a refusal: a replay check that cannot run
	// is not passed.
	Seen(ctx context.Context, key string, ttl time.Duration) (bool, error)
}
