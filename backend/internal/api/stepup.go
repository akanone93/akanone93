// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/dpop"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// StepUpGrantHeader carries the step-up grant of the CLI on a call that
// needs one (SEC-103, ADR-0075). The call also carries the CLI's token and a
// DPoP proof of the key the grant is bound to.
const StepUpGrantHeader = "Plux-Step-Up-Grant"

// StepUpSecretHeader carries the one-time secret a CLI polls its step-up
// request with.
const StepUpSecretHeader = "Plux-Step-Up-Secret"

// ProofChecker verifies the DPoP proof a person's CLI sends with a token. The
// token is a bearer token, so the proof is not required by every call; it is
// required where the CLI proves it holds the key a step-up is bound to. The
// proof must name the method and URL of the request, the hash of the token
// and a moment inside Window, and it is accepted once (RFC 9449, SEC-021,
// SEC-022). Unlike a device's, it carries no server nonce: the proof is
// single-use and bound to the token, and a nonce round trip would burden
// every command for nothing the replay check does not already give.
type ProofChecker struct {
	// Proofs verifies the proof; its Nonces must be nil.
	Proofs *dpop.Verifier
	// Replay refuses a proof that was used before.
	Replay *dpop.Replay
	// BaseURL is the externally reachable base URL, which the htu claim of
	// every proof names with the request's path.
	BaseURL string
	// Window is how far a proof's iat may differ from the clock, and
	// FallbackWindow the narrower window that applies while the replay
	// cache runs degraded (SEC-023).
	Window, FallbackWindow time.Duration
	// Now is the clock; nil uses time.Now.
	Now func() time.Time
}

// Check verifies the proof in h for a request with the method to the path,
// made with the bearer token, and returns it.
func (c ProofChecker) Check(ctx context.Context, h http.Header, method, path, token string) (dpop.Proof, error) {
	raw, err := proofOf(h)
	if err != nil {
		return dpop.Proof{}, err
	}
	proof, err := c.Proofs.Verify(raw, dpop.Expect{
		Method: method, URL: strings.TrimSuffix(c.BaseURL, "/") + path, AccessToken: token, Window: c.Window,
	})
	if err != nil {
		return dpop.Proof{}, err //nolint:wrapcheck // a domain error
	}
	degraded, err := c.Replay.Check(ctx, proof.JKT, proof.JTI, 2*c.Window+replayMargin)
	if err != nil {
		return dpop.Proof{}, err //nolint:wrapcheck // a domain error
	}
	if degraded && c.distance(proof.IssuedAt) > c.FallbackWindow {
		return dpop.Proof{}, plxerr.New(plxerr.DPoPProofInvalid, "dpop proof is outside the window that applies while the replay cache is degraded")
	}
	return proof, nil
}

// distance is how far a moment is from the clock.
func (c ProofChecker) distance(t time.Time) time.Duration {
	now := time.Now
	if c.Now != nil {
		now = c.Now
	}
	d := now().Sub(t)
	if d < 0 {
		return -d
	}
	return d
}

// StepUpGrants makes a call that carries a step-up grant count as stepped up.
// It runs after authentication: when the caller is a person's token and the
// call carries StepUpGrantHeader, the call's DPoP proof must verify and be
// new, and the grant must be the one issued to this token for the key of
// that proof. Then the identity is the one the grant makes. A grant that does
// not match adds nothing, so a call that does not need a step-up is not
// failed for carrying one; a proof that does not verify fails the call,
// because it is not what the CLI sends (SEC-103).
func StepUpGrants(checker ProofChecker, grants interface {
	CheckStepUpGrant(ctx context.Context, id auth.Identity, grant, jkt string) (auth.Identity, error)
},
) Around {
	return func(ctx context.Context, c Call, next func(context.Context) error) error {
		grant := c.Header.Get(StepUpGrantHeader)
		id, ok := IdentityFrom(ctx)
		if grant == "" || !ok || id.Kind != auth.KindToken {
			return next(ctx)
		}
		token, _ := bearerToken(c.Header)
		proof, err := checker.Check(ctx, c.Header, proofMethod, c.Procedure, token)
		if err != nil {
			return err
		}
		stepped, err := grants.CheckStepUpGrant(ctx, id, grant, proof.JKT)
		if err != nil {
			return err //nolint:wrapcheck // a domain error
		}
		return next(WithIdentity(ctx, stepped))
	}
}

// Chain runs first and then second: second sees the context first leaves.
func Chain(first, second Around) Around {
	return func(ctx context.Context, c Call, next func(context.Context) error) error {
		return first(ctx, c, func(ctx context.Context) error { return second(ctx, c, next) })
	}
}
