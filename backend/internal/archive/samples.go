// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package archive

import (
	"embed"
	"fmt"
	"slices"
	"strings"

	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

// samplesFS holds the sample apps (STU-015, F17): .plux archives built from
// the reference apps' projects and embedded in the server image, so that
// they work without a network. A test rebuilds each from its source project
// and compares bytes, so a sample cannot go stale unnoticed.
//
//go:embed samples/*.plux
var samplesFS embed.FS

// Sample is one embedded sample app.
type Sample struct {
	// ID is the archive's file name without .plux.
	ID          string
	Key         string
	Name        string
	Description string
	Data        []byte
	SHA256      string
}

// Embedded returns the embedded sample apps in ID order. Each is read as an
// untrusted archive would be, with the default limits.
func Embedded() ([]Sample, error) {
	entries, err := samplesFS.ReadDir("samples")
	if err != nil {
		return nil, fmt.Errorf("archive: read the samples: %w", err)
	}
	var out []Sample
	for _, e := range entries {
		data, err := samplesFS.ReadFile("samples/" + e.Name())
		if err != nil {
			return nil, fmt.Errorf("archive: read %s: %w", e.Name(), err)
		}
		m, _, err := Unpack(data, LimitsOf(limits.Defaults()))
		if err != nil {
			return nil, fmt.Errorf("archive: sample %s: %w", e.Name(), err)
		}
		out = append(out, Sample{
			ID: strings.TrimSuffix(e.Name(), ".plux"), Key: m.App.Key, Name: m.App.Name, Description: m.App.Description,
			Data: data, SHA256: sum(data),
		})
	}
	slices.SortFunc(out, func(a, b Sample) int { return strings.Compare(a.ID, b.ID) })
	return out, nil
}
