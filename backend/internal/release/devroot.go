// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package release

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/jcs"
	"github.com/nightCode42/plux3/backend/internal/signing"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
	"github.com/nightCode42/plux3/backend/internal/updatemeta"
)

// DevKeyCertificateType names the statement an organisation's development
// root signs for an environment's development key (ADR-0109).
const DevKeyCertificateType = "plux.dev-key-certificate"

// devRootInfix sits between the signing key prefix and the organisation's ID
// in the reference of its development root: apart from every environment's
// key, which is the prefix, a dash and the environment's ID.
const devRootInfix = "-devroot-"

// statementDepth bounds the nesting of a certificate's statement, a flat
// object.
const statementDepth = 2

// DevKeyCertificate is an environment's development signing key, certified
// by its organisation's development root: the canonical statement and the
// root's signature of it.
type DevKeyCertificate struct {
	Statement []byte
	Signature []byte
}

// devKeyStatement is what the root signs, marshalled canonically (RFC 8785).
type devKeyStatement struct {
	Algorithm       string `json:"algorithm"`
	AppID           string `json:"appId"`
	Environment     string `json:"environment"`
	EnvironmentID   string `json:"environmentId"`
	EnvironmentType string `json:"environmentType"`
	KeyID           string `json:"keyId"`
	OrganizationID  string `json:"organizationId"`
	PublicKey       string `json:"publicKey"`
	Role            string `json:"role"`
	Type            string `json:"type"`
	Version         int    `json:"version"`
}

// devRootRef is the signing key reference of an organisation's development
// root.
func (s *Service) devRootRef(org string) (string, error) {
	if s.o.DevelopmentRootPrefix == "" {
		return "", errors.New("release: no development root prefix is configured")
	}
	if _, err := storage.UUID(org); err != nil {
		return "", plxerr.New(plxerr.InvalidFormat, "the organisation ID is not valid")
	}
	return s.o.DevelopmentRootPrefix + devRootInfix + org, nil
}

// DevelopmentRoot returns the public key of an organisation's development
// root (ADR-0109), the anchor a signed-in Dev app pins when its phone is
// connected: it certifies the development key of every non-production
// environment of the organisation's apps, and nothing else. The key is
// created by the signing backend on first use; only signing/ holds its
// private half (L-3).
func (s *Service) DevelopmentRoot(ctx context.Context, org string) (ed25519.PublicKey, error) {
	if s.o.Signer == nil {
		return nil, plxerr.New(plxerr.PreconditionFailed, "this server role holds no signer, so it cannot read the development root")
	}
	ref, err := s.devRootRef(org)
	if err != nil {
		return nil, err
	}
	pub, _, err := s.o.Signer.PublicKey(ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("release: the development root: %w", err)
	}
	return pub, nil
}

// CertifyDevelopmentKey signs, with the organisation's development root, a
// statement that the environment's signing key is the development key of
// that app and environment (ADR-0109). A production environment is refused:
// the development root never vouches for a production key (SEC-056), and dev
// sessions exist only for non-production environments (DEV-003).
func (s *Service) CertifyDevelopmentKey(ctx context.Context, env dbgen.Environment) (DevKeyCertificate, error) {
	if env.Production {
		return DevKeyCertificate{}, plxerr.New(plxerr.PermissionDenied, "a production environment has no development key (SEC-056)")
	}
	if s.o.Signer == nil {
		return DevKeyCertificate{}, plxerr.New(plxerr.PreconditionFailed, "this server role holds no signer, so it cannot certify a development key")
	}
	org := storage.ID(env.OrganizationID)
	ref, err := s.devRootRef(org)
	if err != nil {
		return DevKeyCertificate{}, err
	}
	pub, keyID, err := s.o.Signer.PublicKey(ctx, env.SigningKeyRef)
	if err != nil {
		return DevKeyCertificate{}, fmt.Errorf("release: the environment's key: %w", err)
	}
	raw, err := json.Marshal(devKeyStatement{
		Algorithm: signing.Algorithm, AppID: storage.ID(env.AppID), Environment: env.Key,
		EnvironmentID: storage.ID(env.ID), EnvironmentType: updatemeta.EnvDevelopment, KeyID: keyID,
		OrganizationID: org, PublicKey: base64.StdEncoding.EncodeToString(pub), Role: updatemeta.RoleTargets,
		Type: DevKeyCertificateType, Version: 1,
	})
	if err != nil {
		return DevKeyCertificate{}, fmt.Errorf("release: the certificate: %w", err)
	}
	statement, err := jcs.Canonicalize(raw, statementDepth)
	if err != nil {
		return DevKeyCertificate{}, fmt.Errorf("release: the certificate: %w", err)
	}
	sig, _, err := s.o.Signer.Sign(ctx, ref, statement)
	if err != nil {
		return DevKeyCertificate{}, fmt.Errorf("release: certify the development key: %w", err)
	}
	return DevKeyCertificate{Statement: statement, Signature: sig}, nil
}
