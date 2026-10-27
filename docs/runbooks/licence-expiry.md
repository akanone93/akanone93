# Licence Expiry Runbook

The enterprise licence has expired, is about to, or is no longer valid, so enterprise features
are read-only or off (`GOV-030`, [ADR-0084](../adr/0084-enterprise-edition-and-licence-files.md)).
Use it for the `PluxLicenseExpiringSoon` and `PluxLicenseExpiryImminent` alerts, and for an
installation whose enterprise features stopped working.

## Symptom

- The alerts `PluxLicenseExpiringSoon` and `PluxLicenseExpiryImminent` fire.
- Writes to enterprise features are refused with `PLX-8600` (`ENTERPRISE_FEATURE_NOT_LICENSED`), for
  example custom roles, SCIM provisioning or a new SAML provider.
- The server logs a warning while the licence needs attention.
- `plux admin license` reports a state other than `valid`.

## Impact

The states, from the licence package:

| State | What it means | Effect |
|---|---|---|
| absent | No licence file | Enterprise features off; the community edition runs in full |
| valid | Signed by a trusted key, inside its validity | Everything the licence lists |
| grace | Past expiry, inside the grace period (30 days by default unless the licence sets it) | Features keep working; the process warns |
| expired | Past the grace period | Enterprise features read-only; existing data stays readable and exportable, and nothing is deleted |
| invalid | Unreadable, malformed, unsigned, signed by an unknown key, not yet valid, bound to another installation, or naming a limit the registry does not know | Enterprise features off |

No state refuses device traffic or turns off a security capability (`GOV-032`). An expired SAML
provider keeps its sign-ins working; an absent or invalid one does not. Existing custom roles keep
working. Only new writes to enterprise features are refused.

## Diagnosis

1. Read the installation's state: `plux admin license --json` (needs `organization.manage`). It shows
   the state and the features it enables.
2. Read the file itself, offline, if you have it: `plux license inspect <file>` prints the organisation,
   the expiry, the grace period, the features and the limits. Add `--public-key <base64>` only when you
   need to check against a key other than the ones the build holds.
3. Find where the installation reads the file: `PLUX_LICENSE_FILE`, or the `license` Helm secret. The
   file in use may not be the one you expect.
4. For `invalid`, the file is the problem, not the clock: a licence edited by hand fails its signature.
   Never edit a licence file; replace it.

## Remediation

- **Expiring soon.** Request the renewal from the licensing contact before the expiry. Plan the
  replacement for a time the organisation's admins can watch.
- **Renewed licence.** Inspect the new file offline with `plux license inspect`, check that it names the
  right organisation and installation, then put it in place: update the `license` Helm secret or the file
  `PLUX_LICENSE_FILE` names. An installation administrator can instead upload the file in Studio
  (Installation, Licence), which calls `LicenseService.InstallLicense` (at most 64 KiB, verified offline,
  audited; [ADR-0084](../adr/0084-enterprise-edition-and-licence-files.md)). The licence is verified again every hour, so the new state appears within an
  hour; check it with `plux admin license`.
- **Grace period.** Features still work. Use the time to renew, and tell the owners of the enterprise
  features (approvals, SCIM, SAML, SIEM export) what changes at the end of grace.
- **Expired.** Writes to enterprise features are refused until the renewal arrives. Read-only access and
  exports keep working, so audit and compliance reads are not lost. Do not delete data to recover space.
- **Invalid.** Find the cause from `plux license inspect`: a wrong signing key, a file for another
  installation or an unknown limit. Ask the licensing contact for a corrected file. Installing a file that
  is not signed by a trusted key does not help: it is still invalid.
- **Absent, by mistake.** Restore the file from the secret store; do not create one by hand.

## Escalation

- Renewal, a wrong organisation or installation on the file, or a state the inspection does not explain:
  the licensing contact, with the output of `plux admin license --json`.
- A licence file that may have been tampered with (an invalid signature on a file that was valid before):
  the security contact. Enterprise features are off until a trusted file is in place, and the device
  security is unaffected either way.
