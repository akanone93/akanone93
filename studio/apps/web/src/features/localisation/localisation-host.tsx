// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { Document } from "@plux/api";
import { useMemo } from "react";
import { useEditor } from "../editor/index.ts";
import type { JsonValue } from "../editor/json-patch.ts";
import { uuidv7 } from "../plugin-canvas/screens.ts";
import { readMessages, referencedKeys } from "./l10n-model.ts";
import { useL10nSessions } from "./l10n-sessions.tsx";
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
    return null;
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
      schemaVersion={schemaVersion}
      newId={uuidv7}
    />
  );
}
