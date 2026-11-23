// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package release

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/signing"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

const (
	testOrg   = "01a20000-0000-7000-8000-000000000001"
	otherOrg  = "01a20000-0000-7000-8000-000000000002"
	testApp   = "01a20000-0000-7000-8000-0000000000a1"
	testEnvID = "01a20000-0000-7000-8000-0000000000e1"
)

func devRootService(t *testing.T) (*Service, *signing.File) {
	t.Helper()
	signer, err := signing.NewFile(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return &Service{o: Options{Signer: signer, DevelopmentRootPrefix: "targets"}}, signer
}

func devEnv(production bool) dbgen.Environment {
	return dbgen.Environment{
		ID: storage.MustUUID(testEnvID), OrganizationID: storage.MustUUID(testOrg), AppID: storage.MustUUID(testApp),
		Key: "development", Production: production, SigningKeyRef: "targets-" + testEnvID,
	}
}

// TestDevelopmentKeyCertificate_DEV_003 checks that an organisation's
// development root certifies an environment's development key in a statement
// the phone can check and read (ADR-0109): the root's signature verifies, the
// statement names the app, the environment and the very key the environment
// signs dev bundles with, and it is typed development.
// Verifies: DEV-003, SEC-056.
func TestDevelopmentKeyCertificate_DEV_003(t *testing.T) {
	s, signer := devRootService(t)
	ctx := t.Context()
	root, err := s.DevelopmentRoot(ctx, testOrg)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := s.CertifyDevelopmentKey(ctx, devEnv(false))
	if err != nil {
		t.Fatal(err)
	}
	if !ed25519.Verify(root, cert.Statement, cert.Signature) {
		t.Fatal("the root's signature of the statement does not verify")
	}
	var st devKeyStatement
	if err := json.Unmarshal(cert.Statement, &st); err != nil {
		t.Fatal(err)
	}
	envKey, keyID, err := signer.PublicKey(ctx, "targets-"+testEnvID)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := base64.StdEncoding.DecodeString(st.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(pub, envKey) || st.KeyID != keyID || st.AppID != testApp || st.EnvironmentID != testEnvID ||
		st.Environment != "development" || st.OrganizationID != testOrg || st.EnvironmentType != "development" ||
		st.Role != "targets" || st.Type != DevKeyCertificateType || st.Version != 1 {
		t.Fatalf("the statement does not name the environment's key: %+v", st)
	}
	// The root is the organisation's alone, and never the environment's own key.
	other, err := s.DevelopmentRoot(ctx, otherOrg)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(other, root) || bytes.Equal(root, envKey) {
		t.Fatal("the development root is shared with another organisation or with an environment")
	}
}

// TestDevelopmentKeyCertificateRefusals_SEC_056 checks that the root never
// vouches for a production key, and that a role without a signer cannot use
// it.
// Verifies: SEC-056.
func TestDevelopmentKeyCertificateRefusals_SEC_056(t *testing.T) {
	s, _ := devRootService(t)
	_, err := s.CertifyDevelopmentKey(t.Context(), devEnv(true))
	var pe *plxerr.Error
	if !errors.As(err, &pe) || pe.Code != plxerr.PermissionDenied {
		t.Fatalf("a production environment was certified: %v", err)
	}
	unsigned := &Service{o: Options{DevelopmentRootPrefix: "targets"}}
	if _, err := unsigned.DevelopmentRoot(t.Context(), testOrg); err == nil {
		t.Fatal("a role without a signer read the development root")
	}
	if _, err := unsigned.CertifyDevelopmentKey(t.Context(), devEnv(false)); err == nil {
		t.Fatal("a role without a signer certified a key")
	}
	if _, err := s.DevelopmentRoot(t.Context(), "not-an-id"); err == nil {
		t.Fatal("a root was made for an invalid organisation")
	}
}
