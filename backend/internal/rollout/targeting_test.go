// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package rollout

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema"
)

// ruleOf decodes a rule written as JSON.
func ruleOf(t *testing.T, doc string) schema.SegmentRule {
	t.Helper()
	var r schema.SegmentRule
	if err := json.Unmarshal([]byte(doc), &r); err != nil {
		t.Fatalf("rule %s: %v", doc, err)
	}
	return r
}

// phone is a device that has every attribute a rule can test.
func phone() Device {
	return Device{
		ID: "d1",
		Attributes: Attributes{
			Build: "42", RuntimeVersion: "1.10.0", OS: "android", OSVersion: "14", DeviceClass: "phone",
			Locale: "de-DE", Region: "DE", Channel: "beta",
		},
		UserAttributes: map[string]string{"tier": HashUserAttribute("tier", "gold")},
	}
}

// Verifies: REL-051.
func TestEvaluate_REL_051_Conditions(t *testing.T) {
	tests := []struct {
		name string
		rule string
		want bool
	}{
		{"os equals", `{"attribute":"os","operator":"eq","value":"android"}`, true},
		{"os equals, case-insensitive", `{"attribute":"os","operator":"eq","value":"Android"}`, true},
		{"os differs", `{"attribute":"os","operator":"eq","value":"ios"}`, false},
		{"os not equal", `{"attribute":"os","operator":"neq","value":"ios"}`, true},
		{"region in list", `{"attribute":"region","operator":"in","value":["AT","DE","CH"]}`, true},
		{"region not in list", `{"attribute":"region","operator":"notIn","value":["AT","CH"]}`, true},
		{"region in list, absent", `{"attribute":"region","operator":"in","value":["AT","CH"]}`, false},
		{"locale", `{"attribute":"locale","operator":"eq","value":"de-de"}`, true},
		{"device class", `{"attribute":"deviceClass","operator":"eq","value":"tablet"}`, false},
		{"channel", `{"attribute":"channel","operator":"eq","value":"beta"}`, true},
		{"build at least", `{"attribute":"build","operator":"gte","value":42}`, true},
		{"build below", `{"attribute":"build","operator":"lt","value":42}`, false},
		{"build above", `{"attribute":"build","operator":"gt","value":41}`, true},
		{"build as string number", `{"attribute":"build","operator":"eq","value":"42"}`, true},
		{"version compares numerically", `{"attribute":"runtimeVersion","operator":"gt","value":"1.9.0"}`, true},
		{"version padded", `{"attribute":"osVersion","operator":"eq","value":"14.0.0"}`, true},
		{"version lte", `{"attribute":"osVersion","operator":"lte","value":"13"}`, false},
		{"user attribute equals", `{"attribute":"user.tier","operator":"eq","value":"gold"}`, true},
		{"user attribute differs", `{"attribute":"user.tier","operator":"eq","value":"silver"}`, false},
		{"user attribute in", `{"attribute":"user.tier","operator":"in","value":["silver","gold"]}`, true},
		{"user attribute not in", `{"attribute":"user.tier","operator":"notIn","value":["silver"]}`, true},
		{"missing user attribute", `{"attribute":"user.plan","operator":"eq","value":"pro"}`, false},
		{"missing user attribute is not 'not equal'", `{"attribute":"user.plan","operator":"neq","value":"pro"}`, false},
		{"all holds", `{"all":[{"attribute":"os","operator":"eq","value":"android"},{"attribute":"region","operator":"eq","value":"DE"}]}`, true},
		{"all fails on one", `{"all":[{"attribute":"os","operator":"eq","value":"android"},{"attribute":"region","operator":"eq","value":"FR"}]}`, false},
		{"any holds on one", `{"any":[{"attribute":"os","operator":"eq","value":"ios"},{"attribute":"region","operator":"eq","value":"DE"}]}`, true},
		{"any fails", `{"any":[{"attribute":"os","operator":"eq","value":"ios"},{"attribute":"region","operator":"eq","value":"FR"}]}`, false},
		{"nested", `{"all":[{"any":[{"attribute":"os","operator":"eq","value":"ios"},{"attribute":"os","operator":"eq","value":"android"}]},{"attribute":"build","operator":"gte","value":40}]}`, true},
	}
	d := phone()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Evaluate(ruleOf(t, tc.rule), d); got != tc.want {
				t.Errorf("Evaluate(%s) = %v, want %v", tc.rule, got, tc.want)
			}
		})
	}
}

// A device that did not send an attribute is in no condition on it: a rule
// never sweeps in devices it knows nothing about.
//
// Verifies: REL-051.
func TestEvaluate_REL_051_MissingAttributesNeverHold(t *testing.T) {
	var bare Device
	for _, rule := range []string{
		`{"attribute":"os","operator":"eq","value":"android"}`,
		`{"attribute":"os","operator":"neq","value":"android"}`,
		`{"attribute":"region","operator":"notIn","value":["DE"]}`,
		`{"attribute":"build","operator":"gte","value":0}`,
	} {
		if Evaluate(ruleOf(t, rule), bare) {
			t.Errorf("a device without attributes satisfies %s", rule)
		}
	}
}

// A device value that is not a number or a version fails every comparison,
// including "not equal".
//
// Verifies: REL-051.
func TestEvaluate_REL_051_MalformedDeviceValues(t *testing.T) {
	d := phone()
	d.Build, d.RuntimeVersion = "forty-two", "dev"
	for _, rule := range []string{
		`{"attribute":"build","operator":"gte","value":1}`,
		`{"attribute":"build","operator":"neq","value":1}`,
		`{"attribute":"runtimeVersion","operator":"neq","value":"1.0"}`,
		`{"attribute":"runtimeVersion","operator":"notIn","value":["1.0"]}`,
	} {
		if Evaluate(ruleOf(t, rule), d) {
			t.Errorf("a malformed value satisfies %s", rule)
		}
	}
}

// The attributes a device sent are compared as hashes: the plain value of a
// user attribute is not what the server holds, and the name is part of the
// hash.
//
// Verifies: REL-051.
func TestHashUserAttribute_REL_051(t *testing.T) {
	if HashUserAttribute("tier", "gold") == HashUserAttribute("plan", "gold") {
		t.Error("the same value of two attributes hashes alike")
	}
	if len(HashUserAttribute("tier", "gold")) != 64 {
		t.Error("the hash is not a hex SHA-256")
	}
	d := phone()
	d.UserAttributes = map[string]string{"tier": "gold"}
	if Evaluate(ruleOf(t, `{"attribute":"user.tier","operator":"eq","value":"gold"}`), d) {
		t.Error("an unhashed attribute value matched")
	}
}

// Verifies: REL-051.
func TestTargeting_REL_051_RuleAndSegmentsAllHold(t *testing.T) {
	rule := ruleOf(t, `{"attribute":"os","operator":"eq","value":"android"}`)
	beta := ruleOf(t, `{"attribute":"channel","operator":"eq","value":"beta"}`)
	other := ruleOf(t, `{"attribute":"region","operator":"eq","value":"FR"}`)
	d := phone()
	for name, tc := range map[string]struct {
		t    Targeting
		want bool
	}{
		"nothing matches everyone":  {Targeting{}, true},
		"rule only":                 {Targeting{Rule: &rule}, true},
		"rule and a segment":        {Targeting{Rule: &rule, Segments: []Segment{{Key: "beta", Rule: beta}}}, true},
		"one segment fails":         {Targeting{Rule: &rule, Segments: []Segment{{Key: "beta", Rule: beta}, {Key: "fr", Rule: other}}}, false},
		"segments without the rule": {Targeting{Segments: []Segment{{Key: "fr", Rule: other}}}, false},
	} {
		if got := tc.t.Matches(d); got != tc.want {
			t.Errorf("%s: Matches = %v, want %v", name, got, tc.want)
		}
	}
}

// Verifies: REL-051.
func TestValidateRule_REL_051_Refusals(t *testing.T) {
	deep := `{"attribute":"os","operator":"eq","value":"ios"}`
	for range maxRuleDepth + 1 {
		deep = `{"all":[` + deep + `]}`
	}
	tests := []struct {
		name, rule, path string
	}{
		{"unknown attribute", `{"attribute":"colour","operator":"eq","value":"red"}`, "/r/attribute"},
		{"bad user attribute name", `{"attribute":"user.Bad-Name","operator":"eq","value":"x"}`, "/r/attribute"},
		{"unknown operator", `{"attribute":"os","operator":"like","value":"a"}`, "/r/operator"},
		{"no operator", `{"attribute":"os","value":"a"}`, "/r/operator"},
		{"ordering a string", `{"attribute":"os","operator":"lt","value":"z"}`, "/r/operator"},
		{"ordering a hashed attribute", `{"attribute":"user.tier","operator":"gt","value":"a"}`, "/r/operator"},
		{"build is not a number", `{"attribute":"build","operator":"eq","value":"x"}`, "/r/value"},
		{"version is not a version", `{"attribute":"osVersion","operator":"gte","value":"latest"}`, "/r/value"},
		{"in needs a list", `{"attribute":"os","operator":"in","value":"ios"}`, "/r/value"},
		{"in needs a non-empty list", `{"attribute":"os","operator":"in","value":[]}`, "/r/value"},
		{"eq takes one value", `{"attribute":"os","operator":"eq","value":["a","b"]}`, "/r/value"},
		{"object value", `{"attribute":"os","operator":"eq","value":{"a":1}}`, "/r/value"},
		{"no value", `{"attribute":"os","operator":"eq"}`, "/r/value"},
		{"group and condition", `{"all":[{"attribute":"os","operator":"eq","value":"ios"}],"attribute":"os"}`, "/r"},
		{"all and any", `{"all":[{"attribute":"os","operator":"eq","value":"ios"}],"any":[{"attribute":"os","operator":"eq","value":"ios"}]}`, "/r"},
		{"bad rule in a group", `{"any":[{"attribute":"os","operator":"eq","value":"ios"},{"attribute":"nope","operator":"eq","value":"x"}]}`, "/r/any/1/attribute"},
		{"nested too deep", deep, "/r"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateRule(ruleOf(t, tc.rule), "/r")
			var pe *plxerr.Error
			if !errors.As(err, &pe) || pe.Code != plxerr.TargetingRuleInvalid {
				t.Fatalf("ValidateRule = %v, want PLX-3152", err)
			}
			if !strings.Contains(pe.Message, tc.path) {
				t.Errorf("message %q does not name %s", pe.Message, tc.path)
			}
		})
	}
}

// Verifies: REL-051.
func TestValidateRule_REL_051_AcceptsRules(t *testing.T) {
	for _, rule := range []string{
		`{"attribute":"os","operator":"in","value":["ios","android"]}`,
		`{"attribute":"build","operator":"gte","value":100}`,
		`{"attribute":"runtimeVersion","operator":"gte","value":"1.2"}`,
		`{"attribute":"user.tier","operator":"in","value":["gold","platinum"]}`,
		`{"all":[{"attribute":"region","operator":"eq","value":"DE"},{"any":[{"attribute":"os","operator":"eq","value":"ios"}]}]}`,
	} {
		if err := ValidateRule(ruleOf(t, rule), "/r"); err != nil {
			t.Errorf("ValidateRule(%s) = %v", rule, err)
		}
	}
}

// Verifies: REL-051.
func TestParseTargeting_REL_051(t *testing.T) {
	segment := func(key, rule string) []byte {
		return []byte(`{"schemaVersion":"1","kind":"segment","id":"x","key":"` + key + `","name":"N","rule":` + rule + `}`)
	}
	ok := `{"attribute":"os","operator":"eq","value":"ios"}`
	got, err := ParseTargeting([]byte(ok), [][]byte{segment("testers", ok)})
	if err != nil || got.Rule == nil || len(got.Segments) != 1 || got.Segments[0].Key != "testers" {
		t.Fatalf("ParseTargeting = %+v, %v", got, err)
	}
	if got, err := ParseTargeting(nil, nil); err != nil || got.Rule != nil || len(got.Segments) != 0 {
		t.Errorf("empty targeting = %+v, %v", got, err)
	}
	refused := map[string]struct {
		rule     string
		segments [][]byte
	}{
		"rule is not JSON":            {`{`, nil},
		"rule is invalid":             {`{"attribute":"x"}`, nil},
		"segment is not JSON":         {"", [][]byte{[]byte("nope")}},
		"segment is not a segment":    {"", [][]byte{[]byte(`{"kind":"flag","key":"k"}`)}},
		"segment has an invalid rule": {"", [][]byte{segment("bad", `{"attribute":"x","operator":"eq","value":1}`)}},
		"segment named twice":         {"", [][]byte{segment("a", ok), segment("a", ok)}},
	}
	for name, tc := range refused {
		_, err := ParseTargeting([]byte(tc.rule), tc.segments)
		var pe *plxerr.Error
		if !errors.As(err, &pe) || pe.Code != plxerr.TargetingRuleInvalid {
			t.Errorf("%s: ParseTargeting = %v, want PLX-3152", name, err)
		}
	}
}
