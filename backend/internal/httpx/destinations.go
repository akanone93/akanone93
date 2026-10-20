// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

//go:generate go run ./internal/gen ../../../docs/operations/egress.md

package httpx

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// ErrUnregistered is returned when a client, transport or dialer is built
// for a destination kind the registry does not list (SEC-163).
var ErrUnregistered = errors.New("httpx: the destination kind is not registered")

// Kind names one kind of outbound destination. Every outbound connection
// the server makes is built for a Kind, so the registry below is the
// complete list of what can leave an installation (SEC-163).
type Kind string

// The registered destination kinds.
const (
	// KindAndroidStatusList is Google's attestation status list.
	KindAndroidStatusList Kind = "attestation.android-status-list"
	// KindAppleReceipt is Apple's App Attest receipt service.
	KindAppleReceipt Kind = "attestation.apple-receipt"
	// KindPush is the push services, FCM and APNs.
	KindPush Kind = "push"
	// KindNotify is the HTTP notification channels and outbound webhooks.
	KindNotify Kind = "notify.http"
	// KindSMTP is the mail relay of the notification channels.
	KindSMTP Kind = "notify.smtp"
	// KindAnalyticsOTLP is the OTLP metrics export of an organisation.
	KindAnalyticsOTLP Kind = "analytics.otlp"
	// KindAuditExport is the audit export sinks of an organisation.
	KindAuditExport Kind = "audit.export"
	// KindGatewayUpstream is an upstream service of the secure gateway.
	KindGatewayUpstream Kind = "gateway.upstream"
	// KindIdentityKeySet is the key set of an identity provider the
	// gateway exchanges tokens of.
	KindIdentityKeySet Kind = "gateway.identity"
	// KindOIDC is the OpenID Connect provider people sign in with.
	KindOIDC Kind = "auth.oidc"
	// KindCIIssuer is the discovery document and key set of a CI provider.
	KindCIIssuer Kind = "auth.ci-issuer"
	// KindGitRemote is the Git remote an app is exported to.
	KindGitRemote Kind = "gitsync.remote"
	// KindTraceExport is the OTLP trace export of the server itself.
	KindTraceExport Kind = "observability.otlp"
	// KindFunctionEgress is the upstream of the function runner's egress
	// proxy (FN-013).
	KindFunctionEgress Kind = "functions.egress"
)

// Configurator says who decides that a destination is used.
type Configurator string

// The parties that configure a destination.
const (
	// ByOperator means the installation's configuration file or
	// environment names it.
	ByOperator Configurator = "operator"
	// ByOrganisation means an administrator of an organisation, app or
	// function names it in Studio or the API.
	ByOrganisation Configurator = "organisation"
)

// DefaultState is what a destination does in an installation nobody has
// configured.
type DefaultState string

// The default states.
const (
	// DefaultOff means the feature is off until it is switched on.
	DefaultOff DefaultState = "off"
	// DefaultNone means the feature is available, but no destination
	// exists until someone enters one.
	DefaultNone DefaultState = "none until configured"
	// DefaultOn means the server contacts the destination without
	// configuration; the operator can replace it with a local file.
	DefaultOn DefaultState = "on"
)

// Destination describes one kind of outbound connection.
type Destination struct {
	// Kind identifies the destination.
	Kind Kind
	// Name is a short title.
	Name string
	// Purpose says why the server connects.
	Purpose string
	// ConfiguredBy says who decides that the destination is used.
	ConfiguredBy Configurator
	// Setting names the configuration key or the setting in Studio.
	Setting string
	// Hosts lists the only hosts the protocol ever contacts. Empty means
	// the customer enters the address; the SSRF policy then applies.
	Hosts []string
	// Default is the state of an unconfigured installation.
	Default DefaultState
	// Data summarises what is sent.
	Data string
}

// FixedHosts reports whether the destination contacts only the listed
// hosts.
func (d Destination) FixedHosts() bool { return len(d.Hosts) > 0 }

// allows reports whether a host may be contacted for the destination.
func (d Destination) allows(host string) bool {
	if !d.FixedHosts() {
		return true
	}
	return slices.Contains(d.Hosts, strings.ToLower(host))
}

// registry is the list of destinations, in the order the egress list
// prints them. It is read-only after package initialisation.
var registry = []Destination{
	{
		Kind: KindAndroidStatusList, Name: "Android attestation status list",
		Purpose:      "Revocation check of Android key attestation certificates (SEC-002).",
		ConfiguredBy: ByOperator, Setting: "attestation.android.statusListFile (a local file replaces the download)",
		Hosts: []string{"android.googleapis.com"}, Default: DefaultOn,
		Data: "A plain GET of Google's public list; nothing about devices or apps.",
	},
	{
		Kind: KindAppleReceipt, Name: "Apple App Attest receipts",
		Purpose:      "Refreshes the App Attest receipt to read the fraud metric.",
		ConfiguredBy: ByOperator, Setting: "attestation.apple.fraudReceipt.enabled",
		Hosts: []string{"data.appattest.apple.com", "data-development.appattest.apple.com"}, Default: DefaultOff,
		Data: "The receipt of a device and a request token signed with the team's key.",
	},
	{
		Kind: KindPush, Name: "Push services",
		Purpose:      "Silent push that tells a device to sync (SYN-060).",
		ConfiguredBy: ByOperator, Setting: "push.enabled, plus per-app credentials",
		Hosts:   []string{"api.push.apple.com", "api.sandbox.push.apple.com", "fcm.googleapis.com", "oauth2.googleapis.com"},
		Default: DefaultOff, Data: "A push token and a content-free wake-up payload; the credential exchange sends a signed assertion.",
	},
	{
		Kind: KindNotify, Name: "Notification channels and webhooks",
		Purpose:      "Slack, Teams, Telegram and outbound webhook messages about events in an organisation.",
		ConfiguredBy: ByOrganisation, Setting: "Notification channels (Studio, Settings)",
		Default: DefaultNone, Data: "The event text; Telegram goes to api.telegram.org, the others to the address entered.",
	},
	{
		Kind: KindSMTP, Name: "Mail relay",
		Purpose:      "Email notifications.",
		ConfiguredBy: ByOrganisation, Setting: "Notification channels (Studio, Settings)",
		Default: DefaultNone, Data: "The event text and the recipients.",
	},
	{
		Kind: KindAnalyticsOTLP, Name: "Analytics OTLP export",
		Purpose:      "Sends daily aggregates of an organisation's analytics to its own collector.",
		ConfiguredBy: ByOrganisation, Setting: "Analytics export (Studio, Analytics)",
		Default: DefaultNone, Data: "Rollups only: counts and sums, never an event or a device identifier.",
	},
	{
		Kind: KindAuditExport, Name: "Audit export",
		Purpose:      "Streams the audit log to a SIEM as syslog, CEF, OTLP logs or a signed webhook.",
		ConfiguredBy: ByOrganisation, Setting: "Audit export destinations (Studio, Settings)",
		Default: DefaultNone, Data: "Audit entries of the organisation; secrets are never part of an entry.",
	},
	{
		Kind: KindGatewayUpstream, Name: "Gateway upstreams",
		Purpose:      "The services an app's devices reach through the secure gateway (SEC-030).",
		ConfiguredBy: ByOrganisation, Setting: "Gateway upstreams (Studio, Gateway)",
		Default: DefaultNone, Data: "The device's request, with the credentials the upstream's configuration names.",
	},
	{
		Kind: KindIdentityKeySet, Name: "Identity provider key sets",
		Purpose:      "Verifies the identity tokens a device exchanges at the gateway.",
		ConfiguredBy: ByOrganisation, Setting: "Gateway identity providers (Studio, Gateway)",
		Default: DefaultNone, Data: "A GET of the provider's discovery document and key set.",
	},
	{
		Kind: KindOIDC, Name: "OpenID Connect sign-in",
		Purpose:      "Single sign-on for people (SEC-100).",
		ConfiguredBy: ByOperator, Setting: "auth.studio.oidc.issuer",
		Default: DefaultNone, Data: "The authorisation code exchange and the provider's key set.",
	},
	{
		Kind: KindCIIssuer, Name: "CI provider keys",
		Purpose:      "Verifies the identity tokens of CI pipelines (SRV-064).",
		ConfiguredBy: ByOperator, Setting: "auth.ci.issuers",
		Default: DefaultNone, Data: "A GET of the provider's discovery document and key set.",
	},
	{
		Kind: KindGitRemote, Name: "Git remotes",
		Purpose:      "Exports an app to a Git repository on publish (GOV-011).",
		ConfiguredBy: ByOrganisation, Setting: "Git export (Studio, App settings)",
		Default: DefaultNone, Data: "The app's documents in the SCH-006 layout, with the stored credential.",
	},
	{
		Kind: KindTraceExport, Name: "Server traces",
		Purpose:      "Exports the server's own traces to the operator's collector (OBS-001).",
		ConfiguredBy: ByOperator, Setting: "observability.otlpEndpoint",
		Default: DefaultOff, Data: "Spans of the server's requests; no payloads, no secrets.",
	},
	{
		Kind: KindFunctionEgress, Name: "Function egress proxy",
		Purpose:      "The upstream calls of a Plux Function, through the CONNECT proxy of the function runner (FN-013).",
		ConfiguredBy: ByOrganisation, Setting: "The domains a function declares and an administrator approves",
		Default: DefaultOff, Data: "Whatever the function sends to a domain it is allowed.",
	},
}

// infrastructure lists the connections an installation makes to services
// its operator runs and names. They are not egress: nothing reaches a
// third party through them.
var infrastructure = []string{
	"PostgreSQL (`database.url`)",
	"The cache, Valkey or Redis (`cache.valkeyURL`)",
	"The object store (`objectStorage.*`)",
	"HashiCorp Vault or a PKCS#11 module for the signing keys (`signing.vault`, `signing.pkcs11`)",
	"The document scanner (`assets.malwareScanner`)",
	"Mutual-TLS calls between the roles of one installation (`internalTLS.*`)",
}

// Lookup returns the destination registered for a kind.
func Lookup(kind Kind) (Destination, error) {
	for _, d := range registry {
		if d.Kind == kind {
			return d, nil
		}
	}
	return Destination{}, fmt.Errorf("%w: %q", ErrUnregistered, kind)
}

// Destinations returns the registry, in the order of the egress list.
func Destinations() []Destination {
	out := make([]Destination, len(registry))
	for i, d := range registry {
		d.Hosts = slices.Clone(d.Hosts)
		out[i] = d
	}
	return out
}

// Infrastructure returns the connections the egress list names as not
// being egress.
func Infrastructure() []string { return slices.Clone(infrastructure) }
