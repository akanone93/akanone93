# 0068. Translation workflow: statuses in the document, roles, formats

- **Status:** Proposed
- **Date:** 2026-10-16
- **Requirements:** `I18N-007`, `I18N-008`, `I18N-009`, `I18N-010`, `GOV-002`, `CLI-003`

## Context and problem

Translations are app-level documents (`translations/keys.json` and one `translations/<locale>.json`
per locale), saved today only with `app.manage` under the app-level lock (`SRV-042`). P8 needs
per-key statuses, a translator role, optional approval before a release uses a translation,
import and export in XLIFF 2.0, ARB, JSON and CSV, extraction of literal text, unused-key
detection and pseudo-locales, without waiting for Studio (P11) or P9's custom roles and approvals.

## Decision drivers

- Statuses travel with a project folder (`plux pull`/`plux publish`, teams' own git) and with
  import/export.
- Least privilege: a translator edits messages and nothing else.
- Untrusted files are parsed safely.

## Considered options

1. Statuses in a server table only.
2. **Statuses in the translations document, enforced by the server.**

## Decision

Chosen option: **2** (P8 plan D6, D7).

- **Statuses:** `states` in each translations document (key ID → `new`, `translated`,
  `reviewed`, `approved`), mapped to XLIFF 2.0 `state` `initial`, `translated`, `reviewed`,
  `final`.
- **Permissions:** `translations.edit` and `translations.approve` (step-up); the built-in
  `translator` role holds `organization.read`, `app.read`, `plugin.read` and `translations.edit`;
  `developer` gains `translations.edit`; `owner` and `admin` hold both. A caller with only
  `translations.edit` saves only `translations` documents and only their messages and states
  (`PLX-8093`); setting or keeping a changed message at `approved` needs `translations.approve`
  (`PLX-8092`); a text change without it resets that key to `translated`.
- **Locks:** translations documents are edited under a lock per locale, separate from the
  app-level lock, so a translator never blocks the theme or `app.json`.
- **Approval-only releases:** with `localisation.requireApprovedTranslations`, unapproved messages
  are left out of the release and fall back (`PLX-1610`, info); the release's localisation report
  lists them. The release is not refused.
- **Formats** (`backend/internal/l10n/xchange`): XLIFF 2.0 (`<unit id>` = key ID, notes for
  description and maximum length), ARB (`@key` metadata, placeholders), JSON (the document
  itself), CSV (RFC 4180: key ID, key, source, target, state, description, maximum length).
  Importers are bounded by `translation.importSize`, refuse DTDs and entities, never resolve
  external references, and are fuzzed. A changed source text since export marks the key
  `translated` (`PLX-1622`).
- **Service and CLI:** `LocalisationService` (export, import, status, set states) and
  `plux l10n pull|push|export|import|status|extract|pseudo`.
- **Compiler:** unused keys (`PLX-1608`), literal text in `translatable` props (`PLX-1609`),
  missing translations (`PLX-1606`), too-long translations (`PLX-1607`); pseudo-locales `en-XA`
  (accented, about 35% longer, bracketed) and `ar-XB` (right-to-left) generated from the default
  locale when `localisation.pseudoLocales` is on, refused in production (`PLX-1614`).
- **Report:** a localisation report per release (coverage per locale, missing, unapproved,
  too long, unused), served like the accessibility report (ADR-0069).

## Consequences

- **Positive:** git-friendly, round-trips through every format, least privilege now; P9's custom
  roles compose the same permissions.
- **Negative:** the server diffs translations documents on save to enforce statuses.
- **Follow-up:** L5; Studio's Localisation area (P11, `STU-111`) on the same service.
