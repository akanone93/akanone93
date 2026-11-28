<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Publishing and Releases

Publishing freezes the drafts, builds signed bundles, checks them, and creates a release. A rollout
then decides which devices get it. The commands behind each step are in the
[first release](../first-release.md) guide and the [CLI reference](../../reference/cli.md); this
guide covers Studio's screens.

## Publish

1. Choose **Publish** on the plugin canvas or in the app's navigation. Studio resumes the app's open
   release request, wherever its target step sent it (opened from a page with an environment, that
   environment's request), and opens a new one only when none is open. If none is open and neither a
   plugin nor the app's own documents have unpublished changes, it says so.
   Publishing needs your security key confirmed in the last five minutes (step-up, `SEC-103`): when it has
   not been, Studio asks you to confirm it is you (**Use security key or passkey**) and then carries on, at the
   start and at any later step.
   Add a key first under **Profile › Security keys**.
2. **Select plugins.** Plugins with unpublished changes are listed; a plugin another one needs is
   added for you ("Added because another plugin needs it"). When the app's own documents (its
   settings, theme or translations) changed since its last release, or it was never released, the
   **App (theme, translations, settings)** bundle is added as well: the plugins may read its new
   keys and tokens, so it is released with them.
3. **Freeze and build.** Editing may continue; the builds freeze the draft as it is now. The app and
   the plugins of the request are built against each other's frozen drafts, so a change that spans
   them (a type renamed in the app and in the plugins that use it) builds in one request. Each plugin
   shows *Queued*, *Running*, *Built* or *Failed* with its progress; a failed build lists the findings
   that stopped it (code, message and file). Errors fail a build; warnings do not, they wait for the next
   step. **Freeze again** takes the current draft and builds it again, in one press: use it after editing,
   or to try builds that failed for a passing reason.
4. **Reports.** Validation, bundle **Size** (and the change against the version before),
   **Accessibility** findings and the other **Warnings**, each with its code, message, file and the
   fix to make. The acknowledgement dialog lists the same details. Acknowledge every warning here
   (**Acknowledge warnings**): the request
   does not move on, and nothing is released, until you have (`SRV-051`). A request can be *Blocked* by
   errors; see
   [Publish blocked](../../runbooks/publish-blocked-quality.md).
5. **Visual diffs**: the before-and-after images of changed screens are parked
   ([ADR-0113](../../adr/0113-visual-diff-parked.md)); the step says "Visual diff: coming soon" and
   **Continue** moves on without a render. The **Changed documents**
   list shows added, removed and changed JSON paths per plugin. **Changelog** takes the release notes.
6. **Target** chooses the environment and channel. **Approval** appears when a policy applies; the
   request is *Awaiting approval* until enough approvers decide ([approvals](security-and-admin.md#approvals)).
7. **Rollout plan** sets the stages, the share of devices, and the health gate; **Release** creates
   the release. The request ends *Released*, *Rejected*, *Expired* or *Cancelled*.

You need the permission `release.publish` to change a request; without it you can view it. When the
server refuses a step, Studio shows its reason ("The step failed: …") under the step.

## Follow and control a release

- **Releases** lists every release of an environment, newest first, with its channel, notes and the
  rollout that serves it.
- Open a release for its **Content**, **Changelog**, **Bundle SHA-256**, **Adoption** and **Health
  against the previous release** (sessions, crashed sessions and errors; metrics that regressed
  beyond the gate are named).
- **Rollout controls:** **Pause**, **Resume**, **Advance to next stage**, **Change end** and **Abort
  rollout**. A stuck or paused rollout: [Stuck rollout](../../runbooks/stuck-rollout.md) and
  [Health-gate false positives](../../runbooks/health-gate-false-positives.md).
- **Promote** sends the release, unchanged (the same bundles and signatures), to another
  environment: choose the **Target environment** and **Promote release**; it asks for a security
  check (step-up) and needs `release.promote`. Every function version the release pins must be in
  that environment first (`plux fn promote`). When the environment's approval policy holds the
  promotion, the panel says so with the server's reason and an approval request opens in
  **Approvals**; promote again once it is approved.
- **Roll back to this release** creates a new release with this content on the channel; it asks for
  a security check (step-up).
- **Kill switch** switches a plugin off for every device of the environment without a new release.
  Enter the **Plugin key** and **Switch off**; **Switch on** reverses it.

Background: [Rollouts and experiments](../rollouts-and-experiments.md).
