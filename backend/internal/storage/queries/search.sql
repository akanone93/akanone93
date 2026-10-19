-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- Full-text search (SRV-061, ADR-0082). The query is one statement over
-- apps, plugins and the documents that carry a name worth finding. Which
-- apps the caller may search is part of the statement: allowed is the list
-- of app identifiers the caller holds search.read on, or NULL when the
-- permission is held for the whole organisation, and every branch is
-- restricted by it before a row is ranked or returned. Row-level security
-- adds the organisation boundary underneath (SRV-022).
--
-- query is a tsquery the service built from the caller's words, every one
-- of them a prefix term; kinds, when not empty, restricts the kinds.
-- Every branch repeats the index expression of its table, which lets the
-- planner use the GIN indexes of migration 0026. The snippet is cut from
-- the matching text with the separators of paths blanked, as the index
-- sees it, and marks each hit with « and ».
-- name: Search :many
SELECT hit.kind::text AS kind, hit.ref, hit.app_id, hit.name::text AS name, hit.path::text AS path,
       hit.rank::float4 AS rank,
       ts_headline('simple', translate(hit.body, '/._', '   '), to_tsquery('simple', sqlc.arg(query)::text),
                   'StartSel=«, StopSel=», MaxFragments=1, MinWords=3, MaxWords=14')::text AS snippet
  FROM (
    SELECT * FROM (
      SELECT 'app'::text AS kind, a.id AS ref, a.id AS app_id, a.name AS name, a.key AS path,
             a.name || ' ' || a.key AS body,
             ts_rank_cd(plux_search_vector(a.name, a.key, ''), to_tsquery('simple', sqlc.arg(query)::text)) AS rank
        FROM apps a
       WHERE a.deleted_at IS NULL
         AND plux_search_vector(a.name, a.key, '') @@ to_tsquery('simple', sqlc.arg(query)::text)
         AND (sqlc.narg(allowed)::uuid[] IS NULL OR a.id = ANY(sqlc.narg(allowed)::uuid[]))
         AND (cardinality(sqlc.arg(kinds)::text[]) = 0 OR 'app' = ANY(sqlc.arg(kinds)::text[]))
      UNION ALL
      SELECT 'plugin'::text, pl.id, pl.app_id, pl.name, 'plugins/' || pl.key,
             pl.name || ' ' || pl.key,
             ts_rank_cd(plux_search_vector(pl.name, pl.key, ''), to_tsquery('simple', sqlc.arg(query)::text))
        FROM plugins pl
        JOIN apps pa ON pa.id = pl.app_id AND pa.deleted_at IS NULL
       WHERE pl.deleted_at IS NULL
         AND plux_search_vector(pl.name, pl.key, '') @@ to_tsquery('simple', sqlc.arg(query)::text)
         AND (sqlc.narg(allowed)::uuid[] IS NULL OR pl.app_id = ANY(sqlc.narg(allowed)::uuid[]))
         AND (cardinality(sqlc.arg(kinds)::text[]) = 0 OR 'plugin' = ANY(sqlc.arg(kinds)::text[]))
      UNION ALL
      SELECT CASE d.kind WHEN 'translationKeys' THEN 'translation_key' ELSE d.kind END,
             d.id, dr.app_id,
             CASE WHEN d.entity_key <> '' THEN d.entity_key ELSE d.path END,
             d.path,
             CASE WHEN d.search_text <> '' THEN d.search_text ELSE d.path END,
             ts_rank_cd(plux_search_vector(d.path, d.entity_key, d.search_text), to_tsquery('simple', sqlc.arg(query)::text))
        FROM documents d
        JOIN drafts dr ON dr.id = d.draft_id
        JOIN apps da ON da.id = dr.app_id AND da.deleted_at IS NULL
        LEFT JOIN plugins dp ON dp.id = dr.plugin_id
       WHERE d.deleted_at IS NULL
         AND d.kind IN ('page', 'component', 'template', 'translationKeys', 'function')
         AND (dr.plugin_id IS NULL OR dp.deleted_at IS NULL)
         AND plux_search_vector(d.path, d.entity_key, d.search_text) @@ to_tsquery('simple', sqlc.arg(query)::text)
         AND (sqlc.narg(allowed)::uuid[] IS NULL OR dr.app_id = ANY(sqlc.narg(allowed)::uuid[]))
         AND (cardinality(sqlc.arg(kinds)::text[]) = 0
              OR (CASE d.kind WHEN 'translationKeys' THEN 'translation_key' ELSE d.kind END) = ANY(sqlc.arg(kinds)::text[]))
    ) found
    ORDER BY found.rank DESC, found.kind, found.ref
    LIMIT sqlc.arg(page_size) OFFSET sqlc.arg(page_offset)
  ) hit
 ORDER BY hit.rank DESC, hit.kind, hit.ref;
