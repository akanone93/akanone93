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

// l10nPage is the standalone page of a localisation report: inline style
// only, no scripts and no external assets, so it opens anywhere and the same
// report always renders to the same bytes. html/template escapes every value
// for its context.
const l10nPage = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Plux localisation report</title>
<style>
body{font-family:system-ui,sans-serif;margin:2rem auto;max-width:60rem;padding:0 1rem;color:#111;background:#fff}
table{border-collapse:collapse;width:100%;margin:1rem 0}
th,td{border:1px solid #767676;padding:.4rem .6rem;text-align:left;vertical-align:top}
th{background:#eee}
td.n{text-align:right}
code{word-break:break-all}
</style>
</head>
<body>
<h1>Plux localisation report</h1>
<p>Default locale <code>{{.DefaultLocale}}</code>.{{if .RequireApproved}} Translations that are not approved are left out of the release and fall back to the default locale.{{end}}</p>
<h2>Coverage</h2>
<table>
<thead><tr><th>Locale</th><th>Plugin</th><th>Keys</th><th>Translated</th><th>Approved</th><th>Missing</th><th>Left out (not approved)</th></tr></thead>
<tbody>
{{range .Locales}}<tr><td><code>{{.Locale}}</code>{{if .Pseudo}} (pseudo){{end}}</td><td>{{if .Plugin}}{{.Plugin}}{{else}}whole app{{end}}</td><td class="n">{{.Keys}}</td><td class="n">{{.Translated}}</td><td class="n">{{.Approved}}</td><td class="n">{{.Missing}}</td><td class="n">{{.LeftOut}}</td></tr>
{{end}}</tbody>
</table>
<h2>Missing and left out</h2>
{{range .Locales}}{{if or .MissingKeys .LeftOutKeys}}<h3><code>{{.Locale}}</code></h3>
{{if .MissingKeys}}<p>Missing: {{range $i, $k := .MissingKeys}}{{if $i}}, {{end}}<code>{{$k}}</code>{{end}}</p>
{{end}}{{if .LeftOutKeys}}<p>Left out: {{range $i, $k := .LeftOutKeys}}{{if $i}}, {{end}}<code>{{$k}}</code>{{end}}</p>
{{end}}{{end}}{{end}}<h2>Unused keys</h2>
{{if .UnusedKeys}}<p>{{range $i, $k := .UnusedKeys}}{{if $i}}, {{end}}<code>{{$k}}</code>{{end}}</p>
{{else}}<p>None.</p>
{{end}}</body>
</html>
`

// renderL10nHTML renders the stored JSON of a localisation report as a
// standalone page (I18N-008).
func renderL10nHTML(report []byte) ([]byte, error) {
	var r compiler.L10nReport
	if err := json.Unmarshal(report, &r); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	page, err := template.New("l10n").Parse(l10nPage)
	if err != nil {
		return nil, fmt.Errorf("template: %w", err)
	}
	var buf bytes.Buffer
	if err := page.Execute(&buf, r); err != nil {
		return nil, fmt.Errorf("render: %w", err)
	}
	return buf.Bytes(), nil
}
