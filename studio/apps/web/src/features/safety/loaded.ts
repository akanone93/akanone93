// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { Document } from "@plux/api";
import type { LoadedDoc } from "./where-used.ts";

/** The documents of an app as the where-used index and the change sets read them; unparsable ones are skipped. */
export function loadedDocs(documents: readonly Document[]): LoadedDoc[] {
  const decoder = new TextDecoder();
  return documents.flatMap((d) => {
    try {
      const json: unknown = JSON.parse(decoder.decode(d.content));
      return [{ pluginId: d.pluginId, path: d.path, kind: d.kind, revision: Number(d.revision), json }];
    } catch {
      return [];
    }
  });
}

/** A plugin of the app, from its plugin document, for pickers. */
export interface PluginRef {
  readonly id: string;
  readonly label: string;
}

export function pluginRefs(docs: readonly LoadedDoc[]): PluginRef[] {
  return docs
    .filter((d) => d.kind === "plugin" && d.pluginId !== "")
    .map((d) => {
      const j = d.json !== null && typeof d.json === "object" ? (d.json as Record<string, unknown>) : {};
      const name = typeof j["name"] === "string" ? j["name"] : typeof j["key"] === "string" ? j["key"] : d.pluginId;
      return { id: d.pluginId, label: name };
    });
}
