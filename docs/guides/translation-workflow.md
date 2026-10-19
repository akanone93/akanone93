<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Translation Workflow Guide

How translations get written, reviewed and released: the translator role, review states,
approval-only releases, the `plux l10n` commands, exchange formats and pseudo-locales
(`I18N-007` to `I18N-010`). The design is
[ADR-0068](../adr/0068-translation-workflow.md). The documents and messages are in the
[localisation guide](localisation.md).

## 1. Roles and permissions

Two permissions control translations:

- `translations.edit` saves translations documents, and only their messages and states.
- `translations.approve` (step-up) sets or keeps a changed message at `approved`.

The built-in `translator` role holds `organization.read`, `app.read`, `plugin.read` and
`translations.edit`: a translator cannot touch a page, the theme or `app.json`. `developer`
also holds `translations.edit`; `owner` and `admin` hold both. A caller with only
`translations.edit` who saves anything but a translations document is refused
(`PLX-8093`), and one who sets `approved` is refused (`PLX-8092`). Translations documents
are locked per locale, so a translator never blocks a designer working on the theme.

## 2. States

Each message has a state in its document's `states` map:

| State | XLIFF 2.0 `state` | Meaning |
|---|---|---|
| `new` | `initial` | Not translated. |
| `translated` | `translated` | Written. |
| `reviewed` | `reviewed` | Checked by a second person. |
| `approved` | `final` | Cleared for release. |

Changing a message's text without `translations.approve` resets that key to `translated`.
The states travel with the project folder (`plux pull`, `plux publish`) and with exports.

## 3. Release only what is approved

```json
"localisation": {"requireApprovedTranslations": true}
```

With this setting, messages not at `approved` are left out of the release and fall back
along the locale chain. The release is not refused; the compiler reports `PLX-1610`
(info) per message, and the localisation report lists them.

## 4. The CLI

All commands take `-C dir` for the project and `--app id`; `plux l10n -h` lists them.

```text
plux l10n pull                                   # server's translations/<locale>.json into the project
plux l10n push [--dry-run]                       # upload the project's translations
plux l10n export --locale de --format xliff [--out de.xlf]
plux l10n import --locale de [--format xliff] [--dry-run] de.xlf
plux l10n status                                 # keys per locale by review state
plux l10n extract [--dir dir] [--dry-run]        # literal text -> new keys
plux l10n pseudo [--dir dir]                     # write en-XA and ar-XB
```

`export` writes a locale for a translation tool, with the default locale as the source.
`import` applies a file to a locale; the server checks it and applies the review rules, and
the format is taken from the file name unless `--format` is given. `extract` replaces the
literal text of translatable props in pages and components with references to new keys and
adds the texts to the default locale. `pull`, `push`, `export`, `import` and `status` need a
server; `extract` and `pseudo` work on the folder alone.

## 5. Exchange formats

| Format | `--format` | Shape |
|---|---|---|
| XLIFF 2.0 | `xliff` | `<unit id>` is the key's identifier; notes carry the description and maximum length. |
| ARB | `arb` | `@key` metadata and placeholders. |
| JSON | `json` | The translations document itself. |
| CSV | `csv` | RFC 4180: key ID, key, source, target, state, description, maximum length. |

Files are untrusted: imports are bounded by `translation.importSize` (see
[limits](../reference/limits.md)), refuse DTDs and entities, and never resolve external
references. A malformed file is `PLX-1620` and an unknown key `PLX-1621`. If a key's source
text changed since the export, that key is imported as `translated` with a warning
(`PLX-1622`).

**The import downgrade.** A file that asks for `approved` (XLIFF `final`) from a caller
without `translations.approve` is not refused. Those entries are imported as `translated`,
and one warning, `PLX-1623`, names them. An approver reviews and approves them afterwards.

## 6. Compiler checks

| Code | Severity | Meaning |
|---|---|---|
| `PLX-1606` | warning | A supported locale has no translation for a key. |
| `PLX-1607` | error | A translation is longer than the key's `maxLength`. |
| `PLX-1608` | warning | A key no page, component or plugin uses. |
| `PLX-1609` | warning | Literal text in a `translatable` prop. |
| `PLX-1610` | info | A message left out because it is not approved. |

`PLX-1609` is reported only when the app supports two or more locales. A single-locale app
has nothing to translate, and a warning stops a publish until it is acknowledged. Use
`plux l10n extract` to fix it.

## 7. Pseudo-locales

`plux l10n pseudo` writes two test locales from the default locale's messages: `en-XA`
(accented, about 35% longer, bracketed) shows truncation and hard-coded text, and `ar-XB`
(right to left) shows mirroring. Turn them on with `localisation.pseudoLocales` so the
golden matrix and the Dev app include them ([golden tests](golden-tests.md)).

They are for development only. Compiling for production with them is refused
(`PLX-1614`), and a release that carries them is never published or promoted into a
production environment (`PLX-3064`).

## 8. The localisation report

Each release has a localisation report: coverage per locale, and the missing, unapproved,
too-long and unused messages. Fetch it like the accessibility report:

```text
plux release report --kind localisation --format html --out l10n.html <sequence>
```

See `plux release report -h` for the channel and environment flags.
