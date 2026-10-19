// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package observability

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// repoRoot is the repository root relative to this package.
const repoRoot = "../../.."

// write creates a file under root, creating parent directories.
func write(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestMetrics_OBS_004 reads series names out of PromQL and leaves functions,
// keywords, label names, strings and numbers alone.
//
// Verifies: OBS-004.
func TestMetrics_OBS_004(t *testing.T) {
	tests := []struct {
		name string
		expr string
		want []string
	}{
		{"selector and range", `sum(rate(plux_rpc_requests_total{code=~"internal|unavailable"}[5m]))`, []string{"plux_rpc_requests_total"}},
		{"grouping labels are not series", `histogram_quantile(0.95, sum by (le, kind)(rate(plux_delta_bytes_bucket[5m])))`, []string{"plux_delta_bytes_bucket"}},
		{"ratio", `a_total / on (x) group_left (y) b_total`, []string{"a_total", "b_total"}},
		{"recording rule and number", `plux:manifest_error_ratio:rate1h > (14.4 * 0.0005)`, []string{"plux:manifest_error_ratio:rate1h"}},
		{"strings", `up{job=~"plux_not_a_metric"} == 0`, []string{"up"}},
		{"keywords and time", "time() - max(x_ts) > 300\nand\ny_up == 1", []string{"x_ts", "y_up"}},
		{"offset", `rate(a_total[5m] offset 1h)`, []string{"a_total"}},
		{"only a number", `1`, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Metrics(tt.expr); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Metrics(%q) = %v, want %v", tt.expr, got, tt.want)
			}
		})
	}
}

// TestParseRules_OBS_004 reads the layout and refuses what it cannot read.
//
// Verifies: OBS-004.
func TestParseRules_OBS_004(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "ok.yml", `groups:
  - name: g
    interval: 1m
    rules:
      - record: a:b:rate5m
        expr: |
          sum(rate(x_total[5m]))
      - alert: A
        expr: |
          a:b:rate5m > 1
          and
          up == 0
        for: 5m
        labels:
          severity: page
        annotations:
          summary: "s: with colon"
          runbook_url: https://example.test/A.md
`)
	rules, err := ParseRules(filepath.Join(dir, "ok.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 2 || rules[0].Alert || !rules[1].Alert {
		t.Fatalf("rules = %+v", rules)
	}
	if rules[1].Expr != "a:b:rate5m > 1\nand\nup == 0" || rules[1].Labels["severity"] != "page" ||
		rules[1].Annotations["summary"] != "s: with colon" {
		t.Errorf("alert = %+v", rules[1])
	}
	write(t, dir, "inline.yml", "groups:\n  - name: g\n    rules:\n      - alert: A\n        expr: up == 0\n")
	if _, err := ParseRules(filepath.Join(dir, "inline.yml")); err == nil || !strings.Contains(err.Error(), "| block") {
		t.Errorf("an inline expr: err = %v, want a refusal", err)
	}
	write(t, dir, "odd.yml", "groups:\n  - name: g\n    rules:\n      - alert: A\n        expr: |\n          up\n   odd: line\n")
	if _, err := ParseRules(filepath.Join(dir, "odd.yml")); err == nil {
		t.Error("an unreadable layout was accepted")
	}
}

// fixture writes a small repository: one alert with a runbook.
func fixture(t *testing.T, runbook string) string {
	t.Helper()
	root := t.TempDir()
	write(t, root, "docs/runbooks/alerts/A.md", runbook)
	return root
}

const goodRunbook = "# A\n\n## Symptom\n\nx\n\n## Impact\n\nx\n\n## Diagnosis\n\nx\n\n## Remediation\n\nx\n\n## Escalation\n\nx\n"

// TestCheckRules_OBS_004 refuses an alert without a runbook, a metric nobody
// exports, a recording rule that does not exist and an orphan runbook.
//
// Verifies: OBS-004.
func TestCheckRules_OBS_004(t *testing.T) {
	cat := Catalogue{Server: map[string]bool{"plux_x_total": true}, External: map[string]string{"up": "Prometheus"}, Planned: map[string]string{}}
	alert := func(expr, url string) Rule {
		return Rule{
			Name: "A", Alert: true, Expr: expr, Labels: map[string]string{"severity": "page"},
			Annotations: map[string]string{"summary": "s", "runbook_url": url},
		}
	}
	good := RunbookURL + "A.md"
	tests := []struct {
		name    string
		runbook string
		rule    Rule
		want    string
	}{
		{"fine", goodRunbook, alert("sum(rate(plux_x_total[5m])) > 1 and up == 0", good), ""},
		{"histogram suffix", goodRunbook, alert("plux_x_total_bucket > 1", good), ""},
		{"unknown metric", goodRunbook, alert("plux_typo_total > 1", good), "plux_typo_total"},
		{"unknown recording rule", goodRunbook, alert("plux:nope:rate5m > 1", good), "no recording rule"},
		{"wrong url", goodRunbook, alert("up == 0", "https://example.test/A.md"), "runbook_url must be"},
		{"missing section", strings.Replace(goodRunbook, "## Impact", "## Effect", 1), alert("up == 0", good), `lacks "## Impact"`},
		{"missing file", "", alert("up == 0", RunbookURL+"A.md"), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			if tt.name != "missing file" {
				write(t, root, "docs/runbooks/alerts/A.md", tt.runbook)
			} else {
				write(t, root, "docs/runbooks/alerts/README.md", "x")
			}
			got := strings.Join(CheckRules(root, []Rule{tt.rule}, cat), "\n")
			if tt.name == "missing file" {
				tt.want = "runbook file missing"
			}
			if tt.want == "" && got != "" || tt.want != "" && !strings.Contains(got, tt.want) {
				t.Errorf("problems = %q, want %q", got, tt.want)
			}
		})
	}
	root := fixture(t, goodRunbook)
	write(t, root, "docs/runbooks/alerts/Orphan.md", goodRunbook)
	if got := strings.Join(CheckRules(root, []Rule{alert("up == 0", good)}, cat), "\n"); !strings.Contains(got, "Orphan.md belongs to no alert") {
		t.Errorf("orphan: problems = %q", got)
	}
}

// TestCheckDashboards_OBS_005 refuses invalid JSON, a repeated uid and a
// query on a series nobody exports.
//
// Verifies: OBS-005.
func TestCheckDashboards_OBS_005(t *testing.T) {
	cat := Catalogue{Server: map[string]bool{"plux_x_total": true}, External: map[string]string{}, Planned: map[string]string{}}
	dir := t.TempDir()
	write(t, dir, "a.json", `{"uid":"one","title":"A","panels":[{"targets":[{"expr":"sum by (job)(rate(plux_x_total{job=~\"$job\"}[5m]))"}]}]}`)
	if got := CheckDashboards(dir, nil, cat); len(got) != 0 {
		t.Fatalf("a valid dashboard: %v", got)
	}
	write(t, dir, "b.json", `{"uid":"one","title":"B","panels":[{"targets":[{"expr":"plux_typo_total"}]}]}`)
	write(t, dir, "c.json", `{not json`)
	got := strings.Join(CheckDashboards(dir, nil, cat), "\n")
	for _, want := range []string{`uid "one" is also used by a.json`, "plux_typo_total", "c.json: invalid JSON"} {
		if !strings.Contains(got, want) {
			t.Errorf("problems lack %q:\n%s", want, got)
		}
	}
	if got := CheckDashboards(t.TempDir(), nil, cat); len(got) != 1 {
		t.Errorf("an empty directory: %v", got)
	}
}

// TestCheckPlanned_OBS_004 flags a planned series the server now registers.
//
// Verifies: OBS-004.
func TestCheckPlanned_OBS_004(t *testing.T) {
	cat := Catalogue{Server: map[string]bool{"plux_a": true}, Planned: map[string]string{"plux_a": "x", "plux_b": "y"}}
	got := CheckPlanned(cat)
	if len(got) != 1 || !strings.Contains(got[0], "plux_a") {
		t.Errorf("CheckPlanned() = %v", got)
	}
}

// TestRepository_OBS_004 checks the real rules, runbooks and dashboards of
// the repository, and that the objectives and the backup alerts are there.
//
// Verifies: OBS-004, OBS-005, DEP-010.
func TestRepository_OBS_004(t *testing.T) {
	problems, err := Check(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
	rules, err := ParseRules(filepath.Join(repoRoot, "deploy/observability/rules/plux.rules.yml"))
	if err != nil {
		t.Fatal(err)
	}
	have := map[string]bool{}
	for _, r := range rules {
		have[r.Name] = true
	}
	for _, name := range []string{
		"plux:manifest_error_ratio:rate5m", "plux:function_error_ratio:rate1h", "plux:publish_error_ratio:rate6h", "plux:control_propagation_seconds:p95_30m",
		"PluxManifestBurnFast", "PluxManifestBurnSlow", "PluxFunctionBurnFast", "PluxPublishBurnFast", "PluxControlPropagationSlow",
		"PluxAttestationUnavailable", "PluxReplayCacheDegraded", "PluxGatewayUpstreamFailing", "PluxFunctionEgressRefusals", "PluxKeyCompromiseSignals",
		"PluxWalArchiveStalled", "PluxBackupTooOld", "PluxConsistencyMarkerStale",
	} {
		if !have[name] {
			t.Errorf("rule %s is missing", name)
		}
	}
}
