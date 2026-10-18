# Translation Import Runbook

An imported translation file is refused, or its entries are stored at a lower status than the
file asked for (`I18N-008`, `I18N-009`, ADR-0068).

## Rules the importer applies

- Formats: XLIFF 2.0, ARB, JSON and CSV (RFC 4180). The importers are bounded by
  `translation.importSize` (default 16777216 bytes), refuse DTDs and entities, never resolve
  external references, and are fuzzed.
- A caller with only `translations.edit` saves only `translations` documents, only their messages
  and states (`PLX-8093`). Setting or keeping a changed message at `approved` (XLIFF `final`)
  needs `translations.approve` (`PLX-8092`).

## Codes

| Code | Meaning | Action |
|---|---|---|
| `PLX-1620` | The file is not valid for its format; nothing from it was applied | Export again from the translation tool; check the structure |
| `PLX-1621` | A key the plugin does not declare | Remove the key from the file or declare it |
| `PLX-1622` | The source text changed since export; that key is reset to `translated` (warning) | Review against the new source and approve again |
| `PLX-1623` | The file asked for `approved` or `final` from a caller without `translations.approve`; those entries were stored as `translated` (warning) | Have an approver approve in Studio, or import as an approver |

## Diagnose

1. **Refused for size.** The file exceeds `translation.importSize`. Split it by locale or plugin.
   Raise the limit only through a reviewed configuration, within the registry's maximum.
2. **Refused as invalid.** A file with a DTD or entity declaration is refused on purpose,
   including from tools that add one; export a plain XLIFF 2.0 file. Check the encoding, the CSV
   columns (key ID, key, source, target, state, description, maximum length) and that the XLIFF
   `<unit id>` is the key ID.
3. **Entries not approved after import (`PLX-1623`).** This is the designed outcome for a
   translator: the whole file is not refused, the entries are stored as `translated`, and one
   warning names them.
4. **Many `PLX-1622`.** The source changed after export. Export again and send the translator the
   new file.

## Fix

Correct the file and import again; a refused import applied nothing, so it is safe to repeat.
`plux l10n status` shows the states per locale.
