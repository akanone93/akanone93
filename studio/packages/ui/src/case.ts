// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { ReactElement } from "react";

/** One state of a component, rendered by the development-only catalogue page and by the tests (studio-development 5.2). */
export interface CaseDefinition {
  readonly name: string;
  readonly element: ReactElement;
}

/** Placeholder copy for the cases; real screens take their text from the message catalogue. */
export const caseText = {
  save: "Save",
  cancel: "Cancel",
  close: "Close",
  delete: "Delete",
  confirm: "Delete page",
  loading: "Saving",
  name: "Name",
  hint: "Shown in the plugin list.",
  required: "A name is required.",
  longText: "A very long label that has to wrap or truncate inside its container without breaking the layout",
  placeholder: "Choose a language",
  english: "English",
  arabic: "Arabic",
  german: "German",
  open: "Open",
  rename: "Rename",
  duplicate: "Duplicate",
  pages: "Pages",
  actions: "Actions",
  details: "Details",
  tipCaption: "Add a page",
  regionLabel: "Notifications",
  title: "Delete this page?",
  body: "The page and its history are removed.",
} as const;

/** Row height of the compact `density.tableRow` token, for the table cases. */
export const caseRowHeight = 30;
