// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { Document } from "@plux/api";
import { limitDefault } from "@plux/schema";
import { createContext, type ReactElement, type ReactNode, useContext, useEffect, useRef, useState } from "react";
import { useStore } from "zustand";
import { useApp } from "../../lib/use-app.tsx";
import { ConflictDialog } from "./conflict-dialog.tsx";
import { documentTransport, toServerDocument } from "./document-transport.ts";
import { type EditRecord, EditSession, type EditState } from "./edit-session.ts";
import { useReportPending } from "./pending-changes.tsx";

/** Called on the first edit of a document; S2.3 acquires the plugin's lock here. */
export type AcquireLock = (target: {
  readonly appId: string;
  readonly pluginId: string;
  readonly path: string;
}) => void;

export interface EditorProviderProps {
  readonly appId: string;
  readonly pluginId: string;
  /** The plugin's documents as the Query cache holds them (server-confirmed). */
  readonly documents: readonly Document[] | undefined;
  /** False until S2.3 grants the lock: the canvas draws feedback but its edits are dropped. */
  readonly canEdit?: boolean;
  readonly onFirstEdit?: AcquireLock;
  /** Every edit with its inverse patches, for the S2.4 undo stack. */
  readonly onEdit?: (record: EditRecord) => void;
  readonly onRejected?: (path: string, error: unknown, undone: readonly string[]) => void;
  readonly children: ReactNode;
}

const EditorContext = createContext<EditSession | null>(null);
const CanEditContext = createContext(false);

function documentsKey(appId: string, pluginId: string) {
  return ["plugin-documents", appId, pluginId] as const;
}

/**
 * The edit pipeline of one open plugin (ADR-0014: a Zustand store per area, created here and never
 * at module level). The edited documents live in its store; Query keeps the server-confirmed ones.
 */
export function EditorProvider(props: EditorProviderProps): ReactElement {
  const { appId, pluginId, documents } = props;
  const { session } = useApp();
  // The callbacks are read through a ref so the session, made once, always calls the latest ones.
  const latest = useRef(props);
  latest.current = props;
  const [edit] = useState(() => {
    const confirmed = (doc: Document) =>
      session.queryClient.setQueryData<Document[]>(documentsKey(appId, pluginId), (old) =>
        old?.map((d) => (d.path === doc.path ? doc : d)),
      );
    return new EditSession({
      transport: documentTransport(session.transport, { appId, pluginId }, confirmed),
      debounceMs: limitDefault("studio.autosaveDebounceMs"),
      maxReplays: limitDefault("studio.autosaveReplays"),
      onFirstEdit: (path) => latest.current.onFirstEdit?.({ appId, pluginId, path }),
      onEdit: (record) => latest.current.onEdit?.(record),
      onRejected: (path, error, undone) => latest.current.onRejected?.(path, error, undone),
    });
  });

  useEffect(() => {
    for (const d of documents ?? []) {
      edit.open(toServerDocument(d));
    }
  }, [edit, documents]);

  useEffect(() => {
    const detach = edit.attach({ window, document });
    return () => {
      detach();
      edit.dispose();
    };
  }, [edit]);

  const waiting = useStore(edit.store, (s) => Object.values(s.docs).reduce((n, d) => n + d.waiting, 0));
  useReportPending(`${appId}/${pluginId}`, waiting);

  return (
    <EditorContext.Provider value={edit}>
      <CanEditContext.Provider value={props.canEdit ?? false}>
        {props.children}
        <ConflictDialog session={edit} />
      </CanEditContext.Provider>
    </EditorContext.Provider>
  );
}

/** The pipeline of the surrounding EditorProvider: `edit(path, recipe, {group})` and friends. */
export function useEditSession(): EditSession {
  const edit = useContext(EditorContext);
  if (edit === null) {
    throw new Error("useEditSession outside EditorProvider");
  }
  return edit;
}

/** Selects from the surrounding editor's store. */
export function useEditor<T>(selector: (state: EditState) => T): T {
  return useStore(useEditSession().store, selector);
}

/** Whether the editor may change documents (the plugin lock is held); false by default. */
export function useCanEdit(): boolean {
  return useContext(CanEditContext);
}
