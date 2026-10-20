// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package saml

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/beevik/etree"

	"github.com/nightCode42/plux3/backend/edition"
	"github.com/nightCode42/plux3/backend/ext/identity"
)

// statusSuccess is the status of a response that vouches for someone.
const statusSuccess = "urn:oasis:names:tc:SAML:2.0:status:Success"

// Verify checks a posted response and returns the person it vouches for.
// Every failure wraps identity.ErrInvalidResponse, and a replayed assertion
// identity.ErrReplayed; the text says why, for the log, and is not shown to
// the person who signed in.
func (Provider) Verify(ctx context.Context, v identity.SAMLVerify) (identity.Identity, error) {
	if err := edition.RequireRead(ctx, edition.FeatureSAML); err != nil {
		return identity.Identity{}, err //nolint:wrapcheck // a domain error
	}
	if v.RequestID == "" && !v.Config.AllowIdPInitiated {
		return identity.Identity{}, refused(errors.New("the response answers no sign-in of ours, and the provider does not allow that"))
	}
	a, err := authenticate(v)
	if err != nil {
		return identity.Identity{}, refused(err)
	}
	w := window{now: v.Now, skew: v.ClockSkew}
	until, err := a.validate(v.Config, v.RequestID, w)
	if err != nil {
		return identity.Identity{}, refused(err)
	}
	if err := remember(ctx, v.Replay, a, until, v.Now); err != nil {
		return identity.Identity{}, err
	}
	return a.identity(v.Config), nil
}

// refused marks a failure as an invalid response.
func refused(err error) error {
	return fmt.Errorf("saml.Verify: %w: %w", identity.ErrInvalidResponse, err)
}

// authenticate reads a response down to the assertion its signature vouches
// for. What it returns was read from signed content only.
func authenticate(v identity.SAMLVerify) (assertion, error) {
	t, err := newTrust(v.Config.IdP.SigningCertificates, v.Now)
	if err != nil {
		return assertion{}, err
	}
	raw, err := decodeBase64(v.SAMLResponse)
	if err != nil || len(raw) == 0 {
		return assertion{}, errors.New("the response is not base64")
	}
	root, err := parseXML(raw)
	if err != nil {
		return assertion{}, err
	}
	if !is(root, nsProtocol, "Response") {
		return assertion{}, errors.New("the document is not a SAML response")
	}
	envelope, signed, err := t.envelope(root)
	if err != nil {
		return assertion{}, err
	}
	if err := checkResponse(envelope, signed, v); err != nil {
		return assertion{}, err
	}
	el, err := t.assertionElement(envelope, signed, v.Config.DecryptionKeyPEM)
	if err != nil {
		return assertion{}, err
	}
	return parseAssertion(el)
}

// envelope returns the response as it can be read: the signed content when the
// response is signed, the document itself when it is not and the assertion
// must carry the signature.
func (t trust) envelope(root *etree.Element) (el *etree.Element, signed bool, err error) {
	sig, err := signature(root)
	if err != nil {
		return nil, false, err
	}
	if sig == nil {
		return root, false, nil
	}
	el, err = t.verify(root)
	return el, err == nil, err
}

// checkResponse checks what the response itself says: that it succeeded, that
// it came to the right place, and that it answers the right request.
func checkResponse(el *etree.Element, signed bool, v identity.SAMLVerify) error {
	if el.SelectAttrValue("Version", "") != "2.0" {
		return errors.New("the response is not SAML 2.0")
	}
	status, err := responseStatus(el)
	if err != nil {
		return err
	}
	if status != statusSuccess {
		return fmt.Errorf("the identity provider answered %q", status)
	}
	destination := el.SelectAttrValue("Destination", "")
	if (destination != "" && destination != v.Config.ACSURL) || (destination == "" && signed) {
		return errors.New("the response is not destined for this service provider")
	}
	if got := el.SelectAttrValue("InResponseTo", ""); got != v.RequestID {
		return errors.New("the response answers another request")
	}
	issuer, err := optional(el, nsAssertion, "Issuer")
	if err != nil {
		return err
	}
	if issuer != nil && text(issuer) != v.Config.IdP.EntityID {
		return errors.New("the response was issued by another identity provider")
	}
	return nil
}

// responseStatus returns the status code of a response.
func responseStatus(el *etree.Element) (string, error) {
	status, err := required(el, nsProtocol, "Status")
	if err != nil {
		return "", err
	}
	code, err := required(status, nsProtocol, "StatusCode")
	if err != nil {
		return "", err
	}
	return code.SelectAttrValue("Value", ""), nil
}

// assertionElement returns the single assertion of a response, opened when it
// is encrypted, and verified: by the response's signature when the response is
// signed, and by its own signature when it has one, which it must when the
// response is not signed.
func (t trust) assertionElement(envelope *etree.Element, signedEnvelope bool, keyPEM []byte) (*etree.Element, error) {
	plain := children(envelope, nsAssertion, "Assertion")
	encrypted := children(envelope, nsAssertion, "EncryptedAssertion")
	if len(plain)+len(encrypted) != 1 {
		return nil, fmt.Errorf("the response has %d assertions, not one", len(plain)+len(encrypted))
	}
	var el *etree.Element
	if len(plain) == 1 {
		el = plain[0]
	} else {
		var err error
		if el, err = decryptAssertion(encrypted[0], keyPEM); err != nil {
			return nil, err
		}
	}
	sig, err := signature(el)
	if err != nil {
		return nil, err
	}
	switch {
	case sig != nil:
		return t.verify(el)
	case signedEnvelope:
		return el, nil
	default:
		return nil, errors.New("the assertion is not signed")
	}
}

// remember records an assertion as used, for as long as it could still be
// accepted. Last of the checks, so that only an assertion that passed all the
// others takes room in the cache. A cache that cannot answer refuses the
// sign-in: a replay check that did not run has not been passed.
func remember(ctx context.Context, cache identity.ReplayCache, a assertion, until, now time.Time) error {
	if cache == nil {
		return refused(errors.New("there is no replay cache"))
	}
	sum := sha256.Sum256([]byte(a.issuer + "\x00" + a.id))
	seen, err := cache.Seen(ctx, "saml:assertion:"+hex.EncodeToString(sum[:]), until.Sub(now))
	if err != nil {
		return refused(fmt.Errorf("the replay cache cannot answer: %w", err))
	}
	if seen {
		return fmt.Errorf("saml.Verify: %w: assertion %q", identity.ErrReplayed, a.id)
	}
	return nil
}
