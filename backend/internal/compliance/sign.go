// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package compliance

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/ext/governance"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// signingBatch bounds how many reports one signing run handles.
const signingBatch = 50

// Sign signs the stored statements of an organisation that the worker has not
// yet signed and returns how many it signed. The audit key signs the DSSE
// pre-authentication encoding of the statement (governance.PAE), which binds
// the payload type and so cannot be mistaken for anything else the key
// signs. Only a service built with a signer does anything; the api role has
// none (L-3, ADR-0006).
func (s *Service) Sign(ctx context.Context, organizationID string) (int, error) {
	if s.o.Signer == nil {
		return 0, nil
	}
	var pending []dbgen.ComplianceReport
	tenant := storage.Tenant{OrganizationID: organizationID}
	err := s.o.DB.InTx(ctx, tenant, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		pending, err = dbgen.New(tx).ListUnsignedComplianceReports(ctx, signingBatch)
		return err
	})
	if err != nil {
		return 0, fmt.Errorf("compliance: list the unsigned reports: %w", err)
	}
	signed := 0
	for _, r := range pending {
		sig, keyID, err := s.o.Signer.Sign(ctx, s.o.SignerKey, governance.PAE(governance.PayloadType, r.Statement))
		if err != nil {
			return signed, fmt.Errorf("compliance: sign a report: %w", err)
		}
		err = s.o.DB.InTx(ctx, tenant, func(ctx context.Context, tx pgx.Tx) error {
			_, err := dbgen.New(tx).SignComplianceReport(ctx, dbgen.SignComplianceReportParams{
				ID: r.ID, KeyID: &keyID, Signature: sig, SignedAt: storage.Timestamp(s.now()),
			})
			return err
		})
		if err != nil {
			return signed, fmt.Errorf("compliance: store a report signature: %w", err)
		}
		signed++
	}
	return signed, nil
}
