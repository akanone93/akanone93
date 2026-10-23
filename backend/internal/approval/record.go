// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package approval

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/schema/jcs"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// recordVersion is the version of the record format. A change to the fields
// signed below adds a version; the verifier refuses one it does not know.
const recordVersion = 1

// recordDomain separates what the audit key signs for a decision from every
// other message it signs, so a checkpoint's signature is never a decision's.
const recordDomain = "plux-approval-record-v1\n"

// Record is what a decider confirmed: one decision on one artifact, bound
// to its content hash (GOV-023). Its canonical encoding is what the audit key
// signs.
type Record struct {
	Version        int    `json:"version"`
	RequestID      string `json:"requestId"`
	OrganizationID string `json:"organizationId"`
	AppID          string `json:"appId"`
	Resource       string `json:"resource"`
	Action         string `json:"action"`
	Subject        string `json:"subject"`
	ContentHash    string `json:"contentHash"`
	Decision       string `json:"decision"`
	DeciderID      string `json:"deciderId"`
	Comment        string `json:"comment"`
	DecidedAt      string `json:"decidedAt"`
}

// Payload returns the record's canonical JSON (RFC 8785).
func (r Record) Payload() ([]byte, error) {
	r.Version = recordVersion
	tree := map[string]any{
		"version": float64(r.Version), "requestId": r.RequestID, "organizationId": r.OrganizationID,
		"appId": r.AppID, "resource": r.Resource, "action": r.Action, "subject": r.Subject,
		"contentHash": r.ContentHash, "decision": r.Decision, "deciderId": r.DeciderID,
		"comment": r.Comment, "decidedAt": r.DecidedAt,
	}
	out, err := jcs.Marshal(tree)
	if err != nil {
		return nil, fmt.Errorf("approval: encode a decision record: %w", err)
	}
	return out, nil
}

// signedMessage is the bytes the audit key signs for a payload.
func signedMessage(payload []byte) []byte {
	return append([]byte(recordDomain), payload...)
}

// VerifyRecord checks a decision record's signature against the audit keys
// an auditor trusts, without the server's database: the payload and the
// signature are all it needs. A key or algorithm it does not know is
// refused rather than guessed at (SEC-122).
func VerifyRecord(payload, signature []byte, keyID string, keys map[string]ed25519.PublicKey) error {
	key, ok := keys[keyID]
	if !ok {
		return fmt.Errorf("approval: a decision record was signed by the unknown key %q", keyID)
	}
	if !ed25519.Verify(key, signedMessage(payload), signature) {
		return errors.New("approval: a decision record has an invalid signature")
	}
	return nil
}

// formatTime renders a time the way records carry it.
func formatTime(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

// signingBatch bounds how many records one signing run handles.
const signingBatch = 200

// SignDecisions signs the decision records of an organisation that the
// worker has not yet signed and returns how many it signed. Only a service
// built with a signer does anything; the api role has none (L-3, ADR-0006).
func (s *Service) SignDecisions(ctx context.Context, organizationID string) (int, error) {
	if s.o.Signer == nil {
		return 0, nil
	}
	var pending []dbgen.ApprovalDecision
	tenant := storage.Tenant{OrganizationID: organizationID}
	err := s.o.DB.InTx(ctx, tenant, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		pending, err = dbgen.New(tx).ListUnsignedApprovalDecisions(ctx, signingBatch)
		if err != nil {
			return fmt.Errorf("query: %w", err)
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("approval: list the unsigned decisions: %w", err)
	}
	signed := 0
	for _, d := range pending {
		sig, keyID, err := s.o.Signer.Sign(ctx, s.o.SignerKey, signedMessage(d.Payload))
		if err != nil {
			return signed, fmt.Errorf("approval: sign a decision record: %w", err)
		}
		err = s.o.DB.InTx(ctx, tenant, func(ctx context.Context, tx pgx.Tx) error {
			if _, err := dbgen.New(tx).SignApprovalDecision(ctx, dbgen.SignApprovalDecisionParams{ID: d.ID, KeyID: &keyID, Signature: sig}); err != nil {
				return fmt.Errorf("update: %w", err)
			}
			return nil
		})
		if err != nil {
			return signed, fmt.Errorf("approval: store a decision signature: %w", err)
		}
		signed++
	}
	return signed, nil
}

// PublicKeys returns the audit key's public half by key identifier, for
// VerifyRecord; empty without a signer.
func (s *Service) PublicKeys(ctx context.Context) (map[string]ed25519.PublicKey, error) {
	if s.o.Signer == nil {
		return map[string]ed25519.PublicKey{}, nil
	}
	key, id, err := s.o.Signer.PublicKey(ctx, s.o.SignerKey)
	if err != nil {
		return nil, fmt.Errorf("approval: read the audit key: %w", err)
	}
	return map[string]ed25519.PublicKey{id: key}, nil
}
