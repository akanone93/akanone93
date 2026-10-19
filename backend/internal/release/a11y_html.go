// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package release

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"

	"github.com/nightCode42/plux3/backend/internal/compiler"
)

// a11yPage is the standalone page of an accessibility report: inline
// style only, no scripts and no external assets, so it opens anywhere and
// the same report always renders to the same bytes. html/template escapes
// every value for its context.
const a11yPage = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Plux accessibility report</title>
<style>
body{font-family:system-ui,sans-serif;margin:2rem auto;max-width:60rem;padding:0 1rem;color:#111;background:#fff}
table{border-collapse:collapse;width:100%;margin:1rem 0}
th,td{border:1px solid #767676;padding:.4rem .6rem;text-align:left;vertical-align:top}
th{background:#eee}
code{word-break:break-all}
.error{font-weight:bold}
</style>
</head>
<body>
<h1>Plux accessibility report</h1>
<p>{{len .Violations}} unwaived finding(s), {{len .Waivers}} waiver(s), {{.NotChecked}} node(s) whose colours could not be checked.</p>
<h2>Checks</h2>
<table>
<thead><tr><th>Check</th><th>Scope</th><th>Examined</th><th>Violations</th></tr></thead>
<tbody>
{{range .Checks}}<tr><td>{{.Name}}</td><td>{{.Scope}}</td><td>{{.Run}}</td><td>{{.Violations}}</td></tr>
{{end}}</tbody>
</table>
<h2>Violations</h2>
{{if .Violations}}<table>
<thead><tr><th>Severity</th><th>Code</th><th>Check</th><th>Location</th><th>Message</th></tr></thead>
<tbody>
{{range .Violations}}<tr><td class="{{.Severity}}">{{.Severity}}</td><td>{{.Code}}</td><td>{{.Check}}</td><td><code>{{.File}}{{.Path}}</code></td><td>{{.Message}}</td></tr>
{{end}}</tbody>
</table>
{{else}}<p>None.</p>
{{end}}<h2>Waivers</h2>
{{if .Waivers}}<table>
<thead><tr><th>Check</th><th>Location</th><th>Justification</th></tr></thead>
<tbody>
{{range .Waivers}}<tr><td>{{.Check}}</td><td><code>{{.File}}{{.Path}}</code></td><td>{{.Justification}}</td></tr>
{{end}}</tbody>
</table>
{{else}}<p>None.</p>
{{end}}</body>
</html>
`

// renderA11yHTML renders the stored JSON of an accessibility report as a
// standalone page (A11Y-008).
func renderA11yHTML(report []byte) ([]byte, error) {
	var r compiler.A11yReport
	if err := json.Unmarshal(report, &r); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	page, err := template.New("a11y").Parse(a11yPage)
	if err != nil {
		return nil, fmt.Errorf("template: %w", err)
	}
	var buf bytes.Buffer
	if err := page.Execute(&buf, r); err != nil {
		return nil, fmt.Errorf("render: %w", err)
	}
	return buf.Bytes(), nil
}
