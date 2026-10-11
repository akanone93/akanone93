// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package device

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// raspEvent is the telemetry event the runtime's self-protection sends for
// every detection (SEC-071, ADR-0057).
const raspEvent = "rasp_detection"

// raspStickyKinds are the detections that suggest a modified environment:
// what they saw cannot be undone by their going away, so they lower the
// device's level until it attests again. The other kinds (screen capture,
// overlays, accessibility services) follow the moment and lower nothing.
var raspStickyKinds = []string{"debugger", "emulator", "hooking", "repackaging", "root", "untrusted_installer"}

// raspCap is the level a device with such a detection is held to.
const raspCap = AL1

// RiskEvent is a telemetry event as the device sent it, for the ones that
// say something about the device's trustworthiness.
type RiskEvent struct {
	// Name is the event's name.
	Name string
	// Fields are the event's fields as JSON.
	Fields []byte
}

// RecordRiskEvents lowers the level of a device that reports a detection
// of a modified environment (SEC-071): its kind is recorded and its level
// is capped at AL1 until a clean attestation clears it (SEC-006), which
// the next token reflects. Events of other names, kinds that follow the
// moment and fields that are not valid are ignored; they stay telemetry.
func (s *Service) RecordRiskEvents(ctx context.Context, d Identity, events []RiskEvent) error {
	kinds := stickyKinds(events)
	if len(kinds) == 0 {
		return nil
	}
	id, err := storage.UUID(d.DeviceID)
	if err != nil {
		return fmt.Errorf("device: %w", err)
	}
	err = s.db.InTx(ctx, storage.Tenant{OrganizationID: d.OrganizationID}, func(ctx context.Context, tx pgx.Tx) error {
		_, err := dbgen.New(tx).RecordDeviceRASP(ctx, dbgen.RecordDeviceRASPParams{
			ID: id, DetectedAt: storage.Timestamp(s.now()), Kinds: kinds,
			Capped: levelsAbove(raspCap), Cap: string(raspCap),
		})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("device: record detection: %w", err)
		}
		return nil
	})
	return err //nolint:wrapcheck // InTx wraps its own failures
}

// levelsAbove lists the levels higher than l.
func levelsAbove(l Level) []string {
	var out []string
	for _, c := range []Level{AL0, AL1, AL2, AL3} {
		if c.Rank() > l.Rank() {
			out = append(out, string(c))
		}
	}
	return out
}

// stickyKinds are the kinds of modified environment the events report.
func stickyKinds(events []RiskEvent) []string {
	var kinds []string
	for _, e := range events {
		if e.Name != raspEvent {
			continue
		}
		var f struct {
			Kind string `json:"kind"`
		}
		if json.Unmarshal(e.Fields, &f) == nil && slices.Contains(raspStickyKinds, f.Kind) {
			kinds = append(kinds, f.Kind)
		}
	}
	return kinds
}
