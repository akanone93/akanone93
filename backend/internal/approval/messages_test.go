// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package approval

import (
	"testing"

	"github.com/nightCode42/plux3/backend/internal/schema"
)

// A refusal names the policies a change must satisfy as a sentence does, not
// as a Go slice ("policy [a b]").
func TestPolicyListReadsAsASentence(t *testing.T) {
	of := func(keys ...string) []Policy {
		out := make([]Policy, len(keys))
		for i, k := range keys {
			out[i] = Policy{Doc: schema.PolicyDocument{Key: k}}
		}
		return out
	}
	for _, c := range []struct {
		keys []string
		want string
	}{
		{[]string{"prod-four-eyes"}, "policy prod-four-eyes"},
		{[]string{"prod-four-eyes", "bank-strict"}, "policies prod-four-eyes and bank-strict"},
		{[]string{"a", "b", "c"}, "policies a, b and c"},
	} {
		if got := policyList(of(c.keys...)); got != c.want {
			t.Errorf("policyList(%v) = %q, want %q", c.keys, got, c.want)
		}
	}
}
