// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { Code, ConnectError } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import type { Document } from "@plux/api";
import { limitDefault, type PermissionName } from "@plux/schema";
import { notify } from "@plux/ui";
import { createContext, type ReactElement, type ReactNode, useContext, useEffect, useRef, useState } from "react";
import { useStore } from "zustand";
import { lockWorkerScriptURL } from "../../lib/trusted-worker.ts";
import { useApp } from "../../lib/use-app.tsx";
import { useCurrentUser } from "../../lib/use-current-user.ts";
import { ConflictDialog } from "./conflict-dialog.tsx";
import { documentTransport, loadSessionEntries, tabEditorSession, toServerDocument } from "./document-transport.ts";
import { type EditRecord, EditSession, type EditState } from "./edit-session.ts";
import { editorMessages } from "./editor.messages.ts";
import { LockController, type LockStatus, LockUnavailable } from "./lock-controller.ts";
import { lockTransport, type TickWorker, tabChannel, workerTicker } from "./lock-transport.ts";
import { LockBanner, LockLostDialog } from "./lock-ui.tsx";
import lockWorkerURL from "./lock-worker.ts?worker&url";
import { useReportPending } from "./pending-changes.tsx";
import type { UndoManager } from "./undo-stack.ts";

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
  /**
   * Whether this user may edit the plugin at all (default true). Beyond that the lock decides
   * (F1, S2.3): editable while this tab holds it or nobody else does, read-only otherwise.
   */
  readonly canEdit?: boolean;
  readonly onFirstEdit?: AcquireLock;
  /** Every edit with its inverse patches, for the S2.4 undo stack. */
  readonly onEdit?: (record: EditRecord) => void;
  readonly onRejected?: (path: string, error: unknown, undone: readonly string[]) => void;
  /** The tab's editor session, which tags every write for the undo rebuild (STU-069). */
  readonly editorSessionId?: string;
  /** The lock this editor takes: empty for the draft's, "translations/<locale>" for one locale's messages (I18N-008). */
  readonly lockScope?: string;
  /** Leaves the read-only banner to the caller, for an editor whose children are not the page (the app session of the designers). */
  readonly hideBanner?: boolean;
  /** The undo stacks this plugin's edits go to (stack key: the plugin ID, "" for app documents). */
  readonly undo?: UndoManager | undefined;
  /** The stack key of this editor's edits when it is not the plugin ID: a locale's messages beside the app's keys. */
  readonly undoStack?: string;
  readonly children: ReactNode;
}

/** What the server said when it refused a save, without the error code prefix, for the person to read. */
function refusalOf(error: unknown): string {
  if (error instanceof LockUnavailable) {
    return error.message;
  }
  return ConnectError.from(error).rawMessage.replace(/^PLX-\d+:\s*/, "");
}

/** The tab's sessionStorage, or undefined where the browser blocks it. It holds only the editor-session ID. */
function tabStorage(): Storage | undefined {
  try {
    return globalThis.sessionStorage;
  } catch {
    return undefined;
  }
}

const EditorContext = createContext<EditSession | null>(null);
const LockContext = createContext<LockController | null>(null);
const UndoContext = createContext<UndoManager | undefined>(undefined);

/** The heartbeat worker, or undefined where workers cannot start (the ticker then uses timers). */
function makeLockWorker(): TickWorker {
  return new Worker(lockWorkerScriptURL(lockWorkerURL, window, import.meta.env.DEV), {
    type: "module",
  }) as unknown as TickWorker;
}

function lockWorkerFactory(): (() => TickWorker) | undefined {
  if (typeof Worker === "undefined") {
    return undefined;
  }
  try {
    makeLockWorker().terminate();
    return makeLockWorker;
  } catch {
    return undefined;
  }
}

/** Resolves once the pipeline has nothing unconfirmed, after flushing it. */
export function flushed(edit: EditSession): Promise<void> {
  edit.flushAll();
  return new Promise((resolve) => {
    if (edit.waiting() === 0) {
      resolve();
      return;
    }
    const stop = edit.store.subscribe(() => {
      if (edit.waiting() === 0) {
        stop();
        resolve();
      }
    });
  });
}

export const CanEditContext = createContext(false);

const LOCK_OVERRIDE: PermissionName = "plugin.lock.override";

/** Editable when this tab holds the lock or can take it on the first edit (nobody else holds it). */
export function editableWhile(status: LockStatus): boolean {
  return status === "held" || status === "free" || status === "acquiring";
}

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
  const { i18n } = useLingui();
  const messages = useRef(editorMessages(i18n));
  messages.current = editorMessages(i18n);
  const [lock] = useState(
    () =>
      new LockController({
        transport: lockTransport(session.transport, { appId, pluginId, scope: props.lockScope ?? "" }),
        session: crypto.randomUUID(),
        idleReleaseMs: limitDefault("studio.lockIdleReleaseMinutes") * 60_000,
        takeoverIdleMs: limitDefault("studio.lockTakeoverIdleMinutes") * 60_000,
        scheduler: { setTimeout: (fn, ms) => setTimeout(fn, ms), clearTimeout: (h) => clearTimeout(h as number) },
        ticker: workerTicker(lockWorkerFactory()),
        channel: tabChannel(appId, pluginId, props.lockScope ?? ""),
        beforeRelease: () => flushed(edit),
      }),
  );
  const [edit] = useState(() => {
    const confirmed = (doc: Document) =>
      session.queryClient.setQueryData<Document[]>(documentsKey(appId, pluginId), (old) =>
        old?.map((d) => (d.path === doc.path ? doc : d)),
      );
    // Every write is tagged with the tab's editor session: the server refuses an undo step without one (PLX-1005), and
    // an editor whose caller passed none (the app's translations and theme) still groups its edits into undo steps.
    const base = documentTransport(
      session.transport,
      { appId, pluginId, editorSessionId: latest.current.editorSessionId ?? tabEditorSession(tabStorage()) },
      confirmed,
      () => lock.whenHeld(),
    );
    return new EditSession({
      transport: {
        ...base,
        // F1.1: a save the lock refuses undoes its edit; a save refused because the lock moved keeps
        // its edits in memory while the lock is checked (F1.4).
        classify(error) {
          if (error instanceof LockUnavailable) {
            return "rejected";
          }
          if (ConnectError.from(error).code === Code.FailedPrecondition) {
            lock.suspect();
            return "transient";
          }
          return base.classify(error);
        },
      },
      debounceMs: limitDefault("studio.autosaveDebounceMs"),
      maxReplays: limitDefault("studio.autosaveReplays"),
      onFirstEdit: (path) => {
        void lock.ensure().catch(() => undefined);
        latest.current.onFirstEdit?.({ appId, pluginId, path });
      },
      stepFor: (_path, group) => latest.current.undo?.stepFor(latest.current.undoStack ?? pluginId, group),
      onEdit: (record) => {
        latest.current.undo?.record(latest.current.undoStack ?? pluginId, record);
        latest.current.onEdit?.(record);
      },
      onCreated: (record) => latest.current.undo?.recordCreated(latest.current.undoStack ?? pluginId, record),
      onRejected: (path, error, undone, steps) => {
        // An edit applies as a whole or not at all (F4): the parts of the refused steps in other documents (the
        // page beside a new translation key, the other plugin of a move) are rolled back too, and the person is
        // told; nothing vanishes silently.
        const stack = latest.current.undoStack ?? pluginId;
        const rolledBack = latest.current.undo?.abandon(steps, { stack, path }) ?? [];
        notify("error", messages.current.saveRefused(refusalOf(error), [path, ...rolledBack].length));
        latest.current.onRejected?.(path, error, undone);
      },
    });
  });

  useEffect(() => {
    for (const d of documents ?? []) {
      edit.open(toServerDocument(d));
    }
  }, [edit, documents]);

  const { undo, editorSessionId } = props;
  const stack = props.undoStack ?? pluginId;
  useEffect(() => {
    if (undo === undefined) {
      return;
    }
    const detach = undo.attach(stack, edit);
    if (editorSessionId !== undefined && editorSessionId !== "") {
      const depth = limitDefault("studio.undoDepth");
      loadSessionEntries(session.transport, { appId, pluginId, editorSessionId }, depth).then(
        (entries) => undo.rebuild(stack, entries),
        // Without the entries the stack starts empty; editing is unaffected.
        () => undefined,
      );
    }
    return detach;
  }, [undo, edit, appId, pluginId, stack, editorSessionId, session.transport]);
  useEffect(() => {
    const detach = edit.attach({ window, document });
    const detachLock = lock.attach(window);
    void lock.refresh();
    return () => {
      detach();
      detachLock();
      void lock.release(true);
      lock.dispose();
      edit.dispose();
    };
  }, [edit, lock]);

  const status = useStore(lock.store, (st) => st.status);
  const canEdit = (props.canEdit ?? true) && editableWhile(status);
  // D36 "Take over" is offered only to holders of plugin.lock.override; the server checks it too.
  const user = useCurrentUser();
  const canOverride = user.data?.permissions.includes(LOCK_OVERRIDE) ?? false;

  const waiting = useStore(edit.store, (s) => Object.values(s.docs).reduce((n, d) => n + d.waiting, 0));
  useReportPending(`${appId}/${pluginId}`, waiting);

  return (
    <EditorContext.Provider value={edit}>
      <LockContext.Provider value={lock}>
        <UndoContext.Provider value={undo}>
          <CanEditContext.Provider value={canEdit}>
            {props.hideBanner !== true && (
              <LockBanner lock={lock} canEdit={props.canEdit ?? true} canOverride={canOverride} />
            )}
            {props.children}
            <ConflictDialog session={edit} />
            <LockLostDialog lock={lock} session={edit} />
          </CanEditContext.Provider>
        </UndoContext.Provider>
      </LockContext.Provider>
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

/** The editing lock of the surrounding EditorProvider (F1). */
export function useLock(): LockController {
  const lock = useContext(LockContext);
  if (lock === null) {
    throw new Error("useLock outside EditorProvider");
  }
  return lock;
}

/** The undo stacks of the surrounding EditorProvider, for editors beside it that join its steps (F3). */
export function useUndoManager(): UndoManager | undefined {
  return useContext(UndoContext);
}

/** Whether the editor may change documents (the plugin lock is held); false by default. */
export function useCanEdit(): boolean {
  return useContext(CanEditContext);
}
