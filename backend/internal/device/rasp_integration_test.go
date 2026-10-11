// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package device_test

import (
	"context"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/attest/keyattest"
	"github.com/nightCode42/plux3/backend/internal/attest/playintegrity"
	"github.com/nightCode42/plux3/backend/internal/device"
	"github.com/nightCode42/plux3/backend/internal/device/devicetest"
)

// Verifies: SEC-071, SEC-006, SEC-007.
// A device that reports a detection of a modified environment is held to
// AL1; a detection that follows the moment changes nothing; a clean
// attestation restores what the evidence proves.
func TestRASPCapsAndReattestationClears(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	ctx := context.Background()
	key, jwk := devicetest.NewKey(t)
	r.android(&key.PublicKey, keyattest.SecurityStrongBox, playintegrity.LabelBasic, playintegrity.LabelStrong)
	d, err := r.register(t, "production", "android", jwk, device.KeyStorageStrongBox, androidEvidence())
	if err != nil || d.AssuranceLevel != "AL3" {
		t.Fatalf("register = %+v, %v", d, err)
	}
	id := device.Identity{DeviceID: d.ID, OrganizationID: r.org, AppID: d.AppID, EnvironmentID: d.EnvironmentID}
	level := func() string {
		t.Helper()
		got, err := r.svc.Get(ctx, r.owner, d.ID)
		if err != nil {
			t.Fatal(err)
		}
		return got.AssuranceLevel
	}
	event := func(name, fields string) device.RiskEvent {
		return device.RiskEvent{Name: name, Fields: []byte(fields)}
	}

	for _, live := range []string{"screen_capture", "overlay", "accessibility"} {
		if err := r.svc.RecordRiskEvents(ctx, id, []device.RiskEvent{event("rasp_detection", `{"kind":"`+live+`"}`)}); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.svc.RecordRiskEvents(ctx, id, []device.RiskEvent{event("error", `{"kind":"root"}`)}); err != nil {
		t.Fatal(err)
	}
	if got := level(); got != "AL3" {
		t.Fatalf("after detections that follow the moment the level is %s", got)
	}

	if err := r.svc.RecordRiskEvents(ctx, id, []device.RiskEvent{event("rasp_detection", `{"kind":"root","severity":"high"}`)}); err != nil {
		t.Fatal(err)
	}
	if got := level(); got != "AL1" {
		t.Fatalf("after a root detection the level is %s, want AL1", got)
	}
	// A second report does not raise or change it.
	if err := r.svc.RecordRiskEvents(ctx, id, []device.RiskEvent{event("rasp_detection", `{"kind":"hooking"}`)}); err != nil {
		t.Fatal(err)
	}
	if got := level(); got != "AL1" {
		t.Fatalf("after a second detection the level is %s", got)
	}

	r.now = r.now.Add(time.Hour)
	again, err := r.svc.Reattest(ctx, d.ID, r.challenge(t, "production"), androidEvidence())
	if err != nil || again.AssuranceLevel != "AL3" {
		t.Fatalf("a clean attestation = %+v, %v", again, err)
	}
	if got := level(); got != "AL3" {
		t.Fatalf("after re-attesting the level is %s, want AL3", got)
	}
}

// Verifies: SEC-071.
// A device that is already at AL1 or lower keeps its level.
func TestRASPDoesNotRaise(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	ctx := context.Background()
	key, jwk := devicetest.NewKey(t)
	r.android(&key.PublicKey, keyattest.SecurityTrustedEnvironment)
	d, err := r.register(t, "production", "android", jwk, device.KeyStorageTEE, androidEvidence())
	if err != nil || d.AssuranceLevel != "AL1" {
		t.Fatalf("register = %+v, %v", d, err)
	}
	id := device.Identity{DeviceID: d.ID, OrganizationID: r.org}
	if err := r.svc.RecordRiskEvents(ctx, id, []device.RiskEvent{{Name: "rasp_detection", Fields: []byte(`{"kind":"root"}`)}}); err != nil {
		t.Fatal(err)
	}
	if got, err := r.svc.Get(ctx, r.owner, d.ID); err != nil || got.AssuranceLevel != "AL1" {
		t.Errorf("level = %+v, %v", got, err)
	}
}
