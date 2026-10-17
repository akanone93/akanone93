// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

// Command flutterl10n derives schema/l10n/flutter from the widget strings of
// the pinned Flutter SDK (ADR-0066 § Flutter's widget strings, I18N-002).
// For every locale that has ARB files in the SDK's flutter_localizations and
// data in schema/l10n/cldr it writes <locale>.json: the Material, Cupertino
// and widgets strings, each under its own key because the three sets reuse
// key names, with the ARB metadata (keys starting with "@") dropped.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// groups are the ARB file families of flutter_localizations, and the key
// each is written under.
var groups = []string{"cupertino", "material", "widgets"}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run returns 0 on success and 2 on usage, input or output errors.
func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("flutterl10n", flag.ContinueOnError)
	fs.SetOutput(stderr)
	sdk := fs.String("sdk", "", "the Flutter SDK root (default: $FLUTTER_ROOT, else the directory of `flutter`)")
	cldr := fs.String("cldr", "", "the schema/l10n/cldr directory, whose index.json lists the CLDR locales")
	out := fs.String("out", "", "the directory to write the locale files to")
	if err := fs.Parse(args); err != nil || *cldr == "" || *out == "" {
		_, _ = fmt.Fprintln(stderr, "usage: flutterl10n [-sdk <flutter root>] -cldr <dir> -out <dir>")
		return 2
	}
	root := *sdk
	if root == "" {
		var err error
		if root, err = sdkRoot(); err != nil {
			_, _ = fmt.Fprintln(stderr, "flutterl10n:", err)
			return 2
		}
	}
	n, err := derive(filepath.Join(root, "packages", "flutter_localizations", "lib", "src", "l10n"), *cldr, *out)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "flutterl10n:", err)
		return 2
	}
	_, _ = fmt.Fprintf(stdout, "flutterl10n: wrote %d locales\n", n)
	return 0
}

// sdkRoot finds the Flutter SDK from FLUTTER_ROOT, else from the flutter
// executable on the PATH (<root>/bin/flutter).
func sdkRoot() (string, error) {
	if r := os.Getenv("FLUTTER_ROOT"); r != "" {
		return r, nil
	}
	p, err := exec.LookPath("flutter")
	if err != nil {
		return "", errors.New("no Flutter SDK: set FLUTTER_ROOT or put flutter on the PATH")
	}
	if r, err := filepath.EvalSymlinks(p); err == nil {
		p = r
	}
	return filepath.Dir(filepath.Dir(p)), nil
}

// derive writes the locale files and returns how many it wrote.
func derive(arbDir, cldrDir, out string) (int, error) {
	raw, err := os.ReadFile(filepath.Join(cldrDir, "index.json"))
	if err != nil {
		return 0, fmt.Errorf("read the CLDR index: %w", err)
	}
	var idx struct {
		Locales []string `json:"locales"`
	}
	if err := json.Unmarshal(raw, &idx); err != nil {
		return 0, fmt.Errorf("parse the CLDR index: %w", err)
	}
	cldr := map[string]bool{}
	for _, l := range idx.Locales {
		cldr[l] = true
	}
	// locale -> group -> file
	files := map[string]map[string]string{}
	for _, g := range groups {
		matches, err := filepath.Glob(filepath.Join(arbDir, g+"_*.arb"))
		if err != nil {
			return 0, fmt.Errorf("list %s: %w", arbDir, err)
		}
		for _, m := range matches {
			tag := strings.ReplaceAll(strings.TrimSuffix(strings.TrimPrefix(filepath.Base(m), g+"_"), ".arb"), "_", "-")
			if !cldr[tag] {
				continue
			}
			if files[tag] == nil {
				files[tag] = map[string]string{}
			}
			files[tag][g] = m
		}
	}
	if len(files) == 0 {
		return 0, fmt.Errorf("%s has no ARB file for a CLDR locale", arbDir)
	}
	if err := os.MkdirAll(out, 0o750); err != nil {
		return 0, fmt.Errorf("create %s: %w", out, err)
	}
	old, err := filepath.Glob(filepath.Join(out, "*.json"))
	if err != nil {
		return 0, fmt.Errorf("list %s: %w", out, err)
	}
	for _, f := range old {
		if err := os.Remove(f); err != nil {
			return 0, fmt.Errorf("remove %s: %w", f, err)
		}
	}
	tags := make([]string, 0, len(files))
	for t := range files {
		tags = append(tags, t)
	}
	sort.Strings(tags)
	for _, tag := range tags {
		doc := map[string]map[string]string{}
		for g, f := range files[tag] {
			strs, err := readARB(f)
			if err != nil {
				return 0, err
			}
			doc[g] = strs
		}
		b, err := json.MarshalIndent(doc, "", "  ")
		if err != nil {
			return 0, fmt.Errorf("encode %s: %w", tag, err)
		}
		if err := os.WriteFile(filepath.Join(out, tag+".json"), append(b, '\n'), 0o600); err != nil {
			return 0, fmt.Errorf("write %s: %w", tag, err)
		}
	}
	return len(tags), nil
}

// readARB returns the strings of an ARB file, without its metadata.
func readARB(path string) (map[string]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var all map[string]any
	if err := json.Unmarshal(b, &all); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	out := map[string]string{}
	for k, v := range all {
		if strings.HasPrefix(k, "@") {
			continue
		}
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("%s: %q is not a string", path, k)
		}
		out[k] = s
	}
	return out, nil
}
