// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { type Client, createClient } from "@connectrpc/connect";
import {
  type BuildRef,
  type DocumentChange,
  DocumentService,
  type PluginVersion,
  PublishService,
  type ReleaseRequest,
  ReleaseService,
  Severity,
} from "@plux/api";
import { useQuery } from "@tanstack/react-query";
import { useApp } from "../../lib/use-app.tsx";

/**
 * The accessibility diagnostics of the error registry (A11Y-002 to A11Y-005, A11Y-008): PLX-1401 to PLX-1411 and the
 * code that blocks a publish (PLX-3060). The publish job reports them with its other diagnostics.
 */
export function isAccessibilityCode(code: string): boolean {
  const number = /^PLX-(\d{4})$/.exec(code)?.[1];
  if (number === undefined) {
    return false;
  }
  const n = Number(number);
  return (n >= 1401 && n <= 1411) || n === 3060;
}

/** What the reports and diffs steps show about one built plugin, read from the APIs that already hold it. */
export interface PluginEvidence {
  readonly pluginKey: string;
  /** Size of the built bundle, from the plugin version. */
  readonly size: bigint | undefined;
  /** Size of the version before it, when there is one. */
  readonly previousSize: bigint | undefined;
  /** The accessibility diagnostics of the publish job. */
  readonly accessibility: readonly { code: string; message: string; warning: boolean }[];
  /** The documents that changed against the previous version; undefined when the plugin has none (a first publish). */
  readonly changes: readonly DocumentChange[] | undefined;
}

async function versionsOf(releases: Client<typeof ReleaseService>, pluginId: string): Promise<PluginVersion[]> {
  const out: PluginVersion[] = [];
  let token = "";
  do {
    const page = await releases.listPluginVersions({ pluginId, page: { pageSize: 100, pageToken: token } });
    out.push(...page.versions);
    token = page.page?.nextPageToken ?? "";
  } while (token !== "");
  return out;
}

async function evidenceOf(
  clients: {
    readonly releases: Client<typeof ReleaseService>;
    readonly documents: Client<typeof DocumentService>;
    readonly publish: Client<typeof PublishService>;
  },
  build: BuildRef,
  pluginId: string,
): Promise<PluginEvidence> {
  const [versions, job] = await Promise.all([
    pluginId === "" ? Promise.resolve([]) : versionsOf(clients.releases, pluginId),
    build.jobId === "" ? Promise.resolve(undefined) : clients.publish.getPublishJob({ jobId: build.jobId }),
  ]);
  const built = versions.find((v) => v.version === build.version);
  const previous = versions
    .filter((v) => v.version < build.version)
    .reduce<PluginVersion | undefined>(
      (best, v) => (best === undefined || v.version > best.version ? v : best),
      undefined,
    );
  const comparable =
    built !== undefined && previous !== undefined && built.sourceSnapshotId !== "" && previous.sourceSnapshotId !== "";
  const changes = comparable
    ? (await clients.documents.compareSnapshots({ fromId: previous.sourceSnapshotId, toId: built.sourceSnapshotId }))
        .changes
    : undefined;
  return {
    pluginKey: build.pluginKey,
    size: built?.bundleSize,
    previousSize: previous?.bundleSize,
    accessibility: (job?.job?.diagnostics ?? [])
      .filter((d) => isAccessibilityCode(d.code))
      .map((d) => ({ code: d.code, message: d.message, warning: d.severity !== Severity.ERROR })),
    changes,
  };
}

/** The size, accessibility and document-change evidence of the builds of a request; it waits until the builds exist. */
export function useEvidence(request: ReleaseRequest, pluginIds: ReadonlyMap<string, string>) {
  const { session } = useApp();
  return useQuery({
    queryKey: ["publish-evidence", request.id, request.contentHash],
    enabled: request.builds.length > 0 && pluginIds.size > 0,
    queryFn: () => {
      const clients = {
        releases: createClient(ReleaseService, session.transport),
        documents: createClient(DocumentService, session.transport),
        publish: createClient(PublishService, session.transport),
      };
      return Promise.all(request.builds.map((b) => evidenceOf(clients, b, pluginIds.get(b.pluginKey) ?? "")));
    },
  });
}
