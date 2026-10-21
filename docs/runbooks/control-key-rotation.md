# Control-Document Key Rotation Runbook

Rotate the key that signs the control document of an environment: the kill switches, the mandatory
update, the forced rollback, the feature flags and the running experiments that devices read
(`SYN-060`, `REL-070`, [ADR-0077](../adr/0077-signed-control-document.md)). Use it on a planned
schedule, when the control key is suspected of exposure, or when the root that names it changes.

## Symptom

There is no symptom in normal operation: rotation is planned. These signals mean the rotation is
overdue or going wrong:

- `PluxControlPropagationSlow` or `PluxControlPropagationDegraded` fires after a control change.
- Devices do not receive a control change that the server reports as signed. A control document is
  valid for 24 hours (`control` expiry in the update metadata), so a signing key that stops working
  shows within a day.
- `plux keys` lists a control key that the environment no longer means to use.

## Impact

The control document is the fastest lever Plux has over a fleet. Its signature is what stops a
forged kill switch or a forged flag from reaching devices (`SEC-052`). A leaked control key lets
an attacker switch features off, force a rollback or set a mandatory update for every device of
the environment. A rotation done badly can make every control change fail to verify on devices
that still hold the old root.

## Diagnosis

1. Identify the current control key. The control role is an online role, like `targets`,
   `snapshot` and `timestamp`, and it is named by the environment's root. A root without a control
   key delegates no control document at all, so check that the root names one.
2. Export the environment's public online keys, as in step 1 of the
   [key compromise runbook, section 2](key-compromise.md#2-online-metadata-keys):
   `plux-server metadata keys -config plux-server.yaml -organization <org> -environment <env>`.
   Compare the control key with the key the signing backend holds for the environment.
3. Check the control document the server currently serves for each channel: its signature names
   the key that signed it. A document signed by a key other than the one the root names is a
   finding, not a rotation.
4. If the rotation is for exposure, follow the key compromise runbook first: stop the leak, then
   rotate.

## Remediation

The procedure is the rotation of the [key ceremony](key-ceremony.md), which is the only way a root
changes. The control key rotates like the other online keys:

1. Create the replacement control key in the signing backend, and export its public key with the
   command above.
2. Run the rotation of the key ceremony: a new root version that names the new control key in place
   of the old one. It is signed by two holders of the new keys and two of the previous keys, then
   uploaded with `upload-root`.
3. Make the server sign a fresh control document with the new key. Publish a release, or promote the
   current one; the `worker` signs the new control document with the new key.
4. Confirm that the newest control document of each channel names the new key, using the export of
   step 2 under Diagnosis.
5. Only after devices have had time to fetch the new root (a day is enough for devices that sync
   daily), remove the old control key from the signing backend.
6. Until step 5, a holder of the old key who can also reach a device's network path could serve a
   control document that verifies. Pinned TLS and the anti-rollback rules limit this; shorten the
   interval between steps 2 and 5 when the old key is suspected.

Do not rotate the control key by editing a control document, by changing a channel's signature
by hand or by disabling verification on any device.

## Escalation

- The rotation itself: the key ceremony holders (two of three) and the platform on-call.
- Suspected exposure of the control key: the security contact at once, then the
  [key compromise runbook](key-compromise.md).
