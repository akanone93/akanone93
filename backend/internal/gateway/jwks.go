// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package gateway

import (
	"encoding/json"
	"fmt"

	"github.com/go-jose/go-jose/v4"

	"github.com/nightCode42/plux3/backend/internal/signing"
)

// JWKS renders the public keys that verify assertions for environments of
// one class, as a JSON Web Key Set (RFC 7517). An upstream trusts the set
// of its environment's class only: a development key never vouches for a
// production environment (SEC-056). The keys also verify the installation's
// access tokens, so an upstream must require the assertion's typ,
// AssertionType.
func JWKS(keys []signing.TokenKey, class signing.TokenClass) ([]byte, error) {
	set := jose.JSONWebKeySet{Keys: []jose.JSONWebKey{}}
	for _, k := range keys {
		if k.Class != class || k.Public == nil {
			continue
		}
		set.Keys = append(set.Keys, jose.JSONWebKey{Key: k.Public, KeyID: k.ID, Algorithm: string(jose.ES256), Use: "sig"})
	}
	out, err := json.Marshal(set)
	if err != nil {
		return nil, fmt.Errorf("gateway: encode the key set: %w", err)
	}
	return out, nil
}
