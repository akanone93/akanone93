// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package approval

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/schema"
	"github.com/nightCode42/plux3/backend/internal/security/settings"
)

// sequentialIDs yields identifiers for default documents.
func sequentialIDs() func() (string, error) {
	n := 0
	return func() (string, error) {
		n++
		return fmt.Sprintf("01b0c450-6c00-7000-8000-%012d", n), nil
	}
}

// parser returns a service that can only parse policies.
func parser(t *testing.T) *Service {
	t.Helper()
	v, err := schema.NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	return &Service{validator: v}
}

// promoteTo is a promotion of a release to an environment.
func promoteTo(key string, production bool) Artifact {
	return Artifact{Resource: schema.PolicyResourceRelease, Action: ActionPromote, Environment: Environment{Key: key, Production: production}}
}

// requirementFor returns the most approvals the defaults of a profile ask of a change.
func requirementFor(t *testing.T, profile settings.Profile, a Artifact) int {
	t.Helper()
	docs, err := DefaultPolicies(profile, sequentialIDs())
	if err != nil {
		t.Fatal(err)
	}
	var policies []Policy
	for _, d := range docs {
		policies = append(policies, Policy{Doc: d})
	}
	most := 0
	for _, p := range matching(policies, a) {
		most = max(most, int(p.Doc.Require.Approvers))
	}
	return most
}

// Verifies: GOV-020, GOV-021.
func TestDefaultPolicies_ValidAgainstTheSchema_GOV_020(t *testing.T) {
	s := parser(t)
	for _, profile := range []settings.Profile{settings.Standard, settings.Strict, settings.Maximum} {
		docs, err := DefaultPolicies(profile, sequentialIDs())
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range docs {
			raw, err := json.Marshal(d)
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := s.ParsePolicy(raw); err != nil {
				t.Errorf("%s: %v", d.Key, err)
			}
			if !strings.HasPrefix(d.Key, defaultPrefix+string(profile)+"-") {
				t.Errorf("%s does not carry its profile", d.Key)
			}
			if needsEnterprise(d) {
				t.Errorf("%s needs the enterprise edition, and defaults are core", d.Key)
			}
		}
	}
}

// Verifies: GOV-020, GOV-021.
func TestDefaultPolicies_PerProfile_GOV_020(t *testing.T) {
	prod, dev := promoteTo("production", true), promoteTo("dev", false)
	cases := []struct {
		profile settings.Profile
		art     Artifact
		want    int
	}{
		{settings.Standard, prod, 1},
		{settings.Standard, dev, 0},
		{settings.Strict, prod, 2},
		{settings.Strict, dev, 0},
		{settings.Maximum, prod, 2},
		{settings.Maximum, dev, 0},
		{settings.Standard, Artifact{Resource: schema.PolicyResourceFunctionVersion, Action: ActionDeploy, Environment: Environment{Key: "production", Production: true}}, 0},
		{settings.Maximum, Artifact{Resource: schema.PolicyResourceFunctionVersion, Action: ActionDeploy, Environment: Environment{Key: "production", Production: true}}, 2},
		{settings.Maximum, Artifact{Resource: schema.PolicyResourceFunctionVersion, Action: ActionDeploy, Environment: Environment{Key: "dev"}}, 0},
		{settings.Maximum, Artifact{Resource: schema.PolicyResourceSecurityConfiguration, Action: ActionSet, Environment: Environment{Key: "dev"}}, 2},
		{settings.Maximum, Artifact{Resource: schema.PolicyResourceSecurityConfiguration, Action: ActionEnableDeviceFunctions}, 2},
		{settings.Standard, Artifact{Resource: schema.PolicyResourceSecurityConfiguration, Action: ActionSet}, 0},
		{settings.Maximum, Artifact{Resource: schema.PolicyResourceWaiver, Action: ActionWaive}, 2},
		{settings.Maximum, Artifact{Resource: schema.PolicyResourceGoldenBaseline, Action: ActionApproveGoldens}, 2},
		{settings.Standard, Artifact{Resource: schema.PolicyResourceWaiver, Action: ActionWaive}, 0},
		{settings.Maximum, Artifact{Resource: schema.PolicyResourceLimit, Action: ActionRaiseLimit}, 0},
	}
	for i, c := range cases {
		if got := requirementFor(t, c.profile, c.art); got != c.want {
			t.Errorf("case %d: %s asks %d approvals of %s %s, want %d", i, c.profile, got, c.art.Resource, c.art.Action, c.want)
		}
	}
}

// Verifies: ABT-007, GOV-020.
func TestDefaultPolicies_ExperimentLaunch_ABT_007(t *testing.T) {
	launch := func(env Environment, risk ...schema.PolicyRisk) Artifact {
		return Artifact{Resource: schema.PolicyResourceExperiment, Action: ActionLaunch, Environment: env, Risk: risk}
	}
	dev, prod := Environment{Key: "dev"}, Environment{Key: "production", Production: true}
	cases := []struct {
		profile settings.Profile
		art     Artifact
		want    int
	}{
		{settings.Maximum, launch(dev, schema.PolicyRiskTouchesSecurePage), 2},
		{settings.Maximum, launch(prod, schema.PolicyRiskTouchesMoneyFlow), 2},
		{settings.Maximum, launch(dev, schema.PolicyRiskTouchesSecurePage, schema.PolicyRiskTouchesMoneyFlow), 2},
		{settings.Maximum, launch(prod), 0},
		{settings.Maximum, launch(prod, schema.PolicyRiskChangesFunctionUsedBy), 0},
		{settings.Strict, launch(prod, schema.PolicyRiskTouchesSecurePage), 0},
		{settings.Standard, launch(prod, schema.PolicyRiskTouchesMoneyFlow), 0},
	}
	for i, c := range cases {
		if got := requirementFor(t, c.profile, c.art); got != c.want {
			t.Errorf("case %d: %s asks %d approvals of an experiment launch with %v, want %d", i, c.profile, got, c.art.Risk, c.want)
		}
	}
}

// Verifies: GOV-020.
func TestParsePolicy_Refusals_GOV_020(t *testing.T) {
	s := parser(t)
	base := `{"schemaVersion":"1.0.0","kind":"policy","id":"01b0c450-6c00-7000-8000-000000000001","key":"p","name":"P",%s}`
	cases := map[string]string{
		"not JSON":                   `{`,
		"no require":                 fmt.Sprintf(base, `"match":{}`),
		"unknown resource":           fmt.Sprintf(base, `"match":{"resources":["nope"]},"require":{"approvers":1}`),
		"zero approvers":             fmt.Sprintf(base, `"match":{},"require":{"approvers":0}`),
		"risk on a waiver":           fmt.Sprintf(base, `"match":{"resources":["waiver"],"risk":["touchesSecurePage"]},"require":{"approvers":1}`),
		"more approvers than names":  fmt.Sprintf(base, `"match":{},"require":{"approvers":3,"users":["a","b"]}`),
		"distinct teams for one":     fmt.Sprintf(base, `"match":{},"require":{"approvers":1,"distinctTeams":true}`),
		"sequential with no order":   fmt.Sprintf(base, `"match":{},"require":{"approvers":2,"stages":"sequential"}`),
		"unknown property":           fmt.Sprintf(base, `"match":{},"require":{"approvers":1,"colour":"red"}`),
		"expiry past the bound":      fmt.Sprintf(base, `"match":{},"require":{"approvers":1,"expiryHours":9999}`),
		"production not a boolean":   fmt.Sprintf(base, `"match":{"production":"yes"},"require":{"approvers":1}`),
		"wrong kind":                 `{"schemaVersion":"1.0.0","kind":"page","id":"01b0c450-6c00-7000-8000-000000000001","key":"p","name":"P"}`,
		"too many approvers at once": fmt.Sprintf(base, `"match":{},"require":{"approvers":11}`),
	}
	for name, raw := range cases {
		if _, _, err := s.ParsePolicy([]byte(raw)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	good := fmt.Sprintf(base, `"match":{"resources":["release"],"production":true,"risk":["touchesSecurePage"]},"require":{"approvers":2,"roles":["owner"],"expiryHours":24}`)
	doc, canonical, err := s.ParsePolicy([]byte(good))
	if err != nil {
		t.Fatal(err)
	}
	if doc.Key != "p" || !strings.HasPrefix(string(canonical), `{"id"`) {
		t.Fatalf("parsed %+v, canonical %s", doc, canonical)
	}
}
