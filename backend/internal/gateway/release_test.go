// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package gateway

import (
	"testing"

	"github.com/nightCode42/plux3/backend/internal/compiler"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// Verifies: SEC-028, SEC-030, SEC-031.
// A release is checked against the environment's registered upstreams:
// nothing that the documents and the registry could disagree about passes.
func TestCheckSources(t *testing.T) {
	t.Parallel()
	pay := &compiler.GatewayTransaction{Amount: "/amount", Currency: "/currency", Payee: "/to/iban"}
	source := func(edit func(*compiler.GatewaySource)) compiler.GatewaySource {
		s := compiler.GatewaySource{Name: "bank", File: "plugins/bank/plugin.json", Pointer: "/dataSources/0", Operations: []string{"pay", "list"}, Transaction: pay}
		if edit != nil {
			edit(&s)
		}
		return s
	}
	upstream := func(edit func(*Upstream)) Upstream {
		u := Upstream{
			Name: "bank", Assurance: "AL1", Methods: []string{"GET", "POST"},
			Operations: []Operation{
				{Name: "list", Methods: []string{"GET"}},
				{Name: "pay", Methods: []string{"POST"}, Assurance: "AL2", Transaction: &Transaction{Amount: "/amount", Currency: "/currency", Payee: "/to/iban"}},
			},
		}
		if edit != nil {
			edit(&u)
		}
		return u
	}
	tests := []struct {
		name     string
		src      compiler.GatewaySource
		upstream []Upstream
		want     []plxerr.Code
	}{
		{"matched", source(nil), []Upstream{upstream(nil)}, nil},
		{"no upstream at all", source(nil), nil, []plxerr.Code{plxerr.GatewayUpstreamUnknown}},
		{"an upstream of another name", source(nil), []Upstream{upstream(func(u *Upstream) { u.Name = "other" })}, []plxerr.Code{plxerr.GatewayUpstreamUnknown}},
		{"an operation the registry lacks", source(func(s *compiler.GatewaySource) { s.Operations = []string{"pay", "refund"} }), []Upstream{upstream(nil)}, []plxerr.Code{plxerr.GatewayDestinationRefused}},
		{"an upstream with no operations is open", source(func(s *compiler.GatewaySource) { s.Operations, s.Transaction = nil, nil }), []Upstream{upstream(func(u *Upstream) { u.Operations = nil })}, nil},
		{"a transaction no operation registers", source(nil), []Upstream{upstream(func(u *Upstream) { u.Operations[1].Transaction = nil })}, []plxerr.Code{plxerr.SCADynamicLinkingMismatch}},
		{"another amount pointer", source(nil), []Upstream{upstream(func(u *Upstream) { u.Operations[1].Transaction.Amount = "/sum" })}, []plxerr.Code{plxerr.SCADynamicLinkingMismatch}},
		{"another payee pointer", source(nil), []Upstream{upstream(func(u *Upstream) { u.Operations[1].Transaction.Payee = "/payee" })}, []plxerr.Code{plxerr.SCADynamicLinkingMismatch}},
		{"a summary only one side maps", source(nil), []Upstream{upstream(func(u *Upstream) { u.Operations[1].Transaction.Summary = "/memo" })}, []plxerr.Code{plxerr.SCADynamicLinkingMismatch}},
		{"a registered transaction the document lacks", source(func(s *compiler.GatewaySource) { s.Transaction = nil }), []Upstream{upstream(nil)}, []plxerr.Code{plxerr.SCADynamicLinkingMismatch}},
		{"assurance the upstream and operation do not reach", source(func(s *compiler.GatewaySource) { s.Assurance = "AL3" }), []Upstream{upstream(nil)}, []plxerr.Code{plxerr.AssuranceInsufficient, plxerr.AssuranceInsufficient}},
		{"assurance the upstream reaches", source(func(s *compiler.GatewaySource) { s.Assurance = "AL1" }), []Upstream{upstream(nil)}, nil},
		{"assurance only an operation reaches", source(func(s *compiler.GatewaySource) { s.Assurance, s.Operations = "AL2", []string{"pay"} }), []Upstream{upstream(nil)}, nil},
		{"assurance the operation does not reach", source(func(s *compiler.GatewaySource) {
			s.Assurance, s.Operations, s.Transaction = "AL2", []string{"list"}, nil
		}), []Upstream{upstream(func(u *Upstream) { u.Operations = u.Operations[:1] })}, []plxerr.Code{plxerr.AssuranceInsufficient}},
		{"assurance with no operations named", source(func(s *compiler.GatewaySource) { s.Assurance, s.Operations, s.Transaction = "AL2", nil, nil }), []Upstream{upstream(func(u *Upstream) { u.Operations = nil })}, []plxerr.Code{plxerr.AssuranceInsufficient}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := CheckSources([]compiler.GatewaySource{tt.src}, tt.upstream)
			if len(got) != len(tt.want) {
				t.Fatalf("diagnostics = %v, want codes %v", got, tt.want)
			}
			for i, d := range got {
				if d.Code != tt.want[i] || d.Severity != plxerr.SeverityError || d.File != tt.src.File || d.Path != tt.src.Pointer {
					t.Errorf("diagnostic %d = %+v, want code %d at the source", i, d, tt.want[i])
				}
			}
		})
	}
}
