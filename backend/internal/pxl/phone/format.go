// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package phone

import (
	"strconv"
	"strings"
	"sync"
)

// numberFormat is one entry of the format table (schema/pxl/phone.md §2).
type numberFormat struct {
	pattern  *pattern
	leading  *pattern
	national string
	intl     string // "" when there is no international format
}

var loadFormats = sync.OnceValue(func() map[string][]numberFormat { return parseFormats(formatTable) })

// parseFormats reads the generated format table.
func parseFormats(text string) map[string][]numberFormat {
	out := map[string][]numberFormat{}
	for line := range strings.SplitSeq(strings.TrimSpace(text), "\n") {
		f := strings.Split(line, " ")
		if len(f) < 5 || (len(f)-1)%4 != 0 {
			continue
		}
		var list []numberFormat
		for i := 1; i < len(f); i += 4 {
			e := numberFormat{pattern: newPattern(f[i]), national: strings.ReplaceAll(f[i+2], "_", " ")}
			if f[i+1] != "~" {
				e.leading = newPattern(f[i+1])
			}
			switch f[i+3] {
			case "=":
				e.intl = e.national
			case "-":
			default:
				e.intl = strings.ReplaceAll(f[i+3], "_", " ")
			}
			list = append(list, e)
		}
		out[f[0]] = list
	}
	return out
}

// Format formats a valid phone number for display (format.phone,
// schema/pxl/phone.md §4): in national format when the number belongs to
// region, else in international format, "+" and the calling code first.
// An invalid number is returned unchanged.
func Format(number, region string) string {
	code, national, g := parse(number, region)
	if g == nil {
		return number
	}
	isNational := strings.EqualFold(g.id, region)
	body := national
	for _, f := range loadFormats()[load().byCode[code][0].id] {
		if f.leading != nil && f.leading.prefix(national) == nil {
			continue
		}
		m := f.pattern.fullFind(national)
		if m == nil {
			continue
		}
		layout := f.intl
		if isNational {
			layout = f.national
		}
		if layout != "" {
			body = expand(layout, m, national)
		}
		break
	}
	if isNational {
		return body
	}
	return "+" + strconv.Itoa(code) + " " + body
}
