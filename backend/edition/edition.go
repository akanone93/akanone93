// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Package edition is the boundary between the open-source core and the
// enterprise edition under ee/ (GOV-033, ADR-0084).
//
// The core knows which enterprise features exist and asks, per request,
// whether the installation may use one. The answer comes from a Provider
// that the process registers at start-up: the core registers Community,
// which enables nothing, and the enterprise binary registers a provider that
// verifies the licence file (GOV-030). Without ee/ the core therefore builds,
// tests and runs with every enterprise feature off.
//
// The package is outside internal/ on purpose: the enterprise module is a
// separate Go module and may import only public packages of the core.
//
// Security capabilities are never gated by an edition (GOV-032); a feature
// name is added to this package only for something the edition split of
// ADR-0022 puts in ee/.
package edition

import (
	"context"
	"log/slog"
	"time"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// Feature names an enterprise feature. The names are the ones a licence
// file lists in its features.
type Feature string

// The enterprise features of the edition split (ADR-0022, Revision of
// 2026-10-18).
const (
	// FeatureCustomRoles is custom roles.
	FeatureCustomRoles Feature = "custom-roles"
	// FeatureSAML is SAML 2.0 single sign-on.
	FeatureSAML Feature = "saml"
	// FeatureSCIM is SCIM 2.0 provisioning.
	FeatureSCIM Feature = "scim"
	// FeatureMultiStageApprovals is multi-stage and parallel approval
	// policies and distinct-team rules.
	FeatureMultiStageApprovals Feature = "multi-stage-approvals"
	// FeatureChangeFreezes is change freezes.
	FeatureChangeFreezes Feature = "change-freezes"
	// FeatureComplianceReports is signed compliance reports.
	FeatureComplianceReports Feature = "compliance-reports"
	// FeatureClickHouse is ClickHouse telemetry storage.
	FeatureClickHouse Feature = "clickhouse"
	// FeatureParquetExport is Parquet export.
	FeatureParquetExport Feature = "parquet-export"
	// FeatureWebhookExport is signed-webhook event export.
	FeatureWebhookExport Feature = "webhook-export"
	// FeatureSIEMExtended is CEF, OTLP-logs and signed-webhook SIEM export.
	FeatureSIEMExtended Feature = "siem-extended"
	// FeatureAirGap is the air-gapped installation bundle and its tooling.
	FeatureAirGap Feature = "air-gap"
)

// Features returns every feature the core knows, in a fixed order.
func Features() []Feature {
	return []Feature{
		FeatureCustomRoles, FeatureSAML, FeatureSCIM, FeatureMultiStageApprovals,
		FeatureChangeFreezes, FeatureComplianceReports, FeatureClickHouse,
		FeatureParquetExport, FeatureWebhookExport, FeatureSIEMExtended, FeatureAirGap,
	}
}

// State is the state of the installation's licence.
type State string

// The states a licence is in (GOV-030).
const (
	// StateAbsent is no licence file: the core runs, enterprise features are
	// off.
	StateAbsent State = "absent"
	// StateValid is a verified licence inside its validity window.
	StateValid State = "valid"
	// StateGrace is a verified licence past its expiry but inside its grace
	// period: enterprise features keep working and warn.
	StateGrace State = "grace"
	// StateExpired is a verified licence past its grace period: enterprise
	// features are read-only, nothing is deleted.
	StateExpired State = "expired"
	// StateInvalid is a licence file that is unreadable, not signed by a
	// trusted key, not yet valid, bound to another installation or names an
	// unknown limit: enterprise features are off.
	StateInvalid State = "invalid"
)

// Status is a snapshot of the licence.
type Status struct {
	State State
	// LicenseID and Organization identify a verified licence; empty
	// otherwise. Invalid files are never quoted.
	LicenseID    string
	Organization string
	// ExpiresAt and GraceEndsAt are zero without a verified licence.
	ExpiresAt   time.Time
	GraceEndsAt time.Time
	// DaysRemaining is the whole number of days until ExpiresAt; it is
	// negative during the grace period and zero without a verified licence.
	// It is the value of plux_license_days_remaining.
	DaysRemaining int
	// Features are the enterprise features the licence lists.
	Features []Feature
	// Limits are the registry limits the licence caps.
	Limits map[string]int64
	// Message says in one line why the state is what it is, without quoting
	// the file.
	Message string
}

// Allows reports whether the status permits a feature: for writing while the
// licence is valid or in grace, for reading also once it has expired.
func (s Status) Allows(f Feature, write bool) bool {
	listed := false
	for _, have := range s.Features {
		if have == f {
			listed = true
			break
		}
	}
	if !listed {
		return false
	}
	switch s.State {
	case StateValid, StateGrace:
		return true
	case StateExpired:
		return !write
	case StateAbsent, StateInvalid:
		return false
	}
	return false
}

// Provider answers what the installation's licence allows. Implementations
// are safe for concurrent use and cheap: Status is called per request.
type Provider interface {
	Status() Status
}

// Settings are what a Verifier needs to open a provider.
type Settings struct {
	// File is the licence file's path (PLUX_LICENSE_FILE); empty means none.
	File string
	// InstallationID is the installation's identifier, for licences bound to
	// one.
	InstallationID string
	// Log receives the provider's warnings; it never receives the licence's
	// content beyond its identifier. Nil discards them.
	Log *slog.Logger
}

// Verifier is the registration hook of the enterprise edition: the
// enterprise binary hands one to the server, which opens it with the
// configured settings. Open reads and verifies the licence now and then
// re-verifies it every hour until ctx ends.
type Verifier interface {
	Open(ctx context.Context, s Settings) (Provider, error)
}

type community struct{}

func (community) Status() Status {
	return Status{State: StateAbsent, Message: "no licence: enterprise features are off"}
}

// Community returns the provider of an installation without a licence: no
// enterprise feature is enabled. It is what the core registers.
func Community() Provider { return community{} }

type providerKey struct{}

// WithProvider returns a context carrying p.
func WithProvider(ctx context.Context, p Provider) context.Context {
	return context.WithValue(ctx, providerKey{}, p)
}

// From returns the provider in ctx, or Community when there is none.
func From(ctx context.Context) Provider {
	if p, ok := ctx.Value(providerKey{}).(Provider); ok && p != nil {
		return p
	}
	return Community()
}

// Enabled reports whether f may be used in full, including for writes. It is
// false without a verified licence that lists f and false once the grace
// period has passed.
func Enabled(ctx context.Context, f Feature) bool { return EnabledForWrite(ctx, f) }

// EnabledForWrite reports whether f may change data (GOV-030): the licence
// is valid or in grace and lists f.
func EnabledForWrite(ctx context.Context, f Feature) bool {
	return From(ctx).Status().Allows(f, true)
}

// Readable reports whether f's existing data may still be read and exported:
// as EnabledForWrite, and also after the licence has expired.
func Readable(ctx context.Context, f Feature) bool {
	return From(ctx).Status().Allows(f, false)
}

// Require returns PLX-8600 unless f may be used for writing. Call it at the
// start of an operation that changes data through an enterprise feature.
func Require(ctx context.Context, f Feature) error {
	if EnabledForWrite(ctx, f) {
		return nil
	}
	return notLicensed(f)
}

// RequireRead returns PLX-8600 unless f may be read.
func RequireRead(ctx context.Context, f Feature) error {
	if Readable(ctx, f) {
		return nil
	}
	return notLicensed(f)
}

func notLicensed(f Feature) error {
	return plxerr.New(plxerr.EnterpriseFeatureNotLicensed,
		"the enterprise feature %q is not enabled by this installation's licence", string(f))
}
