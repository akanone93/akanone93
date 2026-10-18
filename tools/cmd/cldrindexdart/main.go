// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

// Command cldrindexdart writes the part of schema/l10n/cldr/index.json the
// Flutter runtime's locale resolution needs (parent locales and likely
// scripts) as Dart source, so the runtime embeds it without reading a file
// and the table cannot drift from the data (I18N-003, CI-003). The output is
// deterministic: sorted keys, fixed layout.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

//go:generate go run . ../../../schema/l10n/cldr/index.json ../../../packages/plux_flutter/lib/src/l10n/cldr_index.g.dart

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: cldrindexdart <index.json> <out.g.dart>")
		os.Exit(2)
	}
	in, err := os.ReadFile(os.Args[1]) //nolint:gosec // The index the caller named.
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	out, err := render(in)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.WriteFile(os.Args[2], out, 0o644); err != nil { //nolint:gosec // A committed file, readable by all.
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// render returns the Dart source for the index JSON.
func render(index []byte) ([]byte, error) {
	var idx struct {
		ParentLocales map[string]string `json:"parentLocales"`
		LikelyScripts map[string]string `json:"likelyScripts"`
	}
	if err := json.Unmarshal(index, &idx); err != nil {
		return nil, fmt.Errorf("cldrindexdart: %w", err)
	}
	var b bytes.Buffer
	b.WriteString("// SPDX-FileCopyrightText: 2026 Plux contributors\n")
	b.WriteString("// SPDX-" + "License-Identifier: Apache-2.0\n\n")
	b.WriteString("// GENERATED CODE - DO NOT MODIFY BY HAND. Run 'make gen'\n")
	b.WriteString("// (tools/cmd/cldrindexdart, from schema/l10n/cldr/index.json).\n\n")
	b.WriteString("/// The part of `schema/l10n/cldr/index.json` that locale resolution needs,\n")
	b.WriteString("/// embedded so the runtime resolves a requested locale against a release's\n")
	b.WriteString("/// supported locales without reading a file (ADR-0066 § Resolution).\n")
	b.WriteString("library;\n\n")
	b.WriteString("import 'package:plux_flutter/src/l10n/resolve.dart';\n\n")
	table(&b, "CLDR parent locales: a locale to the locale it inherits from.", "cldrParentLocales", idx.ParentLocales)
	b.WriteString("\n")
	table(&b, "Language-region pairs to the script they imply.", "cldrLikelyScripts", idx.LikelyScripts)
	b.WriteString("\n/// The embedded index.\n")
	b.WriteString("const PluxLocaleIndex cldrLocaleIndex = PluxLocaleIndex(\n")
	b.WriteString("  parentLocales: cldrParentLocales,\n  likelyScripts: cldrLikelyScripts,\n);\n")
	return b.Bytes(), nil
}

func table(b *bytes.Buffer, doc, name string, m map[string]string) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Fprintf(b, "/// %s\nconst Map<String, String> %s = {\n", doc, name)
	for _, k := range keys {
		fmt.Fprintf(b, "  '%s': '%s',\n", k, m[k])
	}
	b.WriteString("};\n")
}
