// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package function

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// Capabilities are the host capabilities a function declares; everything
// not declared is denied (FN-012).
type Capabilities struct {
	// HTTP are the domains the function may reach through egress.
	HTTP []string `json:"http"`
	// KV says whether the function may use its key-value namespace.
	KV bool `json:"kv"`
	// Secrets are the names of the secrets the function may read.
	Secrets []string `json:"secrets"`
	// Functions are the names of the functions it may call.
	Functions []string `json:"functions"`
	// Claims says whether it may read the verified user claims.
	Claims bool `json:"claims"`
}

// DeclaredLimits are the limits a function document sets; each only
// tightens the registry value (FN-011, LIM-002). Zero means undeclared.
type DeclaredLimits struct {
	TimeoutMs   int64 `json:"timeoutMs"`
	MemoryBytes int64 `json:"memoryBytes"`
	OutputBytes int64 `json:"outputBytes"`
	HTTPCalls   int64 `json:"httpCalls"`
}

// Declaration is what the invocation path reads of a function document:
// the declared capabilities, limits and requirements (FN-012, FN-020).
type Declaration struct {
	Name         string         `json:"name"`
	Placement    Placement      `json:"placement"`
	Capabilities Capabilities   `json:"capabilities"`
	Limits       DeclaredLimits `json:"limits"`
	// Assurance is the level a device must hold; empty means the
	// environment's defaultFunctionAssurance applies (SEC-007).
	Assurance string `json:"assurance"`
	// Attestation requires a fresh attestation of the device.
	Attestation bool `json:"attestation"`
	// SCA requires strong customer authentication (SEC-027).
	SCA bool `json:"sca"`
	// Env names the environment variables the function reads; the
	// invocation passes their values for its environment.
	Env []string `json:"env"`
}

// Invocable is a version with its declaration: what a host call or an
// invocation needs to decide.
type Invocable struct {
	Version     Version
	Declaration Declaration
}

// LoadInvocable reads a version of a function and the declaration of the
// build that made it. It is for the server's own callers, which have
// authorised the request. A version that does not exist is PLX-7012.
func (s *Service) LoadInvocable(ctx context.Context, organizationID, appID, environmentID, name string, version int32) (Invocable, error) {
	var out Invocable
	err := s.inTenant(ctx, storage.Tenant{OrganizationID: organizationID}, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		env, err := environmentOf(ctx, q, appID, environmentID)
		if err != nil {
			return err
		}
		row, err := q.GetFunctionVersion(ctx, dbgen.GetFunctionVersionParams{EnvironmentID: env.ID, Name: name, Version: version})
		if errors.Is(err, pgx.ErrNoRows) {
			return plxerr.New(plxerr.FunctionNotFound, "no version %d of function %s is deployed in this environment", version, name)
		}
		if err != nil {
			return fmt.Errorf("function: read a version: %w", err)
		}
		if out.Version, err = versionOf(row); err != nil {
			return err
		}
		decl, err := q.GetFunctionDeclaration(ctx, row.BuildID)
		if err != nil {
			return fmt.Errorf("function: read the declaration of version %d of %s: %w", version, name, err)
		}
		if err := json.Unmarshal(decl.Declaration, &out.Declaration); err != nil {
			return fmt.Errorf("function: read the declaration of version %d of %s: %w", version, name, err)
		}
		return nil
	})
	return out, err
}

// Module returns the bytes of a version's module from the object store.
// It refuses bytes whose SHA-256 is not the version's: the store is not
// trusted to be the build's (FN-005).
func (s *Service) Module(ctx context.Context, v Version) ([]byte, error) {
	data, _, err := s.o.Objects.Get(ctx, v.ModuleKey)
	if err != nil {
		return nil, fmt.Errorf("function: read the module of version %d of %s: %w", v.Version, v.Name, err)
	}
	if sum := sha256.Sum256(data); !bytes.Equal(sum[:], v.ModuleSHA256) {
		return nil, fmt.Errorf("function: the stored module of version %d of %s is not the one that was built", v.Version, v.Name)
	}
	return data, nil
}

// HexSHA256 is the form of a module's SHA-256 on the internal protocol.
func HexSHA256(sum []byte) string { return hex.EncodeToString(sum) }

// Pin is what an active release records of a function version it pins
// (FN-006).
type Pin struct {
	ModuleSHA256 []byte
	Placement    Placement
}

// PinChecker says whether an active release of an app and environment pins
// a version of a function (FN-006, ADR-0063): a device invokes only
// versions its release carries.
type PinChecker interface {
	// Pinned returns the pin, and false when no active release pins the
	// version.
	Pinned(ctx context.Context, organizationID, appID, environmentID, name string, version int32) (Pin, bool, error)
}

var _ PinChecker = (*Service)(nil)

// Pinned reads release_functions: a version is pinned when a release that
// a channel of the environment currently points at records it.
func (s *Service) Pinned(ctx context.Context, organizationID, appID, environmentID, name string, version int32) (Pin, bool, error) {
	app, err := parseID(appID, "app")
	if err != nil {
		return Pin{}, false, err
	}
	env, err := parseID(environmentID, "environment")
	if err != nil {
		return Pin{}, false, err
	}
	var (
		pin   Pin
		found bool
	)
	err = s.inTenant(ctx, storage.Tenant{OrganizationID: organizationID}, func(ctx context.Context, tx pgx.Tx) error {
		row, err := dbgen.New(tx).GetPinnedFunction(ctx, dbgen.GetPinnedFunctionParams{AppID: app, EnvironmentID: env, Name: name, Version: version})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("function: read a pin: %w", err)
		}
		pin, found = Pin{ModuleSHA256: row.ModuleSha256, Placement: Placement(row.Placement)}, true
		return nil
	})
	return pin, found, err
}

// EnvironmentVariables returns the values of the named environment
// variables of an environment; a name it holds no value for is absent from
// the result.
func (s *Service) EnvironmentVariables(ctx context.Context, organizationID, appID, environmentID string, names []string) (map[string]string, error) {
	out := make(map[string]string, len(names))
	if len(names) == 0 {
		return out, nil
	}
	err := s.inTenant(ctx, storage.Tenant{OrganizationID: organizationID}, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		env, err := environmentOf(ctx, q, appID, environmentID)
		if err != nil {
			return err
		}
		rows, err := q.GetEnvironmentVariablesByKey(ctx, dbgen.GetEnvironmentVariablesByKeyParams{EnvironmentID: env.ID, Keys: names})
		if err != nil {
			return fmt.Errorf("function: read environment variables: %w", err)
		}
		for _, r := range rows {
			out[r.Key] = r.Value
		}
		return nil
	})
	return out, err
}
