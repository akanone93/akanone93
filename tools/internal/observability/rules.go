// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package observability

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// Rule is one recording rule or alert of the rules file.
type Rule struct {
	// Name is the alert or recorded series name.
	Name string
	// Alert is true for an alert, false for a recording rule.
	Alert bool
	// Expr is the PromQL expression, lines joined by a newline.
	Expr string
	// Labels are the alert's labels.
	Labels map[string]string
	// Annotations are the alert's annotations.
	Annotations map[string]string
}

var (
	ruleStart = regexp.MustCompile(`^ {6}- (alert|record): (\S+)$`)
	ruleField = regexp.MustCompile(`^ {8}([a-z_]+):(?: (.*))?$`)
	subField  = regexp.MustCompile(`^ {10}([a-z_]+): (.*)$`)
)

// parser accumulates the rule being read.
type parser struct {
	path    string
	rules   []Rule
	cur     *Rule
	section string
	inExpr  bool
	expr    []string
}

// ParseRules reads a rules file in the layout the package documents.
func ParseRules(path string) ([]Rule, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- a repository path chosen by the caller.
	if err != nil {
		return nil, fmt.Errorf("observability.ParseRules: %w", err)
	}
	p := &parser{path: path}
	for i, line := range strings.Split(string(data), "\n") {
		if err := p.line(i+1, line); err != nil {
			return nil, fmt.Errorf("observability.ParseRules: %w", err)
		}
	}
	p.flush()
	return p.rules, nil
}

// flush stores the rule being read.
func (p *parser) flush() {
	if p.cur != nil {
		p.cur.Expr = strings.Join(p.expr, "\n")
		p.rules = append(p.rules, *p.cur)
	}
	p.cur, p.expr, p.section, p.inExpr = nil, nil, "", false
}

// line reads one line of the file.
func (p *parser) line(n int, line string) error {
	if m := ruleStart.FindStringSubmatch(line); m != nil {
		p.flush()
		p.cur = &Rule{Name: m[2], Alert: m[1] == "alert", Labels: map[string]string{}, Annotations: map[string]string{}}
		return nil
	}
	if strings.HasPrefix(line, "  - name:") {
		p.flush()
		return nil
	}
	if p.cur == nil {
		return nil
	}
	if p.inExpr && strings.HasPrefix(line, "          ") {
		p.expr = append(p.expr, strings.TrimPrefix(line, "          "))
		return nil
	}
	if m := ruleField.FindStringSubmatch(line); m != nil {
		p.inExpr, p.section = false, m[1]
		if m[1] != "expr" {
			return nil
		}
		if m[2] != "|" {
			return fmt.Errorf("%s:%d: expr must be a | block", p.path, n)
		}
		p.inExpr = true
		return nil
	}
	if m := subField.FindStringSubmatch(line); m != nil {
		switch p.section {
		case "labels":
			p.cur.Labels[m[1]] = strings.Trim(m[2], `"`)
		case "annotations":
			p.cur.Annotations[m[1]] = strings.Trim(m[2], `"`)
		}
		return nil
	}
	if strings.TrimSpace(line) != "" {
		return fmt.Errorf("%s:%d: unsupported layout %q", p.path, n, line)
	}
	return nil
}

var (
	literal    = regexp.MustCompile(`"(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*'`)
	selector   = regexp.MustCompile(`\{[^}]*\}`)
	grouping   = regexp.MustCompile(`\b(?:by|without|on|ignoring|group_left|group_right)\s*\([^)]*\)`)
	rangeSel   = regexp.MustCompile(`\[[^\]]*\]`)
	offset     = regexp.MustCompile(`\boffset\s+\S+`)
	identifier = regexp.MustCompile(`(?:^|[^\w:.])([a-zA-Z_:][\w:]*)`)
	keywords   = map[string]bool{"and": true, "or": true, "unless": true, "bool": true, "inf": true, "nan": true, "by": true, "without": true}
)

// Metrics returns the series names a PromQL expression reads, in order of
// appearance without repeats. Function names, keywords, label names and
// string contents are not series.
func Metrics(expr string) []string {
	s := literal.ReplaceAllString(expr, `""`)
	s = selector.ReplaceAllString(s, "")
	s = grouping.ReplaceAllString(s, "")
	s = rangeSel.ReplaceAllString(s, "")
	s = offset.ReplaceAllString(s, "")
	seen := map[string]bool{}
	var out []string
	for _, m := range identifier.FindAllStringSubmatchIndex(s, -1) {
		name := s[m[2]:m[3]]
		if keywords[name] || seen[name] || strings.HasPrefix(strings.TrimLeft(s[m[3]:], " \t"), "(") {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}
