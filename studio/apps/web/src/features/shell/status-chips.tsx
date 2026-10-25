// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { StatusPill } from "@plux/ui";
import { type ReactElement, useEffect, useState } from "react";
import { editorMessages } from "../editor/editor.messages.ts";
import { notifyMessages } from "./notify.messages.ts";
import type { StreamState } from "./studio-events.ts";

/** Tracks navigator.onLine. */
export function useOnline(): boolean {
  const [online, setOnline] = useState(() => navigator.onLine);
  useEffect(() => {
    const up = () => setOnline(true);
    const down = () => setOnline(false);
    window.addEventListener("online", up);
    window.addEventListener("offline", down);
    return () => {
      window.removeEventListener("online", up);
      window.removeEventListener("offline", down);
    };
  }, []);
  return online;
}

/**
 * E13: offline wins over reconnecting, then edits still waiting to be saved (decisions F4); nothing
 * shows while all is well.
 */
export function StatusChips({
  online,
  stream,
  waiting = 0,
}: {
  readonly online: boolean;
  readonly stream: StreamState;
  readonly waiting?: number;
}): ReactElement | null {
  const { i18n } = useLingui();
  const m = notifyMessages(i18n);
  const e = editorMessages(i18n);
  if (!online) {
    return <StatusPill status="danger" label={waiting > 0 ? e.offlineWaiting(waiting) : m.chipOffline} />;
  }
  if (stream === "reconnecting") {
    return <StatusPill status="warning" label={m.chipReconnecting} />;
  }
  if (waiting > 0) {
    return <StatusPill status="warning" label={e.waiting(waiting)} />;
  }
  return null;
}

/** E14: shown when the server's Studio version differs from the loaded build. */
export function UpdateBanner({
  loadedVersion,
  serverVersion,
}: {
  readonly loadedVersion: string;
  readonly serverVersion: string | undefined;
}): ReactElement | null {
  const m = notifyMessages(useLingui().i18n);
  if (serverVersion === undefined || serverVersion === "" || serverVersion === loadedVersion) {
    return null;
  }
  return (
    <div role="status" className="flex items-center gap-3 bg-warning-surface px-4 py-2 text-body text-warning">
      <span className="flex-1">{m.updatedBanner}</span>
      <button type="button" className="underline" onClick={() => window.location.reload()}>
        {m.reload}
      </button>
    </div>
  );
}
