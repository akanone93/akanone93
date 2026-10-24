// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** Messages of the layouts, the top bar and the error pages (STU-012). */
export function chromeMessages(i18n: I18n) {
  return {
    topBar: i18n._({ id: "chrome.topbar", message: "Studio navigation", comment: "Accessible name of the top bar." }),
    organisation: i18n._({
      id: "chrome.org.switcher",
      message: "Organisation",
      comment: "Label of the organisation switcher.",
    }),
    profile: i18n._({
      id: "chrome.profile.menu",
      message: "Profile",
      comment: "Label of the profile menu button and its first entry.",
    }),
    signOut: i18n._({
      id: "chrome.profile.signout",
      message: "Sign out",
      comment: "Menu entry that ends the session.",
    }),
    home: i18n._({ id: "chrome.home", message: "Home", comment: "Name of the personal home page." }),
    notFoundTitle: i18n._({
      id: "chrome.notfound.title",
      message: "Page not found",
      comment: "Heading of the 404 page, also shown for an organisation the person is not in.",
    }),
    notFoundBody: i18n._({
      id: "chrome.notfound.body",
      message: "This page does not exist or you cannot see it.",
      comment: "Body of the 404 page.",
    }),
    forbiddenTitle: i18n._({
      id: "chrome.forbidden.title",
      message: "You do not have access",
      comment: "Heading of the 403 page.",
    }),
    forbiddenBody: i18n._({
      id: "chrome.forbidden.body",
      message: "Your role does not include the permission this page needs.",
      comment: "Body of the 403 page.",
    }),
    explain: i18n._({
      id: "chrome.forbidden.explain",
      message: "Explain access",
      comment: "Button that asks the server why access is denied.",
    }),
    decision: (decision: string) =>
      i18n._({
        id: "chrome.forbidden.decision",
        message: "Decision: {decision}",
        values: { decision },
        comment: "Result of explaining access; decision is the server's verdict.",
      }),
    explainFailed: i18n._({
      id: "chrome.forbidden.explain-failed",
      message: "Access could not be explained right now.",
      comment: "Shown when the explain-access request fails.",
    }),
    licenceTitle: i18n._({
      id: "chrome.licence.title",
      message: "Enterprise: requires a licence",
      comment: "Heading of the page shown for enterprise-only pages without a licence.",
    }),
    licenceBody: i18n._({
      id: "chrome.licence.body",
      message: "This page is part of the enterprise edition. Ask an installation administrator to install a licence.",
      comment: "Body of the licence page.",
    }),
    noOrgTitle: i18n._({
      id: "chrome.noorg.title",
      message: "You are not in an organisation yet",
      comment: "Heading shown to a signed-in person with no organisation.",
    }),
    noOrgBody: i18n._({
      id: "chrome.noorg.body",
      message: "Ask an administrator to invite you to an organisation.",
      comment: "Body shown to a signed-in person with no organisation.",
    }),
    signedOut: i18n._({
      id: "chrome.signedout.title",
      message: "You are signed out",
      comment: "Heading of the page shown after signing out.",
    }),
    signInAgain: i18n._({
      id: "chrome.signedout.again",
      message: "Sign in again",
      comment: "Link from the signed-out page to the sign-in page.",
    }),
    loading: i18n._({ id: "chrome.loading", message: "Loading", comment: "Shown while a page loads." }),
  };
}
