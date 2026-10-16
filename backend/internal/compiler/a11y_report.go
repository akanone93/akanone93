// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	"sort"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// A11yReport is the compiler's part of a release's accessibility report
// (A11Y-008): every check run, every unwaived violation and every waiver.
// The server adds the author and the release; the JSON form is described
// by schema/json/a11y-report.schema.json. It is deterministic: the same
// project always yields the same report.
type A11yReport struct {
	// Checks has one entry per check, sorted by name.
	Checks []CheckSummary `json:"checks"`
	// Violations are the unwaived findings, sorted by file, path, check.
	Violations []Violation `json:"violations"`
	// Waivers are the valid waivers the documents declare, sorted by file,
	// path.
	Waivers []Waiver `json:"waivers"`
	// NotChecked counts the nodes whose colours the compiler could not
	// resolve (PLX-1409).
	NotChecked int `json:"notChecked"`
}

// CheckSummary says what one check examined.
type CheckSummary struct {
	// Name is the check, as an a11yWaivers key.
	Name string `json:"name"`
	// Scope is what the check examines: "node", "group" or "page".
	Scope string `json:"scope"`
	// Run is how many subjects the check examined.
	Run int `json:"run"`
	// Violations is how many of them it reported, waived ones excluded.
	Violations int `json:"violations"`
}

// Violation is an unwaived finding.
type Violation struct {
	Check string `json:"check"`
	// Code is the diagnostic code, such as "PLX-1403".
	Code string `json:"code"`
	// Severity is "error", "warning" or "info".
	Severity string `json:"severity"`
	File     string `json:"file"`
	// Path is the JSON Pointer of the node in File.
	Path    string `json:"path"`
	Message string `json:"message"`
}

// Waiver is a waived check with its justification.
type Waiver struct {
	Check string `json:"check"`
	File  string `json:"file"`
	// Path is the JSON Pointer of the waiver in File.
	Path          string `json:"path"`
	Justification string `json:"justification"`
}

// a11yChecks lists the checks in name order with their scopes. The names
// are the keys of a11yWaivers (the a11yCheck enumeration of the schema).
var a11yChecks = []struct{ name, scope string }{
	{checkAccessibleName, "node"},
	{checkContrast, "node"},
	{checkFixedHeightText, "node"},
	{checkFocusOrder, "group"},
	{checkHeadingLevel, "page"},
	{checkImageAlternative, "node"},
	{checkLiveRegion, "node"},
	{checkTouchTarget, "node"},
}

// Check names, the a11yCheck enumeration of the document schema.
const (
	checkAccessibleName   = "accessibleName"
	checkImageAlternative = "imageAlternative"
	checkContrast         = "contrast"
	checkTouchTarget      = "touchTarget"
	checkFixedHeightText  = "fixedHeightText"
	checkFocusOrder       = "focusOrder"
	checkHeadingLevel     = "headingLevel"
	checkLiveRegion       = "liveRegion"
)

// a11yState collects what the accessibility checks find during the policy
// stage.
type a11yState struct {
	ran        bool
	run        map[string]int
	violations []Violation
	waivers    []Waiver
	notChecked int
}

// examined counts one subject of a check.
func (s *a11yState) examined(check string) {
	if s.run == nil {
		s.run = map[string]int{}
	}
	s.run[check]++
}

// report builds the sorted report; nil when the policy stage did not run.
func (s *a11yState) report() *A11yReport {
	if !s.ran {
		return nil
	}
	r := &A11yReport{
		Checks:     make([]CheckSummary, 0, len(a11yChecks)),
		Violations: append([]Violation{}, s.violations...),
		Waivers:    append([]Waiver{}, s.waivers...),
		NotChecked: s.notChecked,
	}
	sort.SliceStable(r.Violations, func(i, j int) bool {
		a, b := r.Violations[i], r.Violations[j]
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.Check != b.Check {
			return a.Check < b.Check
		}
		return a.Message < b.Message
	})
	sort.SliceStable(r.Waivers, func(i, j int) bool {
		a, b := r.Waivers[i], r.Waivers[j]
		if a.File != b.File {
			return a.File < b.File
		}
		return a.Path < b.Path
	})
	count := map[string]int{}
	for _, v := range r.Violations {
		count[v.Check]++
	}
	for _, c := range a11yChecks {
		r.Checks = append(r.Checks, CheckSummary{Name: c.name, Scope: c.scope, Run: s.run[c.name], Violations: count[c.name]})
	}
	return r
}

// violation records an unwaived finding.
func (s *a11yState) violation(check string, d plxerr.Diagnostic) {
	s.violations = append(s.violations, Violation{
		Check: check, Code: d.Code.String(), Severity: d.Severity.String(),
		File: d.File, Path: d.Path, Message: d.Message,
	})
}
