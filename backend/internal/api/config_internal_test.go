// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"errors"
	"net/http"
	"testing"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// Verifies: DAT-095.
// A refusal of a gateway call carries the configuration epoch as response
// metadata, so the device learns it from the refusal too.
func TestRefusalCarriesTheEpoch(t *testing.T) {
	t.Parallel()
	err := Error(&withHeader{err: plxerr.New(plxerr.LiveConfigStale, "stale"), header: epochHeader(9)}, "i-1")
	var ce *connect.Error
	if !asConnect(err, &ce) || ce.Code() != connect.CodeFailedPrecondition {
		t.Fatalf("err = %v", err)
	}
	if got := ce.Meta().Get(HeaderConfigEpoch); got != "9" {
		t.Errorf("%s = %q", HeaderConfigEpoch, got)
	}
	var plain *connect.Error
	if !asConnect(Error(plxerr.New(plxerr.LiveConfigStale, "stale"), "i-2"), &plain) || plain.Meta().Get(HeaderConfigEpoch) != "" {
		t.Error("an epoch appeared from nowhere")
	}
}

func asConnect(err error, target **connect.Error) bool { return errors.As(err, target) }

// Verifies: DAT-096.
// Plux-Config-Version is a non-negative integer; absent means the device
// holds no live configuration.
func TestConfigVersionHeader(t *testing.T) {
	t.Parallel()
	for value, want := range map[string]int64{"0": 0, "42": 42} {
		got, err := configVersion(http.Header{headerConfigVersion: {value}})
		if err != nil || got == nil || *got != want {
			t.Errorf("%q: %v %v", value, got, err)
		}
	}
	if got, err := configVersion(http.Header{}); err != nil || got != nil {
		t.Errorf("absent: %v %v", got, err)
	}
	for _, bad := range []string{"-1", "x", "1.5", "99999999999999999999"} {
		if _, err := configVersion(http.Header{headerConfigVersion: {bad}}); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
}
