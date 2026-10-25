// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package document

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/compiler"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
	"github.com/nightCode42/plux3/backend/internal/tenancy"
)

// UpdatePluginMetadata changes a plugin's name, description, icon, owning
// team, tags and archived state (plan P11 §4.3 #13, STU-034). An empty name
// keeps the stored one.
func (s *Service) UpdatePluginMetadata(ctx context.Context, p auth.Principal, pluginID, name string, m tenancy.Metadata) (Plugin, error) {
	name = strings.TrimSpace(name)
	if len(name) > 200 {
		return Plugin{}, plxerr.New(plxerr.InvalidFormat, "a plugin name is 1 to 200 characters")
	}
	args, err := tenancy.CheckMetadata(m)
	if err != nil {
		return Plugin{}, fmt.Errorf("document: %w", err)
	}
	var out Plugin
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		d, err := s.plugin(ctx, q, p, pluginID)
		if err != nil {
			return err
		}
		if err := authorize(p, auth.AppManage, d.app); err != nil {
			return err
		}
		row, err := q.UpdatePluginMetadata(ctx, dbgen.UpdatePluginMetadataParams{
			ID: d.plugin.ID, Name: name, SetDescription: args.SetDescription, Description: args.Description,
			SetIcon: args.SetIcon, IconAssetID: args.Icon, SetTeam: args.SetTeam, OwningTeamID: args.Team,
			SetTags: m.SetTags, Tags: args.Tags, SetArchived: args.SetArchived, Archived: args.Archived,
		})
		if err != nil {
			return failure(err, "plugin")
		}
		out = pluginOf(row, d.row)
		return s.record(ctx, tx, p, audit.PluginUpdated, "plugin", pluginID, "", "")
	})
	return out, err
}

// DuplicatePlugin copies a plugin's draft documents under a new key. Every
// document keeps its content except for its path and the identifiers of the
// documents themselves, which are new so that the copy refers to its own
// pages. The new plugin's lock is taken by session while the copy is
// written and released after (STU-034).
func (s *Service) DuplicatePlugin(ctx context.Context, p auth.Principal, sourceID, key, name, session string) (Plugin, error) {
	src, err := s.GetPlugin(ctx, p, sourceID)
	if err != nil {
		return Plugin{}, err
	}
	docs, err := s.allDocuments(ctx, p, src)
	if err != nil {
		return Plugin{}, err
	}
	created, err := s.CreatePlugin(ctx, p, src.AppID, key, name)
	if err != nil {
		return Plugin{}, err
	}
	if _, _, err := s.AcquireScopedLock(ctx, p, src.AppID, created.ID, session, "", false); err != nil {
		return Plugin{}, err
	}
	released := false
	defer func() {
		if !released {
			_ = s.ReleaseLock(ctx, p, src.AppID, created.ID, session)
		}
	}()
	if err := s.copyDocuments(ctx, p, src, created, key, session, docs); err != nil {
		return Plugin{}, err
	}
	released = true
	if err := s.ReleaseLock(ctx, p, src.AppID, created.ID, session); err != nil {
		return Plugin{}, err
	}
	return s.GetPlugin(ctx, p, created.ID)
}

// allDocuments reads every document of a plugin's draft with its content.
func (s *Service) allDocuments(ctx context.Context, p auth.Principal, src Plugin) ([]Document, error) {
	var docs []Document
	for after := ""; ; {
		page, err := s.ListDocuments(ctx, p, src.AppID, src.ID, after, 200, true)
		if err != nil {
			return nil, err
		}
		docs = append(docs, page...)
		if len(page) < 200 {
			return docs, nil
		}
		after = page[len(page)-1].Path
	}
}

// copyDocuments writes the source's documents into the new plugin under its
// key, with new document identifiers and the plugin document renamed.
func (s *Service) copyDocuments(ctx context.Context, p auth.Principal, src, created Plugin, key, session string, docs []Document) error {
	renamed, err := s.freshIDs(docs)
	if err != nil {
		return err
	}
	from, to := "plugins/"+src.Key+"/", "plugins/"+key+"/"
	for _, d := range docs {
		if !strings.HasPrefix(d.Path, from) {
			continue
		}
		content := d.Content
		for old, fresh := range renamed {
			content = bytes.ReplaceAll(content, []byte(old), []byte(fresh))
		}
		rel := strings.TrimPrefix(d.Path, from)
		if rel == "plugin.json" {
			if content, err = renamePluginDocument(content, key, created.Name); err != nil {
				return err
			}
		}
		if _, err := s.PutDocument(ctx, p, src.AppID, created.ID, session, to+rel, content, 0); err != nil {
			return fmt.Errorf("document: copy %s: %w", d.Path, err)
		}
	}
	return nil
}

// renamePluginDocument sets the key and name of a plugin document.
func renamePluginDocument(content []byte, key, name string) ([]byte, error) {
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(content, &doc); err != nil {
		return nil, fmt.Errorf("document: read the plugin document: %w", err)
	}
	doc["key"], _ = json.Marshal(key)
	doc["name"], _ = json.Marshal(name)
	out, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("document: write the plugin document: %w", err)
	}
	return out, nil
}

// freshIDs maps the top-level "id" of every document to a new identifier.
func (s *Service) freshIDs(docs []Document) (map[string]string, error) {
	out := map[string]string{}
	for _, d := range docs {
		var head struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(d.Content, &head) != nil || head.ID == "" {
			continue
		}
		if _, ok := out[head.ID]; ok {
			continue
		}
		id, err := s.newID()
		if err != nil {
			return nil, err
		}
		out[head.ID] = id
	}
	return out, nil
}

// PluginSummary is what the app canvas shows of one plugin.
type PluginSummary struct {
	Plugin             Plugin
	Screens            int64
	Versions           int64
	UnpublishedChanges bool
	LockHolder         string
	ValidationErrors   int64
	Killed             bool
	BundleSize         int64
	LinksOut           []string
	LinksIn            []string
	FunctionsUsed      []string
	PublishedAt        time.Time
}

// ListPluginSummaries returns one summary per live plugin of an app in key
// order, in one read: the plugin rows from the database, the validation
// errors, links and functions from one compilation of the draft. The count
// of plugins is bounded by app.plugins (LIM-001), so the answer is too.
func (s *Service) ListPluginSummaries(ctx context.Context, p auth.Principal, appID string) ([]PluginSummary, error) {
	var out []PluginSummary
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		d, err := s.resolve(ctx, q, p, appID, "")
		if err != nil {
			return err
		}
		if err := authorize(p, auth.PluginRead, appID); err != nil {
			return err
		}
		lim, err := s.limitsFor(ctx, tx, d)
		if err != nil {
			return err
		}
		rows, err := q.SummarisePlugins(ctx, dbgen.SummarisePluginsParams{AppID: d.row.AppID, MaxRows: int32(lim.Get(limits.AppPlugins))}) //nolint:gosec // a registry limit
		if err != nil {
			return failure(err, "plugin")
		}
		killed, err := q.ListKilledPlugins(ctx, d.row.AppID)
		if err != nil {
			return failure(err, "plugin")
		}
		owners, err := q.ListPageEntities(ctx, d.row.AppID)
		if err != nil {
			return failure(err, "document")
		}
		keyByID := map[string]string{}
		for _, r := range rows {
			keyByID[storage.ID(r.ID)] = r.Key
		}
		pageOwner := map[string]string{}
		for _, o := range owners {
			pageOwner[storage.ID(o.EntityID)] = keyByID[storage.ID(o.PluginID)]
		}
		fsys, err := s.project(ctx, q, d)
		if err != nil {
			return err
		}
		facts := factsOf(compiler.Compile(fsys, s.compileOptions(lim)), pageOwner)
		for _, r := range rows {
			out = append(out, summaryOf(r, appID, slices.Contains(killed, r.Key), facts))
		}
		return nil
	})
	return out, err
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func uniqueSorted(in []string) []string {
	out := slices.Clone(in)
	slices.Sort(out)
	return slices.Compact(out)
}

// compiled holds what one compilation says about each plugin, by plugin key.
type compiled struct {
	errors    map[string]int64
	out, in   map[string][]string
	functions map[string][]string
}

// factsOf reads the error counts, the links between plugins and the
// functions used from a compilation. pageOwner maps a page to its plugin.
func factsOf(res *compiler.Result, pageOwner map[string]string) compiled {
	c := compiled{errors: map[string]int64{}, out: map[string][]string{}, in: map[string][]string{}, functions: map[string][]string{}}
	for _, diag := range res.Diagnostics {
		if diag.Severity == plxerr.SeverityError {
			c.errors[pluginKeyOf(diag.File)]++
		}
	}
	if res.Graph == nil {
		return c
	}
	for _, e := range res.Graph.Edges {
		from := pluginKeyOf(e.File)
		switch e.Kind {
		case compiler.EdgeNavigatesPlugin:
			if to := pageOwner[e.To]; to != "" && from != "" && to != from {
				c.out[from] = append(c.out[from], to)
				c.in[to] = append(c.in[to], from)
			}
		case compiler.EdgeUsesFunction:
			c.functions[from] = append(c.functions[from], e.To)
		}
	}
	return c
}

// summaryOf builds one plugin's summary from its row and the compilation.
func summaryOf(r dbgen.SummarisePluginsRow, appID string, killed bool, facts compiled) PluginSummary {
	sum := PluginSummary{
		Plugin: Plugin{
			ID: storage.ID(r.ID), AppID: appID, Key: r.Key, Name: r.Name, LatestVersion: r.LatestVersion,
			DraftRevision: r.DraftRevision, Archived: r.ArchivedAt.Valid, OwningTeamID: storage.ID(r.OwningTeamID),
		},
		Screens: r.Screens, Versions: r.LatestVersion, LockHolder: deref(r.LockHolder), BundleSize: r.BundleSize,
		Killed: killed, ValidationErrors: facts.errors[r.Key],
		LinksOut: uniqueSorted(facts.out[r.Key]), LinksIn: uniqueSorted(facts.in[r.Key]),
		FunctionsUsed: uniqueSorted(facts.functions[r.Key]), PublishedAt: storage.Time(r.PublishedAt),
	}
	if r.PublishedAt.Valid {
		sum.UnpublishedChanges = r.DraftUpdatedAt.Time.After(r.PublishedAt.Time)
	} else {
		sum.UnpublishedChanges = r.DraftRevision > 0
	}
	return sum
}
