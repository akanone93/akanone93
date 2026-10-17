// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/nightCode42/plux3/backend/internal/l10n/literals"
	"github.com/nightCode42/plux3/backend/internal/l10n/mf"
	"github.com/nightCode42/plux3/backend/internal/l10n/pseudo"
	"github.com/nightCode42/plux3/backend/internal/schema/uuid7"
)

// Paths of the documents the local l10n commands read and write.
const (
	appDocument  = "app.json"
	keysDocument = "translations/keys.json"
)

// maxKeyLength is the longest translation key the schema allows.
const maxKeyLength = 128

// keyNameChars matches what a key segment may not hold.
var keyNameChars = regexp.MustCompile(`[^a-z0-9]+`)

// readDocument reads a JSON document into a generic map, keeping numbers
// exact.
func readDocument(path string) (map[string]any, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: the developer's own project.
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var doc map[string]any
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return doc, nil
}

// encodeDocument formats a document the way the project's documents are
// written: sorted keys, two-space indent, a final newline.
func encodeDocument(doc map[string]any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return nil, fmt.Errorf("encode: %w", err)
	}
	return buf.Bytes(), nil
}

// writeDocument writes a document, creating its directory.
func writeDocument(path string, doc map[string]any) error {
	data, err := encodeDocument(doc)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { //nolint:gosec // G301: project files.
		return fmt.Errorf("write %s: %w", path, err)
	}
	return writeFile(path, data)
}

// newID returns a fresh UUIDv7.
func newID() (string, error) {
	id, err := uuid7.NewGenerator(time.Now, rand.Reader).New()
	if err != nil {
		return "", fmt.Errorf("new identifier: %w", err)
	}
	return id.String(), nil
}

// dirFlag registers --dir, with -C as its short form, on a local command.
func dirFlag(set *flag.FlagSet) *string {
	dir := set.String("dir", ".", "the project `directory`")
	set.StringVar(dir, "C", ".", "the project `directory` (same as --dir)")
	return dir
}

// defaultLocale returns the default locale of the project in dir and the
// schema version of its documents.
func defaultLocale(dir string) (locale, version string, err error) {
	app, err := readDocument(filepath.Join(dir, appDocument))
	if err != nil {
		return "", "", err
	}
	locale, _ = app["defaultLocale"].(string)
	version, _ = app["schemaVersion"].(string)
	if locale == "" {
		return "", "", fmt.Errorf("%s has no defaultLocale", appDocument)
	}
	return locale, version, nil
}

// l10nPseudo runs plux l10n pseudo: it writes the pseudo-locale documents
// of the project's default locale (I18N-002, I18N-010).
func (e env) l10nPseudo(args []string) int {
	set := flag.NewFlagSet("l10n pseudo", flag.ContinueOnError)
	dir := dirFlag(set)
	jsonOut := set.Bool("json", false, "print the result as JSON on stdout")
	if _, code, ok := parse(set, args, e.stderr, "Usage: plux l10n pseudo [--dir dir] [--json]\n\nWrites translations/en-XA.json (accented, longer) and translations/ar-XB.json (right to left)\nfrom the default locale's messages.", 0); !ok {
		return code
	}
	written, err := writePseudoLocales(*dir)
	if err != nil {
		return e.fail("l10n pseudo", err)
	}
	var b strings.Builder
	for _, w := range written {
		fmt.Fprintf(&b, "Wrote %s (%d messages).\n", w.File, w.Messages)
	}
	return e.emit(*jsonOut, map[string]any{"files": written}, strings.TrimSuffix(b.String(), "\n"))
}

// pseudoFile is a pseudo-locale document written.
type pseudoFile struct {
	Locale   string `json:"locale"`
	File     string `json:"file"`
	Messages int    `json:"messages"`
}

// writePseudoLocales writes the pseudo-locale documents of a project from
// its default locale's, keeping the identifier of one that exists.
func writePseudoLocales(dir string) ([]pseudoFile, error) {
	locale, version, err := defaultLocale(dir)
	if err != nil {
		return nil, err
	}
	srcPath := filepath.Join(dir, "translations", locale+".json")
	src, err := readDocument(srcPath)
	if err != nil {
		return nil, err
	}
	messages := map[string]string{}
	raw, _ := src["messages"].(map[string]any)
	for k, v := range raw {
		if s, ok := v.(string); ok {
			messages[k] = s
		}
	}
	var out []pseudoFile
	for _, tag := range pseudo.Tags() {
		rel := "translations/" + tag + ".json"
		path := filepath.Join(dir, filepath.FromSlash(rel))
		id := ""
		if prev, err := readDocument(path); err == nil {
			id, _ = prev["id"].(string)
		}
		if id == "" {
			if id, err = newID(); err != nil {
				return nil, err
			}
		}
		made := pseudo.Messages(tag, messages)
		doc := map[string]any{"schemaVersion": version, "kind": "translations", "id": id, "locale": tag, "messages": made}
		if err := writeDocument(path, doc); err != nil {
			return nil, err
		}
		out = append(out, pseudoFile{Locale: tag, File: rel, Messages: len(made)})
	}
	return out, nil
}

// extracted is one literal text moved into a translation key.
type extracted struct {
	File string `json:"file"`
	Path string `json:"path"`
	Key  string `json:"key"`
	Text string `json:"text"`
	ID   string `json:"id"`
}

// l10nExtract runs plux l10n extract: it moves the literal text of
// translatable props in the project's pages and components into new
// translation keys of the default locale (I18N-007).
func (e env) l10nExtract(args []string) int {
	set := flag.NewFlagSet("l10n extract", flag.ContinueOnError)
	dir := dirFlag(set)
	dry := set.Bool("dry-run", false, "report what would change and write nothing")
	jsonOut := set.Bool("json", false, "print the result as JSON on stdout")
	if _, code, ok := parse(set, args, e.stderr, "Usage: plux l10n extract [--dir dir] [--dry-run] [--json]\n\nReplaces the literal text of translatable props in pages and components with references to\nnew translation keys, and adds the texts to the default locale's translations.", 0); !ok {
		return code
	}
	done, err := extractLiterals(*dir, *dry)
	if err != nil {
		return e.fail("l10n extract", err)
	}
	verb := "Extracted"
	if *dry {
		verb = "Would extract"
	}
	var b strings.Builder
	for _, x := range done {
		fmt.Fprintf(&b, "  %s%s -> %s\n", x.File, x.Path, x.Key)
	}
	files := map[string]bool{}
	for _, x := range done {
		files[x.File] = true
	}
	text := "No literal text in translatable props."
	if len(done) > 0 {
		text = fmt.Sprintf("%s %d texts from %d documents into new translation keys.\n%s", verb, len(done), len(files), strings.TrimSuffix(b.String(), "\n"))
	}
	return e.emit(*jsonOut, map[string]any{"dryRun": *dry, "extracted": done}, text)
}

// extractor holds the state of one extraction.
type extractor struct {
	dir      string
	keys     map[string]any
	messages map[string]any
	names    map[string]bool
	done     []extracted
	ids      func() (string, error)
}

// extractLiterals extracts the literals of the project in dir. With dry
// set nothing is written.
func extractLiterals(dir string, dry bool) ([]extracted, error) {
	locale, version, err := defaultLocale(dir)
	if err != nil {
		return nil, err
	}
	x := &extractor{dir: dir, names: map[string]bool{}, ids: newID}
	keysPath := filepath.Join(dir, filepath.FromSlash(keysDocument))
	if x.keys, err = readOrNew(keysPath, func() (map[string]any, error) {
		return newDocument(version, "translationKeys", map[string]any{"keys": []any{}})
	}); err != nil {
		return nil, err
	}
	msgPath := filepath.Join(dir, "translations", locale+".json")
	if x.messages, err = readOrNew(msgPath, func() (map[string]any, error) {
		return newDocument(version, "translations", map[string]any{"locale": locale, "messages": map[string]any{}})
	}); err != nil {
		return nil, err
	}
	list, _ := x.keys["keys"].([]any)
	for _, k := range list {
		if m, ok := k.(map[string]any); ok {
			name, _ := m["key"].(string)
			x.names[name] = true
		}
	}
	files, err := nodeDocuments(dir)
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		if err := x.document(f, dry); err != nil {
			return nil, err
		}
	}
	if len(x.done) == 0 || dry {
		return x.done, nil
	}
	if err := writeDocument(keysPath, x.keys); err != nil {
		return nil, err
	}
	if err := writeDocument(msgPath, x.messages); err != nil {
		return nil, err
	}
	return x.done, nil
}

// readOrNew reads a document, or makes one when the file does not exist.
func readOrNew(path string, build func() (map[string]any, error)) (map[string]any, error) {
	doc, err := readDocument(path)
	if err == nil {
		return doc, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return build()
	}
	return nil, err
}

// newDocument returns a document of a kind with a new identifier.
func newDocument(version, kind string, fields map[string]any) (map[string]any, error) {
	id, err := newID()
	if err != nil {
		return nil, err
	}
	fields["schemaVersion"], fields["kind"], fields["id"] = version, kind, id
	return fields, nil
}

// nodeDocuments lists the page and component documents of a project, by
// path.
func nodeDocuments(dir string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if n := d.Name(); n != "." && (strings.HasPrefix(n, ".") || n == "node_modules" || n == "build") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(p, ".page.json") || strings.HasSuffix(p, ".component.json") {
			out = append(out, p)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list documents: %w", err)
	}
	slices.Sort(out)
	return out, nil
}

// document extracts the literals of one page or component and writes it back
// when something changed and dry is not set.
func (x *extractor) document(path string, dry bool) error {
	doc, err := readDocument(path)
	if err != nil {
		return err
	}
	root, ok := doc["root"].(map[string]any)
	if !ok {
		return nil
	}
	rel, err := filepath.Rel(x.dir, path)
	if err != nil {
		rel = path
	}
	rel = filepath.ToSlash(rel)
	before := len(x.done)
	scope := x.scope(rel, doc)
	if err := x.title(rel, scope, doc); err != nil {
		return err
	}
	if err := x.node(rel, scope, root, "/root"); err != nil {
		return err
	}
	if len(x.done) == before || dry {
		return nil
	}
	return writeDocument(path, doc)
}

// scope names the plugin and the document a key belongs to.
func (x *extractor) scope(rel string, doc map[string]any) []string {
	var parts []string
	if after, ok := strings.CutPrefix(rel, "plugins/"); ok {
		plugin, _, _ := strings.Cut(after, "/")
		parts = append(parts, plugin)
	}
	key, _ := doc["key"].(string)
	if key == "" {
		key, _, _ = strings.Cut(filepath.Base(rel), ".")
	}
	return append(parts, key)
}

// title extracts the literal title of a page (I18N-007).
func (x *extractor) title(file string, scope []string, doc map[string]any) error {
	for _, l := range literals.Title(doc) {
		id, err := x.add(file, scope, map[string]any{"type": "page"}, l)
		if err != nil {
			return err
		}
		setAt(doc, l.Path, map[string]any{"$t": id})
		x.done[len(x.done)-1].Path = "/" + strings.Join(l.Path, "/")
	}
	return nil
}

// node extracts the literals of a node and of everything below it.
func (x *extractor) node(file string, scope []string, n map[string]any, ptr string) error {
	for _, l := range literals.Node(n) {
		id, err := x.add(file, scope, n, l)
		if err != nil {
			return err
		}
		setAt(n, l.Path, map[string]any{"$t": id})
		x.done[len(x.done)-1].Path = ptr + "/" + strings.Join(l.Path, "/")
	}
	if kids, ok := n["children"].([]any); ok {
		for i, k := range kids {
			if m, ok := k.(map[string]any); ok {
				if err := x.node(file, scope, m, fmt.Sprintf("%s/children/%d", ptr, i)); err != nil {
					return err
				}
			}
		}
	}
	slots, _ := n["slots"].(map[string]any)
	for _, name := range slices.Sorted(mapKeys(slots)) {
		switch s := slots[name].(type) {
		case map[string]any:
			if err := x.node(file, scope, s, ptr+"/slots/"+name); err != nil {
				return err
			}
		case []any:
			for i, k := range s {
				if m, ok := k.(map[string]any); ok {
					if err := x.node(file, scope, m, fmt.Sprintf("%s/slots/%s/%d", ptr, name, i)); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

// add creates the key and the default-locale message of a literal and
// returns the key's identifier.
func (x *extractor) add(file string, scope []string, n map[string]any, l literals.Literal) (string, error) {
	id, err := x.ids()
	if err != nil {
		return "", err
	}
	name := x.keyName(scope, n, l)
	x.names[name] = true
	text := (&mf.Message{Nodes: []mf.Node{{Kind: mf.KindText, Text: l.Text}}}).String()
	typ, _ := n["type"].(string)
	desc := fmt.Sprintf("Extracted from %s of a %s on %s.", l.Prop, typ, scope[len(scope)-1])
	keys, _ := x.keys["keys"].([]any)
	x.keys["keys"] = append(keys, map[string]any{"id": id, "key": name, "description": desc})
	msgs, _ := x.messages["messages"].(map[string]any)
	msgs[id] = text
	x.done = append(x.done, extracted{File: file, Key: name, Text: l.Text, ID: id})
	return id, nil
}

// maxNameWords is how many words of a text name its key.
const maxNameWords = 4

// slug lowercases s and joins its ASCII letters and digits with hyphens.
func slug(s string) string {
	return strings.Trim(keyNameChars.ReplaceAllString(strings.ToLower(s), "-"), "-")
}

// keyName derives a unique key name from the plugin, the page or component
// and the first words of the text, or the node's test ID and the prop.
func (x *extractor) keyName(scope []string, n map[string]any, l literals.Literal) string {
	path := l.Path
	if len(path) > 0 && path[0] == "props" {
		path = path[1:]
	}
	where := strings.Join(path, "-")
	leaf := ""
	if id, _ := n["testId"].(string); id != "" {
		leaf = id + "-" + where
	} else if words := strings.Fields(l.Text); len(words) > 0 {
		leaf = strings.Join(words[:min(len(words), maxNameWords)], " ")
	}
	if slug(leaf) == "" { // no Latin letters to name the key after
		typ, _ := n["type"].(string)
		leaf = typ + "-" + where
	}
	segs := make([]string, 0, len(scope)+1)
	for _, p := range append(slices.Clone(scope), leaf) {
		if s := slug(p); s != "" {
			segs = append(segs, s)
		}
	}
	base := strings.Join(segs, ".")
	if base == "" || base[0] < 'a' || base[0] > 'z' {
		base = "text." + base
	}
	if len(base) > maxKeyLength-5 {
		base = strings.TrimRight(base[:maxKeyLength-5], ".-")
	}
	name := base
	for i := 2; x.names[name]; i++ {
		name = fmt.Sprintf("%s-%d", base, i)
	}
	return name
}

// setAt replaces the value at a path of object keys and list indices.
func setAt(n map[string]any, path []string, v any) {
	var cur any = n
	for i, seg := range path {
		last := i == len(path)-1
		switch c := cur.(type) {
		case map[string]any:
			if last {
				c[seg] = v
				return
			}
			cur = c[seg]
		case []any:
			idx := 0
			if _, err := fmt.Sscanf(seg, "%d", &idx); err != nil || idx < 0 || idx >= len(c) {
				return
			}
			if last {
				c[idx] = v
				return
			}
			cur = c[idx]
		default:
			return
		}
	}
}
