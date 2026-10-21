# SIEM Backlog Runbook

Audit entries are delivered to an organisation's SIEM destination in order, at least once, from a
durable cursor (`SEC-143`, [ADR-0082](../adr/0082-audit-export-reconstruction-and-egress.md)). A backlog
means the destination has not acknowledged the newest entries yet: the SIEM is behind, and no entry
is lost. Use it when a destination shows `PLX-8500`, when the SIEM lacks recent audit entries, or when
the audit export alert and the destination's own status disagree.

## Symptom

- `AuditService.GetExportDestination` shows a last error (`PLX-8500`, `SIEM_DESTINATION_UNREACHABLE`)
  and a growing count of entries not yet delivered.
- The SIEM has a gap at the time the errors began, and then catches up.
- A destination of an enterprise kind (CEF, OTLP logs, signed webhook) delivers nothing and reports
  `PLX-8600`: its licence feature is not in force. Syslog is the core's sink and needs no licence.

## Impact

Nothing is lost: the cursor moves only after the destination has acknowledged a batch, so every entry
is delivered once the SIEM accepts it. The cost is latency for whoever watches the SIEM, and a window in
which detections based on the SIEM are blind. A redelivered batch can repeat entries that the SIEM had
already received; the SIEM must tolerate duplicates, and the records carry the entry hash and the
previous entry's hash so it can verify the chain.

## Diagnosis

1. Read the destination with `AuditService.GetExportDestination`: its cursor, the entries not yet
   delivered and the last error. The last error is one code; the cause is in the SIEM or on the path.
2. Classify the error:
   - **Unreachable or refused connection.** Check the SIEM's address and port, the firewall and the
     network policy between the server and the SIEM, and whether the address has changed.
   - **TLS.** Syslog over TLS needs the SIEM's certificate to verify. An expired or rotated certificate
     on the SIEM side is a likely cause.
   - **Private address refused.** A destination on a private, loopback or metadata address is refused
     (`SEC-104`). A destination in a development installation may be explicitly allowed; that prints a
     warning. Production destinations are public addresses.
   - **Licence.** An enterprise kind without its feature is refused with `PLX-8600`; see the
     [licence expiry runbook](licence-expiry.md).
3. Check the path against the egress list: the audit export destination is the address the customer
   enters (see [egress](../operations/egress.md)).
4. Check the backoff. Failed batches wait an exponential backoff with jitter before the next attempt, so
   the first attempt after a fix may be a little later than the fix. A destination that stays unreachable
   is also reported through the notifier.

## Remediation

- **Fix the path or the SIEM, then wait.** When the SIEM accepts connections, the backlog drains in
  order at the next attempt. Nothing needs re-enqueuing and the cursor needs no reset.
- **Certificate rotation on the SIEM.** Update the trust the destination uses, through
  `AuditService.UpdateExportDestination`, which needs the audit export permission. Credentials are sealed
  and never returned, so supply the new material rather than reading the old one.
- **Do not delete and recreate the destination** to clear an error. A new destination is a new delivery
  with its own cursor, and the gap it leaves in the SIEM is hard to explain to an auditor.
- **Large backlog.** The exporter sends bounded batches, so a backlog of many thousands of entries drains
  over several cycles. Expect the SIEM's ingest rate to set the pace, not Plux.
- **Enterprise kind without a licence.** Restore the licence, then confirm the backlog drains.
- Record the window of the gap and the drain time in the change record, so the SIEM's detections can be
  judged against it.

## Escalation

- The SIEM is not accepting entries: the SIEM owner, with the destination's last error.
- The chain does not verify in the SIEM (`PluxAuditChainBroken`): the security contact at once, since
  this is an integrity question and not a delivery one (see the [alert runbooks](alerts/README.md)).
- Licence faults: the licensing contact.
