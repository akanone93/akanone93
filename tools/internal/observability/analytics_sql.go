// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package observability

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// analyticsTable is the only table an analytics dashboard may read (ANL-020).
// The rollups carry no personal data and row-level security binds them to the
// organisation; raw events are never queried from a dashboard.
const analyticsTable = "analytics_rollups"

var (
	// sqlRelation matches the relation a FROM or JOIN names. A name followed by
	// a parenthesis is a function, such as unnest, and is not a relation.
	sqlRelation = regexp.MustCompile(`(?i)\b(?:from|join)\s+(?:lateral\s+)?([a-z_][a-z0-9_]*)\s*(\()?`)
	// sqlCTE matches the name of a common table expression.
	sqlCTE = regexp.MustCompile(`(?i)(?:\bwith|,)\s*([a-z_][a-z0-9_]*)\s+as\s*(?:materialized\s*)?\(`)
	// sqlAnalyticsTable matches every analytics table name in a query.
	sqlAnalyticsTable = regexp.MustCompile(`(?i)\banalytics_[a-z_]+`)
	// sqlBindsOrganisation matches the call that binds row-level security to the organisation.
	sqlBindsOrganisation = regexp.MustCompile(`(?i)set_config\s*\(\s*'plux\.organization_id'\s*,\s*'\$organization'`)
)

// CheckAnalyticsSQL returns the problems of one dashboard query: it must bind
// row-level security to the organisation and read only the rollups (ANL-020).
func CheckAnalyticsSQL(file, query string) []string {
	var problems []string
	if !sqlBindsOrganisation.MatchString(query) {
		problems = append(problems, fmt.Sprintf("%s: a query does not bind the organisation with set_config('plux.organization_id', '$organization', ...)", file))
	}
	ctes := map[string]bool{}
	for _, m := range sqlCTE.FindAllStringSubmatch(query, -1) {
		ctes[strings.ToLower(m[1])] = true
	}
	for _, m := range sqlRelation.FindAllStringSubmatch(query, -1) {
		name := strings.ToLower(m[1])
		if m[2] == "(" || name == analyticsTable || ctes[name] {
			continue
		}
		problems = append(problems, fmt.Sprintf("%s: a query reads %s, but dashboards may read only %s", file, name, analyticsTable))
	}
	for _, name := range sqlAnalyticsTable.FindAllString(query, -1) {
		if !strings.EqualFold(name, analyticsTable) {
			problems = append(problems, fmt.Sprintf("%s: a query names %s, but dashboards may read only %s", file, name, analyticsTable))
		}
	}
	return problems
}

// variableSQL returns the SQL of a template variable's query, or "" when the
// query is not a SQL query (a textbox or custom variable holds a string).
func variableSQL(query json.RawMessage) string {
	var q struct {
		RawSQL string `json:"rawSql"`
	}
	if err := json.Unmarshal(query, &q); err != nil {
		return ""
	}
	return q.RawSQL
}
