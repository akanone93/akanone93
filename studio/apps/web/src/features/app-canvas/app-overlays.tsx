// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { Document } from "@plux/api";
import type { ReactNode } from "react";
import { HistoryOverlay } from "../history/index.ts";
import { AppJsonEditor } from "./app-json-editor.tsx";
import { AppLogic } from "./app-logic.tsx";
import { AppStateDesigner } from "./app-state.tsx";

/** The full-screen editors of the app's own documents the app canvas opens, one at a time. */
export type AppOverlay = "json" | "state" | "logic" | "history";

/**
 * The app canvas's overlays on the app's own documents: the JSON editor (#260, #285), the State designer (#283) and
 * the Action graph editor on the app's triggers (#284) and the history (#267). The editor and the designer are mounted only while open, so their validator starts only then.
 */
export function AppOverlays(props: {
  readonly open: AppOverlay | undefined;
  readonly appId: string;
  readonly documents: readonly Document[] | undefined;
  readonly project: readonly Document[] | undefined;
  readonly onClose: () => void;
}): ReactNode {
  const { open, onClose } = props;
  return (
    <>
      <HistoryOverlay open={open === "history"} onClose={onClose} />
      {open === "state" && (
        <AppStateDesigner appId={props.appId} documents={props.documents} project={props.project} onClose={onClose} />
      )}
      {open === "logic" && (
        <AppLogic appId={props.appId} documents={props.documents} project={props.project} onClose={onClose} />
      )}
      {open === "json" && (
        <AppJsonEditor appId={props.appId} documents={props.documents} project={props.project} onClose={onClose} />
      )}
    </>
  );
}
