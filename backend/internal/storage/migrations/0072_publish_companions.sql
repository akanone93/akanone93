-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- Build log row 309. A release request builds its parts (the app bundle and
-- each plugin) as separate publishes. Each compiled the other parts at their
-- newest versions, so a change spanning the app and a plugin (a type renamed
-- in both) failed in every part. A publish now names its companions, the
-- other parts of the same request, and compiles them at the snapshots frozen
-- with its own draft: a list of {"plugin_id", "snapshot_id"}, an empty
-- plugin ID naming the app-level draft. Expand-only.
ALTER TABLE publish_jobs ADD COLUMN companions jsonb NOT NULL DEFAULT '[]';
