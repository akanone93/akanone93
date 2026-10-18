// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package pxl

import (
	"slices"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// Verifies: I18N-002.
// A program records the literal currencies it names and whether a currency
// is left open.
func TestProgramCurrencies(t *testing.T) {
	env, err := NewEnv(EnvSpec{Roots: map[string]string{"cur": "string", "price": "money", "n": "list<money>"}})
	if err != nil {
		t.Fatal(err)
	}
	for src, want := range map[string]Currencies{
		`1 + 1`:                {},
		`money("1.50", "CHF")`: {Codes: []string{"CHF"}},
		`add(money("1", "EUR"), money("2", "EUR"))`: {Codes: []string{"EUR"}},
		`format.money(money("1", "JPY"))`:           {Codes: []string{"JPY"}},
		`money("1", cur)`:                           {Dynamic: true},
		`price`:                                     {Dynamic: true},
		`add(price, money("1", "USD"))`:             {Codes: []string{"USD"}, Dynamic: true},
		`n.map(x, x)`:                               {Dynamic: true},
	} {
		p, _, diags := Compile(src, env, DefaultOptions(), plxerr.Location{})
		if p == nil {
			t.Fatalf("%s: %v", src, diags)
		}
		if !slices.Equal(p.Currencies.Codes, want.Codes) || p.Currencies.Dynamic != want.Dynamic {
			t.Errorf("%s: %+v, want %+v", src, p.Currencies, want)
		}
	}
}
