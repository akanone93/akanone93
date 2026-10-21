// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package approval

import (
	"fmt"

	"github.com/nightCode42/plux3/backend/internal/schema"
	"github.com/nightCode42/plux3/backend/internal/security/settings"
)

const (
	// documentSchemaVersion is the schema version policy documents are
	// written in.
	documentSchemaVersion = "1.0.0"
	// DefaultExpiryHours is how long a request stays open and an approval
	// counts when a policy states no expiry: seven days.
	DefaultExpiryHours = 168
	// defaultPrefix starts the key of every seeded policy.
	defaultPrefix = "default-"
)

// defaultRule describes one seeded policy compactly.
type defaultRule struct {
	key         string
	name        string
	description string
	match       schema.PolicyMatch
	approvers   int64
}

// production is the match value of "production environments only".
func production() *bool {
	yes := true
	return &yes
}

// DefaultPolicies returns the policy documents a new app starts with under a
// security profile (maintainer decision Q16, spec §18.2): standard asks one
// approver for a production publish, strict two, and maximum four-eyes on
// production publish, function deployment, device-function enablement,
// security-configuration change, accessibility waiver and golden approval.
// Every default also asks that the author is not the approver, which the
// engine enforces whatever a policy says. newID supplies the immutable
// identifier of each document.
func DefaultPolicies(profile settings.Profile, newID func() (string, error)) ([]schema.PolicyDocument, error) {
	rules := defaultRules(profile)
	out := make([]schema.PolicyDocument, 0, len(rules))
	for _, r := range rules {
		id, err := newID()
		if err != nil {
			return nil, fmt.Errorf("approval: identify a default policy: %w", err)
		}
		distinct, expiry := true, int64(DefaultExpiryHours)
		out = append(out, schema.PolicyDocument{
			SchemaVersion: documentSchemaVersion, Kind: string(schema.KindPolicy), ID: id,
			Key: r.key, Name: r.name, Description: r.description, Match: r.match,
			Require: schema.PolicyRequire{Approvers: r.approvers, DistinctFromAuthor: &distinct, ExpiryHours: &expiry},
		})
	}
	return out, nil
}

// defaultRules lists the rules of a profile; the development environments of
// any profile are not production, so no default matches them.
func defaultRules(profile settings.Profile) []defaultRule {
	releases := []schema.PolicyResource{schema.PolicyResourceRelease}
	prod := func(res ...schema.PolicyResource) schema.PolicyMatch {
		return schema.PolicyMatch{Resources: res, Production: production()}
	}
	switch profile {
	case settings.Standard:
		return []defaultRule{{
			key: defaultPrefix + "standard-production-release", name: "Production release (standard)",
			description: "One approver for a release promoted to a production environment.",
			match:       prod(releases...), approvers: 1,
		}}
	case settings.Strict:
		return []defaultRule{{
			key: defaultPrefix + "strict-production-release", name: "Production release (strict)",
			description: "Four-eyes approval for a release promoted to a production environment.",
			match:       prod(releases...), approvers: 2,
		}}
	case settings.Maximum:
		return maximumRules(prod, releases)
	}
	return nil
}

// maximumRules are the defaults of the maximum profile (Q16).
func maximumRules(prod func(...schema.PolicyResource) schema.PolicyMatch, releases []schema.PolicyResource) []defaultRule {
	return []defaultRule{
		{
			key: defaultPrefix + "maximum-production-release", name: "Production release (maximum)",
			description: "Four-eyes approval for a release promoted to a production environment.",
			match:       prod(releases...), approvers: 2,
		},
		{
			key: defaultPrefix + "maximum-production-function", name: "Production function deployment (maximum)",
			description: "Four-eyes approval for deploying a function to a production environment.",
			match:       prod(schema.PolicyResourceFunctionVersion), approvers: 2,
		},
		{
			key: defaultPrefix + "maximum-device-functions", name: "Device-function enablement (maximum)",
			description: "Four-eyes approval for enabling functions that run on the device.",
			match: schema.PolicyMatch{
				Resources: []schema.PolicyResource{schema.PolicyResourceSecurityConfiguration},
				Actions:   []string{ActionEnableDeviceFunctions},
			}, approvers: 2,
		},
		{
			key: defaultPrefix + "maximum-security-configuration", name: "Security configuration change (maximum)",
			description: "Four-eyes approval for any change of an environment's security configuration.",
			match:       schema.PolicyMatch{Resources: []schema.PolicyResource{schema.PolicyResourceSecurityConfiguration}}, approvers: 2,
		},
		{
			key: defaultPrefix + "maximum-accessibility-waiver", name: "Accessibility waiver (maximum)",
			description: "Four-eyes approval for adding or changing an accessibility waiver.",
			match:       schema.PolicyMatch{Resources: []schema.PolicyResource{schema.PolicyResourceWaiver}}, approvers: 2,
		},
		{
			key: defaultPrefix + "maximum-golden-baseline", name: "Golden approval (maximum)",
			description: "Four-eyes approval for promoting golden images to baselines.",
			match:       schema.PolicyMatch{Resources: []schema.PolicyResource{schema.PolicyResourceGoldenBaseline}}, approvers: 2,
		},
	}
}
