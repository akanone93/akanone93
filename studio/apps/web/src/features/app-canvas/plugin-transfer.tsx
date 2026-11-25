// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { AppProjectService, type ImportPluginResponse } from "@plux/api";
import { Button, Dialog, Field, Input } from "@plux/ui";
import { type ReactNode, useMemo, useRef, useState } from "react";
import { plxCode, withReason } from "../../components/refusal.tsx";
import { useApp } from "../../lib/use-app.tsx";
import { download } from "../admin/download.ts";
import { uuidv7 } from "../plugin-canvas/screens.ts";
import { appCanvasMessages } from "./app-canvas.messages.ts";
import { pluginKeyFrom } from "./app-canvas.model.ts";

/**
 * A plugin to and from a .plux archive (build log row 264): the export carries the plugin's documents with the
 * translation keys, messages and assets they use, and the import adds them to this app beside what it has. An archive
 * whose plugin key the app uses already asks for another name, and is sent again under it; the server checks every
 * file before anything is written.
 */

/** The refusal of a plugin key the app uses (Appendix F, PLX-8032). */
const KEY_TAKEN = "PLX-8032";

/** What an import did, in one line: what it added and the problems left to fix. */
export function importedText(m: ReturnType<typeof appCanvasMessages>, res: ImportPluginResponse): string {
  const added = m.pluginImported(res.plugin?.name ?? "", res.translationKeys, res.assets);
  return res.errorDiagnostics > 0 ? `${added} ${m.pluginImportProblems(res.errorDiagnostics)}` : added;
}

export interface PluginTransfer {
  /** Downloads one plugin as a .plux archive. */
  readonly exportPlugin: (pluginId: string) => void;
  /** Opens the file picker of an import. */
  readonly openImport: () => void;
  /** The hidden file input and the name dialog of a taken key; rendered once by the page. */
  readonly ui: ReactNode;
}

export function usePluginTransfer(props: {
  readonly appId: string;
  /** Every plugin key of the app, for the key of a renamed import. */
  readonly taken: ReadonlySet<string>;
  readonly onImported: () => Promise<unknown>;
  /** A line for the canvas's message strip; null clears it. */
  readonly onMessage: (text: string | null) => void;
}): PluginTransfer {
  const { i18n } = useLingui();
  const m = appCanvasMessages(i18n);
  const { session } = useApp();
  const client = useMemo(() => createClient(AppProjectService, session.transport), [session.transport]);
  const input = useRef<HTMLInputElement>(null);
  // The archive of an import refused for its key, waiting for another name.
  const [renaming, setRenaming] = useState<Uint8Array<ArrayBuffer> | null>(null);
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);

  const send = async (archive: Uint8Array<ArrayBuffer>, as: string) => {
    setBusy(true);
    try {
      const key = as === "" ? "" : pluginKeyFrom(as, props.taken);
      // A session of its own: the import takes the locks it needs and gives them back (SRV-040).
      const res = await client.importPlugin({ appId: props.appId, archive, key, name: as, session: uuidv7() });
      setRenaming(null);
      props.onMessage(importedText(m, res));
      await props.onImported();
    } catch (error) {
      if (plxCode(error) === KEY_TAKEN) {
        setName("");
        setRenaming(archive);
      } else {
        setRenaming(null);
        props.onMessage(withReason(m.pluginImportFailed, error));
      }
    } finally {
      setBusy(false);
    }
  };

  const exportPlugin = (pluginId: string) => {
    client
      .exportPlugin({ pluginId })
      .then((res) => download(res.filename, new Uint8Array(res.archive), "application/zip"))
      .catch((error: unknown) => props.onMessage(withReason(m.pluginExportFailed, error)));
  };

  const ui = (
    <>
      <input
        ref={input}
        type="file"
        accept=".plux"
        hidden
        aria-label={m.importPlugin}
        onChange={(e) => {
          const file = e.target.files?.[0];
          e.target.value = "";
          if (file !== undefined) {
            void file.arrayBuffer().then((bytes) => send(new Uint8Array(bytes), ""));
          }
        }}
      />
      {renaming !== null && (
        <Dialog
          open
          onOpenChange={(o) => !o && setRenaming(null)}
          title={m.importPlugin}
          size="sm"
          labels={{ close: m.close }}
        >
          <p className="text-sm text-muted-foreground">{m.pluginImportKeyTaken}</p>
          <Field label={m.name}>
            <Input value={name} onChange={(e) => setName(e.target.value)} />
          </Field>
          <div className="flex justify-end gap-2">
            <Button onClick={() => setRenaming(null)}>{m.cancel}</Button>
            <Button disabled={busy || name.trim() === ""} onClick={() => void send(renaming, name.trim())}>
              {m.importPlugin}
            </Button>
          </div>
        </Dialog>
      )}
    </>
  );
  return { exportPlugin, openImport: () => input.current?.click(), ui };
}
