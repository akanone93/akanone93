// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package approval

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema"
)

// validArtifact is an artifact that passes validation.
func validArtifact() Artifact {
	return Artifact{
		Resource:       schema.PolicyResourceRelease,
		OrganizationID: "org",
		Subject:        "release-1",
		Action:         "promote",
		Hash:           HashOf([]byte("content")),
		Requester:      audit.Actor{ID: "user"},
	}
}

// codeOf returns the plxerr code of err, or fails the test.
func codeOf(t *testing.T, err error) plxerr.Code {
	t.Helper()
	code, ok := plxerr.CodeOf(err)
	if !ok {
		t.Fatalf("want a plxerr, got %v", err)
	}
	return code
}

// Verifies: GOV-021.
func TestArtifactValidate_GOV_020(t *testing.T) {
	if err := validArtifact().validate(); err != nil {
		t.Fatalf("a complete artifact is refused: %v", err)
	}
	cases := map[string]func(*Artifact){
		"unknown resource": func(a *Artifact) { a.Resource = "nonsense" },
		"no organisation":  func(a *Artifact) { a.OrganizationID = "" },
		"no subject":       func(a *Artifact) { a.Subject = "" },
		"no action":        func(a *Artifact) { a.Action = "" },
		"short hash":       func(a *Artifact) { a.Hash = "abcd" },
		"no requester":     func(a *Artifact) { a.Requester = audit.Actor{} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			a := validArtifact()
			mutate(&a)
			if err := a.validate(); err == nil {
				t.Fatal("an unbindable artifact was accepted")
			}
		})
	}
}

// Verifies: GOV-021.
func TestHashOf_PartsAreLengthPrefixed_GOV_020(t *testing.T) {
	if HashOf([]byte("ab"), []byte("c")) == HashOf([]byte("a"), []byte("bc")) {
		t.Fatal("content moved between parts keeps its hash")
	}
}

// Verifies: GOV-021.
func TestHashJSON_GOV_020(t *testing.T) {
	a, err := HashJSON(map[string]int{"b": 1, "a": 2})
	if err != nil {
		t.Fatal(err)
	}
	b, err := HashJSON(map[string]int{"a": 2, "b": 1})
	if err != nil || a != b {
		t.Fatalf("the same content hashes differently: %v", err)
	}
	if _, err := HashJSON(make(chan int)); err == nil {
		t.Fatal("content that cannot be encoded was hashed")
	}
}

// Verifies: GOV-020.
func TestCanonicalPayload_GOV_020(t *testing.T) {
	if out, err := CanonicalPayload(nil); err != nil || out != nil {
		t.Fatalf("no payload: %s, %v", out, err)
	}
	out, err := CanonicalPayload(json.RawMessage(`{"b": 1, "a": 2}`))
	if err != nil || string(out) != `{"a":2,"b":1}` {
		t.Fatalf("canonical form: %s, %v", out, err)
	}
	_, err = CanonicalPayload(json.RawMessage(`{"a":`))
	if codeOf(t, err) != plxerr.InvalidFormat {
		t.Fatalf("a broken payload is refused with %v", err)
	}
}

// Verifies: GOV-020.
func TestRiskReason_GOV_020(t *testing.T) {
	for _, r := range []schema.PolicyRisk{
		schema.PolicyRiskTouchesSecurePage,
		schema.PolicyRiskTouchesMoneyFlow,
		schema.PolicyRiskChangesFunctionUsedBy,
	} {
		if got := riskReason(r); got == string(r) || got == "" {
			t.Errorf("%s has no wording: %q", r, got)
		}
	}
	if got := riskReason("unknownRisk"); got != "unknownRisk" {
		t.Errorf("an unknown risk is named as it is, got %q", got)
	}
}

// Verifies: GOV-020.
func TestFailure_GOV_020(t *testing.T) {
	if codeOf(t, failure(pgx.ErrNoRows, "request")) != plxerr.ResourceNotFound {
		t.Error("a missing row is not a not-found refusal")
	}
	boom := errors.New("boom")
	if err := failure(boom, "request"); !errors.Is(err, boom) || !strings.Contains(err.Error(), "request") {
		t.Errorf("other failures are wrapped: %v", err)
	}
}

// Verifies: GOV-001, GOV-020.
func TestScopeHelpers_GOV_001(t *testing.T) {
	if appOf(scopeOf("")) != "" {
		t.Error("the organisation scope does not round-trip")
	}
	const app = "01b0c450-6c00-7000-8000-000000000001"
	if appOf(scopeOf(app)) != app {
		t.Error("an app scope does not round-trip")
	}
	if scopeOf("bad").Valid {
		t.Error("a bad identifier gives a valid scope")
	}
	if _, err := scopeUUID("bad"); codeOf(t, err) != plxerr.InvalidFormat {
		t.Errorf("a bad app identifier is refused with %v", err)
	}
}

// Verifies: GOV-001, GOV-003.
func TestAuthorizeScope_DeniesWithoutPermission_GOV_001(t *testing.T) {
	var nobody auth.Principal
	if authorizeScope(nobody, auth.ApprovalsDecide, "") == nil {
		t.Error("an organisation-wide check passed without the permission")
	}
	if authorizeScope(nobody, auth.ApprovalsDecide, "app") == nil {
		t.Error("an app check passed without the permission")
	}
	if authorizeSomeEnvironment(nobody, "") == nil || authorizeSomeEnvironment(nobody, "app") == nil {
		t.Error("a decision was allowed without the permission")
	}
}

// Verifies: GOV-020.
func TestStudioLink_GOV_020(t *testing.T) {
	if got := studioLink("", "r1"); got != "/approvals/r1" {
		t.Errorf("organisation link %q", got)
	}
	if got := studioLink("a1", "r1"); got != "/apps/a1/approvals/r1" {
		t.Errorf("app link %q", got)
	}
}
