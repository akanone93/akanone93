// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { create, type MessageInitShape } from "@bufbuild/protobuf";
import { timestampFromMs } from "@bufbuild/protobuf/wkt";
import { type App, AppSchema, AppSummarySchema } from "@plux/api";
import {
  type AppEntry,
  filterApps,
  formatAmount,
  healthOf,
  levelOf,
  matchesQuery,
  monogramOf,
  monogramTone,
  ratioOf,
  sortApps,
  tagsOf,
} from "./apps-model.ts";
import { pickVisual } from "./card-visual.tsx";

const app = (over: { id: string; name: string; description?: string; tags?: string[]; owningTeamId?: string }): App =>
  create(AppSchema, { key: over.id, ...over });
const entry = (a: App, summary: MessageInitShape<typeof AppSummarySchema> = {}): AppEntry => ({
  app: a,
  summary: create(AppSummarySchema, { app: a, ...summary }),
});
const words = { operations: "operations", codepoints: "characters", microseconds: "µs", milliseconds: "ms" };

describe("the card visual [STU-021]", () => {
  test("the monogram is the name's first character, upper case, on a colour that stays with the app", () => {
    expect(monogramOf(app({ id: "a", name: "  bank" }))).toBe("B");
    expect(monogramOf(app({ id: "a", name: "Édith" }))).toBe("É");
    expect(monogramTone(app({ id: "a", name: "x" }))).toBe(monogramTone(app({ id: "a", name: "y" })));
    const tones = new Set(["a", "b", "c", "d", "e", "f", "g", "h"].map((id) => monogramTone(app({ id, name: id }))));
    expect(tones.size).toBeGreaterThan(1);
  });

  test("a screenshot of the canvas comes before the uploaded image", () => {
    const asset = (id: string, name: string, url: string) =>
      ({ id, name, variants: [{ artifact: { urls: [url] } }] }) as unknown as Parameters<typeof pickVisual>[1][number];
    const shot = asset("s", "thumbnails/screen-1.png", "/shot.png");
    const icon = asset("i", "icon.png", "/icon.png");
    expect(pickVisual({ iconAssetId: "i" }, [icon, shot])).toEqual({ kind: "screenshot", url: "/shot.png" });
    expect(pickVisual({ iconAssetId: "i" }, [icon])).toEqual({ kind: "image", url: "/icon.png" });
    expect(pickVisual({ iconAssetId: "" }, [icon])).toBeUndefined();
    expect(pickVisual({ iconAssetId: "i" }, [])).toBeUndefined();
  });
});

describe("search, filters and sort [STU-023]", () => {
  const shop = entry(app({ id: "1", name: "Shop", description: "Sells shoes", tags: ["retail"], owningTeamId: "t1" }), {
    crashFreeRate: 1,
  });
  const bank = entry(app({ id: "2", name: "Bank", tags: ["finance", "retail"], owningTeamId: "t2" }), {
    crashFreeRate: 0.9,
    lastPublishedAt: timestampFromMs(2_000),
  });
  const chat = entry(app({ id: "3", name: "Chat" }), { crashFreeRate: 0.99, lastPublishedAt: timestampFromMs(9_000) });
  const none = { q: "", team: "", tag: "", mine: undefined };

  test("the search matches name, key, description and tags, ignoring case", () => {
    expect(matchesQuery(shop.app, "SHOES")).toBe(true);
    expect(matchesQuery(shop.app, "retail")).toBe(true);
    expect(matchesQuery(shop.app, "bank")).toBe(false);
    expect(filterApps([shop, bank, chat], { ...none, q: "fin" }).map((e) => e.app.name)).toEqual(["Bank"]);
  });

  test("team, tag and my-apps filters combine", () => {
    const all = [shop, bank, chat];
    expect(filterApps(all, { ...none, team: "t2" }).map((e) => e.app.name)).toEqual(["Bank"]);
    expect(filterApps(all, { ...none, tag: "retail" }).map((e) => e.app.name)).toEqual(["Shop", "Bank"]);
    expect(filterApps(all, { ...none, tag: "retail", team: "t1" }).map((e) => e.app.name)).toEqual(["Shop"]);
    expect(filterApps(all, { ...none, mine: new Set(["3"]) }).map((e) => e.app.name)).toEqual(["Chat"]);
    expect(tagsOf(all)).toEqual(["finance", "retail"]);
  });

  test("sorting by name, last activity and health, with pinned apps first", () => {
    const all = [shop, bank, chat];
    const names = (sort: "name" | "activity" | "health", pinned: string[] = []) =>
      sortApps(all, sort, new Set(pinned)).map((e) => e.app.name);
    expect(names("name")).toEqual(["Bank", "Chat", "Shop"]);
    expect(names("activity")).toEqual(["Chat", "Bank", "Shop"]);
    expect(names("health")).toEqual(["Bank", "Chat", "Shop"]);
    expect(names("activity", ["3"])).toEqual(["Chat", "Bank", "Shop"]);
    expect(names("name", ["1"])).toEqual(["Shop", "Bank", "Chat"]);
  });
});

describe("health and limits usage [STU-022] [STU-025]", () => {
  test("health reads the crash-free rate; no active device means no rate", () => {
    const summary = (rate: number, devices: number) =>
      create(AppSummarySchema, { crashFreeRate: rate, activeDevices: BigInt(devices) });
    expect(healthOf(summary(1, 0)).kind).toBe("none");
    expect(healthOf(summary(0.995, 10)).kind).toBe("good");
    expect(healthOf(summary(0.97, 10)).kind).toBe("warn");
    expect(healthOf(summary(0.9, 10)).kind).toBe("bad");
  });

  test("a limit warns from 80 percent and is over at the limit; a limit of zero is never reached", () => {
    expect(levelOf(79n, 100n)).toBe("ok");
    expect(levelOf(80n, 100n)).toBe("warning");
    expect(levelOf(100n, 100n)).toBe("over");
    expect(levelOf(5n, 0n)).toBe("ok");
    expect(ratioOf(150n, 100n)).toBe(1);
  });

  test("amounts carry their unit: bytes in binary units, counts plain, words for the rest", () => {
    expect(formatAmount(104_857_600n, "bytes", "en", words)).toBe("100 MiB");
    expect(formatAmount(1_536n, "bytes", "en", words)).toBe("1.5 KiB");
    expect(formatAmount(512n, "bytes", "en", words)).toBe("512 B");
    expect(formatAmount(1_200n, "count", "en", words)).toBe("1,200");
    expect(formatAmount(30n, "milliseconds", "en", words)).toBe("30 ms");
    expect(formatAmount(7n, "operations", "en", words)).toBe("7 operations");
  });
});
