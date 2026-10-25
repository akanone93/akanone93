// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { z } from "zod";

declare const __STUDIO_VERSION__: string | undefined;

/** The BFF's `GET /version` answer; `docs` is present only when the operator configured a docs site. */
const versionSchema = z.object({ studio: z.string(), docs: z.url().optional() });

export type ServerVersion = z.infer<typeof versionSchema>;

/** The version of this build, injected by Vite from package.json; empty outside a build (tests). */
export const loadedStudioVersion: string = typeof __STUDIO_VERSION__ === "string" ? __STUDIO_VERSION__ : "";

/** Reads `/version`; undefined when the BFF is unreachable or answers something unexpected. */
export async function fetchServerVersion(fetcher: typeof fetch = fetch): Promise<ServerVersion | undefined> {
  try {
    const res = await fetcher("/version", { cache: "no-store" });
    if (!res.ok) {
      return undefined;
    }
    const parsed = versionSchema.safeParse(await res.json());
    return parsed.success ? parsed.data : undefined;
  } catch {
    return undefined;
  }
}
