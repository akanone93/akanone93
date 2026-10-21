# Data Subject Request Runbook

Answer a request from a data subject to see or to erase what an organisation holds about them
(`SEC-162`, spec section 15). The subject is a device, or a pseudonymous user reference, and the
request is served by the data subject service with the `dsr.manage` permission. Use this runbook
for the export and the erasure; the [personal-data map](../operations/personal-data.md) lists every
store that holds personal data and what erasure does to each.

## Symptom

A request arrives from a person, or from a privacy process, for an export or an erasure. There is no
technical symptom. The service answers with one of these codes:

- `PLX-8501` (`SUBJECT_NOT_FOUND`): the subject has no data in this organisation. A subject of another
  organisation gets the same answer, so the response never shows whether a subject exists elsewhere.
- `PLX-1320`: the export would exceed the `dsr.exportBytes` limit. The export is refused, never
  truncated, so an archive is always complete.

## Impact

A request has a legal deadline, and the answer must be complete. Erasure is final for the device
record and its credentials: the device must register again. Audit entries are not erased, because the
audit chain must stay verifiable (`SEC-140`); they carry only pseudonymous identifiers, and the
personal-data map says so.

## Diagnosis

1. Confirm who is asking, with the person or their representative, before any action. This is the
   organisation's step; the request itself carries only the subject identifier.
2. Identify the subject: a device identifier, or the pseudonymous user reference the host app set.
   The subject is identified in tombstones and audit entries by a hash of its organisation, kind and
   identifier, so the identifier itself is needed only for the request.
3. Read the personal-data map with `DataSubjectService.GetPersonalDataMap` to see which stores the
   organisation's data is in, and what each store does on erasure.

## Remediation

**Export.**

1. Call `DataSubjectService.ExportSubject` for the subject. The result is one JSON archive with a
   section per store. Key material, secret hashes and the sealed push token are not in it, by design.
2. If the response is `PLX-8501`, record that the subject has no data here, and answer the requester
   with that outcome.
3. If the response is `PLX-1320`, the archive is larger than the limit. Do not cut it down by hand and
   do not send a partial archive. Escalate: the limit is an installation setting and the request needs a
   decision on how to deliver it.
4. Send the archive to the requester by a channel you control. It is personal data: do not paste it into
   a ticket, a chat or a log.

**Erasure.**

1. Call `DataSubjectService.EraseSubject`. The service removes the subject's rows store by store, in a
   fixed order that ends with the device record, one transaction per store. Each store that lost rows
   leaves a tombstone (the subject's hash, the store, a count and the time), and the request is audited.
2. If the erasure is interrupted, call it again. Repeating an erasure is safe: a finished one changes
   nothing and reports the earlier erasure.
3. Check the answer: the tombstones show what was removed, and the store counts add up to what the export
   showed before erasure.
4. Tell the requester what is kept and why: audit entries with pseudonymous identifiers (the audit chain
   must stay verifiable), and copies in backups until the backup retention passes (see the
   [backup runbook](backup-and-restore.md)).
5. The stores the map marks for deletion on erasure, such as `telemetry_events` and `analytics_events`, lose
   the subject's rows. The map's Erase and Note columns say what happens to each store.

**Records.** Keep a record of the request and its outcome outside the subject's data, with the tombstone
references and without the archive.

## Escalation

- Disagreement on scope, a disputed identity or an archive over the limit: the organisation's data
  protection officer.
- A request that must reach a store the personal-data map does not list: treat it as a defect in the map
  and raise it with the platform team; a test fails when a migration adds a personal-data column to a
  table the map does not list.
