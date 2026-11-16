// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// tokenAlias is a W3C design-token alias, `{color.primary}`: the whole value
// is the path of another token in braces (THM-001).
var tokenAlias = regexp.MustCompile(`^\{([a-zA-Z0-9][a-zA-Z0-9-]*(?:\.[a-zA-Z0-9][a-zA-Z0-9-]*)*)\}$`)

// tokenAliases reports every alias of the theme in file that leads to no
// token or loops deeper than maxAliasDepth, in a token's light or dark value
// and inside a composite value. The device follows aliases to the end
// (`tokens.dart` followAliases) and reads such an alias as no token, so a
// prop bound to it would silently take its default; the compiler says so
// instead (PLX-1102).
func (u *unit) tokenAliases(file string) {
	if !u.inFocus(file) {
		return
	}
	for _, path := range sortedKeys(u.tokens) {
		tok := u.tokens[path]
		base := plxerr.Pointer("tokens") + plxerr.Pointer(strings.Split(path, ".")...)
		u.aliasesIn(file, tok.light, base+plxerr.Pointer("$value"))
		u.aliasesIn(file, tok.dark, base+plxerr.Pointer("$extensions", "dev.plux.dark"))
	}
}

// aliasesIn checks the aliases of one value, the composite's fields too.
func (u *unit) aliasesIn(file string, raw json.RawMessage, ptr string) {
	if len(raw) == 0 {
		return
	}
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return
	}
	u.aliasesOf(file, v, ptr)
}

func (u *unit) aliasesOf(file string, v any, ptr string) {
	switch x := v.(type) {
	case string:
		m := tokenAlias.FindStringSubmatch(x)
		if m == nil {
			return
		}
		if problem := u.aliasEnd(m[1], 1); problem != "" {
			u.report(plxerr.UnresolvedReference, file, ptr, "the alias %s %s", x, problem)
		}
	case map[string]any:
		for _, k := range sortedKeys(x) {
			u.aliasesOf(file, x[k], ptr+plxerr.Pointer(k))
		}
	case []any:
		for i, e := range x {
			u.aliasesOf(file, e, ptr+plxerr.Pointer(strconv.Itoa(i)))
		}
	}
}

// aliasEnd follows an alias to a token that is not one, through light
// values (dark values alias the same way); it returns what is wrong, or "".
func (u *unit) aliasEnd(path string, depth int) string {
	if depth > maxAliasDepth {
		return "loops or is more than 8 aliases deep"
	}
	tok, ok := u.tokens[path]
	if !ok {
		return "names no token"
	}
	var s string
	if json.Unmarshal(tok.light, &s) != nil {
		return ""
	}
	if m := tokenAlias.FindStringSubmatch(s); m != nil {
		return u.aliasEnd(m[1], depth+1)
	}
	return ""
}
