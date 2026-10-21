# Air-Gapped Upgrade Runbook

Upgrade an installation that has no route to the internet, from a signed installation bundle carried in
by hand (`DEP-040`, [ADR-0083](../adr/0083-kubernetes-ha-backups-and-air-gap.md)). The steps are in the
[air-gapped guide](../operations/air-gapped.md); this runbook is the checklist for the upgrade itself and
for the cases where the bundle is refused.

## Symptom

- The bundle does not verify: the verifier names a file that is missing, extra or altered, or refuses an
  entry that is not a regular file.
- The image push or the Helm upgrade cannot find an image the bundle should carry.
- The upgrade's pre-upgrade hook or helm test fails inside the installation.

## Impact

The installation keeps running the version it has. A bundle that does not verify must not be installed:
its contents are not trusted, whatever they claim. A failed upgrade is rolled back as in the
[upgrade rollback runbook](upgrade-rollback.md).

## Diagnosis

1. Verify the bundle before anything else, on the machine where it arrived, with the verifier of the
   bundle tool. The bundle's manifest is signed with Ed25519; the verifier checks the signature before it
   reads the manifest, then every file's size and SHA-256 against it. A refusal at the signature means the
   file was altered or signed by another key; a refusal at a file means that file was altered or is not
   in the manifest.
2. Check that the signing key is the one you were given for this installation, through the channel the
   vendor documents. Do not accept a bundle whose key you cannot trace.
3. Check that the bundle is newer than the installed version, and that its release notes name no step
   beyond the standard upgrade.

## Remediation

1. **Bundle refused.** Do not install it, and do not edit its contents to make it pass. Request a fresh
   bundle from the vendor, and verify that one. A bundle built on a machine that changed its contents in
   transit must be copied again, not repaired.
2. **Bundle verified.** Follow the guide's upgrade section ([air-gapped guide, section 5](../operations/air-gapped.md#5-upgrade)):
   push the images into the local registry from the verified bundle, point the chart at it, then run the
   Helm upgrade with the new chart. The chart's pre-upgrade hook migrates the database.
3. **Keep the previous bundle** until the upgrade has passed `helm test plux --namespace plux`. A rollback
   is `helm rollback`, as in the [upgrade rollback runbook](upgrade-rollback.md).
4. **Keep egress denied.** The upgrade needs no route to the internet. Do not open the network to get
   past a step; escalate it instead.
5. Record the bundle's identifier, its verification result and the upgrade's revision in the change record.

## Escalation

- A bundle that does not verify, or whose signing key cannot be traced: the security contact, and the
  vendor through its own channel.
- A failed upgrade or helm test: the platform on-call, with the [upgrade rollback runbook](upgrade-rollback.md).
- Images missing from a verified bundle: the vendor, with the bundle's identifier.
- Any step that appears to need a route to the internet: stop and escalate it as a finding.
