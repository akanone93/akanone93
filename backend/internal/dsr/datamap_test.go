// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package dsr

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// migrationsDir is where the schema lives, relative to this package.
const migrationsDir = "../storage/migrations"

var (
	commentPattern = regexp.MustCompile(`--[^\n]*`)
	tablePattern   = regexp.MustCompile(`(?s)CREATE TABLE (\w+) \((.*?)\n\)`)
	alterPattern   = regexp.MustCompile(`ALTER TABLE (\w+) ADD COLUMN (\w+)`)
	columnPattern  = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	// personalColumn matches the column names that mean personal data:
	// a device or user reference, a user hash, an address, a user agent, a
	// push token, an email, a locale or the subject of a credential.
	personalColumn = regexp.MustCompile(`^(device_id|user_id|user_hash|subject|subject_id|subject_hash|source_ip|ip_\w+|user_agent|push_token|email|locale)$`)
)

// schemaColumns parses the migrations into the columns of every table.
func schemaColumns(t *testing.T) map[string]map[string]bool {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(migrationsDir, "*.sql"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no migrations found: %v", err)
	}
	sort.Strings(files)
	tables := map[string]map[string]bool{}
	for _, f := range files {
		raw, err := os.ReadFile(f) //nolint:gosec // a migration of this repository
		if err != nil {
			t.Fatal(err)
		}
		sql := commentPattern.ReplaceAllString(string(raw), "")
		for _, m := range tablePattern.FindAllStringSubmatch(sql, -1) {
			cols := map[string]bool{}
			for _, line := range strings.Split(m[2], "\n") {
				if fields := strings.Fields(line); len(fields) > 0 && columnPattern.MatchString(strings.TrimSuffix(fields[0], ",")) {
					cols[strings.TrimSuffix(fields[0], ",")] = true
				}
			}
			tables[m[1]] = cols
		}
		for _, m := range alterPattern.FindAllStringSubmatch(sql, -1) {
			if tables[m[1]] != nil {
				tables[m[1]][m[2]] = true
			}
		}
	}
	return tables
}

// TestPersonalDataMap_Complete_SEC_162 fails when a migration gives a table a
// column named like personal data and the map does not list the table, so a
// new store cannot escape export and erasure unnoticed.
//
// Verifies: SEC-162.
func TestPersonalDataMap_Complete_SEC_162(t *testing.T) {
	listed := map[string]bool{}
	for _, s := range Map() {
		listed[s.Name] = true
	}
	for table, cols := range schemaColumns(t) {
		for col := range cols {
			if personalColumn.MatchString(col) && !listed[table] {
				t.Errorf("table %s has column %s, which is named like personal data, but the personal-data map (datamap.go) does not list it", table, col)
			}
		}
	}
}

// TestPersonalDataMap_Consistent_SEC_162 checks the map against the schema
// and the executors: every store is a table, every key a column of it, every
// erasable store has an executor and every executor a store.
//
// Verifies: SEC-162.
func TestPersonalDataMap_Consistent_SEC_162(t *testing.T) {
	schema := schemaColumns(t)
	byName := map[string]Store{}
	for _, s := range Map() {
		if _, dup := byName[s.Name]; dup {
			t.Errorf("store %s is listed twice", s.Name)
		}
		byName[s.Name] = s
		cols, ok := schema[s.Name]
		if !ok {
			t.Errorf("store %s is not a table of the schema", s.Name)
			continue
		}
		for _, key := range []string{s.DeviceKey, s.UserKey} {
			if key != "" && !cols[key] {
				t.Errorf("store %s: key %s is not one of its columns", s.Name, key)
			}
		}
		if s.Retention == "" || s.Description == "" {
			t.Errorf("store %s needs a description and a retention", s.Name)
		}
		if s.Erase == EraseRetained && s.Note == "" {
			t.Errorf("store %s is retained and must say why", s.Name)
		}
	}
	executed := map[string]bool{}
	for _, kind := range []Kind{KindDevice, KindUser} {
		for _, x := range executors(kind) {
			executed[x.store] = true
			s, ok := byName[x.store]
			switch {
			case !ok:
				t.Errorf("executor %s is not in the map", x.store)
			case s.Erase != EraseDelete:
				t.Errorf("executor %s is for a store the map does not erase", x.store)
			case kind == KindDevice && s.DeviceKey == "", kind == KindUser && s.UserKey == "":
				t.Errorf("executor %s has no key for a %s", x.store, kind)
			}
		}
	}
	for name, s := range byName {
		if s.Erase == EraseDelete && !executed[name] {
			t.Errorf("store %s is erased by the map but has no executor", name)
		}
	}
}

// TestPersonalDataMap_DocumentCurrent_SEC_162 fails when the committed
// docs/operations/personal-data.md is not what the map renders; run
// `make gen` to refresh it.
//
// Verifies: SEC-162.
func TestPersonalDataMap_DocumentCurrent_SEC_162(t *testing.T) {
	got, err := os.ReadFile("../../../docs/operations/personal-data.md")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != Render() {
		t.Fatal("docs/operations/personal-data.md is stale; run `make gen`")
	}
}

// TestPersonalDataMap_MapIsACopy_SEC_162 checks that changing the returned
// map cannot change the registry.
//
// Verifies: SEC-162.
func TestPersonalDataMap_MapIsACopy_SEC_162(t *testing.T) {
	first := Map()
	first[0].Name = "changed"
	if Map()[0].Name == "changed" {
		t.Fatal("Map returned shared state")
	}
}
