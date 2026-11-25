// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package document_test

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/compiler/media"
	"github.com/nightCode42/plux3/backend/internal/document"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/objects"
)

// The identifiers of the plugin archive test.
const (
	greetingKey = "01a0c450-6c00-7031-8000-000000031001"
	unusedKey   = "01a0c450-6c00-7031-8000-000000031002"
	cardsPlugin = "01a0c450-6c00-7031-8000-000000031010"
	cardsPage   = "01a0c450-6c00-7031-8000-000000031011"
	cardsText   = "01a0c450-6c00-7031-8000-000000031012"
	takenKey    = "01a0c450-6c00-7031-8000-000000031032"
)

func keysJSON(id string, keys ...[2]string) []byte {
	var list []string
	for _, k := range keys {
		list = append(list, `{"id":"`+k[0]+`","key":"`+k[1]+`","description":"A message."}`)
	}
	return []byte(`{"schemaVersion":"1.0.0","kind":"translationKeys","id":"` + id + `","keys":[` + strings.Join(list, ",") + `]}`)
}

func localeJSON(id, messages, states string) []byte {
	return []byte(`{"schemaVersion":"1.0.0","kind":"translations","id":"` + id + `","locale":"en","messages":{` + messages + `},"states":{` + states + `}}`)
}

func cardsJSON(icon string) []byte {
	return []byte(`{"schemaVersion":"1.0.0","kind":"plugin","id":"` + cardsPlugin + `","key":"cards","name":"Cards",` +
		`"icon":{"$asset":"` + icon + `"},"team":"lending","entryPage":"` + cardsPage + `","pages":["` + cardsPage + `"]}`)
}

func cardsPageJSON() []byte {
	return []byte(`{"schemaVersion":"1.0.0","kind":"page","id":"` + cardsPage + `","key":"cards","route":"cards","pageKind":"screen","title":"Cards",` +
		`"root":{"id":"` + cardsText + `","type":"Text","props":{"data":{"$t":"` + greetingKey + `"}}}}`)
}

// put writes an app-level document under session's lock.
func (f *fixture) put(t *testing.T, appID, session, path string, content []byte) {
	t.Helper()
	if _, err := f.docs.PutDocument(context.Background(), f.owner, appID, "", session, path, content, 0); err != nil {
		t.Fatalf("PutDocument %s: %v", path, err)
	}
}

func fileOf(files []document.File, path string) []byte {
	for _, f := range files {
		if f.Path == path {
			return f.Content
		}
	}
	return nil
}

func doc(t *testing.T, f *fixture, appID, pluginID, path string) string {
	t.Helper()
	d, err := f.docs.GetDocument(context.Background(), f.viewer, appID, pluginID, path)
	if err != nil {
		t.Fatalf("GetDocument %s: %v", path, err)
	}
	return string(d.Content)
}

// Verifies: STU-023, STU-034.
// A plugin exports with only the translation keys, messages, review
// states and assets it names, and imports into another app beside what
// that app has: its documents get new identifiers, an asset whose name
// the app uses gets a name of its own and one whose content the app has
// is reused, a key the app has is reused and one whose name it uses is
// renamed under the plugin's key, and the import gives back only the
// locks it took (build log row 264).
func TestPluginArchive(t *testing.T) {
	t.Parallel()
	store, err := objects.NewFilesystem(t.TempDir(), "https://cdn.example")
	if err != nil {
		t.Fatal(err)
	}
	codecs, err := media.NewCodecs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = codecs.Close(context.Background()) })
	addr := clamd(t)
	f := newFixture(t, func(o *document.Options) {
		o.Objects, o.Jobs, o.Codecs = store, &jobs{}, codecs
		o.Scanner = document.Clamd{Network: "tcp", Address: addr}
	})
	ctx := context.Background()

	// The source app: a logo, two keys of which the plugin uses one, and
	// the plugin.
	const src = "src"
	if _, _, err := f.docs.AcquireLock(ctx, f.owner, f.app, "", src, false); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.docs.AcquireScopedLock(ctx, f.owner, f.app, "", src, document.TranslationsScope("en"), false); err != nil {
		t.Fatal(err)
	}
	logo := pngWithText(t, 20, 20)
	icon, err := f.docs.UploadAsset(ctx, f.owner, f.app, "", src, "images/logo.png", logo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.docs.UploadAsset(ctx, f.owner, f.app, "", src, "images/other.png", pngWithText(t, 10, 10)); err != nil {
		t.Fatal(err)
	}
	f.put(t, f.app, src, "translations/keys.json", keysJSON("01a0c450-6c00-7031-8000-000000031020", [2]string{greetingKey, "greeting"}, [2]string{unusedKey, "unused"}))
	f.put(t, f.app, src, "translations/en.json", localeJSON("01a0c450-6c00-7031-8000-000000031021",
		`"`+greetingKey+`":"Hello","`+unusedKey+`":"Unused"`, `"`+greetingKey+`":"reviewed","`+unusedKey+`":"new"`))
	pl, err := f.docs.CreatePlugin(ctx, f.owner, f.app, "cards", "Cards")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.docs.AcquireLock(ctx, f.owner, f.app, pl.ID, src, false); err != nil {
		t.Fatal(err)
	}
	for path, content := range map[string][]byte{"plugins/cards/plugin.json": cardsJSON(icon.ID), "plugins/cards/pages/cards.page.json": cardsPageJSON()} {
		if _, err := f.docs.PutDocument(ctx, f.owner, f.app, pl.ID, src, path, content, 0); err != nil {
			t.Fatalf("PutDocument %s: %v", path, err)
		}
	}

	// The export holds what the plugin names, and nothing else of the app.
	exported, files, err := f.docs.ExportPlugin(ctx, f.viewer, pl.ID)
	if err != nil || exported.Key != "cards" {
		t.Fatalf("ExportPlugin: %+v %v", exported, err)
	}
	var paths []string
	for _, file := range files {
		paths = append(paths, file.Path)
	}
	want := []string{"assets/images/logo.png", "assets/index.json", "plugins/cards/pages/cards.page.json", "plugins/cards/plugin.json", "translations/en.json", "translations/keys.json"}
	slices.Sort(paths)
	if !slices.Equal(paths, want) {
		t.Fatalf("exported %v, want %v", paths, want)
	}
	for path, absent := range map[string]string{"translations/keys.json": unusedKey, "translations/en.json": "Unused", "assets/index.json": "other.png"} {
		if strings.Contains(string(fileOf(files, path)), absent) {
			t.Errorf("%s carries %s, which the plugin does not name", path, absent)
		}
	}
	if !strings.Contains(string(fileOf(files, "translations/en.json")), "reviewed") {
		t.Errorf("the message's review state is not exported: %s", fileOf(files, "translations/en.json"))
	}

	// The target app has another file under the logo's name, a key under
	// the greeting's name, and English; its session holds the app's lock.
	b, err := f.tenancy.CreateApp(ctx, f.owner, "bank", "Bank")
	if err != nil {
		t.Fatal(err)
	}
	const tgt = "tgt"
	if _, _, err := f.docs.AcquireLock(ctx, f.owner, b.ID, "", tgt, false); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.docs.AcquireScopedLock(ctx, f.owner, b.ID, "", tgt, document.TranslationsScope("en"), false); err != nil {
		t.Fatal(err)
	}
	if _, err := f.docs.UploadAsset(ctx, f.owner, b.ID, "", tgt, "images/logo.png", pngWithText(t, 12, 12)); err != nil {
		t.Fatal(err)
	}
	f.put(t, b.ID, tgt, "translations/keys.json", keysJSON("01a0c450-6c00-7031-8000-000000031030", [2]string{takenKey, "greeting"}))
	f.put(t, b.ID, tgt, "translations/en.json", localeJSON("01a0c450-6c00-7031-8000-000000031031", `"`+takenKey+`":"Hi"`, ""))
	if err := f.docs.ReleaseScopedLock(ctx, f.owner, b.ID, "", tgt, document.TranslationsScope("en")); err != nil {
		t.Fatal(err)
	}

	if _, err := f.docs.ImportPlugin(ctx, f.viewer, b.ID, "cards", "cards", "Cards", tgt, files); code(err) != plxerr.PermissionDenied {
		t.Errorf("a viewer imported a plugin: %v", err)
	}
	in, err := f.docs.ImportPlugin(ctx, f.owner, b.ID, "cards", "cards", "Cards", tgt, files)
	if err != nil {
		t.Fatalf("ImportPlugin: %v", err)
	}
	if in.Documents != 2 || in.Keys != 1 || in.Assets != 1 || in.Plugin.Key != "cards" {
		t.Errorf("imported %+v", in)
	}
	if diags, err := f.docs.ValidateDraft(ctx, f.owner, b.ID, in.Plugin.ID); err != nil || diags.Count(plxerr.SeverityError) != in.ErrorDiagnostics {
		t.Errorf("the import counts %d errors; validation finds %v %v", in.ErrorDiagnostics, diags, err)
	}
	assets, err := f.docs.ListAssets(ctx, f.viewer, b.ID, "", 10)
	if err != nil || len(assets) != 2 {
		t.Fatalf("the target's assets: %+v %v", assets, err)
	}
	var moved document.Asset
	for _, a := range assets {
		if a.File == "images/logo-cards.png" {
			moved = a
		}
	}
	if moved.ID == "" || moved.ID == icon.ID {
		t.Fatalf("the logo was not added under a name of its own: %+v", assets)
	}
	plugin := doc(t, f, b.ID, in.Plugin.ID, "plugins/cards/plugin.json")
	page := doc(t, f, b.ID, in.Plugin.ID, "plugins/cards/pages/cards.page.json")
	if !strings.Contains(plugin, moved.ID) || strings.Contains(plugin, icon.ID) || strings.Contains(plugin, cardsPlugin) {
		t.Errorf("the plugin keeps the source's identifiers: %s", plugin)
	}
	if !strings.Contains(page, greetingKey) || strings.Contains(page, cardsPage) {
		t.Errorf("the page: %s", page)
	}
	var keys struct {
		Keys []struct{ ID, Key string } `json:"keys"`
	}
	if err := json.Unmarshal([]byte(doc(t, f, b.ID, "", "translations/keys.json")), &keys); err != nil || len(keys.Keys) != 2 ||
		keys.Keys[1].ID != greetingKey || keys.Keys[1].Key != "cards.greeting" {
		t.Errorf("the target's keys: %+v %v", keys, err)
	}
	en := doc(t, f, b.ID, "", "translations/en.json")
	if !strings.Contains(en, `"`+greetingKey+`":"Hello"`) || !strings.Contains(en, `"`+greetingKey+`":"reviewed"`) || !strings.Contains(en, `"Hi"`) {
		t.Errorf("the target's English: %s", en)
	}

	// The import gave back the locks it took and kept the one its session held.
	if l, err := f.docs.GetLock(ctx, f.owner, b.ID, ""); err != nil || l.Session != tgt {
		t.Errorf("the app's lock: %+v %v", l, err)
	}
	if l, err := f.docs.GetLock(ctx, f.owner, b.ID, in.Plugin.ID); err != nil || l.Session != "" {
		t.Errorf("the new plugin's lock: %+v %v", l, err)
	}
	if l, err := f.docs.GetScopedLock(ctx, f.owner, b.ID, "", document.TranslationsScope("en")); err != nil || l.Session != "" {
		t.Errorf("the locale's lock: %+v %v", l, err)
	}

	// Again under the same key is refused before anything is written;
	// under another, the logo and the key the app now has are reused.
	if _, err := f.docs.ImportPlugin(ctx, f.owner, b.ID, "cards", "cards", "Cards", tgt, files); code(err) != plxerr.ResourceExists {
		t.Errorf("a second plugin with the key: %v", err)
	}
	again, err := f.docs.ImportPlugin(ctx, f.owner, b.ID, "cards", "cards-2", "Cards 2", tgt, files)
	if err != nil || again.Assets != 0 || again.Keys != 0 || again.Documents != 2 {
		t.Fatalf("a second import: %+v %v", again, err)
	}
	if page2 := doc(t, f, b.ID, again.Plugin.ID, "plugins/cards-2/pages/cards.page.json"); strings.Contains(page2, `"route":"cards"`) {
		t.Errorf("the second copy's route clashes with the first: %s", page2)
	}
}

// Verifies: STU-023.
// A plugin archive is untrusted: a document that fails its schema, a
// file outside the plugin and the app-level documents it may carry, an
// asset file its index does not list, and an index entry without its
// file each refuse the import before anything is written.
func TestPluginArchiveRefusals(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	good := []document.File{{Path: "plugins/cards/plugin.json", Content: cardsJSON("01a0c450-6c00-7031-8000-000000031099")}}
	for _, c := range []struct {
		name  string
		files []document.File
		want  plxerr.Code
	}{
		{"no plugin.json", []document.File{{Path: "plugins/cards/pages/cards.page.json", Content: cardsPageJSON()}}, plxerr.InvalidProjectLayout},
		{"another plugin", append(slices.Clone(good), document.File{Path: "plugins/other/plugin.json", Content: pluginJSON("other", "Other")}), plxerr.InvalidProjectLayout},
		{"app document", append(slices.Clone(good), document.File{Path: "app.json", Content: []byte(`{}`)}), plxerr.InvalidProjectLayout},
		{"bad schema", []document.File{{Path: "plugins/cards/plugin.json", Content: []byte(`{"kind":"plugin"}`)}}, 0},
		{"unlisted asset", append(slices.Clone(good), document.File{Path: "assets/x.png", Content: []byte("x")}), plxerr.InvalidProjectLayout},
		{"locale named after another", append(slices.Clone(good), document.File{Path: "translations/de.json", Content: localeJSON("01a0c450-6c00-7031-8000-000000031097", "", "")}), plxerr.InvalidProjectLayout},
		{"per-locale file not listed", append(slices.Clone(good), document.File{Path: "assets/logo.png", Content: []byte("x")}, document.File{Path: "assets/logo-de.png", Content: []byte("y")},
			document.File{Path: "assets/index.json", Content: []byte(`{"schemaVersion":"1.0.0","kind":"assetIndex","id":"01a0c450-6c00-7031-8000-000000031098","assets":[` +
				`{"id":"01a0c450-6c00-7031-8000-000000031099","key":"logo","file":"logo.png","mediaType":"image/png","locales":{"de":"logo-de.png"}}]}`)}), plxerr.InvalidProjectLayout},
		{"asset without file", append(slices.Clone(good), document.File{Path: "assets/index.json", Content: []byte(
			`{"schemaVersion":"1.0.0","kind":"assetIndex","id":"01a0c450-6c00-7031-8000-000000031098","assets":[{"id":"01a0c450-6c00-7031-8000-000000031099","key":"logo","file":"logo.png","mediaType":"image/png"}]}`)}), plxerr.InvalidProjectLayout},
	} {
		_, err := f.docs.ImportPlugin(ctx, f.owner, f.app, "cards", "cards", "Cards", "imp", c.files)
		if err == nil || c.want != 0 && code(err) != c.want {
			t.Errorf("%s: %v, want %v", c.name, err, c.want)
		}
		if plugins, _ := f.docs.ListPlugins(ctx, f.owner, f.app, storage.Cursor{}, 10); len(plugins) != 0 {
			t.Fatalf("%s left a plugin: %v", c.name, err)
		}
	}
	plugins, err := f.docs.ListPlugins(ctx, f.owner, f.app, storage.Cursor{}, 10)
	if err != nil || len(plugins) != 0 {
		t.Errorf("a refused import left %d plugins: %v", len(plugins), err)
	}
}
