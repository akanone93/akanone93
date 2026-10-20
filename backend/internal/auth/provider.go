// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package auth

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/edition"
	"github.com/nightCode42/plux3/backend/ext/identity"
	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/signing"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// This file administers identity providers (GOV-004, ADR-0074): the OpenID
// Connect and SAML providers an installation or an organisation lets people
// sign in with. A provider's non-secret settings are a JSON document; its
// secret (an OpenID client secret, a SAML service provider's private key)
// is sealed and bound to the provider's identifier, and is never returned.

// The kinds of identity provider.
const (
	// ProviderOIDC is an OpenID Connect provider.
	ProviderOIDC = "oidc"
	// ProviderSAML is a SAML 2.0 provider, an enterprise feature.
	ProviderSAML = "saml"
)

// providerKeyForm is the form of a provider's key, as for every key.
var providerKeyForm = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,62}[a-z0-9])?$`)

// OIDCSettings are an OpenID Connect provider's settings.
type OIDCSettings struct {
	Issuer      string
	ClientID    string
	RedirectURL string
	// HasSecret is true when a client secret is stored.
	HasSecret bool
}

// SAMLSettings are a SAML provider's settings.
type SAMLSettings struct {
	// EntityID is the service provider's identifier, the audience.
	EntityID string
	// ACSURL is Studio's assertion consumer service URL.
	ACSURL            string
	IdP               identity.IdPDescriptor
	AllowIdPInitiated bool
	EmailAttribute    string
	NameAttribute     string
	GroupsAttribute   string
	// ServiceProviderCertificate is the PEM certificate for encrypted
	// assertions.
	ServiceProviderCertificate string
}

// Provider is a configured identity provider.
type Provider struct {
	ID string
	// OrganizationID is "" for a provider of the whole installation.
	OrganizationID string
	Key            string
	Kind           string
	Name           string
	Enabled        bool
	// JIT provisions an account for a person the provider vouches for and
	// no account exists for.
	JIT       bool
	OIDC      *OIDCSettings
	SAML      *SAMLSettings
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ProviderSpec is what a provider is created or changed to.
type ProviderSpec struct {
	// Key and Kind are fixed when the provider is created.
	Key  string
	Kind string
	Name string
	// Enabled and JIT are the provider's switches.
	Enabled bool
	JIT     bool
	OIDC    *OIDCSettings
	// ClientSecret is the OpenID client secret; empty keeps the stored one
	// on update and is refused on create.
	ClientSecret string
	SAML         *SAMLSettings
	// IdPMetadata is the SAML identity provider's metadata document; empty
	// keeps the stored descriptor on update and is refused on create.
	IdPMetadata []byte
}

// storedOIDC and storedSAML are the JSON documents of identity_providers.config.
type storedOIDC struct {
	Issuer      string `json:"issuer"`
	ClientID    string `json:"clientId"`
	RedirectURL string `json:"redirectUrl"`
}

type storedSAML struct {
	EntityID          string                 `json:"entityId"`
	ACSURL            string                 `json:"acsUrl"`
	IdP               identity.IdPDescriptor `json:"idp"`
	AllowIdPInitiated bool                   `json:"allowIdpInitiated"`
	EmailAttribute    string                 `json:"emailAttribute,omitempty"`
	NameAttribute     string                 `json:"nameAttribute,omitempty"`
	GroupsAttribute   string                 `json:"groupsAttribute,omitempty"`
	SPCertificate     string                 `json:"serviceProviderCertificate,omitempty"`
}

// ProviderScope is the authority under which providers are administered:
// an organisation's, with sso.manage, or the installation's, with an
// installation administrator who presented a second factor.
type ProviderScope struct {
	organizationID string
	userID         string
	actor          audit.Actor
	// principal is the caller in an organisation's own scope; the roles a
	// mapping gives are limited to those the caller holds.
	principal *Principal
}

// OrganizationProviders returns the scope of the providers of the
// organisation the principal acts in. It needs sso.manage.
func OrganizationProviders(p Principal) (ProviderScope, error) {
	if err := p.Authorize(SSOManage); err != nil {
		return ProviderScope{}, err
	}
	return ProviderScope{organizationID: p.OrganizationID, userID: p.UserID, actor: p.Actor(), principal: &p}, nil
}

// InstallationProviders returns the scope of the providers of the whole
// installation. It needs an installation administrator whose session
// presented a second factor, as sso.manage does in an organisation.
func InstallationProviders(id Identity) (ProviderScope, error) {
	if !id.InstallationAdmin {
		return ProviderScope{}, plxerr.New(plxerr.PermissionDenied, "only an installation administrator manages the installation's identity providers")
	}
	if !id.SecondFactor {
		return ProviderScope{}, plxerr.New(plxerr.MultiFactorRequired, "%s requires a second factor in this session", SSOManage)
	}
	return ProviderScope{userID: id.UserID, actor: id.Actor()}, nil
}

// tenant is what a transaction under this scope may see.
func (sc ProviderScope) tenant() storage.Tenant {
	if sc.organizationID == "" {
		return storage.Tenant{Scope: storage.ScopeInstallation}
	}
	return storage.Tenant{OrganizationID: sc.organizationID}
}

// owns reports whether a stored provider is one this scope administers.
func (sc ProviderScope) owns(row dbgen.IdentityProvider) bool {
	return storage.ID(row.OrganizationID) == sc.organizationID
}

// providerOf converts a stored provider, without its secret.
func providerOf(row dbgen.IdentityProvider) (Provider, error) {
	p := Provider{
		ID: storage.ID(row.ID), OrganizationID: storage.ID(row.OrganizationID), Key: row.Key, Kind: row.Kind,
		Name: row.Name, Enabled: row.Enabled, JIT: row.JitProvisioning,
		CreatedAt: storage.Time(row.CreatedAt), UpdatedAt: storage.Time(row.UpdatedAt),
	}
	switch row.Kind {
	case ProviderOIDC:
		var c storedOIDC
		if err := json.Unmarshal(row.Config, &c); err != nil {
			return Provider{}, fmt.Errorf("auth: read an OIDC provider's settings: %w", err)
		}
		p.OIDC = &OIDCSettings{Issuer: c.Issuer, ClientID: c.ClientID, RedirectURL: c.RedirectURL, HasSecret: len(row.Secret) > 0}
	case ProviderSAML:
		var c storedSAML
		if err := json.Unmarshal(row.Config, &c); err != nil {
			return Provider{}, fmt.Errorf("auth: read a SAML provider's settings: %w", err)
		}
		p.SAML = &SAMLSettings{
			EntityID: c.EntityID, ACSURL: c.ACSURL, IdP: c.IdP, AllowIdPInitiated: c.AllowIdPInitiated,
			EmailAttribute: c.EmailAttribute, NameAttribute: c.NameAttribute, GroupsAttribute: c.GroupsAttribute,
			ServiceProviderCertificate: c.SPCertificate,
		}
	}
	return p, nil
}

// providerSecretBinding ties a provider's sealed secret to the provider.
func providerSecretBinding(id string) []byte { return []byte("idp-secret:" + id) }

// httpsURL reports whether s is an https URL with a host.
func httpsURL(s string) bool {
	return strings.HasPrefix(s, "https://") && len(s) > len("https://") && !strings.ContainsAny(s, " \t\r\n")
}

// checkOIDCSettings validates an OpenID Connect provider's settings.
func checkOIDCSettings(o *OIDCSettings, secret string, creating bool) error {
	switch {
	case o == nil:
		return plxerr.New(plxerr.InvalidStructure, "an OpenID Connect provider names its issuer, client and redirect URL")
	case !httpsURL(o.Issuer):
		return plxerr.New(plxerr.InvalidFormat, "the OpenID Connect issuer must be an https URL")
	case !httpsURL(o.RedirectURL):
		return plxerr.New(plxerr.InvalidFormat, "the redirect URL must be an https URL registered with the provider")
	case o.ClientID == "":
		return plxerr.New(plxerr.InvalidStructure, "the OpenID Connect client identifier is required")
	case creating && secret == "":
		return plxerr.New(plxerr.InvalidStructure, "the OpenID Connect client secret is required")
	}
	return nil
}

// checkProviderText validates the fields every provider has.
func checkProviderText(spec ProviderSpec, creating bool) error {
	if creating {
		if !providerKeyForm.MatchString(spec.Key) {
			return plxerr.New(plxerr.InvalidFormat, "a provider key is lower-case letters, digits and hyphens")
		}
		if spec.Kind != ProviderOIDC && spec.Kind != ProviderSAML {
			return plxerr.New(plxerr.InvalidEnumValue, "the kind of a provider is %q or %q", ProviderOIDC, ProviderSAML)
		}
	}
	if n := strings.TrimSpace(spec.Name); n == "" || len(n) > 120 {
		return plxerr.New(plxerr.InvalidFormat, "a provider has a name of up to 120 characters")
	}
	return nil
}

// settingsDocument builds a provider's config document and, when new
// material is made for it, its sealed secret. existing is the stored row on
// update and nil on create.
func (s *Service) settingsDocument(ctx context.Context, id string, spec ProviderSpec, existing *dbgen.IdentityProvider) (config []byte, sealed []byte, err error) {
	if spec.Kind == ProviderOIDC {
		if err := checkOIDCSettings(spec.OIDC, spec.ClientSecret, existing == nil); err != nil {
			return nil, nil, err
		}
		config, err = json.Marshal(storedOIDC{Issuer: strings.TrimSuffix(spec.OIDC.Issuer, "/"), ClientID: spec.OIDC.ClientID, RedirectURL: spec.OIDC.RedirectURL})
		if err != nil {
			return nil, nil, fmt.Errorf("auth: encode settings: %w", err)
		}
		if spec.ClientSecret != "" {
			sealed, err = s.sealProviderSecret(ctx, id, []byte(spec.ClientSecret))
		}
		return config, sealed, err
	}
	return s.samlDocument(ctx, id, spec, existing)
}

// samlDocument is settingsDocument for a SAML provider.
func (s *Service) samlDocument(ctx context.Context, id string, spec ProviderSpec, existing *dbgen.IdentityProvider) (config, sealed []byte, err error) {
	if err := edition.Require(ctx, edition.FeatureSAML); err != nil {
		return nil, nil, err //nolint:wrapcheck // a domain error
	}
	c := spec.SAML
	if c == nil || !httpsURL(c.ACSURL) {
		return nil, nil, plxerr.New(plxerr.InvalidFormat, "a SAML provider names its assertion consumer service URL, which must be https")
	}
	doc := storedSAML{
		EntityID: c.EntityID, ACSURL: c.ACSURL, AllowIdPInitiated: c.AllowIdPInitiated,
		EmailAttribute: c.EmailAttribute, NameAttribute: c.NameAttribute, GroupsAttribute: c.GroupsAttribute,
	}
	if doc.EntityID == "" {
		doc.EntityID = c.ACSURL
	}
	if existing != nil {
		var old storedSAML
		if err := json.Unmarshal(existing.Config, &old); err != nil {
			return nil, nil, fmt.Errorf("auth: read a SAML provider's settings: %w", err)
		}
		doc.IdP, doc.SPCertificate = old.IdP, old.SPCertificate
	}
	if len(spec.IdPMetadata) > 0 {
		if doc.IdP, err = s.o.SAML.ImportMetadata(ctx, spec.IdPMetadata); err != nil {
			return nil, nil, importFailure(err)
		}
	}
	if doc.IdP.EntityID == "" {
		return nil, nil, plxerr.New(plxerr.InvalidStructure, "a SAML provider needs the identity provider's metadata")
	}
	if existing == nil {
		key, cert, err := s.o.SAML.GenerateServiceKey(rand.Reader, s.now(), doc.EntityID)
		if err != nil {
			return nil, nil, importFailure(err)
		}
		if sealed, err = s.sealProviderSecret(ctx, id, key); err != nil {
			return nil, nil, err
		}
		doc.SPCertificate = string(cert)
	}
	config, err = json.Marshal(doc)
	if err != nil {
		return nil, nil, fmt.Errorf("auth: encode settings: %w", err)
	}
	return config, sealed, nil
}

// importFailure reports a SAML descriptor or key that could not be made.
func importFailure(err error) error {
	if _, ok := plxerr.CodeOf(err); ok {
		return err
	}
	return plxerr.Wrap(plxerr.InvalidStructure, err, "the SAML identity provider's metadata cannot be used")
}

// sealProviderSecret seals a provider's secret, bound to the provider.
func (s *Service) sealProviderSecret(ctx context.Context, id string, secret []byte) ([]byte, error) {
	sealed, err := signing.Seal(ctx, s.o.Crypter, secret, providerSecretBinding(id))
	if err != nil {
		return nil, fmt.Errorf("auth: %w", err)
	}
	return sealed.Encode(), nil
}

// openProviderSecret opens a provider's sealed secret.
func (s *Service) openProviderSecret(ctx context.Context, row dbgen.IdentityProvider) ([]byte, error) {
	if len(row.Secret) == 0 {
		return nil, nil
	}
	sealed, err := signing.DecodeSealed(row.Secret)
	if err != nil {
		return nil, fmt.Errorf("auth: %w", err)
	}
	secret, err := signing.Open(ctx, s.o.Crypter, sealed, providerSecretBinding(storage.ID(row.ID)))
	if err != nil {
		return nil, fmt.Errorf("auth: %w", err)
	}
	return secret, nil
}

// providerDetail is the audit detail of a provider; it never carries a
// secret.
func providerDetail(p Provider) string {
	return fmt.Sprintf("key=%s kind=%s enabled=%t jit=%t", p.Key, p.Kind, p.Enabled, p.JIT)
}

// CreateProvider configures an identity provider (GOV-004). A SAML provider
// is an enterprise feature (PLX-8600). Just-in-time provisioning starts as
// the spec says, and the spec's zero value is off.
func (s *Service) CreateProvider(ctx context.Context, sc ProviderScope, spec ProviderSpec) (Provider, error) {
	if err := checkProviderText(spec, true); err != nil {
		return Provider{}, err
	}
	id, err := s.newID()
	if err != nil {
		return Provider{}, err
	}
	config, sealed, err := s.settingsDocument(ctx, id, spec, nil)
	if err != nil {
		return Provider{}, err
	}
	org, err := storage.NullUUID(sc.organizationID)
	if err != nil {
		return Provider{}, plxerr.New(plxerr.InvalidFormat, "the organisation identifier is not valid")
	}
	user, _ := storage.NullUUID(sc.userID)
	var out Provider
	err = s.inTx(ctx, sc.tenant(), func(ctx context.Context, tx pgx.Tx) error {
		row, err := dbgen.New(tx).CreateIdentityProvider(ctx, dbgen.CreateIdentityProviderParams{
			ID: storage.MustUUID(id), OrganizationID: org, Key: spec.Key, Kind: spec.Kind, Name: strings.TrimSpace(spec.Name),
			Enabled: spec.Enabled, JitProvisioning: spec.JIT, Config: config, Secret: sealed, CreatedBy: user,
		})
		if err != nil {
			if uniqueViolation(err) {
				return plxerr.New(plxerr.ResourceExists, "a provider with this key exists")
			}
			return fmt.Errorf("auth: create a provider: %w", err)
		}
		if out, err = providerOf(row); err != nil {
			return err
		}
		return s.record(ctx, tx, audit.Entry{
			OrganizationID: sc.organizationID, Actor: sc.actor, Action: audit.ProviderCreated,
			TargetKind: "identity_provider", TargetID: id, Detail: providerDetail(out),
		})
	})
	return out, err
}

// loadOwned reads a provider this scope administers; one it does not is
// reported as not found, so that another scope's providers are not
// disclosed.
func loadOwned(ctx context.Context, q *dbgen.Queries, sc ProviderScope, id string) (dbgen.IdentityProvider, error) {
	uid, err := parseID(id, "provider")
	if err != nil {
		return dbgen.IdentityProvider{}, err
	}
	row, err := q.GetIdentityProvider(ctx, uid)
	if err != nil {
		return dbgen.IdentityProvider{}, notFound(err, "identity provider")
	}
	if !sc.owns(row) {
		return dbgen.IdentityProvider{}, plxerr.New(plxerr.ResourceNotFound, "no such identity provider")
	}
	return row, nil
}

// GetProvider returns one provider of the scope.
func (s *Service) GetProvider(ctx context.Context, sc ProviderScope, id string) (Provider, error) {
	var out Provider
	err := s.inTx(ctx, sc.tenant(), func(ctx context.Context, tx pgx.Tx) error {
		row, err := loadOwned(ctx, dbgen.New(tx), sc, id)
		if err != nil {
			return err
		}
		out, err = providerOf(row)
		return err
	})
	return out, err
}

// ListProviders lists the providers of the scope by key, after a key.
func (s *Service) ListProviders(ctx context.Context, sc ProviderScope, afterKey string, limit int32) ([]Provider, error) {
	org, err := storage.NullUUID(sc.organizationID)
	if err != nil {
		return nil, plxerr.New(plxerr.InvalidFormat, "the organisation identifier is not valid")
	}
	var out []Provider
	err = s.inTx(ctx, sc.tenant(), func(ctx context.Context, tx pgx.Tx) error {
		rows, err := dbgen.New(tx).ListIdentityProviders(ctx, dbgen.ListIdentityProvidersParams{OrganizationID: org, AfterKey: afterKey, PageSize: limit})
		if err != nil {
			return fmt.Errorf("auth: list providers: %w", err)
		}
		for _, row := range rows {
			p, err := providerOf(row)
			if err != nil {
				return err
			}
			out = append(out, p)
		}
		return nil
	})
	return out, err
}

// UpdateProvider changes a provider's name, switches and settings. Its key
// and kind are fixed. An empty client secret or metadata keeps the stored
// one.
func (s *Service) UpdateProvider(ctx context.Context, sc ProviderScope, id string, spec ProviderSpec) (Provider, error) {
	if err := checkProviderText(spec, false); err != nil {
		return Provider{}, err
	}
	var out Provider
	err := s.inTx(ctx, sc.tenant(), func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		row, err := loadOwned(ctx, q, sc, id)
		if err != nil {
			return err
		}
		if spec.Kind != "" && spec.Kind != row.Kind {
			return plxerr.New(plxerr.InvalidEnumValue, "the kind of a provider cannot change")
		}
		if !spec.Enabled && row.Enabled {
			if err := s.keepsPasswordPolicy(ctx, q, row); err != nil {
				return err
			}
		}
		spec.Kind = row.Kind
		config, sealed, err := s.settingsDocument(ctx, id, spec, &row)
		if err != nil {
			return err
		}
		updated, err := q.UpdateIdentityProvider(ctx, dbgen.UpdateIdentityProviderParams{
			ID: row.ID, Name: strings.TrimSpace(spec.Name), Enabled: spec.Enabled, JitProvisioning: spec.JIT,
			Config: config, Secret: sealed,
		})
		if err != nil {
			return fmt.Errorf("auth: update a provider: %w", err)
		}
		if out, err = providerOf(updated); err != nil {
			return err
		}
		s.forgetProvider(row.ID)
		return s.record(ctx, tx, audit.Entry{
			OrganizationID: sc.organizationID, Actor: sc.actor, Action: audit.ProviderUpdated,
			TargetKind: "identity_provider", TargetID: id, Detail: providerDetail(out),
		})
	})
	return out, err
}

// keepsPasswordPolicy refuses taking away the last installation provider
// while password sign-in is off: nobody could then sign in
// (auth.studio.allowPasswordLogin may be false only once a provider
// exists).
func (s *Service) keepsPasswordPolicy(ctx context.Context, q *dbgen.Queries, row dbgen.IdentityProvider) error {
	if !s.o.PasswordLoginDisabled || row.OrganizationID.Valid {
		return nil
	}
	if s.o.OIDC != nil {
		return nil
	}
	others, err := q.CountOtherEnabledInstallationProviders(ctx, row.ID)
	if err != nil {
		return fmt.Errorf("auth: count providers: %w", err)
	}
	if others == 0 {
		return plxerr.New(plxerr.PreconditionFailed,
			"password sign-in is turned off (auth.studio.allowPasswordLogin), so this is the last way in; add another installation provider first")
	}
	return nil
}

// DeleteProvider removes a provider with the grants and team memberships its
// mapping gave, which belong to it and to nobody else.
func (s *Service) DeleteProvider(ctx context.Context, sc ProviderScope, id string) error {
	var row dbgen.IdentityProvider
	err := s.inTx(ctx, sc.tenant(), func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		var err error
		if row, err = loadOwned(ctx, q, sc, id); err != nil {
			return err
		}
		if row.Enabled {
			return s.keepsPasswordPolicy(ctx, q, row)
		}
		return nil
	})
	if err != nil {
		return err
	}
	// What the mapping gave lives in the organisations, each of which is
	// visited in its own transaction; a retry finds the rest.
	if err := s.dropProviderAssignments(ctx, row.ID); err != nil {
		return err
	}
	return s.inTx(ctx, sc.tenant(), func(ctx context.Context, tx pgx.Tx) error {
		if _, err := dbgen.New(tx).DeleteIdentityProvider(ctx, row.ID); err != nil {
			return fmt.Errorf("auth: delete a provider: %w", err)
		}
		s.forgetProvider(row.ID)
		return s.record(ctx, tx, audit.Entry{
			OrganizationID: sc.organizationID, Actor: sc.actor, Action: audit.ProviderDeleted,
			TargetKind: "identity_provider", TargetID: id, Detail: "key=" + row.Key,
		})
	})
}
