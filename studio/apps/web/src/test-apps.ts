// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { create } from "@bufbuild/protobuf";
import { timestampFromMs } from "@bufbuild/protobuf/wkt";
import { Code, ConnectError } from "@connectrpc/connect";
import {
  type App,
  AppLimitUsageSchema,
  AppSchema,
  type AppSummary,
  AppSummarySchema,
  EnvironmentReleaseSchema,
  type LimitUsage,
  LimitUsageSchema,
  type ListAppSummariesRequest,
  type ListAppsRequest,
  type UpdateAppRequest,
} from "@plux/api";

/** One app of the apps grid fixture with the facts its summary reports (STU-020 to STU-025). */
export interface AppsGridApp {
  id: string;
  key: string;
  name: string;
  description?: string;
  owningTeamId?: string;
  tags?: string[];
  archived?: boolean;
  iconAssetId?: string;
  /** Epoch seconds of the newest release; absent means never published. */
  lastPublishedAt?: number;
  /** Who created the newest release. */
  lastPublisher?: { kind: string; id: string; display: string };
  releases?: { environmentKey: string; liveSequence: number }[];
  activeDevices?: number;
  pluginCount?: number;
  crashFreeRate?: number;
  crashFreeTrend?: number;
  limits?: { key: string; used: number; limit: number }[];
}

/** The apps grid and the app settings (STU-020 to STU-025, LIM-005): the apps, and what the upstream received. */
export interface AppsGridFixture {
  apps: AppsGridApp[];
  /** The IDs `ListApps` returns when asked for the caller's own apps. */
  mine?: string[];
  /** The most apps one `ListAppSummaries` call returns, as `app.summariesPerCall` caps it. */
  perCall?: number;
  /** The limits `ListAppLimits` returns. */
  limits?: { key: string; unit: string; value: number; effective: number; hardMax?: number; warning?: boolean }[];
  /** The plugins `ListPlugins` returns, and the limits `ListPluginLimits` returns per plugin ID; a plugin without an entry is not found. */
  plugins?: { id: string; key: string; name: string }[];
  pluginLimits?: Record<string, { key: string; unit: string; value: number; effective: number; warning?: boolean }[]>;
  /** The assets `ListAssets` returns per app ID: name, ID, media type and URL. */
  assets?: Record<string, { id: string; name: string; mediaType: string; url: string }[]>;
  /** Every call received: `summaries:<limit>:<afterKey>`, `archive:<id>:<on|off>`, `update:<id>:<name>:<description>`, `export:<id>`, `duplicate:<id>:<key>:<name>`, `listApps:<status>:<mine>`. */
  calls: string[];
}

/** The App message of a fixture app. */
export function gridApp(a: AppsGridApp): App {
  return create(AppSchema, {
    id: a.id,
    organizationId: "",
    key: a.key,
    name: a.name,
    description: a.description ?? "",
    owningTeamId: a.owningTeamId ?? "",
    tags: a.tags ?? [],
    archived: a.archived === true,
    iconAssetId: a.iconAssetId ?? "",
    defaultPluginKey: "main",
  });
}

function summaryOf(a: AppsGridApp): AppSummary {
  return create(AppSummarySchema, {
    app: gridApp(a),
    releases: (a.releases ?? []).map((r) =>
      create(EnvironmentReleaseSchema, { environmentKey: r.environmentKey, liveSequence: BigInt(r.liveSequence) }),
    ),
    ...(a.lastPublishedAt === undefined ? {} : { lastPublishedAt: timestampFromMs(a.lastPublishedAt * 1000) }),
    ...(a.lastPublisher === undefined ? {} : { lastPublisher: a.lastPublisher }),
    activeDevices: BigInt(a.activeDevices ?? 0),
    pluginCount: BigInt(a.pluginCount ?? 0),
    crashFreeRate: a.crashFreeRate ?? 1,
    crashFreeTrend: a.crashFreeTrend ?? 0,
    limits: (a.limits ?? []).map((l) =>
      create(AppLimitUsageSchema, { key: l.key, used: BigInt(l.used), limit: BigInt(l.limit) }),
    ),
  });
}

/** The handlers of the apps grid: summaries in bounded calls, the status and "mine" filters of ListApps, the archive switch. */
export function createAppsGrid(fixture: AppsGridFixture) {
  const byKey = (a: AppsGridApp, b: AppsGridApp) => a.key.localeCompare(b.key);
  return {
    listAppSummaries(req: ListAppSummariesRequest) {
      fixture.calls.push(`summaries:${req.limit}:${req.afterKey}`);
      const size = Math.min(req.limit > 0 ? req.limit : Number.MAX_SAFE_INTEGER, fixture.perCall ?? req.limit);
      const rest = fixture.apps.filter((a) => a.archived !== true && a.key > req.afterKey).sort(byKey);
      const page = rest.slice(0, size);
      return {
        summaries: page.map(summaryOf),
        nextAfterKey: rest.length > size ? (page[page.length - 1]?.key ?? "") : "",
      };
    },
    listApps(req: ListAppsRequest) {
      fixture.calls.push(`listApps:${req.status || "active"}:${req.mine}`);
      const mine = new Set(fixture.mine ?? []);
      const apps = fixture.apps
        .filter((a) => (req.status === "archived" ? a.archived === true : req.status === "all" || a.archived !== true))
        .filter((a) => !req.mine || mine.has(a.id))
        .sort(byKey);
      return { apps: apps.map(gridApp) };
    },
    getApp(id: string) {
      const found = fixture.apps.find((a) => a.id === id);
      return found === undefined ? undefined : { app: gridApp(found) };
    },
    updateApp(req: UpdateAppRequest) {
      const found = fixture.apps.find((a) => a.id === req.id);
      if (found === undefined) {
        return undefined;
      }
      if (req.metadata?.archived !== undefined) {
        fixture.calls.push(`archive:${req.id}:${req.metadata.archived ? "on" : "off"}`);
        found.archived = req.metadata.archived;
      }
      if (req.metadata?.description !== undefined) {
        found.description = req.metadata.description;
      }
      if (req.metadata?.archived === undefined) {
        fixture.calls.push(`update:${req.id}:${req.name}:${found.description ?? ""}`);
      }
      found.name = req.name;
      return { app: gridApp(found) };
    },
    listPluginLimits(pluginId: string) {
      const rows = fixture.pluginLimits?.[pluginId];
      if (rows === undefined) {
        throw new ConnectError("plugin not found", Code.NotFound);
      }
      fixture.calls.push(`pluginLimits:${pluginId}`);
      return {
        limits: rows.map(
          (l): LimitUsage =>
            create(LimitUsageSchema, {
              key: l.key,
              unit: l.unit,
              scope: "plugin",
              value: BigInt(l.value),
              effective: BigInt(l.effective),
              warning: l.warning === true,
            }),
        ),
      };
    },
    listAppLimits() {
      return {
        limits: (fixture.limits ?? []).map(
          (l): LimitUsage =>
            create(LimitUsageSchema, {
              key: l.key,
              unit: l.unit,
              scope: "app",
              value: BigInt(l.value),
              effective: BigInt(l.effective),
              hardMax: BigInt(l.hardMax ?? 0),
              warning: l.warning === true,
            }),
        ),
      };
    },
  };
}
