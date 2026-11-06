// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useCallback, useMemo, useSyncExternalStore } from "react";
import type { EditSession } from "../editor/edit-session.ts";
import type { JsonValue } from "../editor/json-patch.ts";

type SessionDocs = Readonly<Record<string, { readonly content: JsonValue }>>;

const NONE: SessionDocs = Object.freeze({});

/** The documents an edit session holds, unsaved edits included, as they change; none without a session. */
export function useSessionDocs(session: EditSession | undefined): SessionDocs {
  const subscribe = useCallback(
    (onChange: () => void) => (session === undefined ? () => undefined : session.store.subscribe(onChange)),
    [session],
  );
  return useSyncExternalStore(subscribe, () => (session === undefined ? NONE : session.store.getState().docs));
}

interface DocLike {
  readonly path?: string;
  readonly kind: string;
  readonly content: Uint8Array;
  readonly parsed?: unknown;
}

const EMPTY = new Uint8Array();

/**
 * The app's documents with a session's copies over them (PC-17): a translation saved on the canvas — a new key's
 * message — is drawn at once, before the server has it; a document only the session holds is added.
 */
export function useWithSessionDocs<T extends DocLike>(
  docs: readonly T[] | undefined,
  session: EditSession | undefined,
): readonly DocLike[] | undefined {
  const local = useSessionDocs(session);
  return useMemo(() => {
    const paths = Object.keys(local);
    if (docs === undefined || paths.length === 0) {
      return docs;
    }
    const known = new Set(docs.map((d) => d.path));
    const kindOf = (content: JsonValue) => {
      const kind =
        typeof content === "object" && content !== null ? (content as Record<string, unknown>)["kind"] : undefined;
      return typeof kind === "string" ? kind : "";
    };
    return [
      ...docs.map((d) =>
        d.path !== undefined && local[d.path] !== undefined ? { ...d, parsed: local[d.path]?.content } : d,
      ),
      ...paths
        .filter((p) => !known.has(p))
        .map((p) => ({ path: p, kind: kindOf(local[p]?.content ?? null), content: EMPTY, parsed: local[p]?.content })),
    ];
  }, [docs, local]);
}
