// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/riverqueue/river/rivertype"

	"github.com/nightCode42/plux3/backend/internal/release"
	"github.com/nightCode42/plux3/backend/internal/schema/uuid7"
)

// Verifies: CMP-030.
// A publish waiting for assets is snoozed for the time it asks, which the
// job queue does not count as a failed attempt; other errors pass
// through.
func TestSnoozePending(t *testing.T) {
	t.Parallel()
	err := snoozePending(&release.AssetsPendingError{Pending: 2, RetryAfter: 1500 * time.Millisecond})
	snooze, ok := errors.AsType[*rivertype.JobSnoozeError](err)
	if !ok || snooze.Duration != 1500*time.Millisecond {
		t.Errorf("snoozePending: %v", err)
	}
	other := errors.New("boom")
	if !errors.Is(snoozePending(other), other) || snoozePending(nil) != nil {
		t.Error("other errors must pass through")
	}
}

// Verifies: SEC-160.
// Identifiers, a device's among them, are random UUIDv7 values: the version
// and variant are set, the time-ordered prefix is the clock and the rest is
// entropy, so two generators fed different entropy at the same instant
// differ and none of a device's properties enters the value.
func TestDeviceIdentifiersAreRandomUUIDv7(t *testing.T) {
	t.Parallel()
	gen := NewIDs()
	seen := map[string]bool{}
	for range 64 {
		s, err := gen.New()
		if err != nil {
			t.Fatal(err)
		}
		id, err := uuid7.Parse(s)
		if err != nil || id.Version() != 7 || id[8]&0xC0 != 0x80 {
			t.Fatalf("%q is not a UUIDv7: %v", s, err)
		}
		if seen[s] {
			t.Fatalf("%q was issued twice", s)
		}
		seen[s] = true
	}
	at := func() time.Time { return time.UnixMilli(1_700_000_000_000) }
	a, errA := uuid7.NewGenerator(at, bytes.NewReader(bytes.Repeat([]byte{0x11}, 10))).New()
	b, errB := uuid7.NewGenerator(at, bytes.NewReader(bytes.Repeat([]byte{0x22}, 10))).New()
	if errA != nil || errB != nil {
		t.Fatal(errA, errB)
	}
	if a == b || !bytes.Equal(a[:6], b[:6]) {
		t.Errorf("same instant, different entropy: %s and %s", a, b)
	}
}
