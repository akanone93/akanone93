<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Administration and Security

This guide covers the screens for devices, approvals, people and access, the audit log, and the
installation. The concepts are in [Governance](../governance.md), [Device trust](../device-trust.md),
[Sessions and step-up](../sessions-and-step-up.md) and [SSO and SCIM](../sso-and-scim.md). Every
page and action needs the matching permission; Studio says so when you lack it.

## Pair a device

1. Open **Devices** and choose **Pair a device**. Studio shows a QR code, the pairing link and a
   **Short code**, with the time left. **New code** replaces an expired one.
2. In the Plux Dev app scan the code, or paste the link or type the code
   ([run it locally](../run-locally.md) §4 step 6).
3. **Send to device** opens a screen with a **Screen route**, **Language**, **Theme** and **Text
   scale**. **Highlight selection on device** and **Capture screenshot** work on the paired device.
4. The device drawer has tabs for **Problems**, **Logs**, **Process flow**, **State**, **Network**,
   **Functions**, **Sync**, **Performance** and **Console**. Filter by level, **Pause**, and **Export
   trace**. The pairing token is single-use: show it only to the device you are pairing.

## Devices and security profile

- **Devices** (under *Security*) lists registered devices with platform, app build, assurance and
  last attestation. Open one for its key storage, key thumbprint and attestation provider, and
  choose **Revoke device**: it loses access at once and the reason is kept in the audit log
  ([Device revocation](../../runbooks/device-revocation.md)).
- **Security profile** shows the profile (*Standard*, *Strict* or *Maximum*) and every setting in
  force. **Change profile** first asks for a security-key check (step-up); the environment's overrides stay
  as they were.
  See the [configuration guide](../configuration.md).
- **Signing keys** and **Certificate pins** are read-only: they change in the offline key ceremony
  ([Key ceremony](../../runbooks/key-ceremony.md)). **Capabilities** lists the network domains and functions the app grants its plugins; **Edit
  grants** changes them, one entry per line, and a release is published only when every plugin
  requests a subset.

## Approvals

**Approvals** has an **Inbox** of requests waiting for a decision, **History**, **Break-glass
reviews** and **Policies**. Open a request for its summary, the policies that apply, its risk, the
changes, the reports, the golden-image differences and the audit records, then **Approve** or
**Reject** with a comment. You need the permission to decide approvals, and a policy can exclude the
author's own approval. A **Policy** names the resources, environments, actions and approvers and
lets you try it against a sample change before saving. **Break-glass reviews** list emergency
actions taken without prior approval, each with its justification and review due date; review them
before they become overdue ([Approval deadlock and break-glass](../../runbooks/approval-deadlock-and-break-glass.md)).

## Administration

**Administration** (in the organisation menu) has:

| Page | Use it to |
|---|---|
| Settings, Members, Teams | Rename the organisation, invite people (the invitation is shown once), group them |
| Roles, Restrictions | Define roles; add restrictions that remove permissions whatever the grants (the last owner always keeps the ability to manage members and policies) |
| Explain access | See why a person holds or lacks a permission, naming the grant or restriction |
| SSO and SCIM | Identity providers and SCIM tokens |
| Tokens, Sessions | API and CI tokens, workload identities, and active sessions you can revoke |
| Audit log | Search by date, actor, action and resource; **Export CSV**; export destinations |
| Compliance, Retention | Signed reports of changes in a period; how long each data class is kept |
| Licence, Limits | The enterprise licence (verified offline) and the organisation's limits and usage |

Installation administrators also have **Installation**: health, organisations, storage use, job
queues, backups, trusted CI issuers and the audit chain.

## Notifications and your profile

The bell shows unread notifications by type; you can mute types. Notifications are kept for a
limited time; the audit log is the permanent record. **Profile** sets your theme (*Match system*,
*Light*, *Dark* or *High contrast*), language and density.
