// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package auth

// The permissions of Phase 9 (plan §2.4, ADR-0073). Each is in the
// catalogue, permissions.json, with its description, risk, step-up flag and
// scope; a test refuses a constant without an entry and an entry without a
// constant. A milestone that guards a new call with one of these adds the
// call, not the constant.
const (
	// RolesManage creates, changes and deletes custom roles (GOV-002).
	RolesManage Permission = "roles.manage"
	// TeamsManage creates, changes and deletes teams (GOV-001).
	TeamsManage Permission = "teams.manage"
	// SSOManage configures identity providers, SAML, SCIM and group
	// mappings (GOV-004).
	SSOManage Permission = "sso.manage"
	// SessionsManage lists and revokes the sessions of other members.
	SessionsManage Permission = "sessions.manage"
	// PoliciesManage creates and changes approval policies (GOV-020).
	PoliciesManage Permission = "policies.manage"
	// ApprovalsDecide approves or rejects a change (GOV-023).
	ApprovalsDecide Permission = "approvals.decide"
	// BreakglassInvoke performs an emergency action without prior approval
	// (GOV-024).
	BreakglassInvoke Permission = "breakglass.invoke"
	// FreezesManage creates and changes change-freeze windows (GOV-025).
	FreezesManage Permission = "freezes.manage"
	// ComplianceReport generates and reads compliance reports (GOV-026).
	ComplianceReport Permission = "compliance.report"
	// RolloutManage creates, pauses, resumes and rolls back rollouts.
	RolloutManage Permission = "rollout.manage"
	// KillswitchManage switches a plugin or an app off, or forces a
	// mandatory update.
	KillswitchManage Permission = "killswitch.manage"
	// FlagsManage creates and changes feature flags and segments.
	FlagsManage Permission = "flags.manage"
	// ExperimentsManage creates, starts, stops and concludes experiments.
	ExperimentsManage Permission = "experiments.manage"
	// AnalyticsRead reads analytics, rollups and experiment results.
	AnalyticsRead Permission = "analytics.read"
	// AlertsManage manages alert rules and their routes.
	AlertsManage Permission = "alerts.manage"
	// NotificationsManage manages notification channels and subscriptions.
	NotificationsManage Permission = "notifications.manage"
	// WebhooksManage manages outbound and inbound webhooks.
	WebhooksManage Permission = "webhooks.manage"
	// AuditExport exports the audit log.
	AuditExport Permission = "audit.export"
	// ReconstructionView views a reconstruction of a past release.
	ReconstructionView Permission = "reconstruction.view"
	// DSRManage handles data-subject requests.
	DSRManage Permission = "dsr.manage"
	// SearchRead searches across the apps the caller can read.
	SearchRead Permission = "search.read"
	// GitExport exports an app to a Git repository.
	GitExport Permission = "git.export"
	// LicenseManage installs and replaces the licence file.
	LicenseManage Permission = "license.manage"
	// BackupManage runs, schedules and restores backups.
	BackupManage Permission = "backup.manage"
	// RetentionManage changes how long events and rollups are kept and
	// schedules or cancels such changes (ANL-010, maintainer decision Q17).
	RetentionManage Permission = "retention.manage"
)
