// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package gateway

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/compiler"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// UpstreamsOf lists the upstreams registered for an environment, in the
// caller's transaction.
func (*Registry) UpstreamsOf(ctx context.Context, tx pgx.Tx, environmentID string) ([]Upstream, error) {
	id, err := storage.UUID(environmentID)
	if err != nil {
		return nil, plxerr.New(plxerr.InvalidFormat, "the environment identifier is not valid")
	}
	rows, err := dbgen.New(tx).ListEnvironmentGatewayUpstreams(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("gateway: %w", err)
	}
	out := make([]Upstream, 0, len(rows))
	for _, row := range rows {
		u, err := upstreamOf(row)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, nil
}

// CheckSources checks the data sources of a release against the upstreams
// registered for its environment, so that the registry, which the server
// enforces, and the documents, which the device follows, cannot silently
// differ (SEC-028, SEC-030, SEC-031). Every source that goes through the
// gateway needs an upstream of its name; the upstream must require at least
// the assurance the source does; and a source that declares a transaction
// must be matched by registered transaction operations with the same
// pointers, while an operation registered as a transaction must be the
// document's too. The result is one error per mismatch, at the source.
func CheckSources(sources []compiler.GatewaySource, upstreams []Upstream) plxerr.Diagnostics {
	var out plxerr.Diagnostics
	byName := make(map[string]Upstream, len(upstreams))
	for _, u := range upstreams {
		byName[u.Name] = u
	}
	for _, s := range sources {
		at := plxerr.Location{File: s.File, Path: s.Pointer}
		report := func(code plxerr.Code, format string, args ...any) {
			out = append(out, plxerr.NewDiagnostic(code, at, format, args...))
		}
		up, ok := byName[s.Name]
		if !ok {
			report(plxerr.GatewayUpstreamUnknown, "data source %q goes through the gateway, and the environment has no upstream %q registered; register one or declare the source direct", s.Name, s.Name)
			continue
		}
		candidates := candidateOperations(s, up, report)
		checkSourceAssurance(s, up, candidates, report)
		checkSourceTransaction(s, candidates, report)
	}
	return out
}

// candidateOperations are the registered operations the document's source
// can call: those it names, or, if it names none, all of the upstream's.
// An operation the source names that an upstream with operations lacks is
// reported, because a call to it would be refused.
func candidateOperations(s compiler.GatewaySource, up Upstream, report func(plxerr.Code, string, ...any)) []Operation {
	if len(s.Operations) == 0 {
		return up.Operations
	}
	var out []Operation
	for _, name := range s.Operations {
		if op, ok := up.Operation(name); ok {
			out = append(out, op)
		} else if len(up.Operations) > 0 {
			report(plxerr.GatewayDestinationRefused, "data source %q declares the operation %q, and upstream %q does not register it", s.Name, name, up.Name)
		}
	}
	return out
}

// checkSourceAssurance requires the registry to ask for at least the
// assurance the document does, for the source and for each operation it
// calls.
func checkSourceAssurance(s compiler.GatewaySource, up Upstream, ops []Operation, report func(plxerr.Code, string, ...any)) {
	if s.Assurance == "" {
		return
	}
	need := AssuranceRank(s.Assurance)
	if AssuranceRank(up.Assurance) >= need {
		return
	}
	if len(ops) == 0 {
		report(plxerr.AssuranceInsufficient, "data source %q requires %s, and upstream %q requires %s", s.Name, s.Assurance, up.Name, assuranceOrDefault(up.Assurance))
		return
	}
	for _, op := range ops {
		if AssuranceRank(op.Assurance) < need {
			report(plxerr.AssuranceInsufficient, "data source %q requires %s, and operation %q of upstream %q requires %s",
				s.Name, s.Assurance, op.Name, up.Name, assuranceOrDefault(max2(up.Assurance, op.Assurance)))
		}
	}
}

// max2 returns the higher of two assurance levels.
func max2(a, b string) string {
	if AssuranceRank(b) > AssuranceRank(a) {
		return b
	}
	return a
}

// checkSourceTransaction matches the document's transaction with the
// registered operations, both ways.
func checkSourceTransaction(s compiler.GatewaySource, ops []Operation, report func(plxerr.Code, string, ...any)) {
	registered := slices.DeleteFunc(slices.Clone(ops), func(op Operation) bool { return op.Transaction == nil })
	switch {
	case s.Transaction == nil:
		for _, op := range registered {
			report(plxerr.SCADynamicLinkingMismatch, "operation %q is registered as a transaction, and data source %q declares none", op.Name, s.Name)
		}
	case len(registered) == 0:
		report(plxerr.SCADynamicLinkingMismatch, "data source %q declares a transaction, and no registered operation of upstream %q is one", s.Name, s.Name)
	default:
		want := Transaction{Amount: s.Transaction.Amount, Currency: s.Transaction.Currency, Payee: s.Transaction.Payee, Summary: s.Transaction.Summary}
		for _, op := range registered {
			if *op.Transaction != want {
				report(plxerr.SCADynamicLinkingMismatch, "the transaction mapping of operation %q differs from data source %q: the fields that are signed and the fields that are checked must be the same", op.Name, s.Name)
			}
		}
	}
}
