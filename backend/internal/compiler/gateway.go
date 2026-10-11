// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/nightCode42/plux3/backend/internal/schema"
)

// GatewaySource is a data source that reaches its origin through the
// Plux gateway: what a server must have registered for it before a
// release that uses it may be created (SEC-030, SEC-031).
type GatewaySource struct {
	// Name is the data source's name, which is also the name of its
	// upstream.
	Name string
	// File and Pointer say where the source is declared.
	File, Pointer string
	// Assurance is the level the source requires, "AL0" to "AL3"; empty
	// when it asks for none.
	Assurance string
	// Operations are the names of the operations the source declares.
	Operations []string
	// Transaction is the source's mapping of the fields a transaction
	// signs; nil for a source that is not a transaction.
	Transaction *GatewayTransaction
}

// GatewayTransaction is a source's SCA mapping: JSON Pointers into the
// request (SEC-028).
type GatewayTransaction struct {
	Amount, Currency, Payee, Summary string
}

// gatewayKinds are the kinds of data source that reach a network origin.
var gatewayKinds = map[schema.DataSourceKind]bool{
	schema.DataSourceKindRest: true, schema.DataSourceKindGraphql: true,
	schema.DataSourceKindWebsocket: true, schema.DataSourceKindSSE: true,
}

// noteGatewaySource records a source that goes through the gateway, which
// every network source does unless it is declared direct.
func (u *unit) noteGatewaySource(s *schema.DataSource, file, ptr string) {
	if !gatewayKinds[s.Kind] || s.Route == schema.DataSourceRouteDirect {
		return
	}
	g := GatewaySource{Name: s.Name, File: file, Pointer: ptr}
	if s.RequiresAssurance != "" && s.RequiresAssurance != schema.AssuranceLevelAL0 {
		g.Assurance = string(s.RequiresAssurance)
	}
	if t := s.Transaction; t != nil {
		g.Transaction = &GatewayTransaction{Amount: t.Amount, Currency: t.Currency, Payee: t.Payee, Summary: t.Summary}
	}
	var cfg struct {
		Operations map[string]json.RawMessage `json:"operations"`
	}
	if json.Unmarshal(s.Config, &cfg) == nil {
		for name := range cfg.Operations {
			g.Operations = append(g.Operations, name)
		}
		slices.Sort(g.Operations)
	}
	if !slices.ContainsFunc(u.gateway, func(e GatewaySource) bool { return e.File == file && e.Pointer == ptr }) {
		u.gateway = append(u.gateway, g)
	}
}

// sortedGatewaySources returns the recorded sources in a stable order.
func (u *unit) sortedGatewaySources() []GatewaySource {
	out := slices.Clone(u.gateway)
	slices.SortFunc(out, func(a, b GatewaySource) int {
		if c := strings.Compare(a.File, b.File); c != 0 {
			return c
		}
		return strings.Compare(a.Pointer, b.Pointer)
	})
	return out
}
