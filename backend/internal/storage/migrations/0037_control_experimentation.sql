-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- The feature flags, running experiments and global holdout a control
-- document carries (ABT-006, ABT-001, ABT-004, ADR-0077, ADR-0079). The
-- columns are added beside the kill switches and are backward compatible: a
-- row written before has no flags, no experiments and no holdout.
--
-- flags is a JSON array of {"key", "type", "default", "segments": [{"segment",
-- "rule", "value"}]}, experiments a JSON array of allocations; both are
-- sorted by key, and holdout is {"basisPoints", "salt"} or NULL. The shapes
-- are those of package controldoc.
ALTER TABLE control_documents
    ADD COLUMN flags       jsonb NOT NULL DEFAULT '[]',
    ADD COLUMN experiments jsonb NOT NULL DEFAULT '[]',
    ADD COLUMN holdout     jsonb;
