// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { Document } from "@plux/api";
import { useMemo } from "react";
import type { JsonValue } from "../editor/json-patch.ts";
import { type LoadedDoc, loadedDocs } from "../safety/index.ts";

export interface StateTarget {
  /** The app's loaded documents, for the where-used index. */
  readonly loaded: readonly LoadedDoc[];
  /** The plugin document the state entries live in. */
  readonly pluginPath: string | undefined;
  /** The app's schema version, for documents created beside it. */
  readonly schemaVersion: JsonValue;
}

/** What the state designer and the localisation overlay need from the loaded documents. */
export function useStateTarget(
  plugin: readonly Document[] | undefined,
  app: readonly Document[] | undefined,
): StateTarget {
  const loaded = useMemo(() => loadedDocs(app ?? []), [app]);
  const pluginPath = useMemo(() => plugin?.find((d) => d.kind === "plugin")?.path, [plugin]);
  const schemaVersion = useMemo(() => {
    const json = loaded.find((d) => d.kind === "app")?.json;
    const v =
      typeof json === "object" && json !== null ? (json as Record<string, unknown>)["schemaVersion"] : undefined;
    return (v ?? 1) as JsonValue;
  }, [loaded]);
  return { loaded, pluginPath, schemaVersion };
}
