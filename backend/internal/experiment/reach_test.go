// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package experiment

import (
	"slices"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/document"
	"github.com/nightCode42/plux3/backend/internal/schema"
)

const (
	securePage = `{"key":"checkout","security":{"secure":true},"root":{"type":"Variant","experiment":"hero","control":{"type":"Text"}}}`
	plainPage  = `{"key":"home","root":{"type":"Variant","experiment":"hero","control":{"type":"Text"}}}`
	otherPage  = `{"key":"other","security":{"secure":true},"root":{"type":"Variant","experiment":"banner","control":{"type":"Text"}}}`
	bindPage   = `{"key":"home","root":{"type":"Text","text":{"$expr":"experiment.hero.label"}}}`
	prefixPage = `{"key":"home","root":{"type":"Text","text":{"$expr":"myexperiment.hero + experiment.heroes.label"}}}`
	moneyApp   = `{"key":"bank","dataSources":[{"name":"pay","kind":"rest","type":"Payment","mock":null,"transaction":{"amount":"/amount"}}]}`
	component  = `{"key":"badge","root":{"type":"Text","text":{"$expr":"experiment.hero"}}}`
)

func files(kv ...string) []document.File {
	var out []document.File
	for i := 0; i < len(kv); i += 2 {
		out = append(out, document.File{Path: kv[i], Content: []byte(kv[i+1])})
	}
	return out
}

// Verifies: ABT-007, GOV-020.
// The risk of an experiment follows where the draft shows it: a secure page,
// a plugin or app with a transaction mapping, and nothing for a plain page or
// an experiment the page does not show.
func TestReachRisk_ABT_007(t *testing.T) {
	t.Parallel()
	secure, money := schema.PolicyRiskTouchesSecurePage, schema.PolicyRiskTouchesMoneyFlow
	cases := []struct {
		name  string
		files []document.File
		want  []schema.PolicyRisk
	}{
		{"a secure page showing the experiment", files("plugins/shop/pages/checkout.page.json", securePage), []schema.PolicyRisk{secure}},
		{"a plain page", files("plugins/shop/pages/home.page.json", plainPage), nil},
		{"a secure page showing another experiment", files("plugins/shop/pages/other.page.json", otherPage), nil},
		{"a binding on a plain page", files("plugins/shop/pages/home.page.json", bindPage), nil},
		{"a binding that only resembles the key", files("plugins/shop/pages/home.page.json", prefixPage, "plugins/shop/pages/checkout.page.json", securePage), []schema.PolicyRisk{secure}},
		{"a page of an app that declares a transaction", files("app.json", moneyApp, "plugins/shop/pages/home.page.json", plainPage), []schema.PolicyRisk{money}},
		{
			"a page of a plugin that declares a transaction",
			files("plugins/shop/plugin.json", moneyApp, "plugins/shop/pages/home.page.json", plainPage, "plugins/chat/pages/home.page.json", plainPage),
			[]schema.PolicyRisk{money},
		},
		{"a money plugin the experiment does not reach", files("plugins/shop/plugin.json", moneyApp, "plugins/chat/pages/home.page.json", plainPage), nil},
		{
			"a component of a plugin with a secure page",
			files("plugins/shop/components/badge.component.json", component, "plugins/shop/pages/checkout.page.json", otherPage),
			[]schema.PolicyRisk{secure},
		},
		{
			"a component of another plugin",
			files("plugins/chat/components/badge.component.json", component, "plugins/shop/pages/checkout.page.json", otherPage),
			nil,
		},
		{
			"an app-level component reaches every plugin",
			files("components/badge.component.json", component, "plugins/shop/plugin.json", moneyApp, "plugins/shop/pages/checkout.page.json", securePage),
			[]schema.PolicyRisk{money, secure},
		},
		{"an undecodable page", files("plugins/shop/pages/home.page.json", "{"), nil},
		{"a file that is no document", files("README.md", "experiment.hero"), nil},
	}
	for _, c := range cases {
		got := reachRisk(c.files, "hero")
		if !slices.Equal(got, c.want) {
			t.Errorf("%s: risk = %v, want %v", c.name, got, c.want)
		}
	}
}
