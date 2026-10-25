// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { AlertDialog, Button, Dialog } from "@plux/ui";
import { type ReactElement, useEffect, useState } from "react";
import { useStore } from "zustand";
import type { EditSession } from "./edit-session.ts";
import { lockMessages } from "./lock.messages.ts";
import type { LockController } from "./lock-controller.ts";

/** Re-renders when "Take over" becomes possible (L-1). */
function useTakeoverOpen(lock: LockController, now: () => number): boolean {
  const state = useStore(lock.store);
  const [, tick] = useState(0);
  const enabled = lock.canTakeOver();
  const at = state.lock?.takeoverAllowedAt?.getTime();
  useEffect(() => {
    if (enabled || at === undefined || state.status !== "held-by-other") {
      return;
    }
    const handle = setTimeout(() => tick((n) => n + 1), Math.max(0, at - now()));
    return () => clearTimeout(handle);
  }, [enabled, at, state.status, now]);
  return enabled;
}

/**
 * The read-only banner (F1.6) with "Request edit" and "Take over" (D36), and the holder's notice of
 * requests. Editing controls stay visible but disabled while it shows.
 */
export function LockBanner({
  lock,
  canEdit = true,
  canOverride = false,
  now = Date.now,
}: {
  readonly lock: LockController;
  /** Whether this user may edit at all; viewers get no "Request edit". */
  readonly canEdit?: boolean;
  /** Holds plugin.lock.override: only then is "Take over" (D36) shown. */
  readonly canOverride?: boolean;
  readonly now?: () => number;
}): ReactElement | null {
  const m = lockMessages(useLingui().i18n);
  const state = useStore(lock.store);
  const open = useTakeoverOpen(lock, now);
  const [confirm, setConfirm] = useState(false);
  if (state.status === "held" && state.requests.length > 0) {
    return (
      <div
        role="status"
        className="flex flex-wrap items-center gap-2 border-b border-subtle bg-surface-raised px-4 py-2 text-body"
      >
        <span className="grow">{m.requestsBody(state.requests.join(", "))}</span>
        <Button variant="ghost" onClick={() => lock.keep()}>
          {m.keep}
        </Button>
        <Button variant="secondary" onClick={() => void lock.release()}>
          {m.release}
        </Button>
      </div>
    );
  }
  if (state.status === "other-tab") {
    return (
      <div role="status" className="border-b border-subtle bg-surface-raised px-4 py-2 text-body">
        {m.otherTab}
      </div>
    );
  }
  if (state.status !== "held-by-other" || state.lock === null) {
    return null;
  }
  const held = state.lock;
  return (
    <div
      role="status"
      className="flex flex-wrap items-center gap-2 border-b border-subtle bg-surface-raised px-4 py-2 text-body"
    >
      <span className="grow">{m.heldBy(held.holder, held.acquiredAt)}</span>
      {state.requested && <span className="text-secondary">{m.requested(held.holder)}</span>}
      {canOverride && !open && <span className="text-secondary">{m.takeOverFrom(held.takeoverAllowedAt)}</span>}
      {state.takeoverRefused && <span className="text-secondary">{m.takeoverRefused}</span>}
      {canEdit && (
        <Button variant="secondary" onClick={() => void lock.request()}>
          {m.requestEdit}
        </Button>
      )}
      {canEdit && canOverride && (
        <Button variant="primary" disabled={!open} onClick={() => setConfirm(true)}>
          {m.takeOver}
        </Button>
      )}
      <AlertDialog
        open={confirm}
        onOpenChange={setConfirm}
        title={m.takeoverTitle}
        description={m.takeoverBody(held.holder, held.lastEditAt)}
        labels={{ cancel: m.cancel, confirm: m.takeOver }}
        onConfirm={() => {
          setConfirm(false);
          void lock.takeOver();
        }}
      />
    </div>
  );
}

/** D37: the lock was lost; the unsent changes stay in memory until re-taken, compared or discarded (F1.4). */
export function LockLostDialog({
  lock,
  session,
}: {
  readonly lock: LockController;
  readonly session: EditSession;
}): ReactElement {
  const m = lockMessages(useLingui().i18n);
  const state = useStore(lock.store);
  const docs = useStore(session.store, (s) => s.docs);
  const [comparing, setComparing] = useState(false);
  const unsent = Object.entries(docs).filter(([, d]) => d.waiting > 0);
  const count = unsent.reduce((n, [, d]) => n + d.waiting, 0);
  return (
    <Dialog
      open={state.status === "lost"}
      title={m.lostTitle}
      description={state.lost === "takeover" ? m.lostTakeover(state.lock?.holder ?? "") : m.lostExpired}
      labels={{ close: m.close }}
      disablePointerDismissal
    >
      <p className="text-body">{m.unsaved(count)}</p>
      {state.retakeFailed && (
        <p role="alert" className="mt-2 text-body text-secondary">
          {m.retakeFailed}
        </p>
      )}
      {comparing && (
        <ul className="mt-2 max-h-40 overflow-auto font-mono text-body" dir="ltr">
          {unsent.map(([path, d]) => (
            <li key={path}>
              {path} ({d.waiting})
            </li>
          ))}
        </ul>
      )}
      <div className="mt-4 flex flex-wrap justify-end gap-2">
        <Button variant="ghost" onClick={() => void lock.discard()}>
          {m.discard}
        </Button>
        <Button variant="secondary" onClick={() => setComparing((c) => !c)}>
          {m.compare}
        </Button>
        <Button variant="primary" disabled={state.lock !== null} onClick={() => void lock.retake()}>
          {m.retake}
        </Button>
      </div>
    </Dialog>
  );
}
