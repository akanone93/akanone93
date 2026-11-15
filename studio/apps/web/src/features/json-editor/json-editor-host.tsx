// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { Document } from "@plux/api";
import { useParams } from "@tanstack/react-router";
import { useMemo } from "react";
import { useEditor } from "../editor/editor-provider.tsx";
import { KEYS_PATH } from "../localisation/l10n-model.ts";
import { APP_DESIGNER_PATHS, useAppSession } from "../localisation/l10n-sessions.tsx";
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
  /** A document to open on, such as a finding's file; otherwise the editor opens on the selection's. */
  readonly path?: string | undefined;
  /** The app's documents; those the app session holds (app.json, the theme, the keys) are offered too. */
  readonly appDocuments?: readonly Document[] | undefined;
}

/** The app documents the plugin canvas's app session edits under the app lock. */
const APP_PATHS: ReadonlySet<string> = new Set([...APP_DESIGNER_PATHS, KEYS_PATH]);

/**
 * The JSON editor of the plugin canvas: it opens on the document the selection is in, and compares with the plugin's
 * last release. The app's own documents (app.json, the theme, the translation keys) are listed beside the plugin's and
 * saved by the app session, so app-level settings that have no page of their own can still be edited.
 */
export function JsonEditorHost(props: JsonEditorHostProps) {
  const { plugin } = useParams({ strict: false }) as { plugin?: string };
  const first = useEditor((st) => st.selection.ids[0]);
  const loadPublished = usePublishedLoader(plugin ?? "");
  const appSession = useAppSession();
  const appDocuments = useMemo(
    () => (props.appDocuments ?? []).filter((d) => d.pluginId === "" && APP_PATHS.has(d.path)),
    [props.appDocuments],
  );
  const screen = first === undefined ? undefined : props.screenOf.get(first);
  const initialPath = useMemo(
    () => props.path ?? (screen === undefined ? undefined : props.pathOf.get(screen)),
    [props.path, screen, props.pathOf],
  );
  return (
    <JsonEditorOverlay
      open={props.open}
      onClose={props.onClose}
      documents={props.documents ?? []}
      initialPath={initialPath}
      validatePage={props.validation.state === "done" ? props.validation.validatePage : undefined}
      loadPublished={loadPublished}
      {...(appSession !== null && {
        app: { documents: appDocuments, session: appSession.session, canEdit: appSession.canEdit },
      })}
    />
  );
}
