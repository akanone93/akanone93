// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package fnrunner

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/nightCode42/plux3/backend/internal/fnpb"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// maxStatementBytes bounds the statement the runner parses. The statement is
// signed, but it is parsed only after the signature verified; the bound
// keeps a misconfigured builder from costing memory.
const maxStatementBytes = 1 << 20

// statementDigestField is the field of the artifact statement that names the
// module (ADR-0063). The runner reads no other field: the statement format
// belongs to the builder.
const statementDigestField = "moduleSha256"

// parseDigest decodes the hex SHA-256 of a module reference.
func parseDigest(s string) ([sha256.Size]byte, bool) {
	var out [sha256.Size]byte
	raw, err := hex.DecodeString(s)
	if err != nil || len(raw) != len(out) || s != hex.EncodeToString(raw) {
		return out, false
	}
	copy(out[:], raw)
	return out, true
}

// verifyArtifact checks a fetched module before anything compiles it
// (FN-009): its SHA-256 is the one requested, its statement is signed by a
// configured key, and the statement names that SHA-256. It returns
// PLX-7019 for any failure; the message names the failed check, never the
// bytes.
func verifyArtifact(keys map[string]ed25519.PublicKey, want [sha256.Size]byte, resp *fnpb.GetModuleResponse) error {
	if sha256.Sum256(resp.GetModule()) != want {
		return refused("the module's SHA-256 is not the one requested")
	}
	key, ok := keys[resp.GetKeyId()]
	if !ok {
		return refused("the artifact is signed with a key the runner does not trust")
	}
	signature, statement := resp.GetSignature(), resp.GetStatement()
	if len(signature) != ed25519.SignatureSize || len(statement) > maxStatementBytes ||
		!ed25519.Verify(key, statement, signature) {
		return refused("the artifact signature does not verify")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(statement, &fields); err != nil {
		return refused("the artifact statement is not a JSON object")
	}
	var named string
	if err := json.Unmarshal(fields[statementDigestField], &named); err != nil {
		return refused("the artifact statement does not name the module")
	}
	if !bytes.Equal([]byte(named), []byte(hex.EncodeToString(want[:]))) {
		return refused("the artifact statement names another module")
	}
	return nil
}

// refused is the error of a failed verification (FN-009).
func refused(why string) error {
	return plxerr.New(plxerr.FunctionArtifactSignatureInvalid, "%s", why)
}
