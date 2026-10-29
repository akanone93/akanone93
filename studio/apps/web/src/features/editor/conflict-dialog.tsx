// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button, Dialog } from "@plux/ui";
import { type ReactElement, useState } from "react";
import { useStore } from "zustand";
import type { EditSession } from "./edit-session.ts";
import { editorMessages } from "./editor.messages.ts";

function PathList({
  title,
  paths,
}: {
  readonly title: string;
  readonly paths: readonly string[];
}): ReactElement | null {
  if (paths.length === 0) {
    return null;
  }
  return (
    <section className="mt-3">
      <h3 className="text-label text-secondary">{title}</h3>
      <ul className="mt-1 max-h-40 overflow-auto font-mono text-body" dir="ltr">
        {paths.map((p) => (
          <li key={p}>{p}</li>
        ))}
      </ul>
    </section>
  );
}

/**
 * D35: the autosave conflict (decisions F2). Autosave stays paused until the user keeps theirs
 * (what fits is applied, what does not is listed), takes the server's, or copies theirs first.
 */
export function ConflictDialog({ session }: { readonly session: EditSession }): ReactElement {
  const m = editorMessages(useLingui().i18n);
  const conflict = useStore(session.store, (s) => s.conflict);
  const [dropped, setDropped] = useState<readonly string[]>([]);
  const [copied, setCopied] = useState(false);
  return (
    <>
      <Dialog
        open={conflict !== null}
        title={m.conflictTitle}
        description={m.conflictBody}
        size="lg"
        labels={{ close: m.close }}
        disablePointerDismissal
      >
        {conflict !== null && (
          <>
            <PathList title={m.conflictDifferences} paths={conflict.differences} />
            <PathList title={m.conflictWouldDrop} paths={conflict.wouldDrop} />
            {copied && (
              <p role="status" className="mt-3 text-body text-secondary">
                {m.copied}
              </p>
            )}
            <div className="mt-4 flex flex-wrap justify-end gap-2">
              <Button
                variant="ghost"
                onClick={() => {
                  void navigator.clipboard
                    .writeText(JSON.stringify(conflict.mine, null, 2))
                    .then(() => setCopied(true));
                }}
              >
                {m.copyMine}
              </Button>
              <Button
                variant="secondary"
                onClick={() => {
                  setCopied(false);
                  session.takeServer();
                }}
              >
                {m.takeServer}
              </Button>
              <Button
                variant="primary"
                onClick={() => {
                  setCopied(false);
                  setDropped(session.keepMine());
                }}
              >
                {m.keepMine}
              </Button>
            </div>
          </>
        )}
      </Dialog>
      <Dialog
        open={dropped.length > 0}
        title={m.conflictDropped}
        labels={{ close: m.close }}
        onOpenChange={(open) => {
          if (!open) {
            setDropped([]);
          }
        }}
      >
        <PathList title={m.conflictDropped} paths={dropped} />
      </Dialog>
    </>
  );
}
