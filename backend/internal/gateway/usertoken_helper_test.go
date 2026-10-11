// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package gateway

import (
	"context"

	"github.com/nightCode42/plux3/backend/internal/devtoken"
	"github.com/nightCode42/plux3/backend/internal/signing"
)

// compactForTest signs with an arbitrary typ through the production
// signer.
type compactForTest struct{ jws devtoken.Compact }

func (c *compactForTest) sign(ctx context.Context, signer signing.TokenSigner, typ string, payload []byte) (string, error) {
	return c.jws.Sign(ctx, signer, signing.TokenDevelopment, typ, payload) //nolint:wrapcheck // a test helper
}
