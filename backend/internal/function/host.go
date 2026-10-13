// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package function

import (
	"context"
	"crypto/subtle"
	"errors"
	"slices"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// SecretReader reads a server-held secret of an environment (ADR-0061).
type SecretReader interface {
	// Secret returns the text of the named value, and false when the
	// environment holds no such server-bound value.
	Secret(ctx context.Context, organizationID, appID, environmentID, name string) (string, bool, error)
}

// HostOptions configures a Host.
type HostOptions struct {
	Service *Service
	Grants  *Grants
	Secrets SecretReader
}

// Host is the domain side of FunctionHostService: what the function runner
// may ask of the api role while it runs an invocation (ADR-0064). Every
// call presents the invocation's grant; the host takes the tenant, the
// function and its version from the grant, never from the caller, and
// checks the call against the version's declared capabilities (FN-012).
// It is safe for concurrent use.
type Host struct{ o HostOptions }

// NewHost returns a host.
func NewHost(o HostOptions) (*Host, error) {
	if o.Service == nil || o.Grants == nil {
		return nil, errors.New("function: a host needs the function service and the grants")
	}
	return &Host{o: o}, nil
}

// ModuleArtifact is a version's module with the signed statement the
// runner verifies before it compiles (FN-009, ADR-0063).
type ModuleArtifact struct {
	Module    []byte
	Statement []byte
	Signature []byte
	KeyID     string
}

// authorise verifies the grant and loads the declaration of the version
// it names.
func (h *Host) authorise(ctx context.Context, g Grant) (GrantClaims, Invocable, error) {
	c, err := h.o.Grants.Verify(g)
	if err != nil {
		return GrantClaims{}, Invocable{}, err
	}
	inv, err := h.o.Service.LoadInvocable(ctx, c.OrganizationID, c.AppID, c.EnvironmentID, c.Function, c.Version)
	if err != nil {
		return GrantClaims{}, Invocable{}, err
	}
	return c, inv, nil
}

// GetModule returns the module the grant was issued for. A grant reaches
// its own module only: another hash is refused, however valid the hash
// (ADR-0064).
func (h *Host) GetModule(ctx context.Context, g Grant, moduleSHA256 string) (ModuleArtifact, error) {
	c, inv, err := h.authorise(ctx, g)
	if err != nil {
		return ModuleArtifact{}, err
	}
	if subtle.ConstantTimeCompare([]byte(moduleSHA256), []byte(c.ModuleSHA256)) != 1 || moduleSHA256 != HexSHA256(inv.Version.ModuleSHA256) {
		return ModuleArtifact{}, plxerr.New(plxerr.PermissionDenied, "the grant does not reach this module")
	}
	module, err := h.o.Service.Module(ctx, inv.Version)
	if err != nil {
		return ModuleArtifact{}, err
	}
	return ModuleArtifact{Module: module, Statement: inv.Version.Statement, Signature: inv.Version.Signature, KeyID: inv.Version.KeyID}, nil
}

// kvScope authorises a key-value call: the version must declare kv
// (FN-012), and the namespace is the grant's (FN-018).
func (h *Host) kvScope(ctx context.Context, g Grant) (KVScope, error) {
	c, inv, err := h.authorise(ctx, g)
	if err != nil {
		return KVScope{}, err
	}
	if !inv.Declaration.Capabilities.KV {
		return KVScope{}, plxerr.New(plxerr.FunctionCapabilityDenied, "the function %s does not declare the kv capability", c.Function)
	}
	return c.KVScope(), nil
}

// KVGet reads a key of the grant's namespace.
func (h *Host) KVGet(ctx context.Context, g Grant, key string) ([]byte, bool, error) {
	sc, err := h.kvScope(ctx, g)
	if err != nil {
		return nil, false, err
	}
	return h.o.Service.KVGet(ctx, sc, key)
}

// KVPut writes a key of the grant's namespace, within the registry's
// limits (FN-018, PLX-7016).
func (h *Host) KVPut(ctx context.Context, g Grant, key string, value []byte) error {
	sc, err := h.kvScope(ctx, g)
	if err != nil {
		return err
	}
	return h.o.Service.KVPut(ctx, sc, key, value)
}

// KVDelete removes a key of the grant's namespace.
func (h *Host) KVDelete(ctx context.Context, g Grant, key string) (bool, error) {
	sc, err := h.kvScope(ctx, g)
	if err != nil {
		return false, err
	}
	return h.o.Service.KVDelete(ctx, sc, key)
}

// Secret returns a secret the version declares by name (FN-012, ADR-0064).
// A name that is not declared is PLX-7003, which tells the caller no more
// than the declaration it already has; a function cannot list names.
func (h *Host) Secret(ctx context.Context, g Grant, name string) ([]byte, bool, error) {
	c, inv, err := h.authorise(ctx, g)
	if err != nil {
		return nil, false, err
	}
	if !slices.Contains(inv.Declaration.Capabilities.Secrets, name) {
		return nil, false, plxerr.New(plxerr.FunctionCapabilityDenied, "the function %s does not declare the secret %q", c.Function, name)
	}
	if h.o.Secrets == nil {
		return nil, false, plxerr.New(plxerr.ConfigStoreUnavailable, "this server holds no secrets for functions")
	}
	value, found, err := h.o.Secrets.Secret(ctx, c.OrganizationID, c.AppID, c.EnvironmentID, name)
	if err != nil || !found {
		return nil, false, err
	}
	return []byte(value), true, nil
}
