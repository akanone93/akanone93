// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

// Command studiobounds checks Studio's import boundaries
// (docs/engineering/studio-development.md §2.1, layering L-7):
//
//	studiobounds ../studio
//
// It scans the .ts and .tsx files under apps/*/src and packages/*/src,
// skipping node_modules, and reports each violating import as
// path:line: rule: import. Exit 1 when there is any, 2 on a usage or I/O
// error. The rules:
//
//	packages-no-apps    packages never import @plux/web, @plux/bff or a
//	                    relative path into apps
//	pure-no-ui          @plux/canvas and @plux/layout never import react,
//	                    react-dom, @plux/ui or @base-ui/* (also its former name @base-ui-components/*)
//	bff-no-ui           apps/bff never imports react, @plux/ui,
//	                    @plux/canvas or @plux/layout
//	no-direct-storage   nothing imports pg, postgres, ioredis or @aws-sdk/*
package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"regexp"
	"slices"
	"strings"
)

// importRE matches `from "x"`, `import "x"` and `import("x")`.
var importRE = regexp.MustCompile(`(?:\bfrom\s*|\bimport\s*\(?\s*)["']([^"']+)["']`)

type violation struct {
	file string
	line int
	rule string
	spec string
}

func (v violation) String() string {
	return fmt.Sprintf("%s:%d: %s: %s", v.file, v.line, v.rule, v.spec)
}

// matches reports whether spec is name, a subpath of name, or, when name
// ends in "/", anything under that scope.
func matches(spec, name string) bool {
	if strings.HasSuffix(name, "/*") {
		return strings.HasPrefix(spec, strings.TrimSuffix(name, "*"))
	}
	return spec == name || strings.HasPrefix(spec, name+"/")
}

func anyMatch(spec string, names ...string) bool {
	return slices.ContainsFunc(names, func(n string) bool { return matches(spec, n) })
}

// check returns the rules that import spec breaks in the workspace
// directory dir ("apps/web", "packages/canvas"), for a file at depth below
// dir/src given by rel.
func check(dir, rel, spec string) []string {
	var rules []string
	kind, name, _ := strings.Cut(dir, "/")
	if kind == "packages" {
		escapes := false
		if strings.HasPrefix(spec, ".") {
			target := path.Join(dir, "src", path.Dir(rel), spec)
			escapes = strings.HasPrefix(target, "apps/") || !strings.HasPrefix(target, dir+"/")
		}
		if anyMatch(spec, "@plux/web", "@plux/bff") || escapes {
			rules = append(rules, "packages-no-apps")
		}
	}
	if kind == "packages" && (name == "canvas" || name == "layout") &&
		anyMatch(spec, "react", "react-dom", "@plux/ui", "@base-ui/*", "@base-ui-components/*") {
		rules = append(rules, "pure-no-ui")
	}
	if dir == "apps/bff" && anyMatch(spec, "react", "@plux/ui", "@plux/canvas", "@plux/layout") {
		rules = append(rules, "bff-no-ui")
	}
	if anyMatch(spec, "pg", "postgres", "ioredis", "@aws-sdk/*") {
		rules = append(rules, "no-direct-storage")
	}
	return rules
}

// scanFile returns the violations of one source file of workspace dir.
func scanFile(fsys fs.FS, dir, p string) ([]violation, error) {
	data, err := fs.ReadFile(fsys, p)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", p, err)
	}
	rel := strings.TrimPrefix(p, dir+"/src/")
	var out []violation
	for i, line := range strings.Split(string(data), "\n") {
		for _, m := range importRE.FindAllStringSubmatch(line, -1) {
			for _, rule := range check(dir, rel, m[1]) {
				out = append(out, violation{p, i + 1, rule, m[1]})
			}
		}
	}
	return out, nil
}

// scanDir walks dir/src.
func scanDir(fsys fs.FS, dir string) ([]violation, error) {
	var out []violation
	err := fs.WalkDir(fsys, dir+"/src", func(p string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case d.IsDir() && d.Name() == "node_modules":
			return fs.SkipDir
		case d.IsDir(), !strings.HasSuffix(p, ".ts") && !strings.HasSuffix(p, ".tsx"):
			return nil
		}
		vs, err := scanFile(fsys, dir, p)
		out = append(out, vs...)
		return err
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("walk %s: %w", dir, err)
	}
	return out, nil
}

func scan(fsys fs.FS) ([]violation, error) {
	var out []violation
	for _, kind := range []string{"apps", "packages"} {
		entries, err := fs.ReadDir(fsys, kind)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			vs, err := scanDir(fsys, kind+"/"+e.Name())
			if err != nil {
				return nil, err
			}
			out = append(out, vs...)
		}
	}
	return out, nil
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: studiobounds <studio-root>")
		os.Exit(2)
	}
	vs, err := scan(os.DirFS(os.Args[1]))
	if err != nil {
		fmt.Fprintln(os.Stderr, "studiobounds:", err)
		os.Exit(2)
	}
	for _, v := range vs {
		fmt.Println(v)
	}
	if len(vs) > 0 {
		os.Exit(1)
	}
}
