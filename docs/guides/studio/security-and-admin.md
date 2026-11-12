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

1. **Connect your phone once:** under **Profile › Phones** choose **Connect a phone**. Studio asks
   for your security key, then shows a QR code that works once, for a minute. Scan it with the
   Plux Dev app: Studio then shows a two-digit number and the phone offers three; pick Studio's.
   The phone stays signed in, bound to a key in its keystore, until you sign it out there or it
   goes unused for 30 days ([ADR-0104](../../adr/0104-dev-app-phone-sign-in.md)).
2. **Open the app:** with the app open in Studio — its app canvas or any of its plugins — tap
   the app in the Dev app. The first time in your Studio session, a card shows a number and the
   phone asks you to pick it: on the app canvas above the connection pill at the bottom, in a
   plugin above the console; **Deny** refuses the phone. After a wrong pick the phone stays out
   until you choose **Let it try again**. The app opens at its entry screen, and the pill turns
   **Live**. The connection belongs to the app, not to a plugin: moving between the app canvas
   and its plugins keeps it, and reloading Studio keeps the same session, so the phone stays in
   it. On the phone, the pill at the top of the screen says the same and opens the dev sheet:
   **Reconnect now**, **Show diagnostics** (the debug panel: sync, release, problems and the
   session) and **Leave**.
3. Any other device — a host app in a debug build, or a phone without the Dev app's sign-in —
   pairs with **Pair a device** in a plugin's top bar: a QR code, the pairing link and a **Short
   code**, with the time left (**New code** replaces an expired one). Scan the code, or paste the
   link or type the code ([run it locally](../run-locally.md) §4 step 6).
4. **Send to device** opens a screen with a **Screen route**, **Language**, **Theme** and **Text
   scale**. **Highlight selection on device** and **Capture screenshot** work on the paired device.
5. The **Developer console** under the plugin canvas has its tabs in a rail down its side:
   **Problems**, **Logs**, **Process flow**, **State**, **Network**, **Functions**, **Sync**,
   **Performance** and **Console**, then **Device** (open a screen with a language, theme and text
   scale) and, once one is captured, **Screenshot**. Each tab scrolls on its own. Above a device
   tab's rows, filter by device and level, search, **Pause**, and **Export trace**. When a
   phone joins, it first sends the problems its runtime already reported, so the console shows
   what the Dev app's debug panel shows. The pairing token is single-use: show it only to the device you are pairing.
   The console's state, a pill beside its name, says where it is: **Connecting…** and **Reconnecting…** in amber
   (after the server restarted, for example; it carries on by itself), **No device connected** in grey, or
   **Live** in green with the number of connected devices. A network that holds a streamed response back
   until it ends (some proxies and security scanners do) shows **Reconnecting…** for a few seconds; Studio
   then reads the events in short polls instead, a second or two behind.
   The device list marks each device *connected* or *offline*, and the **Logs** tab shows a line when a device
   connects or drops, with the reason. A device that pairs, or whose app restarted, is sent the current draft at
   once; a device that dropped resumes where it stopped and receives the edits it missed.

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

When the server refuses a page, the page says why and how to get past it, with the error code for
support:

- **Confirm it's you**: the page shows sensitive settings (SSO and SCIM, Sessions); choose
  **Confirm with passkey**, and the page opens for the next five minutes.
- **A second factor is required**: the session was signed in with a password only; sign out and
  sign in with a passkey or your second factor.
- **Not included in your licence**: the feature (SCIM, compliance reports) belongs to the enterprise
  edition; **View licence** opens the licence page.
- **You don't have access**: ask an administrator for the permission.

Installation administrators also have **Installation**: health, organisations, storage use, job
queues, backups, trusted CI issuers and the audit chain.

## Notifications and your profile

The bell shows unread notifications by type; you can mute types. Notifications are kept for a
limited time; the audit log is the permanent record. **Profile** sets your theme (*Match system*,
*Light*, *Dark* or *High contrast*), language and density.
