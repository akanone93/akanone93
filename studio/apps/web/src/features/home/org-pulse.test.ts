// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { create } from "@bufbuild/protobuf";
import { AppSchema, AppSummarySchema, EnvironmentReleaseSchema } from "@plux/api";
import type { AppEntry } from "../apps/apps-model.ts";
import { busiest, orgPulse } from "./org-pulse.ts";

function entry(name: string, devices: number, rate: number, plugins: number, production: number): AppEntry {
  const app = create(AppSchema, { id: name, key: name, name });
  const releases = [
    create(EnvironmentReleaseSchema, { environmentKey: "production", liveSequence: BigInt(production) }),
  ];
  const summary = create(AppSummarySchema, {
    app,
    activeDevices: BigInt(devices),
    crashFreeRate: rate,
    pluginCount: BigInt(plugins),
    releases,
  });
  return { app, summary };
}

describe("[STU-015] the organisation's pulse", () => {
  test("sums devices and plugins, counts apps live in production, weights the crash-free rate by devices", () => {
    const pulse = orgPulse([entry("bank", 900, 0.99, 4, 3), entry("shop", 100, 0.9, 2, 0)]);
    expect(pulse).toMatchObject({ apps: 2, devices: 1000n, live: 1, plugins: 6n });
    expect(pulse.crashFree).toBeCloseTo(0.981, 6);
  });

  test("has no crash-free rate while no app has an active device", () => {
    expect(orgPulse([entry("new", 0, 0, 1, 0)]).crashFree === undefined).toBe(true);
    expect(orgPulse([])).toMatchObject({ apps: 0, devices: 0n, live: 0, plugins: 0n });
  });

  test("the busiest apps come first, ties by name", () => {
    const names = busiest([entry("b", 5, 1, 0, 0), entry("a", 5, 1, 0, 0), entry("c", 9, 1, 0, 0)], 2).map(
      (e) => e.app.name,
    );
    expect(names).toEqual(["c", "a"]);
  });
});
