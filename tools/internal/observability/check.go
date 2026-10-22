// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package observability

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// RunbookURL is the prefix every runbook_url starts with; the rest is
// "<alert>.md" under docs/runbooks/alerts.
const RunbookURL = "https://github.com/nightCode42/plux3/blob/main/docs/runbooks/alerts/"

// runbookSections are the headings every alert runbook carries.
var runbookSections = []string{"## Symptom", "## Impact", "## Diagnosis", "## Remediation", "## Escalation"}

// Catalogue is what the checker knows about series.
type Catalogue struct {
	// Server holds the names the server registers, found in its sources.
	Server map[string]bool
	// External maps a series to the exporter that provides it.
	External map[string]string
	// Planned maps a series of Appendix G.1 not yet registered to its exporter.
	Planned map[string]string
}

var (
	serverName = regexp.MustCompile(`Name:\s*"(plux_[a-z0-9_]+)"`)
	histSuffix = regexp.MustCompile(`_(bucket|sum|count)$`)
)

// LoadCatalogue reads the series the server registers (every non-test Go file
// under backend/) and the external and planned lists in deploy/observability.
func LoadCatalogue(root string) (Catalogue, error) {
	c := Catalogue{Server: map[string]bool{}}
	err := filepath.WalkDir(filepath.Join(root, "backend"), func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		data, rerr := os.ReadFile(path) // #nosec G304,G122 -- walks the repository's own sources.
		if rerr != nil {
			return fmt.Errorf("read %s: %w", path, rerr)
		}
		for _, m := range serverName.FindAllSubmatch(data, -1) {
			c.Server[string(m[1])] = true
		}
		return nil
	})
	if err != nil {
		return c, fmt.Errorf("observability.LoadCatalogue: %w", err)
	}
	if c.External, err = readList(filepath.Join(root, "deploy/observability/external-metrics.txt")); err != nil {
		return c, err
	}
	if c.Planned, err = readList(filepath.Join(root, "deploy/observability/planned-metrics.txt")); err != nil {
		return c, err
	}
	return c, nil
}

// readList reads "<metric> <note>" lines; # starts a comment.
func readList(path string) (map[string]string, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- a repository path.
	if err != nil {
		return nil, fmt.Errorf("observability.readList: %w", err)
	}
	out := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, note, _ := strings.Cut(line, " ")
		out[name] = strings.TrimSpace(note)
	}
	return out, nil
}

// Known reports whether the series can exist: the server registers it (or
// the Go and process collectors it runs), an exporter is named for it, or it
// is planned. A histogram's _bucket, _sum and _count resolve to the histogram.
func (c Catalogue) Known(name string) bool {
	for _, n := range []string{name, histSuffix.ReplaceAllString(name, "")} {
		if c.Server[n] || c.External[n] != "" || c.Planned[n] != "" ||
			strings.HasPrefix(n, "go_") || strings.HasPrefix(n, "process_") {
			return true
		}
	}
	return false
}

// CheckRules returns the problems of the rules: names, severities, runbooks
// and the series every expression reads.
func CheckRules(root string, rules []Rule, cat Catalogue) []string {
	var problems []string
	recorded := map[string]bool{}
	seen := map[string]bool{}
	for _, r := range rules {
		if seen[r.Name] {
			problems = append(problems, "duplicate rule "+r.Name)
		}
		seen[r.Name] = true
		if !r.Alert {
			recorded[r.Name] = true
		}
	}
	alerts := map[string]bool{}
	for _, r := range rules {
		problems = append(problems, checkExpr(r, recorded, cat)...)
		if r.Alert {
			alerts[r.Name] = true
			problems = append(problems, checkAlert(root, r)...)
		}
	}
	return append(problems, orphanRunbooks(root, alerts)...)
}

// checkExpr checks the series one rule reads.
func checkExpr(r Rule, recorded map[string]bool, cat Catalogue) []string {
	var problems []string
	for _, m := range Metrics(r.Expr) {
		switch {
		case strings.Contains(m, ":"):
			if !recorded[m] {
				problems = append(problems, fmt.Sprintf("%s reads %s, which no recording rule defines", r.Name, m))
			}
		case !cat.Known(m):
			problems = append(problems, fmt.Sprintf("%s reads %s, which the server does not register and no exporter list names", r.Name, m))
		}
	}
	return problems
}

// checkAlert checks an alert's labels, annotations and runbook.
func checkAlert(root string, r Rule) []string {
	var problems []string
	if r.Labels["severity"] != "page" && r.Labels["severity"] != "ticket" {
		problems = append(problems, r.Name+": severity must be page or ticket")
	}
	if r.Annotations["summary"] == "" {
		problems = append(problems, r.Name+": no summary")
	}
	url := r.Annotations["runbook_url"]
	if url != RunbookURL+r.Name+".md" {
		return append(problems, fmt.Sprintf("%s: runbook_url must be %s%s.md, got %q", r.Name, RunbookURL, r.Name, url))
	}
	data, err := os.ReadFile(filepath.Join(root, "docs/runbooks/alerts", r.Name+".md")) // #nosec G304 -- a repository path.
	if err != nil {
		return append(problems, fmt.Sprintf("%s: runbook file missing: %v", r.Name, err))
	}
	for _, h := range runbookSections {
		if !bytes.Contains(data, []byte("\n"+h+"\n")) {
			problems = append(problems, fmt.Sprintf("%s: runbook lacks %q", r.Name, h))
		}
	}
	return problems
}

// orphanRunbooks reports runbook files no alert points at.
func orphanRunbooks(root string, alerts map[string]bool) []string {
	entries, err := os.ReadDir(filepath.Join(root, "docs/runbooks/alerts"))
	if err != nil {
		return []string{"docs/runbooks/alerts: " + err.Error()}
	}
	var problems []string
	for _, e := range entries {
		name := strings.TrimSuffix(e.Name(), ".md")
		if name != "README" && !alerts[name] {
			problems = append(problems, "runbook "+e.Name()+" belongs to no alert")
		}
	}
	return problems
}

// CheckDashboards returns the problems of the dashboards in dir: valid JSON,
// a unique uid and title, and only known series in their queries.
func CheckDashboards(dir string, rules []Rule, cat Catalogue) []string {
	files, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	sort.Strings(files)
	if len(files) == 0 {
		return []string{dir + ": no dashboards"}
	}
	recorded := map[string]bool{}
	for _, r := range rules {
		if !r.Alert {
			recorded[r.Name] = true
		}
	}
	uids := map[string]string{}
	var problems []string
	for _, f := range files {
		data, err := os.ReadFile(f) // #nosec G304 -- a repository path.
		if err != nil {
			problems = append(problems, err.Error())
			continue
		}
		var d struct {
			UID        string           `json:"uid"`
			Title      string           `json:"title"`
			Panels     []dashboardPanel `json:"panels"`
			Templating struct {
				List []dashboardVariable `json:"list"`
			} `json:"templating"`
		}
		if err := json.Unmarshal(data, &d); err != nil {
			problems = append(problems, fmt.Sprintf("%s: invalid JSON: %v", filepath.Base(f), err))
			continue
		}
		if d.UID == "" || d.Title == "" {
			problems = append(problems, filepath.Base(f)+": uid and title are required")
		}
		if other, dup := uids[d.UID]; dup {
			problems = append(problems, fmt.Sprintf("%s: uid %q is also used by %s", filepath.Base(f), d.UID, other))
		}
		uids[d.UID] = filepath.Base(f)
		problems = append(problems, checkDashboardQueries(filepath.Base(f), d.Panels, d.Templating.List, recorded, cat)...)
	}
	return problems
}

// dashboardTarget is one query of a dashboard panel: a PromQL expression or a SQL statement.
type dashboardTarget struct {
	Expr   string `json:"expr"`
	RawSQL string `json:"rawSql"`
}

// dashboardPanel is the part of a dashboard panel the checks read.
type dashboardPanel struct {
	Targets []dashboardTarget `json:"targets"`
}

// dashboardVariable is the part of a template variable the checks read; a
// textbox or custom variable has a string query, which holds no SQL.
type dashboardVariable struct {
	Query json.RawMessage `json:"query"`
}

// checkDashboardQueries checks the series of a dashboard's panels and the SQL
// of its panels and template variables.
func checkDashboardQueries(file string, panels []dashboardPanel, variables []dashboardVariable, recorded map[string]bool, cat Catalogue) []string {
	var problems []string
	for _, p := range panels {
		for _, t := range p.Targets {
			problems = append(problems, checkExpr(Rule{Name: file, Expr: t.Expr}, recorded, cat)...)
			if t.RawSQL != "" {
				problems = append(problems, CheckAnalyticsSQL(file, t.RawSQL)...)
			}
		}
	}
	for _, v := range variables {
		if sql := variableSQL(v.Query); sql != "" {
			problems = append(problems, CheckAnalyticsSQL(file, sql)...)
		}
	}
	return problems
}

// CheckPlanned reports a planned series the server already registers: the
// entry is stale.
func CheckPlanned(cat Catalogue) []string {
	var problems []string
	for name := range cat.Planned {
		if cat.Server[name] {
			problems = append(problems, name+" is registered by the server: remove it from planned-metrics.txt")
		}
	}
	sort.Strings(problems)
	return problems
}

// CheckCopies reports a file under deploy/observability that differs from, or
// is missing in, the copy the chart ships under deploy/helm/plux/files/observability.
func CheckCopies(root string) []string {
	var problems []string
	pairs := map[string]string{"rules/plux.rules.yml": "plux.rules.yml"}
	dashboards, _ := filepath.Glob(filepath.Join(root, "deploy/observability/dashboards/*.json"))
	for _, d := range dashboards {
		pairs["dashboards/"+filepath.Base(d)] = "dashboards/" + filepath.Base(d)
	}
	for src, dst := range pairs {
		a, errA := os.ReadFile(filepath.Join(root, "deploy/observability", src)) // #nosec G304 -- repository paths.
		b, errB := os.ReadFile(filepath.Join(root, "deploy/helm/plux/files/observability", dst))
		switch {
		case errA != nil || errB != nil:
			problems = append(problems, fmt.Sprintf("%s: chart copy missing (run deploy/observability/sync.sh)", src))
		case !bytes.Equal(a, b):
			problems = append(problems, fmt.Sprintf("%s: chart copy differs (run deploy/observability/sync.sh)", src))
		}
	}
	sort.Strings(problems)
	return problems
}

// Check runs every check against the repository at root.
func Check(root string) ([]string, error) {
	cat, err := LoadCatalogue(root)
	if err != nil {
		return nil, err
	}
	rules, err := ParseRules(filepath.Join(root, "deploy/observability/rules/plux.rules.yml"))
	if err != nil {
		return nil, err
	}
	if len(rules) == 0 {
		return nil, errors.New("observability.Check: the rules file holds no rules")
	}
	problems := CheckRules(root, rules, cat)
	problems = append(problems, CheckDashboards(filepath.Join(root, "deploy/observability/dashboards"), rules, cat)...)
	problems = append(problems, CheckPlanned(cat)...)
	problems = append(problems, CheckCopies(root)...)
	return problems, nil
}
