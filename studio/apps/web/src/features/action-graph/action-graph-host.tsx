// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useParams } from "@tanstack/react-router";
import { useMemo } from "react";
import { useEditor } from "../editor/editor-provider.tsx";
import { parseDocument } from "../plugin-canvas/plugin-model.ts";
import type { Validation } from "../problems/use-validation.ts";
import { ActionGraphOverlay, type GraphDocument } from "./action-graph-overlay.tsx";
import { extrasOf } from "./graph-model.ts";
import { useNativeActions } from "./use-native-actions.ts";

export interface ActionGraphHostProps {
  readonly open: boolean;
  readonly onClose: () => void;
  readonly documents: readonly GraphDocument[] | undefined;
  /** The screen each node is in, and the document path of each screen: the selection names the page to open. */
  readonly screenOf: ReadonlyMap<string, string>;
  readonly pathOf: ReadonlyMap<string, string>;
  readonly validation: Validation;
}

const NO_DOCUMENTS: readonly GraphDocument[] = [];

/** The action graph editor of the plugin canvas: it opens on the selected widget, with the project's actions, functions and flows. */
export function ActionGraphHost(props: ActionGraphHostProps) {
  const { app } = useParams({ strict: false }) as { app?: string };
  const first = useEditor((st) => st.selection.ids[0]);
  const custom = useNativeActions(app ?? "");
  const documents = props.documents ?? NO_DOCUMENTS;
  const extras = useMemo(
    () =>
      extrasOf(
        documents.map((d) => parseDocument(d.content)),
        custom,
      ),
    [documents, custom],
  );
  const screen = first === undefined ? undefined : props.screenOf.get(first);
  const pagePath = screen === undefined ? undefined : props.pathOf.get(screen);
  const checked = props.validation.state === "done" ? props.validation : undefined;
  return (
    <ActionGraphOverlay
      open={props.open}
      onClose={props.onClose}
      documents={documents}
      pagePath={pagePath}
      nodeId={first}
      extras={extras}
      validatePage={checked?.validatePage}
      language={checked?.language}
    />
  );
}
