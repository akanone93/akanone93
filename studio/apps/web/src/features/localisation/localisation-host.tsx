// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import type { Document } from "@plux/api";
import { Dialog } from "@plux/ui";
import { useMemo } from "react";
import { SheetEmpty } from "../../components/editor-sheet.tsx";
import { useEditor } from "../editor/index.ts";
import type { JsonValue } from "../editor/json-patch.ts";
import { uuidv7 } from "../plugin-canvas/screens.ts";
import { keyUsage, readMessages, referencedKeys } from "./l10n-model.ts";
import { useL10nSessions } from "./l10n-sessions.tsx";
import { localisationMessages } from "./localisation.messages.ts";
import { LocalisationOverlay } from "./localisation-overlay.tsx";

export interface LocalisationHostProps {
  readonly open: boolean;
  readonly onClose: () => void;
  /** The plugin's documents as the server holds them; the plugin session's unsaved content is laid over them. */
  readonly documents: readonly Document[] | undefined;
  /** The whole app's documents, for the messages of the locales that are not being edited. */
  readonly appDocuments: readonly Document[] | undefined;
}

function parse(content: Uint8Array): unknown {
  try {
    return JSON.parse(new TextDecoder().decode(content));
  } catch {
    return undefined;
  }
}

/** The localisation table of the plugin canvas: keys of the plugin or the selection by locale (STU-063). */
export function LocalisationHost(props: LocalisationHostProps) {
  const sessions = useL10nSessions();
  const local = useEditor((st) => st.docs);
  const selected = useEditor((st) => st.selection.ids);
  const { pluginKeys, selectionKeys } = useMemo(() => {
    const picked = new Set(selected);
    const plugin = new Set<string>();
    const selection = new Set<string>();
    for (const d of props.documents ?? []) {
      const content = local[d.path]?.content ?? parse(d.content);
      for (const r of referencedKeys(content)) {
        plugin.add(r.id);
        if (picked.has(r.nodeId)) {
          selection.add(r.id);
        }
      }
    }
    return { pluginKeys: plugin, selectionKeys: selection };
  }, [props.documents, local, selected]);
  // Every document of the app, this plugin's with its unsaved edits, so that a key used anywhere is kept (STU-072).
  const usage = useMemo(
    () =>
      keyUsage(
        (props.appDocuments ?? []).map((d) => ({ path: d.path, content: local[d.path]?.content ?? parse(d.content) })),
      ),
    [props.appDocuments, local],
  );
  const others = useMemo(() => {
    const out = new Map<string, Readonly<Record<string, string>>>();
    for (const d of props.appDocuments ?? []) {
      if (d.pluginId === "" && d.path.startsWith("translations/") && d.path !== "translations/keys.json") {
        out.set(d.path.slice("translations/".length, -".json".length), readMessages(parse(d.content)));
      }
    }
    return out;
  }, [props.appDocuments]);
  const schemaVersion = useMemo(() => {
    const app = props.appDocuments?.find((d) => d.pluginId === "" && d.path === "app.json");
    const json = app === undefined ? undefined : parse(app.content);
    const v =
      typeof json === "object" && json !== null ? (json as Record<string, unknown>)["schemaVersion"] : undefined;
    return (v ?? 1) as JsonValue;
  }, [props.appDocuments]);
  if (sessions === null) {
    // Without the app's languages there is nothing to edit; the editor still opens and says why.
    return <NotReady open={props.open} onClose={props.onClose} />;
  }
  return (
    <LocalisationOverlay
      open={props.open}
      onClose={props.onClose}
      locales={sessions.locales}
      locale={sessions.locale}
      onLocale={sessions.setLocale}
      keysSession={sessions.keys}
      messagesSession={sessions.messages}
      canEditKeys={sessions.canEditKeys}
      canEditMessages={sessions.canEditMessages}
      otherMessages={others}
      pluginKeys={pluginKeys}
      selectionKeys={selectionKeys}
      usage={usage}
      schemaVersion={schemaVersion}
      newId={uuidv7}
    />
  );
}

/** The translations editor before the app's languages are loaded: it opens and says what is missing. */
function NotReady({ open, onClose }: { readonly open: boolean; readonly onClose: () => void }) {
  const m = localisationMessages(useLingui().i18n);
  return (
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
      icon="translate"
    >
      <SheetEmpty icon="translate" title={m.notReady} />
    </Dialog>
  );
}
