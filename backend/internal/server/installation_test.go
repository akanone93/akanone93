// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/config"
)

// Verifies: STU-116, SRV-007.
// The installation's health lists every check by name in a stable order and
// reports only the fact of a failure, never the error text, which may quote
// a connection string.
func TestHealthResultsNameChecksAndHideErrors_STU_116(t *testing.T) {
	t.Parallel()
	h := NewHealth()
	h.Register("database", func(context.Context) error { return errors.New("postgres://user:secret@db/plux refused") })
	h.Register("cache", func(context.Context) error { return nil })

	got := installationReader{health: h}.Health(t.Context())
	if len(got) != 2 || got[0].Name != "cache" || !got[0].Healthy || got[1].Name != "database" || got[1].Healthy {
		t.Fatalf("Health = %+v", got)
	}
	for _, r := range got {
		if strings.Contains(r.Name, "secret") {
			t.Errorf("a result leaked an error text: %+v", r)
		}
	}
}

// Verifies: STU-116.
// The trusted CI issuers are the configured ones, with no credential, and a
// reader without a job queue reports none.
func TestInstallationReaderIssuersAndJobs_STU_116(t *testing.T) {
	t.Parallel()
	r := installationReader{issuers: []config.CIIssuer{{Issuer: "https://ci.example.org", Audience: "plux", SubjectPattern: "repo:acme/*"}}}
	got := r.Issuers()
	if len(got) != 1 || got[0].Issuer != "https://ci.example.org" || got[0].Audience != "plux" || got[0].SubjectPattern != "repo:acme/*" {
		t.Errorf("Issuers = %+v", got)
	}
	depths, err := r.Jobs(t.Context())
	if err != nil || len(depths) != 0 {
		t.Errorf("Jobs without a queue = %+v, %v", depths, err)
	}
}
