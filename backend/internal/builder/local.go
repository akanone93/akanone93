// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/nightCode42/plux3/backend/internal/fnhost"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

// RunLocal runs one invocation of a built module on the function host for a
// developer's machine (`plux fn invoke`): under the registry limits tightened
// by the declaration's own, with the declared capabilities, an in-memory
// key-value store and nothing else, as RunTests does (FN-011, FN-012). The
// module is compiled for the declaration's placement. log, when not nil,
// receives the function's log records. An error from the host is a
// *plxerr.Error for a system failure (FN-010).
func RunLocal(ctx context.Context, engine *fnhost.Engine, module []byte, decl *Declaration, set limits.Set, input []byte, claims json.RawMessage, log func(fnhost.LogRecord)) (fnhost.Outcome, fnhost.Stats, error) {
	placement := fnhost.PlacementServer
	if decl.Placement() == PlacementDevice {
		placement = fnhost.PlacementDevice
	}
	m, err := engine.Compile(ctx, module, placement)
	if err != nil {
		return fnhost.Outcome{}, fnhost.Stats{}, fmt.Errorf("builder.RunLocal: %w", err)
	}
	defer func() { _ = m.Close(context.WithoutCancel(ctx)) }()
	if len(bytes.TrimSpace(claims)) == 0 {
		claims = nil
	}
	out, stats, err := engine.Invoke(ctx, m, fnhost.Call{
		Input: input, Limits: testLimits(set, decl), Grants: testGrants(decl), Claims: claims,
		Context: fnhost.ContextInfo{App: "local", Environment: "local", AssuranceLevel: string(decl.Function.Assurance)},
		Host:    &testHost{kv: map[string][]byte{}}, Log: log,
	})
	if err != nil {
		return out, stats, err //nolint:wrapcheck // the host's own error, a *plxerr.Error for a system failure
	}
	return out, stats, nil
}
