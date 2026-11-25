<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# 0102. The `.plux` archive

- **Status:** Accepted (maintainer, plan P11 §4.3 #24; S8.1)
- **Date:** 2026-10-28
- **Requirements:** `STU-023`, `STU-024`, `STU-015`, `STU-034`, `SCH-006`, `CMP-002`, `SEC-052`, `LIM-001`

## Context and problem

Studio exports an app, imports one, duplicates one and ships sample apps (F17). The plan found the
archive defined nowhere: not in the specification, not in an ADR, not in code. The Git layout of
`SCH-006` already exists and `document.Service.Export` and `Import` already read and write it, with
schema validation of every document.

## Decision drivers

- One layout, not two: an archive must be what Git export already writes.
- Byte-identical output for the same input (`CMP-002` style), so a sample can be checked in and a
  test can prove it is current.
- An archive is untrusted input: sizes, entry counts, paths and every document are checked before
  anything is written (`SEC-052`).
- Limits come from the registry (`LIM-001`).

## Decision

A `.plux` file is a ZIP archive with exactly this content, nothing else:

1. `plux-archive.json`: `{"format":"plux-archive","version":1,"app":{"key","name","description","tags"},"files":[{"path","size","sha256"}]}`, files sorted by path.
2. `project/<path>`: one entry per file of the `SCH-006` layout (documents, then `assets/`), exactly as `document.Service.Export` returns them.

**Deterministic:** entries are sorted by name, stored without compression (method `Store`), with the
ZIP epoch (1980-01-01) as time, mode `0644`, no extra fields and no comment. No identifier, user,
organisation or clock value is written. An import creates new identifiers.

**Untrusted on import:** the archive is refused when it exceeds `archive.bytes`, holds more than
`archive.entries` entries, expands beyond `archive.expandedBytes` (counted on the bytes actually read,
never on the declared size), has a name that is absolute, contains `..`, a backslash, a NUL byte or an
empty element, is not the cleaned form of itself, appears twice, or is not a regular file; when the
manifest is missing, has another `format` or a `version` above 1, or disagrees with an entry in
presence, size or SHA-256; and when any document fails schema validation (`document.Service.Import`
validates every file before writing any). Reasons use the registered codes `INVALID_PROJECT_LAYOUT`,
`LIMIT_EXCEEDED` and `INVALID_FORMAT`.

**Duplicate** is export followed by import in one call; **sample apps** are `.plux` files embedded in
the server image and listed by an RPC; importing one is an ordinary import (F17). Mutations are
idempotent through the `Idempotency-Key` header, and import, export, duplicate and archive are audited.

## Consequences

- **Positive:** one format for Git export, archives, samples and duplication; a sample cannot go stale
  unnoticed (a test rebuilds it from its source project and compares bytes).
- **Negative:** uncompressed archives are larger on disk; transport compression applies on the wire.
- **Follow-up:** a later version may add fields to the manifest; readers ignore unknown fields only
  when the version is unchanged, and refuse a higher version.

## Revision 2026-11-25: plugin archives

The maintainer decided on 2026-11-24 (build log row 264) that one plugin travels between apps as a
`.plux` archive, through additive RPCs (`AppProjectService.ExportPlugin`, `ImportPlugin`), so a plugin
built in one app can be reused in another without copying documents by hand (`STU-034`).

- **Content.** A plugin archive holds the plugin's documents (`project/plugins/<key>/…`) and the parts
  of the app-level documents they name, cut down to them: the translation keys whose identifiers the
  plugin's documents hold, those keys' messages and review states in every locale
  (`translations/keys.json`, `translations/<locale>.json`), and the asset index entries of the assets
  the plugin's documents or those keys hold, with the files and per-locale files they name
  (`assets/index.json`, `assets/…`). Nothing else of the app travels: components, functions, types,
  data and the theme are the target app's to have, and a reference to one it lacks is counted as the
  imported plugin's validation errors.
- **Manifest.** The manifest gains `"plugin": {"key", "name"}`, and `app` names the app the plugin was
  exported from. A plugin archive is written as **version 2**, so that a server reading version 1
  refuses it rather than importing it as an app; an app archive is still written as version 1. A
  version-1 manifest with a `plugin` is refused. An app import refuses a plugin archive, and a plugin
  import refuses an app archive, each with `INVALID_PROJECT_LAYOUT`.
- **Untrusted on import,** as an app archive: every check above, then every document is checked
  against its schema, a file outside the plugin and the three app-level documents, an asset file the
  index does not list and an index entry without its file are refused, and the plugin key is checked
  free and within `app.plugins` — all before anything is written.
- **Merge.** The plugin's documents get new identifiers and route names that clash with none of the
  app's, as a duplicate's do (`STU-034`). A translation key the app has (same identifier) is used as it
  is; an added key whose name the app uses is renamed under the plugin's key (`<plugin>.<name>`), and
  its messages and review states are added to the locales the app has. An asset whose content the app
  has (same SHA-256) is used instead of a copy; any other is uploaded through the ordinary asset path
  (type check, stripping, scanner) under a name the app does not use, with its description and
  per-locale files. The import takes the locks it needs for the caller's session — the app's, each
  changed locale's and the new plugin's — and gives back only those it took.
- Export needs `plugin.read`, import `app.manage` on the app; both are audited (`plugin.exported`,
  `plugin.imported`), and the import is idempotent through the `Idempotency-Key` header.
