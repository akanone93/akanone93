// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package fnhost

import (
	"errors"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// TestTrapAfterDenialIsCapabilityDenied checks that a trap which follows a
// refused capability is reported as PLX-7003, not as a generic trap.
// Verifies: FN-012.
func TestTrapAfterDenialIsCapabilityDenied_FN_012(t *testing.T) {
	st := &callState{memory: &cappedMemory{}}
	if got, _ := plxerr.CodeOf(classify(st, errors.New("wasm error: unreachable"))); got != plxerr.FunctionTrapped {
		t.Fatalf("without a denial: %v, want FUNCTION_TRAPPED", got)
	}
	if s := st.deny(); s != statusDenied {
		t.Fatalf("deny returned %d", s)
	}
	if got, _ := plxerr.CodeOf(classify(st, errors.New("wasm error: unreachable\nwasm stack trace:"))); got != plxerr.FunctionCapabilityDenied {
		t.Fatalf("after a denial: %v, want FUNCTION_CAPABILITY_DENIED", got)
	}
}
