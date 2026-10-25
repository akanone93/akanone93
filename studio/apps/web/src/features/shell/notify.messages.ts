// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** Messages of the notifications, status chips, update banner, help menu and unsupported page (STU-012). */
export function notifyMessages(i18n: I18n) {
  return {
    bell: i18n._({
      id: "notify.bell",
      message: "Notifications",
      comment: "Accessible name of the notifications button.",
    }),
    unread: (count: number) =>
      i18n._({
        id: "notify.unread",
        message: "{count} unread",
        comment: "Number of unread notifications.",
        values: { count },
      }),
    empty: i18n._({ id: "notify.empty", message: "No notifications", comment: "Shown when the inbox has no items." }),
    openInbox: i18n._({
      id: "notify.open-inbox",
      message: "Open all notifications",
      comment: "Link from the popover to the full inbox.",
    }),
    markRead: i18n._({
      id: "notify.mark-read",
      message: "Mark as read",
      comment: "Button that marks one notification read.",
    }),
    inboxTitle: i18n._({
      id: "notify.inbox.title",
      message: "Notifications",
      comment: "Heading of the notification inbox page.",
    }),
    filterUnread: i18n._({
      id: "notify.inbox.unread-only",
      message: "Unread only",
      comment: "Toggle that hides read notifications.",
    }),
    filterType: i18n._({ id: "notify.inbox.type", message: "Event type", comment: "Label of the event type filter." }),
    filterAll: i18n._({
      id: "notify.inbox.all-types",
      message: "All types",
      comment: "Option that shows every event type.",
    }),
    loadFailed: i18n._({
      id: "notify.inbox.failed",
      message: "Notifications could not be loaded.",
      comment: "Shown when the inbox request fails.",
    }),
    chipOffline: i18n._({
      id: "chip.offline",
      message: "Offline",
      comment: "Status chip: the browser has no network.",
    }),
    chipReconnecting: i18n._({
      id: "chip.reconnecting",
      message: "Reconnecting",
      comment: "Status chip: the live connection to the server dropped and is being retried.",
    }),
    updatedBanner: i18n._({
      id: "banner.updated",
      message: "Studio was updated. Reload to use the new version.",
      comment: "Banner shown when the server runs a newer Studio.",
    }),
    reload: i18n._({ id: "banner.reload", message: "Reload", comment: "Button that reloads the page." }),
    help: i18n._({ id: "help.menu", message: "Help", comment: "Label of the help menu." }),
    helpDocs: i18n._({
      id: "help.docs",
      message: "Documentation",
      comment: "Help menu entry that opens the documentation.",
    }),
    helpShortcuts: i18n._({
      id: "help.shortcuts",
      message: "Keyboard shortcuts",
      comment: "Help menu entry that opens the shortcut sheet.",
    }),
    helpOnboarding: i18n._({
      id: "help.onboarding",
      message: "Getting started",
      comment: "Help menu entry that opens the onboarding guide.",
    }),
    unsupportedTitle: i18n._({
      id: "unsupported.title",
      message: "This browser is not supported",
      comment: "Heading shown when WebGL2 is missing.",
    }),
    unsupportedBody: i18n._({
      id: "unsupported.body",
      message:
        "Plux Studio needs WebGL2 to draw the canvas. Use a current version of Chrome, Edge, Firefox or Safari with hardware acceleration on.",
      comment: "Explains why the browser cannot run Studio.",
    }),
  };
}
