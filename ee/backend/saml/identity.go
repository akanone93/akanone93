// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package saml

import (
	"maps"
	"slices"
	"strings"

	"github.com/nightCode42/plux3/backend/ext/identity"
)

// The attribute names providers commonly use when the administrator has not
// named one, most specific first. They are compared without regard to case.
var (
	emailAttributes = []string{
		"email", "mail", "emailaddress",
		"urn:oid:0.9.2342.19200300.100.1.3",
		"http://schemas.xmlsoap.org/ws/2005/05/identity/claims/emailaddress",
		"http://schemas.xmlsoap.org/claims/EmailAddress",
		"http://schemas.microsoft.com/identity/claims/emailaddress",
	}
	nameAttributes = []string{
		"displayName", "name", "cn",
		"urn:oid:2.16.840.1.113730.3.1.241",
		"http://schemas.microsoft.com/identity/claims/displayname",
		"http://schemas.xmlsoap.org/ws/2005/05/identity/claims/name",
	}
	givenNameAttributes = []string{
		"firstName", "givenName", "urn:oid:2.5.4.42",
		"http://schemas.xmlsoap.org/ws/2005/05/identity/claims/givenname",
	}
	surnameAttributes = []string{
		"lastName", "surname", "sn", "urn:oid:2.5.4.4",
		"http://schemas.xmlsoap.org/ws/2005/05/identity/claims/surname",
	}
	groupAttributes = []string{
		"groups", "memberOf", "isMemberOf",
		"urn:oid:1.3.6.1.4.1.5923.1.5.1.1",
		"http://schemas.xmlsoap.org/claims/Group",
		"http://schemas.microsoft.com/ws/2008/06/identity/claims/groups",
	}
)

// identity turns a verified assertion into the person it vouches for.
func (a assertion) identity(cfg identity.SAMLConfig) identity.Identity {
	who := identity.Identity{
		Issuer:       a.issuer,
		Subject:      a.nameID,
		Email:        a.first(cfg.EmailAttribute, emailAttributes),
		DisplayName:  a.displayName(cfg.NameAttribute),
		Groups:       a.values(cfg.GroupsAttribute, groupAttributes),
		Attributes:   make(map[string][]string, len(a.attributes)),
		SessionIndex: a.sessionIndex,
		AuthnContext: a.authnContext,
	}
	if who.Email == "" && a.nameIDFormat == emailFormat {
		who.Email = a.nameID
	}
	// Providers are trusted for the addresses of their own people (ADR-0074).
	who.EmailVerified = who.Email != ""
	for name, values := range a.attributes {
		who.Attributes[name] = slices.Clone(values)
	}
	return who
}

// displayName is the asserted name, or the given name and surname joined.
func (a assertion) displayName(configured string) string {
	if name := a.first(configured, nameAttributes); name != "" {
		return name
	}
	return strings.TrimSpace(a.first("", givenNameAttributes) + " " + a.first("", surnameAttributes))
}

// first returns the first value of the configured attribute or, when none is
// configured, of the first common attribute the assertion has.
func (a assertion) first(configured string, common []string) string {
	if values := a.values(configured, common); len(values) > 0 {
		return values[0]
	}
	return ""
}

// values returns the values of the attribute the administrator named, or of
// the first of the common names the assertion carries a non-empty value for.
func (a assertion) values(configured string, common []string) []string {
	if configured != "" {
		return nonEmpty(a.attributes[configured])
	}
	for _, name := range common {
		for _, have := range slices.Sorted(maps.Keys(a.attributes)) {
			if strings.EqualFold(have, name) {
				if values := nonEmpty(a.attributes[have]); len(values) > 0 {
					return values
				}
			}
		}
	}
	return nil
}

// nonEmpty returns the values that are not empty.
func nonEmpty(values []string) []string {
	var out []string
	for _, v := range values {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}
