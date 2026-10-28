// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { DocumentService } from "@plux/api";
import { limitDefault } from "@plux/schema";
import { useMemo } from "react";
import { useApp } from "../../../lib/use-app.tsx";
import type { EditSession } from "../../editor/edit-session.ts";
import type { JsonValue } from "../../editor/json-patch.ts";
import { sourceReader } from "./copy-over-io.ts";
import type { ClipboardInput } from "./use-clipboard.tsx";

/** The copy-over wiring of a plugin canvas: reads with the user's access, writes through the edit session (F7). */
export function useCopyOver(
  documents: readonly { readonly path: string }[] | undefined,
  session: EditSession,
): ClipboardInput["copyOver"] {
  const { session: app } = useApp();
  const client = useMemo(() => createClient(DocumentService, app.transport), [app.transport]);
  const manifest = documents?.find((d) => d.path.endsWith("plugin.json"))?.path;
  return useMemo(
    () =>
      manifest === undefined
        ? undefined
        : {
            prefix: manifest.slice(0, manifest.length - "plugin.json".length),
            room: () => Math.max(0, limitDefault("studio.changeSetDocuments") - session.waiting()),
            io: (from) => ({
              read: sourceReader(client, from),
              write: async (path: string, content: JsonValue) => (await session.create(path, content)).ok,
            }),
          },
    [manifest, client, session],
  );
}
