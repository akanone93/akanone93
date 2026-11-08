-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- A token may come from `plux-server seed`: a short-lived token of the
-- seeded user that the publishes of `make dev` use on a development
-- installation, where no security key is registered yet (DEP-020, SEC-103).
-- Expand-only: the check admits one more value, so rows and code of the
-- previous version stay valid.
ALTER TABLE access_tokens DROP CONSTRAINT access_tokens_source;
ALTER TABLE access_tokens ADD CONSTRAINT access_tokens_source CHECK (source IN ('pat', 'cli', 'ci', 'seed'));
