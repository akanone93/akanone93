// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** Messages of the editing lock: the read-only banner, D36 and D37 (decisions F1, L-1). */
export function lockMessages(i18n: I18n) {
  const time = (d: Date | undefined) =>
    d === undefined ? "—" : i18n.date(d, { dateStyle: "medium", timeStyle: "short" });
  return {
    heldBy: (holder: string, since: Date | undefined) =>
      i18n._({
        id: "lock.banner.held-by",
        message: "{holder} has been editing this plugin since {since}. You can look around, select and copy.",
        values: { holder, since: time(since) },
        comment: "Read-only banner: someone else holds the plugin's editing lock.",
      }),
    otherTab: i18n._({
      id: "lock.banner.other-tab",
      message: "You are editing this plugin in another tab.",
      comment: "Read-only banner: the same person holds the lock in another browser tab.",
    }),
    requestEdit: i18n._({
      id: "lock.request-edit",
      message: "Request edit",
      comment: "Button: ask the person holding the lock to hand it over.",
    }),
    requested: (holder: string) =>
      i18n._({
        id: "lock.requested",
        message: "{holder} was asked to hand over the plugin.",
        values: { holder },
        comment: "Shown after Request edit was sent.",
      }),
    takeOver: i18n._({
      id: "lock.take-over",
      message: "Take over",
      comment: "Button (D36): take the editing lock from its holder.",
    }),
    takeOverFrom: (at: Date | undefined) =>
      i18n._({
        id: "lock.take-over-from",
        message: "Take over is possible from {at}, once the holder has stopped editing.",
        values: { at: time(at) },
        comment: "Why Take over is disabled: the holder edited recently (L-1).",
      }),
    takeoverTitle: i18n._({
      id: "lock.takeover.title",
      message: "Take over editing?",
      comment: "D36 title.",
    }),
    takeoverBody: (holder: string, last: Date | undefined) =>
      i18n._({
        id: "lock.takeover.body",
        message:
          "{holder} holds the lock; their last edit was {last}. Their work is kept as a snapshot and they are told who took over.",
        values: { holder, last: time(last) },
        comment: "D36 explanation with the holder and their last activity.",
      }),
    takeoverRefused: i18n._({
      id: "lock.takeover.refused",
      message: "The lock could not be taken over: the holder edited again, or you may not take locks over.",
      comment: "Shown when the server refused a takeover.",
    }),
    cancel: i18n._({ id: "lock.cancel", message: "Cancel", comment: "Dialog button." }),
    lostTitle: i18n._({
      id: "lock.lost.title",
      message: "You no longer hold the editing lock",
      comment: "D37 title: the lock was taken over or expired.",
    }),
    lostTakeover: (holder: string) =>
      i18n._({
        id: "lock.lost.takeover",
        message: "{holder} took over editing. Your work up to then is kept as a snapshot.",
        values: { holder },
        comment: "D37: someone took the lock over.",
      }),
    lostExpired: i18n._({
      id: "lock.lost.expired",
      message: "Your lock expired because it could not be renewed.",
      comment: "D37: the heartbeat failed until the lock expired.",
    }),
    unsaved: (count: number) =>
      i18n._({
        id: "lock.lost.unsaved",
        message: "{count, plural, =0 {No unsaved changes} one {# unsaved change} other {# unsaved changes}}",
        values: { count },
        comment: "D37: edits kept in memory that were not sent.",
      }),
    retake: i18n._({
      id: "lock.lost.retake",
      message: "Re-take and apply",
      comment: "D37 button: take the lock again and send the unsaved changes.",
    }),
    retakeFailed: i18n._({
      id: "lock.lost.retake-failed",
      message: "Someone else holds the lock now.",
      comment: "D37: re-taking failed because the lock is not free.",
    }),
    compare: i18n._({
      id: "lock.lost.compare",
      message: "Compare",
      comment: "D37 button: list the documents with unsaved changes.",
    }),
    discard: i18n._({
      id: "lock.lost.discard",
      message: "Discard",
      comment: "D37 button: drop the unsaved changes.",
    }),
    requestsBody: (names: string) =>
      i18n._({
        id: "lock.requests.body",
        message: "{names} asked to edit this plugin.",
        values: { names },
        comment: "Shown to the holder when others requested the lock.",
      }),
    release: i18n._({
      id: "lock.requests.release",
      message: "Hand over",
      comment: "Button: release the lock to whoever asked.",
    }),
    keep: i18n._({
      id: "lock.requests.keep",
      message: "Keep editing",
      comment: "Button: keep the lock despite the request.",
    }),
    close: i18n._({ id: "lock.close", message: "Close", comment: "Dialog close button." }),
  };
}
