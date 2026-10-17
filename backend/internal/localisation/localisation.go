// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Package localisation serves the translation workflow of an app: exchange
// with translation tools, progress, and review states (I18N-008, I18N-009,
// ADR-0068).
//
// It holds no storage of its own. Every read and write goes through the
// document service, so the rules that service enforces on a save of a
// translations document (statuses, translations.edit and translations.
// approve, the lock of the locale) apply to everything done here. The file
// formats are the pure xchange package.
package localisation

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/document"
	"github.com/nightCode42/plux3/backend/internal/l10n/xchange"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema"
	"github.com/nightCode42/plux3/backend/internal/schema/jcs"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

// Documents is what the service needs of the document service.
type Documents interface {
	GetDocument(ctx context.Context, p auth.Principal, appID, pluginID, path string) (document.Document, error)
	ListDocuments(ctx context.Context, p auth.Principal, appID, pluginID, afterPath string, size int32, content bool) ([]document.Document, error)
	PutDocument(ctx context.Context, p auth.Principal, appID, pluginID, session, path string, content []byte, ifRevision int64) (document.Written, error)
	AcquireScopedLock(ctx context.Context, p auth.Principal, appID, pluginID, session, scope string, force bool) (document.Lock, string, error)
	ReleaseScopedLock(ctx context.Context, p auth.Principal, appID, pluginID, session, scope string) error
}

// IDs returns fresh identifiers: for a new translations document and for
// the editing session an import holds the lock of a locale under.
type IDs interface{ New() (string, error) }

// Service is the localisation service.
type Service struct {
	docs   Documents
	ids    IDs
	limits limits.Set
}

// New returns the service.
func New(docs Documents, ids IDs, set limits.Set) *Service {
	if set.IsZero() {
		set = limits.Defaults()
	}
	return &Service{docs: docs, ids: ids, limits: set}
}

const (
	keysPath              = "translations/keys.json"
	appPath               = "app.json"
	translationsDir       = "translations/"
	jsonDepth             = 32
	listPage        int32 = 100
)

// localePattern is the locale of the document schema
// (common.schema.json#/$defs/locale).
var localePattern = regexp.MustCompile(`^[a-z]{2,3}(-[A-Z][a-z]{3})?(-([A-Z]{2}|[0-9]{3}))?$`)

func localePath(locale string) string { return translationsDir + locale + ".json" }

func checkLocale(locale string) error {
	if !localePattern.MatchString(locale) {
		return plxerr.New(plxerr.InvalidFormat, "%q is not a locale; use a BCP 47 tag such as de or pt-BR", clip(locale))
	}
	return nil
}

func clip(s string) string {
	if len(s) > 32 {
		return strings.ToValidUTF8(s[:32], "�") + "..."
	}
	return strings.ToValidUTF8(s, "�")
}

// Exported is the result of an export.
type Exported struct {
	Content      []byte
	SourceLocale string
}

// Export writes the translations of a locale in a format (I18N-009). It
// needs app.read.
func (s *Service) Export(ctx context.Context, p auth.Principal, appID, locale, format string) (Exported, error) {
	if err := p.AuthorizeApp(auth.AppRead, appID); err != nil {
		return Exported{}, fmt.Errorf("localisation: %w", err)
	}
	f, err := xchange.ParseFormat(format)
	if err != nil {
		return Exported{}, fmt.Errorf("localisation: %w", err)
	}
	if err := checkLocale(locale); err != nil {
		return Exported{}, err
	}
	w, err := s.load(ctx, p, appID, locale)
	if err != nil {
		return Exported{}, err
	}
	data, err := xchange.Export(w.keys, w.source, w.target, f)
	if err != nil {
		return Exported{}, fmt.Errorf("localisation: %w", err)
	}
	return Exported{Content: data, SourceLocale: w.source.Locale}, nil
}

// Imported is the result of an import.
type Imported struct {
	Diagnostics plxerr.Diagnostics
	ChangedKeys []string
	Saved       bool
}

// Import applies a translation file to a locale (I18N-009). It needs
// translations.edit; the approved state needs translations.approve. With
// dryRun it saves nothing. A save goes through the document service under
// the locale's lock.
func (s *Service) Import(ctx context.Context, p auth.Principal, appID, locale, format string, content []byte, dryRun bool) (Imported, error) {
	if err := p.AuthorizeApp(auth.TranslationsEdit, appID); err != nil {
		return Imported{}, fmt.Errorf("localisation: %w", err)
	}
	f, err := xchange.ParseFormat(format)
	if err != nil {
		return Imported{}, fmt.Errorf("localisation: %w", err)
	}
	if err := checkLocale(locale); err != nil {
		return Imported{}, err
	}
	w, err := s.load(ctx, p, appID, locale)
	if err != nil {
		return Imported{}, err
	}
	opts := xchange.Options{
		MaxBytes: s.limits.Get(limits.TranslationImportSize),
		Approver: p.HoldsOnApp(auth.TranslationsApprove, appID),
	}
	updated, diags, err := xchange.Import(content, f, w.keys, w.target, w.source, opts)
	if err != nil {
		return Imported{}, err //nolint:wrapcheck // a coded domain error
	}
	out := Imported{Diagnostics: diags, ChangedKeys: xchange.Changed(w.target, updated)}
	if dryRun || len(out.ChangedKeys) == 0 {
		return out, nil
	}
	if err := s.save(ctx, p, appID, locale, w, updated); err != nil {
		return Imported{}, err
	}
	out.Saved = true
	return out, nil
}

// SetStates sets the review state of keys of a locale. It needs
// translations.edit, and translations.approve for the approved state.
func (s *Service) SetStates(ctx context.Context, p auth.Principal, appID, locale string, states map[string]string) ([]string, error) {
	if err := p.AuthorizeApp(auth.TranslationsEdit, appID); err != nil {
		return nil, fmt.Errorf("localisation: %w", err)
	}
	if err := checkLocale(locale); err != nil {
		return nil, err
	}
	if len(states) == 0 {
		return nil, nil
	}
	w, err := s.load(ctx, p, appID, locale)
	if err != nil {
		return nil, err
	}
	declared := make(map[string]bool, len(w.keys))
	for _, k := range w.keys {
		declared[k.ID] = true
	}
	updated := w.target.Clone()
	ids := make([]string, 0, len(states))
	for id := range states {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		state := states[id]
		switch {
		case !schema.TranslationState(state).Valid():
			return nil, plxerr.New(plxerr.InvalidEnumValue, "%q is not a review state; use new, translated, reviewed or approved", clip(state))
		case !declared[id]:
			return nil, plxerr.New(plxerr.TranslationImportKeyUnknown, "%s is not a translation key of the app", clip(id))
		case state == string(schema.TranslationStateApproved) && !p.HoldsOnApp(auth.TranslationsApprove, appID):
			return nil, plxerr.New(plxerr.TranslationStateForbidden, "approving %s needs translations.approve", clip(id))
		}
		if _, translated := updated.Messages[id]; !translated {
			return nil, plxerr.New(plxerr.InvalidFormat, "key %s has no translation in %s to give a state", clip(id), locale)
		}
		if state == string(schema.TranslationStateNew) {
			delete(updated.States, id)
		} else {
			updated.States[id] = state
		}
	}
	changed := xchange.Changed(w.target, updated)
	if len(changed) == 0 {
		return nil, nil
	}
	if err := s.save(ctx, p, appID, locale, w, updated); err != nil {
		return nil, err
	}
	return changed, nil
}

// LocaleStatus counts the keys of one locale by review state.
type LocaleStatus struct {
	Locale                                              string
	Total, New, Translated, Reviewed, Approved, Missing int
}

// Status is the progress of an app's translations.
type Status struct {
	SourceLocale string
	Locales      []LocaleStatus
}

// Status counts, for every supported locale and every locale that has a
// translations document, the declared keys by state. It needs app.read.
func (s *Service) Status(ctx context.Context, p auth.Principal, appID string) (Status, error) {
	if err := p.AuthorizeApp(auth.AppRead, appID); err != nil {
		return Status{}, fmt.Errorf("localisation: %w", err)
	}
	app, err := s.appDocument(ctx, p, appID)
	if err != nil {
		return Status{}, err
	}
	keys, err := s.keys(ctx, p, appID)
	if err != nil {
		return Status{}, err
	}
	docs, err := s.localeDocuments(ctx, p, appID)
	if err != nil {
		return Status{}, err
	}
	locales := map[string]xchange.Doc{}
	for _, l := range app.supported {
		locales[l] = xchange.Doc{Locale: l}
	}
	for l, d := range docs {
		locales[l] = d
	}
	out := Status{SourceLocale: app.defaultLocale}
	names := make([]string, 0, len(locales))
	for l := range locales {
		names = append(names, l)
	}
	sort.Strings(names)
	for _, l := range names {
		d := locales[l]
		st := LocaleStatus{Locale: l, Total: len(keys)}
		for _, k := range keys {
			if _, ok := d.Messages[k.ID]; !ok {
				st.Missing++
				continue
			}
			switch d.State(k.ID) {
			case "translated":
				st.Translated++
			case "reviewed":
				st.Reviewed++
			case "approved":
				st.Approved++
			default:
				st.New++
			}
		}
		out.Locales = append(out.Locales, st)
	}
	return out, nil
}

// work is what an import, export or state change reads.
type work struct {
	keys   []xchange.Key
	source xchange.Doc
	target xchange.Doc
	// revision is the stored revision of the target; zero when the locale
	// has no document yet.
	revision int64
	// tree is the stored target document, to keep its other properties.
	tree map[string]any
}

// load reads the keys, the source locale and the target locale.
func (s *Service) load(ctx context.Context, p auth.Principal, appID, locale string) (work, error) {
	app, err := s.appDocument(ctx, p, appID)
	if err != nil {
		return work{}, err
	}
	keys, err := s.keys(ctx, p, appID)
	if err != nil {
		return work{}, err
	}
	target, revision, tree, _, err := s.locale(ctx, p, appID, locale)
	if err != nil {
		return work{}, err
	}
	source := target
	if app.defaultLocale != locale {
		if source, _, _, _, err = s.locale(ctx, p, appID, app.defaultLocale); err != nil {
			return work{}, err
		}
	}
	return work{keys: keys, source: source, target: target, revision: revision, tree: tree}, nil
}

// locale reads the translations document of a locale; a locale without one
// is an empty document.
func (s *Service) locale(ctx context.Context, p auth.Principal, appID, locale string) (xchange.Doc, int64, map[string]any, bool, error) {
	d, err := s.docs.GetDocument(ctx, p, appID, "", localePath(locale))
	if isNotFound(err) {
		return xchange.Doc{Locale: locale, Messages: map[string]string{}, States: map[string]string{}}, 0, nil, false, nil
	}
	if err != nil {
		return xchange.Doc{}, 0, nil, false, fmt.Errorf("localisation: %w", err)
	}
	doc, tree, err := s.parseLocale(d.Content, locale)
	return doc, d.Revision, tree, true, err
}

func (s *Service) parseLocale(content []byte, locale string) (xchange.Doc, map[string]any, error) {
	tree, err := s.object(content)
	if err != nil {
		return xchange.Doc{}, nil, err
	}
	doc := xchange.Doc{Locale: locale, Messages: map[string]string{}, States: map[string]string{}}
	doc.ID, _ = tree["id"].(string)
	for id, v := range asObject(tree["messages"]) {
		if text, ok := v.(string); ok {
			doc.Messages[id] = text
		}
	}
	for id, v := range asObject(tree["states"]) {
		if state, ok := v.(string); ok {
			doc.States[id] = state
		}
	}
	return doc, tree, nil
}

func (s *Service) object(content []byte) (map[string]any, error) {
	tree, err := jcs.Parse(content, jsonDepth)
	if err != nil {
		return nil, fmt.Errorf("localisation: read a stored document: %w", err)
	}
	obj, ok := tree.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("localisation: a stored document is not an object")
	}
	return obj, nil
}

func asObject(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

// appInfo is the part of app.json the service reads.
type appInfo struct {
	defaultLocale string
	supported     []string
}

func (s *Service) appDocument(ctx context.Context, p auth.Principal, appID string) (appInfo, error) {
	d, err := s.docs.GetDocument(ctx, p, appID, "", appPath)
	if err != nil {
		return appInfo{}, fmt.Errorf("localisation: %w", err)
	}
	obj, err := s.object(d.Content)
	if err != nil {
		return appInfo{}, err
	}
	var info appInfo
	info.defaultLocale, _ = obj["defaultLocale"].(string)
	if info.defaultLocale == "" {
		return appInfo{}, plxerr.New(plxerr.InvalidFormat, "the app declares no defaultLocale, so there is no source language")
	}
	list, _ := obj["supportedLocales"].([]any)
	for _, v := range list {
		if l, ok := v.(string); ok && localePattern.MatchString(l) {
			info.supported = append(info.supported, l)
		}
	}
	return info, nil
}

// keys reads the translation keys of the app.
func (s *Service) keys(ctx context.Context, p auth.Principal, appID string) ([]xchange.Key, error) {
	d, err := s.docs.GetDocument(ctx, p, appID, "", keysPath)
	if isNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("localisation: %w", err)
	}
	obj, err := s.object(d.Content)
	if err != nil {
		return nil, err
	}
	list, _ := obj["keys"].([]any)
	out := make([]xchange.Key, 0, len(list))
	for _, v := range list {
		m := asObject(v)
		k := xchange.Key{}
		k.ID, _ = m["id"].(string)
		k.Key, _ = m["key"].(string)
		k.Description, _ = m["description"].(string)
		if n, ok := m["maxLength"].(interface{ Int64() (int64, error) }); ok {
			if i, err := n.Int64(); err == nil && i > 0 && i < 1<<31 {
				k.MaxLength = int(i)
			}
		}
		args, _ := m["args"].([]any)
		for _, a := range args {
			am := asObject(a)
			arg := xchange.Arg{}
			arg.Name, _ = am["name"].(string)
			arg.Type, _ = am["type"].(string)
			k.Args = append(k.Args, arg)
		}
		if k.ID != "" && k.Key != "" {
			out = append(out, k)
		}
	}
	return out, nil
}

// localeDocuments reads every translations document of the app by locale.
func (s *Service) localeDocuments(ctx context.Context, p auth.Principal, appID string) (map[string]xchange.Doc, error) {
	out := map[string]xchange.Doc{}
	after := translationsDir
	for {
		page, err := s.docs.ListDocuments(ctx, p, appID, "", after, listPage, true)
		if err != nil {
			return nil, fmt.Errorf("localisation: %w", err)
		}
		for _, d := range page {
			after = d.Path
			if !strings.HasPrefix(d.Path, translationsDir) {
				return out, nil
			}
			locale := strings.TrimSuffix(strings.TrimPrefix(d.Path, translationsDir), ".json")
			if d.Path == keysPath || !strings.HasSuffix(d.Path, ".json") || !localePattern.MatchString(locale) {
				continue
			}
			doc, _, err := s.parseLocale(d.Content, locale)
			if err != nil {
				return nil, err
			}
			out[locale] = doc
		}
		if int32(len(page)) < listPage {
			return out, nil
		}
	}
}

// save writes the translations of a locale under the locale's lock. The
// document service applies the review rules of I18N-008 to the save.
func (s *Service) save(ctx context.Context, p auth.Principal, appID, locale string, w work, updated xchange.Doc) (err error) {
	tree := w.tree
	if tree == nil {
		id, err := s.ids.New()
		if err != nil {
			return fmt.Errorf("localisation: %w", err)
		}
		tree = map[string]any{"schemaVersion": "1.0.0", "kind": "translations", "id": id, "locale": locale}
	}
	messages := make(map[string]any, len(updated.Messages))
	for id, text := range updated.Messages {
		messages[id] = text
	}
	tree["messages"] = messages
	if len(updated.States) == 0 {
		delete(tree, "states")
	} else {
		states := make(map[string]any, len(updated.States))
		for id, state := range updated.States {
			states[id] = state
		}
		tree["states"] = states
	}
	content, err := jcs.Marshal(tree)
	if err != nil {
		return fmt.Errorf("localisation: write the document: %w", err)
	}
	session, err := s.ids.New()
	if err != nil {
		return fmt.Errorf("localisation: %w", err)
	}
	session = "l10n-" + session
	scope := document.TranslationsScope(locale)
	if _, _, err := s.docs.AcquireScopedLock(ctx, p, appID, "", session, scope, false); err != nil {
		return fmt.Errorf("localisation: lock %s: %w", scope, err)
	}
	defer func() {
		release := s.docs.ReleaseScopedLock(context.WithoutCancel(ctx), p, appID, "", session, scope)
		if release != nil {
			err = errors.Join(err, fmt.Errorf("localisation: release %s: %w", scope, release))
		}
	}()
	if _, err := s.docs.PutDocument(ctx, p, appID, "", session, localePath(locale), content, w.revision); err != nil {
		return fmt.Errorf("localisation: save %s: %w", localePath(locale), err)
	}
	return nil
}

func isNotFound(err error) bool {
	var pe *plxerr.Error
	return errors.As(err, &pe) && pe.Code == plxerr.ResourceNotFound
}
