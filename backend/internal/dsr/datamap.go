// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

//go:generate go run ./internal/gen ../../../docs/operations/personal-data.md

package dsr

import (
	"fmt"
	"slices"
	"strings"
)

// EraseAction says what a data subject erasure does to a store.
type EraseAction string

// The erase actions of the personal-data map.
const (
	// EraseDelete: the subject's rows are deleted.
	EraseDelete EraseAction = "delete"
	// EraseAggregate: the store holds aggregates only and no identifier, so
	// there is nothing to erase; the subject's contribution cannot be
	// separated from the sum.
	EraseAggregate EraseAction = "aggregate"
	// EraseRetained: the store is kept on purpose, for the reason in its
	// note, and holds the subject only under a pseudonymous identifier.
	EraseRetained EraseAction = "retained"
	// EraseAccount: the store belongs to a staff account and goes with the
	// account when an administrator deletes the user, not with a request.
	EraseAccount EraseAction = "account"
	// EraseNone: the store holds no personal data; its columns are named
	// like personal data only by coincidence, as the note explains.
	EraseNone EraseAction = "none"
)

// Store is one entry of the personal-data map: a table that holds, or by
// its column names could hold, personal data.
type Store struct {
	// Name is the table's name and the store's stable name.
	Name string
	// Description says what the store is for.
	Description string
	// DeviceKey and UserKey are the columns that select a device's or a
	// pseudonymous user's rows; "" where the store is not keyed by that
	// subject.
	DeviceKey string
	UserKey   string
	// Personal lists the fields that are personal data.
	Personal []string
	// Retention says how long the rows are kept when nobody erases them.
	Retention string
	// Erase is what a data subject erasure does.
	Erase EraseAction
	// Note carries what the other fields cannot: reasons and caveats.
	Note string
}

const (
	retentionRaw      = "Raw-event retention, 13 months by default and configurable per organisation (ANL-011)."
	retentionDevice   = "Until the device is erased or its app or organisation is deleted."
	retentionAccount  = "While the account exists."
	retentionAudit    = "Audit retention, 10 years by default for production data (SEC-143); never erased (SEC-140)."
	retentionFunction = "Function-log retention, the telemetry retention of the organisation (FN-032)."
)

// personalDataMap is the registry, listing the device stores in the order
// erasure removes them.
func personalDataMap() []Store {
	return slices.Concat(
		deviceStores(),
		userStores(),
		retainedStores(),
		accountStores(),
		operationalStores(),
	)
}

// deviceStores lists the stores keyed by a device, in the order erasure removes them.
func deviceStores() []Store {
	return []Store{
		{
			Name: "telemetry_events", Description: "Runtime telemetry events of the first analytics tables, kept for existing installations.",
			DeviceKey: "device_id", Personal: []string{"device_id", "fields"},
			Retention: retentionRaw, Erase: EraseDelete,
			Note: "Declared, non-sensitive event properties only; the events are the device's.",
		},
		{
			Name: "analytics_events", Description: "Raw analytics events, partitioned by month.",
			DeviceKey: "device_id", Personal: []string{"device_id", "locale", "device_class", "fields"},
			Retention: retentionRaw, Erase: EraseDelete,
			Note: "Erased row-wise across every partition.",
		},
		{
			Name: "device_push_tokens", Description: "The push token of a device, sealed with envelope encryption (SEC-106).",
			DeviceKey: "device_id", Personal: []string{"ciphertext", "wrapped_key"},
			Retention: retentionDevice, Erase: EraseDelete,
			Note: "Export reports that a token is held and its provider; the token itself is never returned.",
		},
		{
			Name: "device_sca_keys", Description: "The public key a device enrolled for strong customer authentication (SEC-027).",
			DeviceKey: "device_id", Personal: []string{"device_id", "public_key", "jkt"},
			Retention: retentionDevice, Erase: EraseDelete,
		},
		{
			Name: "device_tokens", Description: "Short-lived device access tokens, stored as hashes.",
			DeviceKey: "device_id", Personal: []string{"device_id", "secret_hash"},
			Retention: "Until the token expires, then the maintenance sweep.", Erase: EraseDelete,
			Note: "Deleting them revokes the device's sessions.",
		},
		{
			Name: "error_group_devices", Description: "Which devices an error group was seen on, for its affected-device count (ANL-040).",
			DeviceKey: "device_id", Personal: []string{"device_id"},
			Retention: retentionDevice, Erase: EraseDelete,
		},
		{
			Name: "device_bundles", Description: "The bundles a device reported holding.",
			DeviceKey: "device_id", Personal: []string{"device_id"},
			Retention: retentionDevice, Erase: EraseDelete,
		},
		{
			Name: "device_installs", Description: "What a device reported running over time: release, bundles, channel, locale, device class, theme, text scale and the host's pseudonymous user reference (SEC-142).",
			DeviceKey: "device_id", Personal: []string{"device_id", "locale", "user_ref"},
			Retention: "reconstruction.retentionDays, 365 by default; the maintenance sweep deletes older history.", Erase: EraseDelete,
		},
		{
			Name: "reconstructions", Description: "Reconstruction releases of what a device ran and showed at a past moment, with the device and pseudonymous user they name (SEC-142).",
			DeviceKey: "device_id", Personal: []string{"device_id", "manifest"},
			Retention: "reconstruction.retentionDays, 365 by default; the maintenance sweep deletes older releases.", Erase: EraseDelete,
		},
		{
			Name: "devices", Description: "The device record: platform, versions, locale, hardware-bound key, attestation result and revocation.",
			DeviceKey: "id", Personal: []string{"id", "os_version", "locale", "device_class", "dpop_public_key", "app_attest_public_key", "agreement_public_key"},
			Retention: retentionDevice, Erase: EraseDelete,
			Note: "Erased last. With the record and its credentials gone, the device has to register again.",
		},
	}
}

// userStores lists the stores keyed by a pseudonymous user.
func userStores() []Store {
	return []Store{
		{
			Name: "notifications", Description: "The in-app inbox of a user.",
			UserKey: "user_id", Personal: []string{"user_id", "title", "body"},
			Retention: "Until the notification is dropped or the user is deleted.", Erase: EraseDelete,
		},
		{
			Name: "notification_subscriptions", Description: "What a user or team subscribed to.",
			UserKey: "subject_id", Personal: []string{"subject_id"},
			Retention: "Until the subscription is removed or the organisation is deleted.", Erase: EraseDelete,
			Note: "Only subscriptions whose subject_kind is user; team subscriptions name no person.",
		},
	}
}

// retainedStores lists the aggregates and the evidence that erasure leaves in place.
func retainedStores() []Store {
	return []Store{
		{
			Name: "analytics_rollups", Description: "Five-minute and coarser aggregates of the events by dimension.",
			Personal: []string{"locale"}, Retention: "Indefinite, configurable per organisation (ANL-011).", Erase: EraseAggregate,
			Note: "Aggregate only: counts and histograms with no device or user identifier. A subject contributes through its raw events, which are erased; buckets already summed are not recomputed.",
		},
		{
			Name: "function_invocations", Description: "One row per run of a server function.",
			Retention: retentionFunction, Erase: EraseAggregate,
			Note: "Holds no input, output, user or device; there is no identifier to erase by.",
		},
		{
			Name: "function_logs", Description: "Lines a server function logged.",
			Retention: retentionFunction, Erase: EraseRetained,
			Note: "No column names a subject. The lines are what the function wrote, so functions must not log personal data; the rows expire with the retention.",
		},
		{
			Name: "audit_log", Description: "The hash-chained audit log.",
			Personal: []string{"actor_id", "source_ip", "user_agent"}, Retention: retentionAudit, Erase: EraseRetained,
			Note: "Never erased, because deleting an entry breaks the chain (SEC-140). Entries carry only pseudonymous identifiers: actors are identified by their identifier and a display name that is never an email address, devices and data subjects by identifier or hash. A data subject request is recorded here as an entry that names the subject by hash.",
		},
		{
			Name: "dsr_requests", Description: "The completed data subject requests.",
			Personal: []string{"subject_hash"}, Retention: retentionAudit, Erase: EraseRetained,
			Note: "Holds the subject's hash only, never the identifier.",
		},
		{
			Name: "dsr_tombstones", Description: "What an erasure left behind: subject hash, store, row count and time.",
			Personal: []string{"subject_hash"}, Retention: retentionAudit, Erase: EraseRetained,
			Note: "The proof that an erasure happened; it holds the subject's hash only.",
		},
	}
}

// accountStores lists the stores that go with a staff account.
func accountStores() []Store {
	return []Store{
		{
			Name: "users", Description: "Staff accounts.", Personal: []string{"email", "display name", "password_hash"},
			Retention: retentionAccount, Erase: EraseAccount,
			Note: "Removed when an administrator deletes the user. The account is not a pseudonymous data subject: it is identified by email.",
		},
		{
			Name: "memberships", Description: "Which organisations and teams a user belongs to.",
			UserKey: "user_id", Personal: []string{"user_id"}, Retention: retentionAccount, Erase: EraseAccount,
		},
		{
			Name: "role_grants", Description: "Roles bound to a user at a scope.",
			UserKey: "user_id", Personal: []string{"user_id"}, Retention: retentionAccount, Erase: EraseAccount,
		},
		{
			Name: "restriction_policies", Description: "Restrictions that remove permissions from the organisation, a team or a person (GOV-006).",
			UserKey: "user_id", Personal: []string{"user_id", "created_by"}, Retention: retentionAccount, Erase: EraseAccount,
			Note: "A person's restrictions go with the account; created_by is cleared when the author's account is deleted.",
		},
		{
			Name: "app_access", Description: "Per-app access of a user.",
			UserKey: "user_id", Personal: []string{"user_id"}, Retention: retentionAccount, Erase: EraseAccount,
		},
		{
			Name: "sessions", Description: "Signed-in sessions of a user.",
			UserKey: "user_id", Personal: []string{"user_id", "secret_hash", "ip_prefix", "user_agent"}, Retention: "Until the session expires or is revoked.", Erase: EraseAccount,
		},
		{
			Name: "mfa_factors", Description: "Second factors a user enrolled.",
			UserKey: "user_id", Personal: []string{"user_id"}, Retention: retentionAccount, Erase: EraseAccount,
		},
		{
			Name: "mfa_challenges", Description: "Pending second-factor challenges.",
			UserKey: "user_id", Personal: []string{"user_id"}, Retention: "Minutes; the maintenance sweep removes expired ones.", Erase: EraseAccount,
		},
		{
			Name: "access_tokens", Description: "API tokens; a CI token names the workload it was minted for.",
			UserKey: "user_id", Personal: []string{"user_id", "subject", "last_ip_prefix", "last_user_agent"}, Retention: "Until the token expires or is revoked.", Erase: EraseAccount,
		},
		{
			Name: "step_up_requests", Description: "WebAuthn step-up ceremonies of a user and the CLI grants they issued (SEC-103); only hashes of their secrets.",
			UserKey: "user_id", Personal: []string{"user_id", "jkt"}, Retention: "Minutes; a request or grant expires five minutes after it is made.", Erase: EraseAccount,
		},
		{
			Name: "user_identities", Description: "The single-sign-on identities linked to a user.",
			UserKey: "user_id", Personal: []string{"user_id", "subject"}, Retention: retentionAccount, Erase: EraseAccount,
		},
		{
			Name: "federated_sign_ins", Description: "The single-sign-on sign-ins of a user, by issuer and subject.",
			UserKey: "user_id", Personal: []string{"user_id", "subject"}, Retention: retentionAccount, Erase: EraseAccount,
		},
		{
			Name: "federated_assignments", Description: "The grants and team memberships an identity provider's mapping gave a user.",
			UserKey: "user_id", Personal: []string{"user_id"}, Retention: retentionAccount, Erase: EraseAccount,
		},
		{
			Name: "scim_users", Description: "The users a SCIM client provisioned.",
			UserKey: "user_id", Personal: []string{"user_id"}, Retention: retentionAccount, Erase: EraseAccount,
		},
	}
}

// operationalStores lists the remaining stores: short-lived data and compliance records.
func operationalStores() []Store {
	return []Store{
		{
			Name: "idempotency_keys", Description: "Stored responses of retried requests, sealed.",
			Personal: []string{"subject"}, Retention: "The idempotency window, then the maintenance sweep.", Erase: EraseRetained,
			Note: "Short-lived and sealed (SEC-106); the subject is the credential that made the request.",
		},
		{
			Name: "translation_locks", Description: "The editing lock of one locale of an app's translations.",
			Retention: "Until released or taken over.", Erase: EraseNone,
			Note: "locale is the language being translated, not a person's locale.",
		},
		{
			Name: "approval_requests", Description: "Requests for approval of a change, with who wrote and who asked for it (GOV-020 to GOV-022).",
			Personal: []string{"author_id", "requested_by_id", "requested_by"}, Retention: retentionAudit, Erase: EraseRetained,
			Note: "Kept as compliance evidence (GOV-026), like the audit log. subject names the artifact asked about, not a person; people appear by identifier and display name, never an email address.",
		},
		{
			Name: "approval_decisions", Description: "The signed approval decisions, with who decided (GOV-021, GOV-022).",
			Personal: []string{"decider_id", "decider"}, Retention: retentionAudit, Erase: EraseRetained,
			Note: "Kept as compliance evidence (GOV-026): each decision is a signed record chained into the audit log, which erasure would break (SEC-140).",
		},
		{
			Name: "break_glass_invocations", Description: "Emergency actions taken without prior approval, with the justification and the post-hoc review (GOV-024).",
			Personal: []string{"invoked_by_id", "invoked_by", "invoked_by_user", "reviewed_by_id", "reviewed_by"}, Retention: retentionAudit, Erase: EraseRetained,
			Note: "Kept as compliance evidence (GOV-026), like the audit log. People appear by identifier and display name, never an email address; the justification is free text written by the person who acted.",
		},
		{
			Name: "production_changes", Description: "The ledger of production changes compliance reports are read from, with who made and who wrote each (GOV-026).",
			Personal: []string{"author_id", "changed_by_id", "changed_by"}, Retention: retentionAudit, Erase: EraseRetained,
			Note: "Kept as compliance evidence (GOV-026), like the audit log. People appear by identifier and display name, never an email address.",
		},
		{
			Name: "change_freezes", Description: "Change-freeze windows of an organisation (GOV-025).",
			Personal: []string{"created_by"}, Retention: "Until the window is deleted.", Erase: EraseNone,
			Note: "Holds the display name of the administrator who last wrote the window; the window itself is not about a person.",
		},
		{
			Name: "compliance_reports", Description: "Stored compliance report statements and their signatures (GOV-026).",
			Personal: []string{"requested_by_id", "requested_by"}, Retention: retentionAudit, Erase: EraseRetained,
			Note: "Kept as compliance evidence (GOV-026). The signed statement names people by identifier and display name only, and is never rewritten, because the signature covers it.",
		},
		{
			Name: "golden_cases", Description: "Golden-test cases of an app.",
			Retention: "Until the case is deleted.", Erase: EraseNone,
			Note: "locale is the language the case renders in, not a person's locale.",
		},
	}
}

// Map returns the personal-data map: every store that holds, or could by
// its column names hold, personal data.
func Map() []Store {
	return personalDataMap()
}

// Render renders the map as the Markdown of docs/operations/personal-data.md.
// The output depends on the registry alone, so it is reproducible.
func Render() string {
	var b strings.Builder
	b.WriteString("<!-- Code generated by backend/internal/dsr (go generate). DO NOT EDIT. -->\n\n")
	b.WriteString("# Personal data map\n\n")
	b.WriteString("Where Plux stores personal data, how long it is kept and what a data subject request does to it (`SEC-162`). ")
	b.WriteString("An administrator with `dsr.manage` exports or erases a device or a pseudonymous user with `plux dsr export` and `plux dsr erase`.\n\n")
	b.WriteString("Audit entries are never erased, because deleting one breaks the hash chain (`SEC-140`). ")
	b.WriteString("They carry only pseudonymous identifiers, and a data subject request is recorded as an entry that names the subject by hash.\n\n")
	for _, group := range renderGroups() {
		writeGroup(&b, group)
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

// renderGroup is a section of the generated document.
type renderGroup struct {
	title string
	keep  func(Store) bool
}

// renderGroups orders the document by what an erasure does.
func renderGroups() []renderGroup {
	return []renderGroup{
		{"Erased by a data subject request", func(s Store) bool { return s.Erase == EraseDelete }},
		{"Aggregates and retained records", func(s Store) bool { return s.Erase == EraseAggregate || s.Erase == EraseRetained }},
		{"Staff account data", func(s Store) bool { return s.Erase == EraseAccount }},
		{"Named like personal data, but not personal", func(s Store) bool { return s.Erase == EraseNone }},
	}
}

// writeGroup writes one section of stores.
func writeGroup(b *strings.Builder, g renderGroup) {
	fmt.Fprintf(b, "## %s\n\n", g.title)
	b.WriteString("| Store | Holds | Device key | User key | Personal fields | Retention | Erase | Note |\n")
	b.WriteString("|---|---|---|---|---|---|---|---|\n")
	for _, s := range personalDataMap() {
		if !g.keep(s) {
			continue
		}
		fmt.Fprintf(b, "| `%s` | %s | %s | %s | %s | %s | %s | %s |\n",
			s.Name, cell(s.Description), code(s.DeviceKey), code(s.UserKey), codes(s.Personal), cell(s.Retention), s.Erase, cell(s.Note))
	}
	b.WriteString("\n")
}

// cell escapes text for a table cell.
func cell(s string) string {
	if s == "" {
		return "—"
	}
	return strings.ReplaceAll(s, "|", "\\|")
}

// code renders a column name, or a dash for none.
func code(s string) string {
	if s == "" {
		return "—"
	}
	return "`" + s + "`"
}

// codes renders a list of column names.
func codes(ss []string) string {
	if len(ss) == 0 {
		return "—"
	}
	parts := make([]string, len(ss))
	for i, s := range ss {
		parts[i] = code(s)
	}
	return strings.Join(parts, ", ")
}
