// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package saml

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/beevik/etree"

	"github.com/nightCode42/plux3/backend/ext/identity"
)

// The values of the assertion the checks compare with.
const (
	confirmationMethod = "urn:oasis:names:tc:SAML:2.0:cm:bearer"
	transientFormat    = "urn:oasis:names:tc:SAML:2.0:nameid-format:transient"
	emailFormat        = "urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress"
)

// assertion is what Plux reads of a verified SAML assertion.
type assertion struct {
	id           string
	issuer       string
	issueInstant time.Time
	nameID       string
	nameIDFormat string
	// confirmations are the subject confirmations of the bearer method.
	confirmations []confirmation
	notBefore     time.Time
	notOnOrAfter  time.Time
	audiences     [][]string
	sessionIndex  string
	authnContext  string
	// attributes are the asserted attributes by name.
	attributes map[string][]string
}

// confirmation is the data of one bearer subject confirmation.
type confirmation struct {
	recipient    string
	inResponseTo string
	notBefore    time.Time
	notOnOrAfter time.Time
}

// parseAssertion reads a verified assertion element. An element that is not
// an assertion of SAML 2.0, or that has no subject or no conditions, is not
// accepted.
func parseAssertion(el *etree.Element) (assertion, error) {
	if !is(el, nsAssertion, "Assertion") || el.SelectAttrValue("Version", "") != "2.0" {
		return assertion{}, errors.New("the element is not a SAML 2.0 assertion")
	}
	a := assertion{id: el.SelectAttrValue("ID", ""), attributes: map[string][]string{}}
	var err error
	if a.issueInstant, err = timeAttr(el, "IssueInstant"); err != nil {
		return assertion{}, err
	}
	issuer, err := required(el, nsAssertion, "Issuer")
	if err != nil {
		return assertion{}, err
	}
	a.issuer = text(issuer)
	if err := a.readSubject(el); err != nil {
		return assertion{}, err
	}
	if err := a.readConditions(el); err != nil {
		return assertion{}, err
	}
	if err := a.readStatements(el); err != nil {
		return assertion{}, err
	}
	return a, nil
}

// readSubject reads the name identifier and the bearer confirmations.
func (a *assertion) readSubject(el *etree.Element) error {
	subject, err := required(el, nsAssertion, "Subject")
	if err != nil {
		return err
	}
	name, err := required(subject, nsAssertion, "NameID")
	if err != nil {
		return err
	}
	a.nameID, a.nameIDFormat = text(name), name.SelectAttrValue("Format", "")
	for _, sc := range children(subject, nsAssertion, "SubjectConfirmation") {
		if sc.SelectAttrValue("Method", "") != confirmationMethod {
			continue
		}
		data, err := optional(sc, nsAssertion, "SubjectConfirmationData")
		if err != nil || data == nil {
			continue
		}
		c, err := readConfirmation(data)
		if err != nil {
			return err
		}
		a.confirmations = append(a.confirmations, c)
	}
	return nil
}

// readConfirmation reads the data of a subject confirmation.
func readConfirmation(data *etree.Element) (confirmation, error) {
	c := confirmation{recipient: data.SelectAttrValue("Recipient", ""), inResponseTo: data.SelectAttrValue("InResponseTo", "")}
	var err error
	if c.notBefore, err = timeAttr(data, "NotBefore"); err != nil {
		return confirmation{}, err
	}
	if c.notOnOrAfter, err = timeAttr(data, "NotOnOrAfter"); err != nil {
		return confirmation{}, err
	}
	return c, nil
}

// readConditions reads the validity window and the audiences. A condition
// other than the three the specification defines for every consumer cannot be
// evaluated, and the specification says an assertion with one is not valid.
func (a *assertion) readConditions(el *etree.Element) error {
	cond, err := required(el, nsAssertion, "Conditions")
	if err != nil {
		return err
	}
	if a.notBefore, err = timeAttr(cond, "NotBefore"); err != nil {
		return err
	}
	if a.notOnOrAfter, err = timeAttr(cond, "NotOnOrAfter"); err != nil {
		return err
	}
	for _, c := range cond.ChildElements() {
		switch {
		case is(c, nsAssertion, "AudienceRestriction"):
			var audiences []string
			for _, au := range children(c, nsAssertion, "Audience") {
				audiences = append(audiences, text(au))
			}
			a.audiences = append(a.audiences, audiences)
		case is(c, nsAssertion, "OneTimeUse"), is(c, nsAssertion, "ProxyRestriction"):
		default:
			return fmt.Errorf("the assertion has a condition <%s> that is not understood", c.Tag)
		}
	}
	return nil
}

// readStatements reads the session of the first authentication statement and
// every attribute.
func (a *assertion) readStatements(el *etree.Element) error {
	if authn := children(el, nsAssertion, "AuthnStatement"); len(authn) > 0 {
		if err := a.readAuthn(authn[0]); err != nil {
			return err
		}
	}
	for _, statement := range children(el, nsAssertion, "AttributeStatement") {
		for _, attr := range children(statement, nsAssertion, "Attribute") {
			name := attr.SelectAttrValue("Name", "")
			for _, v := range children(attr, nsAssertion, "AttributeValue") {
				a.attributes[name] = append(a.attributes[name], text(v))
			}
		}
	}
	return nil
}

// readAuthn reads the provider's session and authentication context.
func (a *assertion) readAuthn(authn *etree.Element) error {
	a.sessionIndex = authn.SelectAttrValue("SessionIndex", "")
	ctx, err := optional(authn, nsAssertion, "AuthnContext")
	if err != nil || ctx == nil {
		return err
	}
	ref, err := optional(ctx, nsAssertion, "AuthnContextClassRef")
	if err != nil || ref == nil {
		return err
	}
	a.authnContext = text(ref)
	return nil
}

// window is the validity a check applies: the clock and the tolerance.
type window struct {
	now  time.Time
	skew time.Duration
}

// notYet reports whether t, a NotBefore, is still ahead.
func (w window) notYet(t time.Time) bool { return !t.IsZero() && t.After(w.now.Add(w.skew)) }

// over reports whether t, a NotOnOrAfter, has passed. The bound is
// exclusive: at the instant itself the assertion is no longer valid.
func (w window) over(t time.Time) bool { return !w.now.Before(t.Add(w.skew)) }

// validate checks an assertion against the provider's configuration, the
// request it must answer and the time. It returns the instant after which the
// assertion can no longer be accepted, which is how long it has to be
// remembered.
func (a assertion) validate(cfg identity.SAMLConfig, requestID string, w window) (time.Time, error) {
	if a.id == "" {
		return time.Time{}, errors.New("the assertion has no ID")
	}
	if a.issuer != cfg.IdP.EntityID {
		return time.Time{}, errors.New("the assertion was issued by another identity provider")
	}
	if w.notYet(a.issueInstant) {
		return time.Time{}, errors.New("the assertion was issued in the future")
	}
	if a.nameID == "" || a.nameIDFormat == transientFormat {
		return time.Time{}, errors.New("the assertion has no persistent subject")
	}
	if err := a.checkConditions(cfg, w); err != nil {
		return time.Time{}, err
	}
	confirmed, err := a.confirmedUntil(cfg, requestID, w)
	if err != nil {
		return time.Time{}, err
	}
	return earliest(confirmed, a.notOnOrAfter).Add(w.skew), nil
}

// checkConditions checks the validity window and that the assertion is for
// this service provider. A window without an end, or an assertion without an
// audience, is not accepted: both would make a stolen assertion good for
// anyone, anywhere, for ever.
func (a assertion) checkConditions(cfg identity.SAMLConfig, w window) error {
	if a.notOnOrAfter.IsZero() {
		return errors.New("the assertion's conditions have no end")
	}
	if w.notYet(a.notBefore) {
		return errors.New("the assertion is not valid yet")
	}
	if w.over(a.notOnOrAfter) {
		return errors.New("the assertion has expired")
	}
	if len(a.audiences) == 0 {
		return errors.New("the assertion names no audience")
	}
	for _, restriction := range a.audiences {
		if !slices.Contains(restriction, cfg.EntityID) {
			return errors.New("the assertion is for another audience")
		}
	}
	return nil
}

// confirmedUntil finds the bearer confirmation that holds and returns its
// end: one that is addressed to our assertion consumer service, answers the
// request the sign-in made (or none, for a response the provider started),
// and is inside its window.
func (a assertion) confirmedUntil(cfg identity.SAMLConfig, requestID string, w window) (time.Time, error) {
	for _, c := range a.confirmations {
		switch {
		case c.recipient != cfg.ACSURL, c.inResponseTo != requestID:
		case c.notOnOrAfter.IsZero(), w.over(c.notOnOrAfter), w.notYet(c.notBefore):
		default:
			return c.notOnOrAfter, nil
		}
	}
	return time.Time{}, errors.New("no bearer confirmation of the assertion holds for this sign-in")
}

// earliest returns the earlier of two times.
func earliest(a, b time.Time) time.Time {
	if b.Before(a) {
		return b
	}
	return a
}
