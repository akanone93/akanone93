// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/config"
	"github.com/nightCode42/plux3/backend/internal/httpx"
)

// Verifies: SEC-105.
// The client for identity providers refuses a provider on a private address
// with a private certificate, and reaches it once auth.federation names the
// range and the authority.
func TestFederationHTTPAllowlist_SEC_105(t *testing.T) {
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(provider.Close)
	get := func(cfg *config.Config) error {
		client, err := federationHTTP(cfg)
		if err != nil {
			t.Fatal(err)
		}
		res, err := client.Get(provider.URL) //nolint:noctx // a test of the client
		if err == nil {
			_ = res.Body.Close()
		}
		return err
	}
	var cfg config.Config
	if err := get(&cfg); !errors.Is(err, httpx.ErrBlocked) {
		t.Errorf("default = %v, want the address refused", err)
	}
	cfg.Auth.Federation.AllowPrivateNetworks = []string{"127.0.0.0/8"}
	if err := get(&cfg); err == nil {
		t.Error("a private authority was trusted without being named")
	}
	cfg.Auth.Federation.TrustedCAs = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: provider.Certificate().Raw}))
	if err := get(&cfg); err != nil {
		t.Errorf("with the range and the authority named = %v", err)
	}
}
