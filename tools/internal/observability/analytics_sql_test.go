// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package observability

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// bound is the organisation binding every dashboard query starts with.
const bound = "SELECT set_config('plux.organization_id', '$organization', false);\n"

// TestCheckAnalyticsSQL_ANL_020 accepts a query that binds the organisation and
// reads only the rollups, CTEs and set-returning functions included, and flags
// a query that reads raw events, another table, or skips the binding.
//
// Verifies: ANL-020.
func TestCheckAnalyticsSQL_ANL_020(t *testing.T) {
	valid := bound + `WITH hist AS (
  SELECT r.device_class AS grp, u.idx - 1 AS k
  FROM analytics_rollups r
  CROSS JOIN LATERAL unnest(r.histogram) WITH ORDINALITY AS u(n, idx)
), cum AS (SELECT grp FROM hist)
SELECT grp FROM cum JOIN analytics_rollups r2 ON true;`
	if got := CheckAnalyticsSQL("a.json", valid); len(got) != 0 {
		t.Fatalf("a valid query: %v", got)
	}
	cases := map[string]struct {
		query string
		want  string
	}{
		"raw events":      {bound + "SELECT * FROM analytics_events;", "reads analytics_events"},
		"a joined table":  {bound + "SELECT 1 FROM analytics_rollups r JOIN organizations o ON true;", "reads organizations"},
		"a comma list":    {bound + "SELECT 1 FROM analytics_rollups r, analytics_events e;", "names analytics_events"},
		"no binding":      {"SELECT 1 FROM analytics_rollups;", "does not bind the organisation"},
		"a watermark":     {bound + "SELECT rolled_until FROM analytics_watermarks;", "names analytics_watermarks"},
		"a personal data": {bound + "SELECT email FROM users;", "reads users"},
	}
	for name, c := range cases {
		got := strings.Join(CheckAnalyticsSQL("a.json", c.query), "\n")
		if !strings.Contains(got, c.want) {
			t.Errorf("%s: problems lack %q:\n%s", name, c.want, got)
		}
	}
}

// TestCheckDashboardVariables_ANL_020 checks the SQL of a template variable as
// it checks the SQL of a panel, and ignores a textbox or custom variable.
//
// Verifies: ANL-020.
func TestCheckDashboardVariables_ANL_020(t *testing.T) {
	cat := Catalogue{Server: map[string]bool{}, External: map[string]string{}, Planned: map[string]string{}}
	dir := t.TempDir()
	write(t, dir, "textbox.json", `{"uid":"t","title":"T","templating":{"list":[`+
		`{"name":"organization","type":"textbox","query":"plain text"}]}}`)
	if got := CheckDashboards(dir, nil, cat); len(got) != 0 {
		t.Fatalf("a textbox variable is not SQL: %v", got)
	}
	write(t, dir, "query.json", `{"uid":"q","title":"Q","templating":{"list":[`+
		`{"name":"app","type":"query","query":{"rawSql":"SELECT 1 FROM analytics_events"}}]}}`)
	got := strings.Join(CheckDashboards(dir, nil, cat), "\n")
	if !strings.Contains(got, "query.json: a query reads analytics_events") {
		t.Errorf("a variable's raw SQL was not checked:\n%s", got)
	}
}

// TestAnalyticsDashboards_I18N_006 checks that the localisation dashboard
// reads the l10n_missing event, the event I18N-006 reports to telemetry.
//
// Verifies: I18N-006.
func TestAnalyticsDashboards_I18N_006(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repoRoot, "deploy/observability/dashboards/plux-analytics-localisation.json")) // #nosec G304 -- a repository path.
	if err != nil {
		t.Fatal(err)
	}
	var d struct {
		Panels []dashboardPanel `json:"panels"`
	}
	if err := json.Unmarshal(data, &d); err != nil {
		t.Fatal(err)
	}
	for _, p := range d.Panels {
		for _, target := range p.Targets {
			if strings.Contains(target.RawSQL, "'l10n_missing'") {
				return
			}
		}
	}
	t.Fatal("no panel of the localisation dashboard reads the l10n_missing event")
}
