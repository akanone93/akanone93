// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"reflect"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/function"
)

// Verifies: FN-012.
// The wire form of a version carries exactly the capabilities, the assurance
// and the source its domain value holds: a declaration is copied as it
// stands, and a version whose build declared nothing reports no capabilities
// message at all, not an empty one.
func TestFunctionVersionProtoCarriesDeclaredFields_FN_012(t *testing.T) {
	t.Parallel()
	declared := functionVersionProto(function.Version{
		Name: "payments.charge", SourceID: "src-1",
		Declared: &function.Declaration{
			Assurance: "hardware",
			Capabilities: function.Capabilities{
				HTTP: []string{"api.example.com"}, KV: true, Secrets: []string{"key"}, Functions: []string{"payments.refund"}, Claims: true,
			},
		},
	})
	c := declared.GetCapabilities()
	if declared.GetSourceId() != "src-1" || declared.GetAssurance() != "hardware" || c == nil ||
		!reflect.DeepEqual(c.GetHttp(), []string{"api.example.com"}) || !c.GetKv() || !reflect.DeepEqual(c.GetSecrets(), []string{"key"}) ||
		!reflect.DeepEqual(c.GetFunctions(), []string{"payments.refund"}) || !c.GetClaims() {
		t.Errorf("declared version = %v", declared)
	}
	bare := functionVersionProto(function.Version{Name: "payments.charge", SourceID: "src-2"})
	if bare.GetSourceId() != "src-2" || bare.GetCapabilities() != nil || bare.GetAssurance() != "" {
		t.Errorf("version without a declaration = %v", bare)
	}
	none := functionVersionProto(function.Version{Declared: &function.Declaration{}})
	if none.GetCapabilities() == nil || none.GetCapabilities().GetKv() || len(none.GetCapabilities().GetHttp()) != 0 {
		t.Errorf("a declaration that grants nothing = %v", none)
	}
}
