// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import type { ReactElement } from "react";
import type { DocState } from "./edit-session.ts";
import { editorMessages } from "./editor.messages.ts";
import { useEditor } from "./editor-provider.tsx";

export type SaveState = "saved" | "saving" | "offline" | "conflict";

/** The worst state over the open documents: a conflict, then no network, then edits on their way, then saved. */
export function saveStateOf(docs: Readonly<Record<string, DocState>>): SaveState {
  const all = Object.values(docs).map((d) => d.status);
  if (all.includes("conflict")) {
    return "conflict";
  }
  if (all.includes("waiting")) {
    return "offline";
  }
  return all.includes("saving") || all.includes("pending") ? "saving" : "saved";
}

/** The save state of the editor's documents, as a dot and a short line for a canvas's top bar (STU-121). */
export function SaveStateChip(): ReactElement {
  const m = editorMessages(useLingui().i18n);
  const state = useEditor((st) => saveStateOf(st.docs));
  return (
    <span className="save-state" data-state={state} role="status">
      <span className="save-state-dot" aria-hidden="true" />
      {m.saveState(state)}
    </span>
  );
}
