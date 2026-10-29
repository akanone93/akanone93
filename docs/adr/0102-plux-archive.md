<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# 0102. The `.plux` archive

- **Status:** Accepted (maintainer, plan P11 §4.3 #24; S8.1)
- **Date:** 2026-10-28
- **Requirements:** `STU-023`, `STU-024`, `STU-015`, `SCH-006`, `CMP-002`, `SEC-052`, `LIM-001`

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
