// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api_test

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/api"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

type fakeInstallation struct{}

func (fakeInstallation) Health(context.Context) []api.HealthResult {
	return []api.HealthResult{{Name: "cache", Healthy: true}, {Name: "database", Healthy: false}}
}
func (fakeInstallation) DatabaseBytes(context.Context) (int64, error) { return 4096, nil }
func (fakeInstallation) Jobs(context.Context) ([]api.JobDepth, error) {
	return []api.JobDepth{{Queue: "publish", Count: 3}}, nil
}

func (fakeInstallation) Issuers() []api.TrustedIssuer {
	return []api.TrustedIssuer{{Issuer: "https://ci.example.org", Audience: "plux", SubjectPattern: "repo:acme/*"}}
}

func asPerson(admin bool) context.Context {
	return api.WithIdentity(context.Background(), auth.Identity{Kind: auth.KindUser, ID: "u1", InstallationAdmin: admin})
}

// Verifies: STU-116.
// An installation administrator reads the health, the database size, the
// queue depths and the trusted CI issuers; the answers carry names, sizes
// and counts only.
func TestInstallationServiceReportsToAnAdministrator_STU_116(t *testing.T) {
	t.Parallel()
	svc := (&api.Handlers{InstallationState: fakeInstallation{}}).Installation()
	ctx := asPerson(true)

	health := must(svc.GetInstallationHealth(ctx, connect.NewRequest(&pluxv1.GetInstallationHealthRequest{})))(t)
	if len(health.GetChecks()) != 2 || health.GetChecks()[1].GetHealthy() || health.GetChecks()[1].GetName() != "database" {
		t.Errorf("health = %+v", health)
	}
	storage := must(svc.GetInstallationStorage(ctx, connect.NewRequest(&pluxv1.GetInstallationStorageRequest{})))(t)
	if got := storage.GetAreas(); len(got) != 1 || got[0].GetArea() != "database" || got[0].GetBytes() != 4096 {
		t.Errorf("storage = %+v", storage)
	}
	jobs := must(svc.ListInstallationJobs(ctx, connect.NewRequest(&pluxv1.ListInstallationJobsRequest{})))(t)
	if got := jobs.GetJobs(); len(got) != 1 || got[0].GetQueue() != "publish" || got[0].GetCount() != 3 {
		t.Errorf("jobs = %+v", jobs)
	}
	issuers := must(svc.ListTrustedIssuers(ctx, connect.NewRequest(&pluxv1.ListTrustedIssuersRequest{})))(t)
	if got := issuers.GetIssuers(); len(got) != 1 || got[0].GetSubjectPattern() != "repo:acme/*" {
		t.Errorf("issuers = %+v", issuers)
	}
}

// Verifies: STU-116, SEC-102.
// Everyone but an installation administrator is refused, and a server that
// wired no reader reports nothing rather than failing.
func TestInstallationServiceRefusesOthersAndToleratesNoReader_STU_116(t *testing.T) {
	t.Parallel()
	svc := (&api.Handlers{InstallationState: fakeInstallation{}}).Installation()
	_, err := svc.GetInstallationHealth(asPerson(false), connect.NewRequest(&pluxv1.GetInstallationHealthRequest{}))
	if code, _ := plxerr.CodeOf(err); code != plxerr.PermissionDenied {
		t.Errorf("a person who is not an administrator got %v, want PermissionDenied", err)
	}
	_, err = svc.GetInstallationHealth(context.Background(), connect.NewRequest(&pluxv1.GetInstallationHealthRequest{}))
	if code, _ := plxerr.CodeOf(err); code != plxerr.AuthenticationRequired {
		t.Errorf("an anonymous call got %v, want Unauthenticated", err)
	}
	empty := (&api.Handlers{}).Installation()
	got := must(empty.ListTrustedIssuers(asPerson(true), connect.NewRequest(&pluxv1.ListTrustedIssuersRequest{})))(t)
	if len(got.GetIssuers()) != 0 {
		t.Errorf("issuers without a reader = %+v", got)
	}
}
