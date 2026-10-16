// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package fnhost

import (
	"context"
	"errors"
	"testing"
	"time"

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
	if got, _ := plxerr.CodeOf(classify(st, errors.New("wasm error: unreachable\nwasm stack trace:\n\t.run()"))); got != plxerr.FunctionCapabilityDenied {
		t.Fatalf("after a denial: %v, want FUNCTION_CAPABILITY_DENIED", got)
	}
}

// TestGuestReturningAfterItsDeadlineTimesOut checks that a guest which
// returns after its deadline, before the runtime closed it, is a timeout and
// not a success (an HTTP call ends with the deadline and the guest answers
// its error); a cancellation stays a cancellation.
// Verifies: FN-011.
func TestGuestReturningAfterItsDeadlineTimesOut_FN_011(t *testing.T) {
	t.Parallel()
	if err := expired(t.Context()); err != nil {
		t.Fatalf("before the deadline: %v", err)
	}
	ended, cancel := context.WithDeadline(t.Context(), time.Unix(0, 0))
	defer cancel()
	if got, _ := plxerr.CodeOf(expired(ended)); got != plxerr.FunctionTimeout {
		t.Errorf("after the deadline: %v, want FUNCTION_TIMEOUT", got)
	}
	cancelled, stop := context.WithCancel(t.Context())
	stop()
	if got, _ := plxerr.CodeOf(expired(cancelled)); got != plxerr.FunctionCancelled {
		t.Errorf("after a cancellation: %v, want FUNCTION_CANCELLED", got)
	}
}
