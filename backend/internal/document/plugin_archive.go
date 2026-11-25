// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package document

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"maps"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema"
	"github.com/nightCode42/plux3/backend/internal/schema/jcs"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// A plugin travels between apps as a .plux archive of its own documents and
// the app-level parts they name: the translation keys and their messages,
// and the assets with their per-locale files (ADR-0102, build log row
// 264). Components, functions, types and data the app declares are the
// target app's to have; a reference to one it lacks is a problem of the
// imported plugin, which the import counts.

const keysPath = "translations/keys.json"

// isLocalePath reports whether p is the messages of one of the app's
// locales, translations/<locale>.json.
func isLocalePath(p string) bool {
	rest, ok := strings.CutPrefix(p, "translations/")
	return ok && p != keysPath && path.Ext(rest) == ".json" && !strings.Contains(rest, "/")
}

// isSharedPath reports whether p is an app-level document a plugin
// archive carries a part of.
func isSharedPath(p string) bool { return p == keysPath || p == indexPath || isLocalePath(p) }

// ExportPlugin returns a plugin's documents in the Git layout with the
// parts of the app-level documents they name: the translation keys whose
// identifiers they hold, those keys' messages and review states in every
// locale, and the assets whose identifiers they or those keys hold, with
// the assets' per-locale files. It needs plugin.read on the app.
func (s *Service) ExportPlugin(ctx context.Context, p auth.Principal, pluginID string) (Plugin, []File, error) {
	pl, err := s.GetPlugin(ctx, p, pluginID)
	if err != nil {
		return Plugin{}, nil, err
	}
	var out []File
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		d, err := s.resolve(ctx, q, p, pl.AppID, pl.ID)
		if err != nil {
			return err
		}
		if err := authorize(p, auth.PluginRead, pl.AppID); err != nil {
			return err
		}
		app, err := s.resolve(ctx, q, p, pl.AppID, "")
		if err != nil {
			return err
		}
		own, err := s.rawDocuments(ctx, q, d, func(string) bool { return true })
		if err != nil {
			return err
		}
		shared, err := s.rawDocuments(ctx, q, app, isSharedPath)
		if err != nil {
			return err
		}
		depth := int(s.o.Limits.Get(limits.DocumentJSONDepth))
		parts, files, err := sharedParts(own, shared, depth)
		if err != nil {
			return err
		}
		for _, f := range slices.Concat(own, parts) {
			pretty, err := jcs.Format(f.Content, depth)
			if err != nil {
				return fmt.Errorf("document: format %s: %w", f.Path, err)
			}
			out = append(out, File{Path: f.Path, Content: pretty})
		}
		blobs, err := s.assetFiles(ctx, q, app, files)
		out = append(out, blobs...)
		return err
	})
	return pl, out, err
}

// rawDocuments reads the documents of a draft that keep allows, as stored.
func (s *Service) rawDocuments(ctx context.Context, q *dbgen.Queries, d draft, keep func(string) bool) ([]File, error) {
	rows, err := q.ListAllDocuments(ctx, d.row.ID)
	if err != nil {
		return nil, failure(err, "document")
	}
	var out []File
	for _, row := range rows {
		if !keep(row.Path) {
			continue
		}
		content, err := s.blob(ctx, q, d.row.OrganizationID, row.Sha256)
		if err != nil {
			return nil, err
		}
		out = append(out, File{Path: row.Path, Content: content})
	}
	return out, nil
}

// assetFiles reads the stored files of the named assets.
func (s *Service) assetFiles(ctx context.Context, q *dbgen.Queries, app draft, files map[string]bool) ([]File, error) {
	if len(files) == 0 {
		return nil, nil
	}
	if err := s.assetsRequired(); err != nil {
		return nil, err
	}
	rows, err := q.ListAllAssets(ctx, app.row.AppID)
	if err != nil {
		return nil, failure(err, "asset")
	}
	var out []File
	for _, a := range rows {
		if !files["assets/"+a.File] {
			continue
		}
		data, _, err := s.o.Objects.Get(ctx, objectKey(a.Sha256))
		if err != nil {
			return nil, fmt.Errorf("document: read asset %s: %w", a.File, err)
		}
		out = append(out, File{Path: "assets/" + a.File, Content: data})
	}
	return out, nil
}

// sharedParts cuts the app-level documents down to what a plugin's own
// documents name, and returns them with the asset files they need.
func sharedParts(own, shared []File, depth int) ([]File, map[string]bool, error) {
	refs := map[string]bool{}
	for _, f := range own {
		uuidsIn(f.Content, refs)
	}
	var out []File
	byPath := map[string][]byte{}
	for _, f := range shared {
		byPath[f.Path] = f.Content
	}
	keys := map[string]bool{}
	if content, ok := byPath[keysPath]; ok {
		doc, err := keepEntries(content, "keys", depth, func(e map[string]any) bool {
			id, _ := e["id"].(string)
			if !refs[id] {
				return false
			}
			keys[id] = true
			return true
		})
		if err != nil {
			return nil, nil, err
		}
		uuidsIn(doc, refs) // a key's screenshot is an asset
		out = append(out, File{Path: keysPath, Content: doc})
	}
	for _, f := range shared {
		if !isLocalePath(f.Path) {
			continue
		}
		doc, err := keepMessages(f.Content, keys, depth)
		if err != nil {
			return nil, nil, err
		}
		out = append(out, File{Path: f.Path, Content: doc})
	}
	files := map[string]bool{}
	if content, ok := byPath[indexPath]; ok {
		doc, err := indexOf(content, refs, files, depth)
		if err != nil {
			return nil, nil, err
		}
		out = append(out, File{Path: indexPath, Content: doc})
	}
	return out, files, nil
}

// localeFilesOf is the per-locale files an index entry names.
func localeFilesOf(e map[string]any) []string {
	locales, _ := e["locales"].(map[string]any)
	out := make([]string, 0, len(locales))
	for _, f := range locales {
		if name, ok := f.(string); ok {
			out = append(out, name)
		}
	}
	return out
}

// indexOf keeps the index entries of the referenced assets and of their
// per-locale files, and adds every file they need to files.
func indexOf(content []byte, refs, files map[string]bool, depth int) ([]byte, error) {
	doc, err := objectOf(content, depth)
	if err != nil {
		return nil, err
	}
	localeFiles := map[string]bool{}
	for _, e := range entriesOf(doc, "assets") {
		if id, _ := e["id"].(string); refs[id] {
			for _, name := range localeFilesOf(e) {
				localeFiles[name] = true
			}
		}
	}
	return keepEntries(content, "assets", depth, func(e map[string]any) bool {
		id, _ := e["id"].(string)
		file, _ := e["file"].(string)
		if !refs[id] && !localeFiles[file] {
			return false
		}
		files["assets/"+file] = true
		for _, name := range localeFilesOf(e) {
			files["assets/"+name] = true
		}
		return true
	})
}

// uuidsIn adds every UUID-shaped string in b to into: the identifiers a
// document can name, read in one pass.
func uuidsIn(b []byte, into map[string]bool) {
	for i := 0; i+36 <= len(b); i++ {
		if b[i+8] == '-' && looksUUID(b[i:i+36]) {
			into[string(b[i:i+36])] = true
			i += 35
		}
	}
}

// looksUUID reports whether b is 36 bytes of a UUID's form.
func looksUUID(b []byte) bool {
	for i, c := range b {
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
				return false
			}
		}
	}
	return true
}

// keepEntries keeps the members of a document's list that keep allows.
func keepEntries(content []byte, list string, depth int, keep func(map[string]any) bool) ([]byte, error) {
	doc, err := objectOf(content, depth)
	if err != nil {
		return nil, err
	}
	kept := []any{}
	for _, e := range entriesOf(doc, list) {
		if keep(e) {
			kept = append(kept, e)
		}
	}
	doc[list] = kept
	return marshal(doc)
}

// keepMessages keeps a locale's messages and review states of the given
// keys.
func keepMessages(content []byte, keys map[string]bool, depth int) ([]byte, error) {
	doc, err := objectOf(content, depth)
	if err != nil {
		return nil, err
	}
	for _, member := range []string{"messages", "states"} {
		m, ok := doc[member].(map[string]any)
		if !ok {
			continue
		}
		kept := map[string]any{}
		for id, v := range m {
			if keys[id] {
				kept[id] = v
			}
		}
		doc[member] = kept
	}
	return marshal(doc)
}

// objectOf parses a JSON object document.
func objectOf(content []byte, depth int) (map[string]any, error) {
	tree, err := jcs.Parse(content, depth)
	if err != nil {
		return nil, plxerr.Wrap(plxerr.InvalidFormat, err, "a document is not JSON")
	}
	doc, ok := tree.(map[string]any)
	if !ok {
		return nil, plxerr.New(plxerr.InvalidStructure, "a document is not a JSON object")
	}
	return doc, nil
}

// marshal writes a document in canonical form.
func marshal(doc map[string]any) ([]byte, error) {
	out, err := jcs.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("document: %w", err)
	}
	return out, nil
}

// entriesOf is the objects of a document's list.
func entriesOf(doc map[string]any, list string) []map[string]any {
	items, _ := doc[list].([]any)
	out := make([]map[string]any, 0, len(items))
	for _, e := range items {
		if m, ok := e.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// PluginImport is what ImportPlugin did.
type PluginImport struct {
	Plugin Plugin
	// Documents counts the plugin's documents written.
	Documents int
	// Keys and Assets count the translation keys and assets added to the
	// app; those it already had are used as they are.
	Keys, Assets int
	// ErrorDiagnostics counts the validation errors of the plugin in the
	// app, as ValidateDraft reports them: its own, such as a reference to
	// a component the app does not declare, and those of the app-level
	// documents it depends on.
	ErrorDiagnostics int
}

// pluginArchive is a plugin archive's files, read and checked.
type pluginArchive struct {
	docs    []Document
	keys    map[string]any            // translations/keys.json
	locales map[string]map[string]any // locale documents by path
	index   map[string]any            // assets/index.json
	files   map[string][]byte         // asset files by name under assets/
}

// ImportPlugin creates a plugin in an app from a plugin archive's files,
// which are untrusted: every document is checked against its schema
// before anything is written. srcKey is the plugin's key in the archive.
// The plugin's documents get new identifiers and route names that clash
// with none of the app's, as a duplicate's do (STU-034). Its translation
// keys are added to the app's, with their messages and review states in
// the locales the app has; a key the app already has is used as it is,
// and an added key whose name the app uses is renamed under the plugin's
// key. Its assets are added under names the app does not use, except a
// file the app already has the same content of, which is used instead.
// session takes the locks the writes need, those of the app, its locales
// and the new plugin, and gives back those it did not hold before. It
// needs app.manage on the app.
func (s *Service) ImportPlugin(ctx context.Context, p auth.Principal, appID, srcKey, key, name, session string, files []File) (PluginImport, error) {
	if err := authorize(p, auth.AppManage, appID); err != nil {
		return PluginImport{}, err
	}
	if !schema.ValidKey(srcKey) {
		return PluginImport{}, plxerr.New(plxerr.InvalidFormat, "the archive's plugin key %q is not a plugin key", srcKey)
	}
	if err := checkSession(session); err != nil {
		return PluginImport{}, err
	}
	lim, err := s.appLimits(ctx, p, appID)
	if err != nil {
		return PluginImport{}, err
	}
	a, err := s.readPluginArchive(lim, srcKey, files)
	if err != nil {
		return PluginImport{}, err
	}
	if err := s.keyFree(ctx, p, appID, key, lim); err != nil {
		return PluginImport{}, err
	}
	locks := &held{s: s, p: p, appID: appID, session: session}
	defer locks.release(ctx)
	im := &importer{s: s, p: p, appID: appID, key: key, session: session, a: a, locks: locks}
	out := PluginImport{}
	if out.Assets, err = im.assets(ctx); err != nil {
		return out, err
	}
	if out.Plugin, err = s.createPlugin(ctx, p, appID, key, name, ""); err != nil {
		return out, err
	}
	if out.Keys, err = im.keys(ctx); err != nil {
		return out, err
	}
	if err := locks.take(ctx, out.Plugin.ID, ""); err != nil {
		return out, err
	}
	if err := s.writeImportedDocuments(ctx, p, out.Plugin, srcKey, key, session, a.docs, im.renamed); err != nil {
		return out, err
	}
	out.Documents = len(a.docs)
	diags, err := s.ValidateDraft(ctx, p, appID, out.Plugin.ID)
	if err != nil {
		return out, err
	}
	out.ErrorDiagnostics = diags.Count(plxerr.SeverityError)
	out.Plugin, err = s.GetPlugin(ctx, p, out.Plugin.ID)
	return out, err
}

// readPluginArchive sorts a plugin archive's files into their parts and
// checks every document against its schema, as a write would; a file of
// any other part, or an asset file no index entry names, refuses it.
func (s *Service) readPluginArchive(lim limits.Set, srcKey string, files []File) (pluginArchive, error) {
	out := pluginArchive{locales: map[string]map[string]any{}, files: map[string][]byte{}}
	prefix := "plugins/" + srcKey + "/"
	loader := schema.NewLoader(s.validator(), schema.DefaultMigrator(), lim)
	for _, f := range files {
		if name, ok := strings.CutPrefix(f.Path, "assets/"); ok && f.Path != indexPath {
			out.files[name] = f.Content
			continue
		}
		place, ok := schema.PlaceOf(f.Path)
		switch {
		case !ok:
			return out, plxerr.New(plxerr.InvalidProjectLayout, "%q is not a document path of the project layout", f.Path)
		case place.Plugin != "" && (place.Plugin != srcKey || !strings.HasPrefix(f.Path, prefix)):
			return out, plxerr.New(plxerr.InvalidProjectLayout, "a plugin archive of %q holds no %s", srcKey, f.Path)
		case place.Plugin == "" && !isSharedPath(f.Path):
			return out, plxerr.New(plxerr.InvalidProjectLayout, "a plugin archive holds no %s", f.Path)
		}
		if max := lim.Get(limits.DocumentFileSize); int64(len(f.Content)) > max {
			return out, plxerr.New(plxerr.LimitExceeded, "%s is %d bytes, above the limit document.fileSize = %d", f.Path, len(f.Content), max)
		}
		src, diags := loader.ParseDocument(f.Path, f.Content, place.Kind)
		if diags.HasErrors() || src == nil {
			return out, refusal(diags)
		}
		if err := namedAfterKey(f.Path, place, src.Tree); err != nil {
			return out, err
		}
		out.add(f.Path, src.Tree, f.Content)
	}
	if !slices.ContainsFunc(out.docs, func(d Document) bool { return d.Path == prefix+"plugin.json" }) {
		return out, plxerr.New(plxerr.InvalidProjectLayout, "the archive holds no %splugin.json", prefix)
	}
	return out, out.checkFiles()
}

// add files a checked document under its part.
func (a *pluginArchive) add(p string, tree map[string]any, content []byte) {
	switch {
	case p == keysPath:
		a.keys = tree
	case p == indexPath:
		a.index = tree
	case isLocalePath(p):
		a.locales[p] = tree
	default:
		a.docs = append(a.docs, Document{Path: p, Content: content})
	}
}

// checkFiles refuses an index entry without its file, a per-locale file
// that is not an entry of its own, as an upload makes it, and a file no
// entry lists.
func (a pluginArchive) checkFiles() error {
	listed := map[string]bool{}
	entries := entriesOf(a.index, "assets")
	for _, e := range entries {
		file, _ := e["file"].(string)
		if _, ok := a.files[file]; !ok {
			return plxerr.New(plxerr.InvalidProjectLayout, "the archive's index lists assets/%s, which it does not hold", file)
		}
		listed[file] = true
	}
	for _, e := range entries {
		for _, name := range localeFilesOf(e) {
			if !listed[name] {
				return plxerr.New(plxerr.InvalidProjectLayout, "the archive's index names the per-locale file assets/%s, which it does not list", name)
			}
		}
	}
	for name := range a.files {
		if !listed[name] {
			return plxerr.New(plxerr.InvalidProjectLayout, "the archive holds assets/%s, which its index does not list", name)
		}
	}
	return nil
}

// keyFree refuses a plugin key the app uses, or a plugin more than the
// app may have, before anything is written.
func (s *Service) keyFree(ctx context.Context, p auth.Principal, appID, key string, lim limits.Set) error {
	if !schema.ValidKey(key) {
		return plxerr.New(plxerr.InvalidFormat, "a plugin key is a lower-kebab slug of at most 64 characters, starting with a letter")
	}
	max := lim.Get(limits.AppPlugins)
	plugins, err := s.ListPlugins(ctx, p, appID, storage.Cursor{}, int32(max+1)) //nolint:gosec // a registry limit
	if err != nil {
		return err
	}
	if slices.ContainsFunc(plugins, func(pl Plugin) bool { return pl.Key == key }) {
		return plxerr.New(plxerr.ResourceExists, "a plugin with this key already exists")
	}
	if int64(len(plugins)) >= max {
		return plxerr.New(plxerr.LimitExceeded, "the app has %d plugins, the limit app.plugins", max)
	}
	return nil
}

// held is the locks an import took for its session.
type held struct {
	s       *Service
	p       auth.Principal
	appID   string
	session string
	taken   []Lock
}

// take takes a lock for the session unless the caller holds it with the
// session already.
func (h *held) take(ctx context.Context, pluginID, scope string) error {
	l, err := h.s.GetScopedLock(ctx, h.p, h.appID, pluginID, scope)
	if err != nil {
		return err
	}
	if me := h.p.Actor(); l.Session == h.session && l.Holder.Kind == me.Kind && l.Holder.ID == me.ID {
		return nil
	}
	if _, _, err := h.s.AcquireScopedLock(ctx, h.p, h.appID, pluginID, h.session, scope, false); err != nil {
		return err
	}
	h.taken = append(h.taken, Lock{PluginID: pluginID, Scope: scope})
	return nil
}

// release gives back the locks it took, also when ctx was cancelled.
func (h *held) release(ctx context.Context) {
	ctx = context.WithoutCancel(ctx)
	for _, l := range h.taken {
		_ = h.s.ReleaseScopedLock(ctx, h.p, h.appID, l.PluginID, h.session, l.Scope)
	}
}

// importer adds a plugin archive's app-level parts to an app.
type importer struct {
	s       *Service
	p       auth.Principal
	appID   string
	key     string
	session string
	a       pluginArchive
	locks   *held
	// renamed maps the archive's identifiers to the app's: the plugin
	// documents' and the assets'.
	renamed map[string]string
	// fileOf maps the archive's asset file names to the app's.
	fileOf map[string]string
	// added are the app's identifiers of the assets the import uploaded.
	added map[string]bool
}

// assets adds the archive's assets to the app and maps their identifiers
// and file names; it returns how many it uploaded.
func (im *importer) assets(ctx context.Context) (int, error) {
	im.renamed = collectIDs(im.a.docs, im.key)
	im.fileOf, im.added = map[string]string{}, map[string]bool{}
	entries := entriesOf(im.a.index, "assets")
	if len(entries) == 0 {
		return 0, nil
	}
	if err := im.locks.take(ctx, "", ""); err != nil {
		return 0, err
	}
	existing, err := im.s.allAssets(ctx, im.p, im.appID)
	if err != nil {
		return 0, err
	}
	bySum, names := map[string]Asset{}, map[string]bool{}
	for _, e := range existing {
		bySum[e.SHA256] = e
		names[e.File] = true
	}
	n := 0
	for _, e := range entries {
		id, _ := e["id"].(string)
		file, _ := e["file"].(string)
		data := im.a.files[file]
		sum := sha256.Sum256(data)
		if have, ok := bySum[hex.EncodeToString(sum[:])]; ok {
			im.renamed[id], im.fileOf[file] = have.ID, have.File
			continue
		}
		name := freeName(file, im.key, names)
		made, err := im.s.UploadAsset(ctx, im.p, im.appID, "", im.session, name, data)
		if err != nil {
			return n, fmt.Errorf("document: import asset %s: %w", file, err)
		}
		names[name], bySum[made.SHA256] = true, made
		im.renamed[id], im.fileOf[file], im.added[made.ID] = made.ID, name, true
		n++
	}
	return n, im.describeAssets(ctx, entries)
}

// describeAssets gives the assets the import uploaded the description and
// per-locale files of their entries in the archive.
func (im *importer) describeAssets(ctx context.Context, entries []map[string]any) error {
	extra := map[string]map[string]any{}
	for _, e := range entries {
		id, _ := e["id"].(string)
		mine := im.renamed[id]
		if !im.added[mine] {
			continue
		}
		set := map[string]any{}
		if d, ok := e["description"]; ok {
			set["description"] = d
		}
		if locales, ok := e["locales"].(map[string]any); ok && len(locales) > 0 {
			mapped := map[string]any{}
			for tag, f := range locales {
				name, _ := f.(string)
				mapped[tag] = im.fileOf[name]
			}
			set["locales"] = mapped
		}
		if len(set) > 0 {
			extra[mine] = set
		}
	}
	if len(extra) == 0 {
		return nil
	}
	return im.edit(ctx, indexPath, func(doc map[string]any) bool {
		for _, e := range entriesOf(doc, "assets") {
			id, _ := e["id"].(string)
			maps.Copy(e, extra[id])
		}
		return true
	})
}

// edit changes an app-level document under the lock the session holds.
func (im *importer) edit(ctx context.Context, p string, change func(map[string]any) bool) error {
	doc, err := im.s.GetDocument(ctx, im.p, im.appID, "", p)
	if err != nil {
		return err
	}
	tree, err := objectOf(doc.Content, int(im.s.o.Limits.Get(limits.DocumentJSONDepth)))
	if err != nil {
		return err
	}
	if !change(tree) {
		return nil
	}
	return im.put(ctx, p, tree, doc.Revision)
}

// put writes an app-level document.
func (im *importer) put(ctx context.Context, p string, doc map[string]any, revision int64) error {
	content, err := marshal(doc)
	if err != nil {
		return err
	}
	if _, err := im.s.PutDocument(ctx, im.p, im.appID, "", im.session, p, content, revision); err != nil {
		return fmt.Errorf("document: import into %s: %w", p, err)
	}
	return nil
}

// keys adds the archive's translation keys the app does not have, and
// their messages; it returns how many it added.
func (im *importer) keys(ctx context.Context) (int, error) {
	archived := entriesOf(im.a.keys, "keys")
	if len(archived) == 0 {
		return 0, nil
	}
	if err := im.locks.take(ctx, "", ""); err != nil {
		return 0, err
	}
	added := map[string]bool{}
	merge := func(doc map[string]any) bool {
		ids, names := map[string]bool{}, map[string]bool{}
		for _, e := range entriesOf(doc, "keys") {
			id, _ := e["id"].(string)
			name, _ := e["key"].(string)
			ids[id], names[name] = true, true
		}
		items, _ := doc["keys"].([]any)
		for _, k := range archived {
			id, _ := k["id"].(string)
			if ids[id] {
				continue
			}
			entry, _ := swapIDs(maps.Clone(k), im.renamed).(map[string]any)
			name, _ := entry["key"].(string)
			if names[name] {
				name = freeKeyName(name, im.key, names)
				entry["key"] = name
			}
			names[name], ids[id], added[id] = true, true, true
			items = append(items, entry)
		}
		doc["keys"] = items
		return len(added) > 0
	}
	err := im.edit(ctx, keysPath, merge)
	if errors.Is(err, plxerr.Sentinel(plxerr.ResourceNotFound)) {
		// The app has no keys yet: the archive's document starts its own.
		doc := maps.Clone(im.a.keys)
		doc["keys"] = []any{}
		merge(doc)
		err = im.put(ctx, keysPath, doc, 0)
	}
	if err != nil {
		return 0, err
	}
	return len(added), im.messages(ctx, added)
}

// messages adds the added keys' messages and review states to each locale
// the app has and the archive carries, under that locale's lock.
func (im *importer) messages(ctx context.Context, added map[string]bool) error {
	for _, file := range slices.Sorted(maps.Keys(im.a.locales)) {
		from := im.a.locales[file]
		if !carriesAny(from, added) {
			continue
		}
		locale := strings.TrimSuffix(strings.TrimPrefix(file, "translations/"), ".json")
		if _, err := im.s.GetDocument(ctx, im.p, im.appID, "", file); errors.Is(err, plxerr.Sentinel(plxerr.ResourceNotFound)) {
			continue // the app does not have this locale
		} else if err != nil {
			return err
		}
		if err := im.locks.take(ctx, "", TranslationsScope(locale)); err != nil {
			return err
		}
		if err := im.edit(ctx, file, func(doc map[string]any) bool { return mergeMessages(doc, from, added) }); err != nil {
			return err
		}
	}
	return nil
}

// mergeMessages adds to a locale document the messages and review states
// of the added keys that from has and doc does not.
func mergeMessages(doc, from map[string]any, added map[string]bool) bool {
	changed := false
	for _, member := range []string{"messages", "states"} {
		src, _ := from[member].(map[string]any)
		dst, _ := doc[member].(map[string]any)
		if dst == nil {
			dst = map[string]any{}
		}
		for id, v := range src {
			if _, has := dst[id]; added[id] && !has {
				dst[id], changed = v, true
			}
		}
		if len(dst) > 0 {
			doc[member] = dst
		}
	}
	return changed
}

// carriesAny reports whether a locale document has a message of the keys.
func carriesAny(doc map[string]any, keys map[string]bool) bool {
	messages, _ := doc["messages"].(map[string]any)
	for id := range messages {
		if keys[id] {
			return true
		}
	}
	return false
}

// allAssets lists every asset of an app.
func (s *Service) allAssets(ctx context.Context, p auth.Principal, appID string) ([]Asset, error) {
	const page = 200
	var out []Asset
	for after := ""; ; {
		rows, err := s.ListAssets(ctx, p, appID, after, page)
		if err != nil {
			return nil, err
		}
		out = append(out, rows...)
		if len(rows) < page {
			return out, nil
		}
		after = rows[len(rows)-1].File
	}
}

// freeName is file, or, when the app uses that name, the name with the
// plugin's key and then a number before its extension.
func freeName(file, key string, taken map[string]bool) string {
	if !taken[file] {
		return file
	}
	ext := path.Ext(file)
	base := strings.TrimSuffix(file, ext) + "-" + key
	name := base + ext
	for i := 2; taken[name]; i++ {
		name = base + "-" + strconv.Itoa(i) + ext
	}
	return name
}

// maxKeyName is the longest translation key name (translation-keys
// schema).
const maxKeyName = 128

// freeKeyName is a taken translation key name moved under the plugin's
// key, numbered when that is taken too, within the longest name.
func freeKeyName(name, key string, taken map[string]bool) string {
	base := key + "." + name
	if len(base) > maxKeyName {
		base = name
	}
	out := base
	for i := 2; taken[out]; i++ {
		suffix := "-" + strconv.Itoa(i)
		out = strings.TrimRight(base[:min(len(base), maxKeyName-len(suffix))], ".-") + suffix
	}
	return out
}

// writeImportedDocuments writes the plugin's documents under its key with
// the identifiers renamed and route names that clash with none of the
// app's, as a duplicate's are written.
func (s *Service) writeImportedDocuments(ctx context.Context, p auth.Principal, created Plugin, srcKey, key, session string, docs []Document, renamed map[string]string) error {
	from, to := "plugins/"+srcKey+"/", "plugins/"+key+"/"
	taken, err := s.appRoutes(ctx, p, created.AppID)
	if err != nil {
		return err
	}
	routes := copyRoutes(docs, from, taken)
	for _, d := range docs {
		content, err := rewriteIDs(d.Content, renamed)
		if err != nil {
			return fmt.Errorf("document: rewrite ids of %s: %w", d.Path, err)
		}
		rel := strings.TrimPrefix(d.Path, from)
		if content, err = retargetRoutes(content, to+rel, routes); err != nil {
			return err
		}
		if rel == "plugin.json" {
			if content, err = renamePluginDocument(content, key, created.Name); err != nil {
				return err
			}
		}
		if _, err := s.PutDocument(ctx, p, created.AppID, created.ID, session, to+rel, content, 0); err != nil {
			return fmt.Errorf("document: import %s: %w", d.Path, err)
		}
	}
	return nil
}
