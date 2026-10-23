// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package approval

import (
	"slices"
	"strings"

	"github.com/nightCode42/plux3/backend/internal/schema"
)

// Matches reports whether a change satisfies every attribute a policy's
// match lists; an attribute left out matches anything (GOV-020). A policy
// that lists environments never matches a change outside any environment,
// and one that lists risk attributes needs the change to carry all of them.
func Matches(m schema.PolicyMatch, a Artifact) bool {
	if len(m.Resources) > 0 && !slices.Contains(m.Resources, a.Resource) {
		return false
	}
	if len(m.Environments) > 0 && !slices.Contains(m.Environments, a.Environment.Key) {
		return false
	}
	if len(m.Actions) > 0 && !slices.Contains(m.Actions, a.Action) {
		return false
	}
	if m.Production != nil && *m.Production != a.Environment.Production {
		return false
	}
	for _, r := range m.Risk {
		if !slices.Contains(a.Risk, r) {
			return false
		}
	}
	return true
}

// Policy is a policy document in force, with the hash that names its exact
// content.
type Policy struct {
	Doc schema.PolicyDocument
	// Hash is the SHA-256 of the canonical document, in hex.
	Hash string
	// Version is the document's version.
	Version int32
}

// matching returns the policies that match a change, in key order.
func matching(policies []Policy, a Artifact) []Policy {
	var out []Policy
	for _, p := range policies {
		if Matches(p.Doc.Match, a) {
			out = append(out, p)
		}
	}
	slices.SortFunc(out, func(x, y Policy) int { return strings.Compare(x.Doc.Key, y.Doc.Key) })
	return out
}
