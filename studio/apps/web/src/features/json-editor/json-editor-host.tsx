// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useParams } from "@tanstack/react-router";
import { useMemo } from "react";
import { useEditor } from "../editor/editor-provider.tsx";
import type { Validation } from "../problems/use-validation.ts";
import { type JsonDocument, JsonEditorOverlay } from "./json-editor-overlay.tsx";
import { usePublishedLoader } from "./use-published.ts";

export interface JsonEditorHostProps {
  readonly open: boolean;
  readonly onClose: () => void;
  /** The plugin's documents; none until they load. */
  readonly documents: readonly JsonDocument[] | undefined;
  /** The screen each node is in, and the document path of each screen: the selection names the document to open. */
  readonly screenOf: ReadonlyMap<string, string>;
  readonly pathOf: ReadonlyMap<string, string>;
  readonly validation: Validation;
}

/** The JSON editor of the plugin canvas: it opens on the document the selection is in, and compares with the plugin's last release. */
export function JsonEditorHost(props: JsonEditorHostProps) {
  const { plugin } = useParams({ strict: false }) as { plugin?: string };
  const first = useEditor((st) => st.selection.ids[0]);
  const loadPublished = usePublishedLoader(plugin ?? "");
  const screen = first === undefined ? undefined : props.screenOf.get(first);
  const initialPath = useMemo(
    () => (screen === undefined ? undefined : props.pathOf.get(screen)),
    [screen, props.pathOf],
  );
  return (
    <JsonEditorOverlay
      open={props.open}
      onClose={props.onClose}
      documents={props.documents ?? []}
      initialPath={initialPath}
      validatePage={props.validation.state === "done" ? props.validation.validatePage : undefined}
      loadPublished={loadPublished}
    />
  );
}
