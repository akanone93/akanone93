// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { timestampDate } from "@bufbuild/protobuf/wkt";
import type { Client } from "@connectrpc/connect";
import type { ReleaseService } from "@plux/api";

/** Who published a plugin's newest version, and when (STU-031). */
export interface Publisher {
  readonly by: string;
  readonly at: Date | undefined;
}

/** The publisher of the highest published version of the plugin, or undefined when none was published. */
export async function latestPublisher(
  releases: Client<typeof ReleaseService>,
  pluginId: string,
): Promise<Publisher | undefined> {
  let latest: (Publisher & { version: bigint }) | undefined;
  let token = "";
  do {
    const page = await releases.listPluginVersions({ pluginId, page: { pageSize: 100, pageToken: token } });
    for (const v of page.versions) {
      if (latest === undefined || v.version > latest.version) {
        latest = {
          version: v.version,
          by: v.publishedBy?.display ?? "",
          at: v.createdAt === undefined ? undefined : timestampDate(v.createdAt),
        };
      }
    }
    token = page.page?.nextPageToken ?? "";
  } while (token !== "");
  return latest === undefined ? undefined : { by: latest.by, at: latest.at };
}
