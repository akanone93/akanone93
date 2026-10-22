// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

// Package webauthndriver is a software WebAuthn authenticator and the browser
// that carries it, for the scripted audit walkthrough (SEC-103, GOV-030): the
// walkthrough needs a person to sign in through an OpenID Connect provider
// with a second factor and to confirm step-up requests with a security key, and
// a CI job has neither a person nor a browser.
//
// [Authenticator] is the key: an ES256 credential that answers registrations
// with "none" attestation and sign-ins with user presence and user
// verification asserted, exactly what the server demands of a step-up. [Driver]
// is the browser: it signs a person in through the test provider, enrols the
// authenticator as the person's second factor, mints an access token with the
// resulting session, and completes the step-up the plux CLI asks for.
//
// Invariants: private keys live in memory or in a user-only file in the
// directory the caller names, and are never printed, logged or put in an error;
// the credential counter advances on every assertion and is saved before the
// assertion leaves the process, so the server never sees a counter go
// backwards; the TLS configuration always verifies, against the CA the caller
// supplies; the package depends on the standard library alone.
package webauthndriver
