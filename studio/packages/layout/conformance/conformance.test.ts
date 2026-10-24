// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Layout conformance (STU-005, ADR-0013 "Conformance suite and tolerance"):
// every Flutter golden in test/layout-conformance/goldens is laid out again
// by @plux/layout and compared edge by edge.

import { describe, expect, test } from "bun:test";
import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { type LayoutGolden, TextDirection } from "../src/index.ts";
import { compareRects, edgeTolerance, type GoldenRect } from "./compare.ts";
import {
  buildNode,
  flexTypes,
  layOutScene,
  type Scene,
  sceneGoldenRects,
  sceneTypes,
  supportedTypes,
} from "./scene.ts";

const suite = join(import.meta.dir, "../../../../test/layout-conformance");

function readJson<T>(dir: string, file: string): T {
  return JSON.parse(readFileSync(join(suite, dir, file), "utf8")) as T;
}

const scenes = new Map<string, Scene>(
  readdirSync(join(suite, "scenes"))
    .filter((file) => file.endsWith(".json"))
    .sort()
    .map((file) => {
      const scene = readJson<Scene>("scenes", file);
      return [scene.name, scene];
    }),
);
const goldens = readdirSync(join(suite, "goldens"))
  .filter((file) => file.endsWith(".json"))
  .sort()
  .map((file) => ({ file, golden: readJson<LayoutGolden>("goldens", file) }));

const supported = supportedTypes();
const isSupported = (scene: Scene): boolean => [...sceneTypes(scene.root)].every((type) => supported.has(type));
const usesFlex = (scene: Scene): boolean => [...sceneTypes(scene.root)].some((type) => flexTypes.has(type));
const awaiting = [...scenes.values()].filter((scene) => !isSupported(scene)).map((scene) => scene.name);
if (awaiting.length > 0) {
  process.stdout.write(`layout conformance: awaiting RenderFlex: ${awaiting.join(", ")}\n`);
}

describe("STU-005 layout conformance with Flutter goldens", () => {
  test("the suite has scenes and four goldens per scene", () => {
    expect(scenes.size).toBeGreaterThanOrEqual(12);
    expect(goldens.length).toBe(scenes.size * 4);
    for (const { golden } of goldens) {
      expect(scenes.has(golden.scene)).toBe(true);
    }
  });

  test("only scenes that need RenderFlex await it, and none is lost", () => {
    const flexScenes = [...scenes.values()].filter(usesFlex).map((scene) => scene.name);
    expect(flexScenes.length).toBeGreaterThanOrEqual(6);
    if (supported.has("Row")) {
      expect(awaiting).toEqual([]);
    } else {
      expect(awaiting).toEqual(flexScenes);
    }
  });

  for (const { file, golden } of goldens) {
    const scene = scenes.get(golden.scene);
    if (scene === undefined || !isSupported(scene)) {
      continue;
    }
    test(`${file} matches Flutter within ${edgeTolerance} px`, () => {
      const actual = sceneGoldenRects(scene, layOutScene(scene, golden));
      expect(compareRects(actual, golden.rects)).toEqual([]);
    });
  }
});

describe("conformance tooling", () => {
  test("compareRects allows half a pixel per edge and reports every other difference", () => {
    const golden: Record<string, GoldenRect> = { a: [0, 0, 10, 10], b: [5, 5, 10, 10], c: [0, 0, 1, 1] };
    const actual: Record<string, GoldenRect> = { a: [0.5, 0, 9.5, 10], b: [5, 5, 10.6, 10], d: [0, 0, 1, 1] };
    expect(compareRects(actual, golden).map((d) => d.id)).toEqual(["b", "c", "d"]);
    expect(compareRects(actual, golden)[0]?.delta).toBeCloseTo(0.6);
    expect(compareRects({ a: [Number.NaN, 0, 1, 1] }, { a: [0, 0, 1, 1] })[0]?.delta).toBe(Number.POSITIVE_INFINITY);
  });

  test("scene building rejects unknown types, flex types without RenderFlex and extra children", () => {
    const box = { id: "x", type: "TestBox", props: { width: 1, height: 1 } };
    expect(() => buildNode({ id: "u", type: "Table", props: {} }, TextDirection.ltr)).toThrow(
      /unknown scene node type/,
    );
    expect(() =>
      buildNode(
        { id: "p", type: "Padding", props: { padding: [0, 0, 0, 0] }, children: [box, box] },
        TextDirection.ltr,
      ),
    ).toThrow(/at most one child/);
    expect(() =>
      buildNode({ id: "t", type: "TestBox", props: { width: "wide", height: 1 } }, TextDirection.ltr),
    ).toThrow(/expected a number/);
    if (!supported.has("Row")) {
      expect(() => buildNode({ id: "r", type: "Row", props: {} }, TextDirection.ltr)).toThrow(/needs RenderFlex/);
    }
  });
});
