// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package release

import (
	"testing"

	"github.com/nightCode42/plux3/backend/internal/controldoc"
)

// A break-glass kill switch is named for its review with a verb, so the
// switch's state does not stand next to the channel's "on" ("… on on
// production").
func TestSwitchSubjectReadsAsASentence(t *testing.T) {
	t.Parallel()
	k := controldoc.KillSwitch{Scope: controldoc.ScopeFunction, Target: "pay.simulate"}
	ref := ChannelRef{AppID: "a", EnvironmentID: "e", ChannelKey: "production"}
	for verb, want := range map[string]string{
		"set":     "kill switch function pay.simulate set on production",
		"cleared": "kill switch function pay.simulate cleared on production",
	} {
		if got := switchSubject(ref, k, verb); got != want {
			t.Errorf("switchSubject(%q) = %q, want %q", verb, got, want)
		}
	}
}
