// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package function

import (
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

// ValidateName checks a dotted function name (FN-006).
func ValidateName(name string) error {
	if len(name) > maxNameLength || !nameRE.MatchString(name) {
		return plxerr.New(plxerr.InvalidFormat, "%q is not a function name: dotted lower-camel-case segments, at most %d characters", name, maxNameLength)
	}
	return nil
}

// ValidateAlias checks an alias (FN-006).
func ValidateAlias(alias string) error {
	if len(alias) > maxAliasLength || !aliasRE.MatchString(alias) {
		return plxerr.New(plxerr.InvalidFormat, "%q is not an alias: lower-case letters, digits and hyphens, starting with a letter, at most %d characters", alias, maxAliasLength)
	}
	return nil
}

// deployPermission is the permission a deploy or an alias change needs in
// an environment: function.deploy.production, which needs a fresh step-up
// (GOV-005), in production, and function.deploy elsewhere (FN-007, D6).
func deployPermission(production bool) auth.Permission {
	if production {
		return auth.FunctionDeployProduction
	}
	return auth.FunctionDeploy
}

// checkDeployable decides whether a build may become a version: it must
// have succeeded and carry the builder's signed artifact statement
// (FN-009). In a production environment it must also have passed every
// stored test case (FN-031); elsewhere a version with failing tests is
// deployed, marked as such, so that developers can iterate (maintainer,
// 2026-10-14).
func checkDeployable(b Build, production bool) error {
	switch b.State {
	case BuildSucceeded:
	case BuildFailed:
		return plxerr.New(plxerr.FunctionBuildFailed, "the build failed; fix the reported errors and build again")
	default:
		return plxerr.New(plxerr.PreconditionFailed, "the build is %s; deploy it once it has succeeded", b.State)
	}
	if !b.Signed || b.KeyID == "" {
		return plxerr.New(plxerr.FunctionArtifactSignatureInvalid, "the build carries no signature of the function-artifact key; only artifacts the builder signed can be deployed")
	}
	if !production {
		return nil
	}
	for _, r := range b.TestResults {
		if !r.Passed {
			return plxerr.New(plxerr.FunctionTestsFailed, "the stored test case %q failed; a function with failing tests cannot be deployed", r.Name)
		}
	}
	return nil
}

// testsPassed reports whether every stored test case of a build passed.
func testsPassed(b Build) bool {
	for _, r := range b.TestResults {
		if !r.Passed {
			return false
		}
	}
	return true
}

// kvUsage is what a function's key-value store holds.
type kvUsage struct {
	Keys  int64
	Bytes int64
}

// checkKVPut decides whether a write fits the limits (FN-018). replaced is
// the size in bytes of the entry the write overwrites, or -1 when the key
// is new; key and value are the sizes of the write.
func checkKVPut(lim limits.Set, use kvUsage, replaced, key, value int64) error {
	if most := lim.Get(limits.FunctionKVValueBytes); value > most {
		return plxerr.New(plxerr.FunctionKVLimitExceeded, "a value of %d bytes is above function.kvValueBytes = %d", value, most)
	}
	keys, total := use.Keys, use.Bytes
	if replaced < 0 {
		keys++
	} else {
		total -= replaced
	}
	total += key + value
	if most := lim.Get(limits.FunctionKVKeys); keys > most {
		return plxerr.New(plxerr.FunctionKVLimitExceeded, "the store would hold %d keys, above function.kvKeys = %d", keys, most)
	}
	if most := lim.Get(limits.FunctionKVBytes); total > most {
		return plxerr.New(plxerr.FunctionKVLimitExceeded, "the store would hold %d bytes, above function.kvBytes = %d", total, most)
	}
	return nil
}
