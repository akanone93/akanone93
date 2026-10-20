// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Package audit records every state-changing operation and every
// security-relevant event in an append-only hash chain (SEC-140).
//
// Each entry commits to the one before it, so removing or editing an
// entry breaks the chain and Verify says where. Entries are written in
// the same transaction as the change they describe, so an operation
// cannot succeed unrecorded.
//
// Actions are a closed, registered set: a typo is a compile error rather
// than an entry nobody will ever find again.
package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Action names what was done. Every action a caller may record is
// registered below.
type Action string

// The actions this phase records.
const (
	OrganizationCreated Action = "organization.created"
	OrganizationUpdated Action = "organization.updated"
	TeamCreated         Action = "team.created"
	TeamUpdated         Action = "team.updated"
	TeamDeleted         Action = "team.deleted"
	MemberAdded         Action = "member.added"
	MemberRemoved       Action = "member.removed"

	UserInvited          Action = "user.invited"
	InvitationAccepted   Action = "user.invitation_accepted"
	UserSignedIn         Action = "user.signed_in"
	UserSignInRefused    Action = "user.sign_in_refused"
	UserSignedOut        Action = "user.signed_out"
	PasswordChanged      Action = "user.password_changed"
	SecondFactorVerified Action = "mfa.verified"
	FactorEnrolled       Action = "mfa.factor_enrolled"
	IdentityLinked       Action = "user.identity_linked"
	FactorConfirmed      Action = "mfa.factor_confirmed"
	FactorDeleted        Action = "mfa.factor_deleted"

	TokenCreated            Action = "token.created"
	TokenRevoked            Action = "token.revoked"
	DeviceAuthApproved      Action = "token.device_authorization_approved"
	DeviceAuthDenied        Action = "token.device_authorization_denied"
	WorkloadIdentityCreated Action = "token.workload_identity_created"
	WorkloadIdentityDeleted Action = "token.workload_identity_deleted"
	WorkloadExchanged       Action = "token.workload_exchanged"

	AppCreated       Action = "app.created"
	AppUpdated       Action = "app.updated"
	AppDeleted       Action = "app.deleted"
	AppRestored      Action = "app.restored"
	AppPurged        Action = "app.purged"
	AccessGranted    Action = "app.access_granted"
	AccessRevoked    Action = "app.access_revoked"
	EnvironmentAdded Action = "environment.created"
	EnvironmentSet   Action = "environment.updated"
	EnvironmentGone  Action = "environment.deleted"
	ChannelCreated   Action = "channel.created"
	ChannelDeleted   Action = "channel.deleted"
	VariableSet      Action = "variable.set"
	SecretSet        Action = "secret.set"
	SecretDeleted    Action = "secret.deleted"
	LimitSet         Action = "limit.set"
	TrashRestored    Action = "trash.restored"
	TrashPurged      Action = "trash.purged"

	PluginCreated     Action = "plugin.created"
	PluginUpdated     Action = "plugin.updated"
	PluginDeleted     Action = "plugin.deleted"
	PluginRestored    Action = "plugin.restored"
	PluginPurged      Action = "plugin.purged"
	DocumentWritten   Action = "document.written"
	DocumentDeleted   Action = "document.deleted"
	DocumentRestored  Action = "document.restored"
	DocumentPurged    Action = "document.purged"
	DraftImported     Action = "draft.imported"
	AssetUploaded     Action = "asset.uploaded"
	AssetDeleted      Action = "asset.deleted"
	VersionPublished  Action = "release.version_published"
	PublishCancelled  Action = "release.publish_cancelled"
	ReleaseCreated    Action = "release.created"
	ReleasePromoted   Action = "release.promoted"
	ReleaseRolledBack Action = "release.rolled_back"
	// NativeCatalogueUploaded: a host build's native catalogue was stored
	// (ADR-0041).
	NativeCatalogueUploaded Action = "native_catalogue.uploaded"
	// A11yWaiverChanged: a save added or changed an accessibility waiver
	// (ADR-0069).
	A11yWaiverChanged Action = "a11y.waiver_changed"
	// GoldensApproved: golden images were promoted to baselines (TST-003,
	// ADR-0070).
	GoldensApproved      Action = "goldens.approved"
	ReleasesPurged       Action = "release.purged"
	ControlChanged       Action = "release.control_changed"
	SnapshotRestored     Action = "snapshot.restored"
	TemplateInstantiated Action = "template.instantiated"
	LockAcquired         Action = "plugin.lock.acquired"
	LockTakenOver        Action = "plugin.lock.override"
	LockReleased         Action = "plugin.lock.released"
	LockRequested        Action = "plugin.lock.requested"

	// DeviceRevoked: a device's trust was withdrawn (SEC-006).
	DeviceRevoked Action = "device.revoked"
	// SecurityConfigSet: an environment's security profile or overrides
	// changed (SEC-182).
	SecurityConfigSet Action = "security_config.set"
	// SCAKeyEnrolled: a device enrolled the key that signs transaction
	// confirmations (SEC-027).
	SCAKeyEnrolled Action = "device.sca_key_enrolled"
	// UpstreamCreated, UpstreamUpdated and UpstreamDeleted: an API the
	// gateway may reach was registered, changed or removed (SEC-031).
	UpstreamCreated Action = "gateway.upstream_created"
	UpstreamUpdated Action = "gateway.upstream_updated"
	UpstreamDeleted Action = "gateway.upstream_deleted"
	// UserIdentityProviderSet and UserIdentityProviderDeleted: the
	// identity provider of an environment's users changed (SEC-026).
	UserIdentityProviderSet     Action = "gateway.user_identity_provider_set"
	UserIdentityProviderDeleted Action = "gateway.user_identity_provider_deleted"

	// ConfigValueSet and ConfigValueDeleted: a server-held configuration
	// value of an environment changed; the entry names the key and the
	// version, never the value (DAT-090).
	ConfigValueSet     Action = "config.value_set"
	ConfigValueDeleted Action = "config.value_deleted"

	// FunctionSourceUploaded, FunctionBuildRequested, FunctionBuildFinished,
	// FunctionDeployed, FunctionPromoted and FunctionAliasSet: a function's
	// source was stored, a build was queued or finished, a version was
	// deployed, a version was copied to another environment or an alias
	// moved (FN-006, FN-016).
	FunctionSourceUploaded Action = "function.source_uploaded"
	FunctionBuildRequested Action = "function.build_requested"
	FunctionBuildFinished  Action = "function.build_finished"
	FunctionDeployed       Action = "function.deployed"
	FunctionPromoted       Action = "function.promoted"
	FunctionAliasSet       Action = "function.alias_set"

	// UpdateMetadataRootUploaded: an operator stored a root signed
	// offline (SEC-051, SEC-140).
	UpdateMetadataRootUploaded Action = "update_metadata.root_uploaded"

	// LicenseStateChanged: the enterprise licence entered grace, expired or
	// became invalid, or came back to a valid state (GOV-030). It is
	// recorded in the installation's own chain.
	LicenseStateChanged Action = "license.state_changed"
	// GitExportConfigured: an app's Git export setting was set (GOV-011).
	GitExportConfigured Action = "git.export_configured"
	// GitExported: a publish was pushed to the app's Git remote (GOV-011).
	GitExported Action = "git.exported"
)

// The actions of the audit export (SEC-143, ADR-0082).
const (
	// ExportDestinationCreated, ExportDestinationUpdated and
	// ExportDestinationDeleted: a SIEM destination was registered, changed or
	// removed; the entry names its kind and host, never a credential.
	ExportDestinationCreated Action = "audit_export.destination_created"
	ExportDestinationUpdated Action = "audit_export.destination_updated"
	ExportDestinationDeleted Action = "audit_export.destination_deleted"
)

// The actions of roles and grants (GOV-002, GOV-003, ADR-0073).
const (
	// RoleCreated, RoleUpdated and RoleDeleted: a custom role was made,
	// changed or removed; the entry's detail lists its permissions.
	RoleCreated Action = "role.created"
	RoleUpdated Action = "role.updated"
	RoleDeleted Action = "role.deleted"
	// RoleGranted, RoleGrantChanged and RoleRevoked: a role was bound to a
	// user or a team at a scope, the role of a grant changed, or a grant
	// removed; the entry's detail names the holder, role and scope.
	RoleGranted      Action = "role.granted"
	RoleGrantChanged Action = "role.grant_changed"
	RoleRevoked      Action = "role.revoked"
)

// The actions of notifications, webhooks and function triggers.
const (
	// NotificationChannelSet, NotificationChannelDeleted,
	// NotificationSubscriptionSet, NotificationsDropped, the Webhook*
	// actions and DeliveryFailed: a channel, subscription or outbound
	// webhook changed, a webhook's secret was rotated, notifications over
	// notifications.perHour were dropped, or a delivery was given up on
	// (SRV-062, SRV-063, ADR-0081).
	NotificationChannelSet      Action = "notification.channel_set"
	NotificationChannelDeleted  Action = "notification.channel_deleted"
	NotificationSubscriptionSet Action = "notification.subscription_set"
	NotificationsDropped        Action = "notification.dropped"
	WebhookSet                  Action = "webhook.set"
	WebhookDeleted              Action = "webhook.deleted"
	WebhookSecretRotated        Action = "webhook.secret_rotated"
	DeliveryFailed              Action = "notification.delivery_failed"

	// FunctionTriggerSet, FunctionTriggerDeleted, FunctionHookSet,
	// FunctionHookDeleted and FunctionHookSecretRotated: a schedule or an
	// inbound webhook of a function changed (FN-025, ADR-0081).
	FunctionTriggerSet        Action = "function.trigger_set"
	FunctionTriggerDeleted    Action = "function.trigger_deleted"
	FunctionHookSet           Action = "function.hook_set"
	FunctionHookDeleted       Action = "function.hook_deleted"
	FunctionHookSecretRotated Action = "function.hook_secret_rotated"
)

// actions is the registry, sorted, so that Registered can search it and
// a test can print the catalogue.
var actions = sorted(
	OrganizationCreated, OrganizationUpdated, TeamCreated, TeamUpdated, TeamDeleted,
	MemberAdded, MemberRemoved,
	UserInvited, InvitationAccepted, UserSignedIn, UserSignInRefused, UserSignedOut,
	PasswordChanged, SecondFactorVerified, IdentityLinked, FactorEnrolled, FactorConfirmed, FactorDeleted,
	TokenCreated, TokenRevoked, DeviceAuthApproved, DeviceAuthDenied,
	WorkloadIdentityCreated, WorkloadIdentityDeleted, WorkloadExchanged,
	AppCreated, AppUpdated, AppDeleted, AppRestored, AppPurged, AccessGranted, AccessRevoked,
	EnvironmentAdded, EnvironmentSet, EnvironmentGone, ChannelCreated, ChannelDeleted,
	VariableSet, SecretSet, SecretDeleted, LimitSet, TrashRestored, TrashPurged,
	PluginCreated, PluginUpdated, PluginDeleted, PluginRestored, PluginPurged,
	DocumentWritten, DocumentDeleted, DocumentRestored, DocumentPurged, DraftImported, AssetUploaded, AssetDeleted,
	VersionPublished, PublishCancelled, ReleaseCreated, ReleasePromoted, ReleaseRolledBack, ReleasesPurged, ControlChanged, NativeCatalogueUploaded, A11yWaiverChanged, GoldensApproved,
	SnapshotRestored, TemplateInstantiated, LockAcquired, LockTakenOver, LockReleased, LockRequested,
	DeviceRevoked, SecurityConfigSet, UpdateMetadataRootUploaded,
	SCAKeyEnrolled, UpstreamCreated, UpstreamUpdated, UpstreamDeleted, UserIdentityProviderSet, UserIdentityProviderDeleted,
	ConfigValueSet, ConfigValueDeleted,
	FunctionSourceUploaded, FunctionBuildRequested, FunctionBuildFinished, FunctionDeployed, FunctionPromoted, FunctionAliasSet,
	RoleCreated, RoleUpdated, RoleDeleted,
	RoleGranted, RoleGrantChanged, RoleRevoked,
	LicenseStateChanged,
	NotificationChannelSet, NotificationChannelDeleted, NotificationSubscriptionSet, NotificationsDropped,
	WebhookSet, WebhookDeleted, WebhookSecretRotated, DeliveryFailed,
	FunctionTriggerSet, FunctionTriggerDeleted, FunctionHookSet, FunctionHookDeleted, FunctionHookSecretRotated,
	ExportDestinationCreated, ExportDestinationUpdated, ExportDestinationDeleted,
	GitExportConfigured, GitExported,
)

// sorted returns its arguments in order.
func sorted(a ...Action) []Action {
	slices.Sort(a)
	return a
}

// Actions returns every registered action, in order.
func Actions() []Action { return slices.Clone(actions) }

// Registered reports whether an action may be recorded.
func Registered(a Action) bool {
	_, found := slices.BinarySearch(actions, a)
	return found
}

// Actor is who performed an action.
type Actor struct {
	// Kind is "user", "token", "device", "system" or "ci".
	Kind string
	// ID identifies the actor within its kind.
	ID string
	// Display is a human-readable name. It is never an email address:
	// the audit log is exported, and an address is personal data.
	Display string
}

// Entry is one record.
type Entry struct {
	ID             string
	OrganizationID string
	// Sequence is the entry's position in its organisation's chain,
	// from 1.
	Sequence int64
	At       time.Time
	Actor    Actor
	Action   Action
	// TargetKind and TargetID identify what was acted on.
	TargetKind string
	TargetID   string
	SourceIP   string
	UserAgent  string
	RequestID  string
	// BeforeHash and AfterHash are SHA-256 of the affected content,
	// where there is content; empty otherwise.
	BeforeHash string
	AfterHash  string
	// Detail says what the fixed fields cannot, such as whose lock a
	// takeover displaced; usually empty.
	Detail string
	// PreviousHash is the previous entry's EntryHash, or "" for the
	// first entry of an organisation.
	PreviousHash string
	// EntryHash commits to every field above.
	EntryHash string
}

// Hash computes the entry's hash from its fields and the previous one.
// The encoding is unambiguous: every field is length-prefixed, so no
// value can be moved between fields without changing the hash.
func (e Entry) Hash() string {
	var b strings.Builder
	write := func(s string) {
		b.WriteString(strconv.Itoa(len(s)))
		b.WriteByte(':')
		b.WriteString(s)
	}
	write(e.ID)
	write(e.OrganizationID)
	write(strconv.FormatInt(e.Sequence, 10))
	write(e.At.UTC().Format(time.RFC3339Nano))
	write(e.Actor.Kind)
	write(e.Actor.ID)
	write(e.Actor.Display)
	write(string(e.Action))
	write(e.TargetKind)
	write(e.TargetID)
	write(e.SourceIP)
	write(e.UserAgent)
	write(e.RequestID)
	write(e.BeforeHash)
	write(e.AfterHash)
	write(e.PreviousHash)
	// The detail was added after the first entries were written, so it
	// joins the hash only when present and older hashes still verify.
	if e.Detail != "" {
		write(e.Detail)
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

// Verify walks a contiguous run of entries in sequence order and reports
// the first one that does not follow from the one before it.
// previousHash is the hash of the entry before the run, and "" when the
// run starts at sequence 1.
func Verify(entries []Entry, previousHash string) error {
	for i, e := range entries {
		if i > 0 && e.Sequence != entries[i-1].Sequence+1 {
			return fmt.Errorf("audit: sequence jumps from %d to %d: an entry is missing",
				entries[i-1].Sequence, e.Sequence)
		}
		if err := e.follows(previousHash); err != nil {
			return err
		}
		previousHash = e.EntryHash
	}
	return nil
}

// follows reports why an entry does not follow from the entry whose hash
// is previousHash, or is not what was written.
func (e Entry) follows(previousHash string) error {
	if e.PreviousHash != previousHash {
		return fmt.Errorf("audit: entry %d does not follow the one before it", e.Sequence)
	}
	if e.EntryHash != e.Hash() {
		return fmt.Errorf("audit: entry %d was changed after it was written", e.Sequence)
	}
	return nil
}
