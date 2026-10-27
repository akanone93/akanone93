// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { timestampDate } from "@bufbuild/protobuf/wkt";
import { type Client, createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { type DocumentChange, DocumentService, type Snapshot } from "@plux/api";
import { AlertDialog, Button, Dialog, Select, TypeTag } from "@plux/ui";
import { useParams } from "@tanstack/react-router";
import { Suspense, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useApp } from "../../lib/use-app.tsx";
import { toServerDocument } from "../editor/document-transport.ts";
import { useCanEdit, useEditSession } from "../editor/editor-provider.tsx";
import { MonacoView } from "../json-editor/lazy-view.tsx";
import { type HistoryMessages, historyMessages } from "./history.messages.ts";
import { missingPaths, orderedPair, restoreRecipes, sessionTag } from "./history-model.ts";

export interface HistoryOverlayProps {
  readonly open: boolean;
  readonly onClose: () => void;
}

type Documents = Client<typeof DocumentService>;
const PAGE = 50;

const pretty = (content: Uint8Array | undefined): string => {
  if (content === undefined) {
    return "";
  }
  const raw = new TextDecoder().decode(content);
  try {
    return `${JSON.stringify(JSON.parse(raw), null, 2)}\n`;
  } catch {
    return raw;
  }
};

function reasonOf(reason: string, m: HistoryMessages): string {
  switch (reason) {
    case "write":
      return m.reasonWrite;
    case "lock_takeover":
      return m.reasonLockTakeover;
    case "publish":
      return m.reasonPublish;
    case "restore":
      return m.reasonRestore;
    default:
      return reason;
  }
}

interface Listing {
  readonly state: "loading" | "failed" | "ready";
  readonly rows: readonly Snapshot[];
  readonly next: string;
}

/** The plugin's snapshots, newest first as the server returns them; more pages on request. */
function useSnapshots(open: boolean, documents: Documents, appId: string, pluginId: string) {
  const [listing, setListing] = useState<Listing>({ state: "loading", rows: [], next: "" });
  const load = useCallback(
    async (token: string, keep: readonly Snapshot[]) => {
      try {
        const res = await documents.listSnapshots({ appId, pluginId, page: { pageSize: PAGE, pageToken: token } });
        setListing({ state: "ready", rows: [...keep, ...res.snapshots], next: res.page?.nextPageToken ?? "" });
      } catch {
        setListing({ state: "failed", rows: keep, next: "" });
      }
    },
    [documents, appId, pluginId],
  );
  useEffect(() => {
    if (open) {
      setListing({ state: "loading", rows: [], next: "" });
      void load("", []);
    }
  }, [open, load]);
  return { listing, more: () => load(listing.next, listing.rows) };
}

type Diff =
  | { readonly state: "idle" | "loading" | "failed" }
  | { readonly state: "ready"; readonly older: string; readonly newer: string };

/** The documents that differ between the pair, and the two versions of the chosen one. */
function useComparison(documents: Documents, pair: readonly [string, string] | undefined) {
  const [changes, setChanges] = useState<readonly DocumentChange[]>([]);
  const [path, setPath] = useState("");
  const [diff, setDiff] = useState<Diff>({ state: "idle" });
  const [from, to] = pair ?? ["", ""];
  useEffect(() => {
    setChanges([]);
    setPath("");
    setDiff({ state: "idle" });
    if (from === "") {
      return undefined;
    }
    let live = true;
    documents
      .compareSnapshots({ fromId: from, toId: to })
      .then((res) => {
        if (live) {
          setChanges(res.changes);
          setPath(res.changes[0]?.path ?? "");
        }
      })
      .catch(() => live && setDiff({ state: "failed" }));
    return () => {
      live = false;
    };
  }, [documents, from, to]);
  useEffect(() => {
    if (from === "" || path === "") {
      return undefined;
    }
    let live = true;
    setDiff({ state: "loading" });
    Promise.all([documents.getSnapshot({ id: from, paths: [path] }), documents.getSnapshot({ id: to, paths: [path] })])
      .then(([a, b]) => {
        if (live) {
          setDiff({
            state: "ready",
            older: pretty(a.documents.find((d) => d.path === path)?.content),
            newer: pretty(b.documents.find((d) => d.path === path)?.content),
          });
        }
      })
      .catch(() => live && setDiff({ state: "failed" }));
    return () => {
      live = false;
    };
  }, [documents, from, to, path]);
  return { changes, path, setPath, diff };
}

/** The history overlay V9 (STU-069): the plugin's snapshots, a comparison of any two, and restore as one undo step. */
export function HistoryOverlay({ open, onClose }: HistoryOverlayProps) {
  const { i18n } = useLingui();
  const m = useMemo(() => historyMessages(i18n), [i18n]);
  const { session: app } = useApp();
  const { app: appId = "", plugin = "" } = useParams({ strict: false }) as { app?: string; plugin?: string };
  const documents = useMemo(() => createClient(DocumentService, app.transport), [app]);
  const canEdit = useCanEdit();
  const session = useEditSession();
  const { listing, more } = useSnapshots(open, documents, appId, plugin);
  const [chosen, setChosen] = useState<readonly string[]>([]);
  const [target, setTarget] = useState<Snapshot | null>(null);
  const [note, setNote] = useState("");
  const restores = useRef(0);
  const pair = useMemo(() => orderedPair(listing.rows, chosen), [listing.rows, chosen]);
  const { changes, path, setPath, diff } = useComparison(documents, pair);
  const time = useMemo(() => new Intl.DateTimeFormat(i18n.locale, { dateStyle: "medium", timeStyle: "short" }), [i18n]);
  const whenOf = (s: Snapshot) => (s.createdAt === undefined ? s.id : time.format(timestampDate(s.createdAt)));

  useEffect(() => {
    if (!open) {
      setChosen([]);
      setNote("");
    }
  }, [open]);

  const toggle = (id: string) => setChosen((c) => (c.includes(id) ? c.filter((x) => x !== id) : [...c.slice(-1), id]));

  const restore = async (s: Snapshot) => {
    try {
      const res = await documents.getSnapshot({ id: s.id });
      restores.current += 1;
      const group = `history:restore:${restores.current}`;
      const open = session.store.getState().docs;
      const applied = restoreRecipes(res.documents).filter(
        (e) => open[e.path] !== undefined && session.edit(e.path, e.recipe, { group }) !== undefined,
      );
      // Documents the plugin no longer has come back through the server (STU-069).
      const missing = missingPaths(res.documents, (p) => open[p] !== undefined);
      let brought = 0;
      if (missing.length > 0) {
        await documents.restoreSnapshot({ id: s.id, paths: [...missing] });
        for (const p of missing) {
          const back = await documents.getDocument({ appId, pluginId: plugin, path: p });
          if (back.document !== undefined) {
            session.open(toServerDocument(back.document));
            brought += 1;
          }
        }
      }
      const total = applied.length + brought;
      setNote(total === 0 ? m.restoredNothing : m.restored(total));
    } catch {
      setNote(m.restoreFailed);
    }
  };

  const changeLabel = { added: m.changeAdded, removed: m.changeRemoved, changed: m.changeChanged } as Record<
    string,
    string
  >;

  return (
    <>
      <Dialog
        open={open}
        onOpenChange={(o) => {
          if (!o) {
            onClose();
          }
        }}
        size="full"
        title={m.title}
        labels={{ close: m.close }}
      >
        <div className="flex min-h-0 flex-1 flex-col gap-3">
          {!canEdit && <p className="text-caption text-secondary">{m.readOnly}</p>}
          <span role="status" aria-live="polite" className="text-body text-secondary">
            {note}
          </span>
          {listing.state === "loading" && <p className="text-body text-secondary">{m.loading}</p>}
          {listing.state === "failed" && (
            <p role="alert" className="text-body">
              {m.failed}
            </p>
          )}
          {listing.state === "ready" && listing.rows.length === 0 && (
            <p className="text-body text-secondary">{m.empty}</p>
          )}
          <ul aria-label={m.list} className="flex max-h-64 flex-col gap-1 overflow-y-auto">
            {listing.rows.map((s) => (
              <li key={s.id} className="flex items-center gap-3 text-body">
                <input
                  type="checkbox"
                  aria-label={m.pickRow(whenOf(s))}
                  checked={chosen.includes(s.id)}
                  onChange={() => toggle(s.id)}
                />
                <span className="text-data">{whenOf(s)}</span>
                <span className="text-secondary">
                  {m.by(s.actor?.display === undefined || s.actor.display === "" ? m.unknownAuthor : s.actor.display)}
                </span>
                <TypeTag>{reasonOf(s.reason, m)}</TypeTag>
                {s.editorSessionId !== "" && (
                  <span className="text-data text-secondary" title={m.sessionTitle}>
                    {m.sessionTag(sessionTag(s.editorSessionId))}
                  </span>
                )}
                <Button
                  size="sm"
                  variant="secondary"
                  disabled={!canEdit}
                  aria-label={m.restoreRow(whenOf(s))}
                  onClick={() => setTarget(s)}
                >
                  {m.restore}
                </Button>
              </li>
            ))}
          </ul>
          {listing.next !== "" && (
            <Button variant="secondary" size="sm" onClick={() => void more()}>
              {m.more}
            </Button>
          )}
          {pair === undefined ? (
            <p className="text-body text-secondary">{m.compareHint}</p>
          ) : (
            <section aria-label={m.compareTitle} className="flex min-h-0 flex-1 flex-col gap-2">
              <h3 className="text-label font-medium">{m.compareTitle}</h3>
              {changes.length === 0 && diff.state !== "failed" && (
                <p className="text-body text-secondary">{m.noChanges}</p>
              )}
              {changes.length > 0 && (
                <div className="flex items-center gap-2">
                  <Select
                    label={m.document}
                    options={changes.map((c) => ({ value: c.path, label: c.path }))}
                    value={path}
                    onValueChange={setPath}
                  />
                  <TypeTag>{changeLabel[changes.find((c) => c.path === path)?.change ?? ""] ?? ""}</TypeTag>
                </div>
              )}
              {diff.state === "failed" && (
                <p role="alert" className="text-body">
                  {m.diffFailed}
                </p>
              )}
              <div className="min-h-0 flex-1">
                {diff.state === "ready" && (
                  <Suspense fallback={<p className="text-body text-secondary">{m.loading}</p>}>
                    <MonacoView
                      value={diff.newer}
                      baseline={diff.older}
                      diff
                      readOnly
                      label={m.diffLabel}
                      markers={[]}
                      onChange={() => undefined}
                    />
                  </Suspense>
                )}
              </div>
            </section>
          )}
        </div>
      </Dialog>
      <AlertDialog
        open={target !== null}
        onOpenChange={(o) => {
          if (!o) {
            setTarget(null);
          }
        }}
        title={m.confirmTitle}
        description={target === null ? "" : m.confirmBody(whenOf(target))}
        labels={{ cancel: m.cancel, confirm: m.confirm }}
        onConfirm={() => {
          if (target !== null) {
            void restore(target);
          }
          setTarget(null);
        }}
      />
    </>
  );
}
