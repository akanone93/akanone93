// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package config

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"strconv"
)

var errKeySize = errors.New("config: an Ed25519 public key has 32 bytes")

// FnRunner configures the fnrunner role, which runs function modules in
// WebAssembly sandboxes (FN-009 to FN-013, ADR-0064). It is read only when
// the process runs the role.
type FnRunner struct {
	// Listen is the address of the mutual-TLS listener the api role calls.
	Listen string `json:"listen"`
	// InternalTLS is the role's identity, used for its listener and for
	// its calls to the api and egress roles (SEC-043).
	InternalTLS InternalTLS `json:"internalTLS"`
	// APIURL is the base URL of the api role's internal listener, which
	// serves FunctionHostService.
	APIURL string `json:"apiURL"`
	// EgressAddr is the host:port of the egress role.
	EgressAddr string `json:"egressAddr"`
	// ArtifactKeys are the public keys that may have signed a function
	// artifact. They come from configuration only: the runner never
	// accepts a key from the api role (FN-009).
	ArtifactKeys []ArtifactKey `json:"artifactKeys"`
	// CacheDir persists compiled code between restarts; empty keeps it in
	// memory.
	CacheDir string `json:"cacheDir"`
	// MaxModules bounds the compiled modules kept in memory.
	MaxModules int `json:"maxModules"`
	// MaxModuleBytes bounds the total size of the module bytes the
	// compiled modules were built from.
	MaxModuleBytes int64 `json:"maxModuleBytes"`
	// MaxConcurrent bounds the invocations running at once.
	MaxConcurrent int `json:"maxConcurrent"`
	// MaxMemoryBytes is the largest memory any invocation may be given:
	// the registry maximum of function.memory (FN-011).
	MaxMemoryBytes uint64 `json:"maxMemoryBytes"`
	// HTTPResponseBytes bounds the body of an outbound response a function
	// reads (FN-011, FN-017).
	HTTPResponseBytes int64 `json:"httpResponseBytes"`
}

// ArtifactKey is a trusted Ed25519 public key for function artifacts.
type ArtifactKey struct {
	// KeyID names the key in GetModuleResponse.key_id.
	KeyID string `json:"keyID"`
	// PublicKey is the raw 32-byte Ed25519 public key, base64 encoded.
	PublicKey string `json:"publicKey"`
}

// Decode returns the key's bytes, or an error when it is not an Ed25519
// public key.
func (k ArtifactKey) Decode() (ed25519.PublicKey, error) {
	raw, err := base64.StdEncoding.DecodeString(k.PublicKey)
	if err != nil {
		return nil, err //nolint:wrapcheck // the caller names the field
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, errKeySize
	}
	return ed25519.PublicKey(raw), nil
}

// validateFnRunner checks the fnrunner section when the process runs the
// role. The role runs tenant code, so its listener requires mutual TLS
// (SEC-043), at least one trusted artifact key exists (FN-009), and its
// bounds are never unlimited (FN-011).
func (c *Config) validateFnRunner(p *problems) {
	if !c.Has(RoleFnRunner) {
		return
	}
	f := c.FnRunner
	if f.Listen == "" {
		p.addf("fnrunner.listen", "must be set when the fnrunner role runs")
	}
	if f.InternalTLS.CAFile == "" || f.InternalTLS.CertFile == "" || f.InternalTLS.KeyFile == "" {
		p.addf("fnrunner.internalTLS", "caFile, certFile and keyFile must all be set: the role requires mutual TLS (SEC-043)")
	}
	if f.APIURL == "" {
		p.addf("fnrunner.apiURL", "must be set: the runner fetches modules and reaches the key-value store through the api role")
	} else {
		checkURL(p, "fnrunner.apiURL", f.APIURL)
	}
	if f.EgressAddr == "" {
		p.addf("fnrunner.egressAddr", "must be set: outbound HTTP of functions goes through the egress role (FN-017)")
	}
	c.validateArtifactKeys(p)
	p.positive("fnrunner.maxModules", int64(f.MaxModules))
	p.positive("fnrunner.maxModuleBytes", f.MaxModuleBytes)
	p.positive("fnrunner.maxConcurrent", int64(f.MaxConcurrent))
	if f.MaxMemoryBytes == 0 || f.MaxMemoryBytes > 1<<31 {
		p.addf("fnrunner.maxMemoryBytes", "must be between 1 and 2 GiB")
	}
	p.positive("fnrunner.httpResponseBytes", f.HTTPResponseBytes)
}

// validateArtifactKeys requires at least one well-formed, distinct key.
func (c *Config) validateArtifactKeys(p *problems) {
	keys := c.FnRunner.ArtifactKeys
	if len(keys) == 0 {
		p.addf("fnrunner.artifactKeys", "must hold at least one key: the runner refuses every artifact it cannot verify (FN-009)")
	}
	seen := map[string]bool{}
	for i, k := range keys {
		path := "fnrunner.artifactKeys[" + strconv.Itoa(i) + "]"
		if k.KeyID == "" {
			p.addf(path+".keyID", "must be set")
		} else if seen[k.KeyID] {
			p.addf(path+".keyID", "%q is listed twice", k.KeyID)
		}
		seen[k.KeyID] = true
		if _, err := k.Decode(); err != nil {
			p.addf(path+".publicKey", "must be a base64 Ed25519 public key of 32 bytes")
		}
	}
}
