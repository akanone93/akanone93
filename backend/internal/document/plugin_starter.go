// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package document

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// starterColour is the monogram background of a new plugin; the person changes it in the plugin's settings.
const starterColour = "#5B3DF5"

// CreateStarterPlugin adds a plugin with what makes it a valid plugin at once: its manifest and one blank screen,
// which is its entry page (SCH-021, STU-034). A plugin created without them has no directory in the project, so the
// app failed the layout check (PLX-1020) until someone wrote the manifest by hand. The screen's route is the
// plugin's key when no page uses it, else `<key>-home`, numbered when that is taken too: route names are unique in
// the app (SCH-025). The documents are written under a lock session of the server's own, released after.
func (s *Service) CreateStarterPlugin(ctx context.Context, p auth.Principal, appID, key, name string) (Plugin, error) {
	taken, err := s.appRoutes(ctx, p, appID)
	if err != nil {
		return Plugin{}, err
	}
	created, err := s.createPlugin(ctx, p, appID, key, name, "")
	if err != nil {
		return Plugin{}, err
	}
	session, err := s.newID()
	if err != nil {
		return Plugin{}, err
	}
	if _, _, err := s.AcquireScopedLock(ctx, p, appID, created.ID, session, "", false); err != nil {
		return Plugin{}, err
	}
	released := false
	defer func() {
		if !released {
			_ = s.ReleaseLock(ctx, p, appID, created.ID, session)
		}
	}()
	pluginID, pageID, rootID, err := s.threeIDs()
	if err != nil {
		return Plugin{}, err
	}
	route := starterRoute(key, taken)
	team := key
	if app, err := s.appKey(ctx, p, appID); err == nil && app != "" {
		team = app
	}
	manifest, err := json.Marshal(map[string]any{
		"schemaVersion": "1.0.0", "kind": "plugin", "id": pluginID, "key": key, "name": created.Name, "team": team,
		"entryPage": pageID, "pages": []string{pageID},
		"icon": map[string]any{"monogram": map[string]string{"text": monogram(created.Name), "background": starterColour}},
	})
	if err != nil {
		return Plugin{}, fmt.Errorf("document: write the plugin manifest: %w", err)
	}
	page, err := json.Marshal(map[string]any{
		"schemaVersion": "1.0.0", "kind": "page", "id": pageID, "key": "home", "route": route, "pageKind": "screen",
		"title": created.Name, "root": map[string]string{"id": rootID, "type": "Scaffold"},
	})
	if err != nil {
		return Plugin{}, fmt.Errorf("document: write the first page: %w", err)
	}
	prefix := "plugins/" + key + "/"
	if _, err := s.PutDocument(ctx, p, appID, created.ID, session, prefix+"plugin.json", manifest, 0); err != nil {
		return Plugin{}, err
	}
	if _, err := s.PutDocument(ctx, p, appID, created.ID, session, prefix+"pages/home.page.json", page, 0); err != nil {
		return Plugin{}, err
	}
	released = true
	if err := s.ReleaseLock(ctx, p, appID, created.ID, session); err != nil {
		return Plugin{}, err
	}
	return s.GetPlugin(ctx, p, created.ID)
}

func (s *Service) threeIDs() (string, string, string, error) {
	var ids [3]string
	for i := range ids {
		id, err := s.newID()
		if err != nil {
			return "", "", "", err
		}
		ids[i] = id
	}
	return ids[0], ids[1], ids[2], nil
}

// starterRoute is the first free route of: the plugin's key, `<key>-home`, `<key>-home-2` and so on.
func starterRoute(key string, taken map[string]bool) string {
	if !taken[key] {
		return key
	}
	for n := 1; ; n++ {
		r := key + "-home"
		if n > 1 {
			r += "-" + strconv.Itoa(n)
		}
		if !taken[r] {
			return r
		}
	}
}

// monogram is one to three capital letters: the initials of the first words of the name.
func monogram(name string) string {
	var out []rune
	for _, w := range strings.FieldsFunc(name, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		out = append(out, unicode.ToUpper([]rune(w)[0]))
		if len(out) == 3 {
			break
		}
	}
	if len(out) == 0 {
		return "P"
	}
	return string(out)
}

// appKey is the key of an app the principal can read.
func (s *Service) appKey(ctx context.Context, p auth.Principal, appID string) (string, error) {
	var key string
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		d, err := s.resolve(ctx, dbgen.New(tx), p, appID, "")
		if err != nil {
			return err
		}
		row, err := dbgen.New(tx).GetApp(ctx, d.row.AppID)
		if err != nil {
			return failure(err, "app")
		}
		key = row.Key
		return nil
	})
	return key, err
}
