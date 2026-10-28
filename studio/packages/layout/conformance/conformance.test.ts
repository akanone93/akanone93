// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Layout conformance (STU-005, ADR-0013 "Conformance suite and tolerance"):
// every Flutter golden in test/layout-conformance/goldens is laid out again
// by @plux/layout and compared edge by edge.

import { afterAll, beforeAll, describe, expect, test } from "bun:test";
import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { type CanvasKitParagraphEngine, type LayoutGolden, TextDirection } from "../src/index.ts";
import {
  buildNode,
  childrenOf,
  disposeParagraphs,
  flexTypes,
  labelledTypes,
  layOutScene,
  type Scene,
  type SceneNode,
  sceneGoldenBaselines,
  sceneGoldenRects,
  sceneTypes,
  supportedTypes,
  textTypes,
} from "../src/scene/scene.ts";
import { layer2Types } from "../src/widgets/layer2.ts";
import { loadCanvasKit, testParagraphEngine } from "../test-support/canvaskit.ts";
import { compareBaselines, compareRects, edgeTolerance, type GoldenRect, textTolerance } from "./compare.ts";

let engine: CanvasKitParagraphEngine;
beforeAll(async () => {
  engine = testParagraphEngine(await loadCanvasKit());
});
afterAll(() => {
  engine.dispose();
});

const hasText = (scene: Scene): boolean =>
  [...sceneTypes(scene.root)].some((type) => textTypes.has(type) || labelledTypes.has(type));

/** Ids whose rect is a paragraph's (Text, and an Expanded or Flexible around one): ADR-0013's text tolerance. */
function textIds(node: SceneNode, into: Set<string> = new Set()): Set<string> {
  const child = node.children?.[0];
  if (
    node.type === "Text" ||
    layer2Types.has(node.type) ||
    labelledTypes.has(node.type) ||
    ((node.type === "Expanded" || node.type === "Flexible") && child?.type === "Text")
  ) {
    into.add(node.id);
  }
  for (const c of childrenOf(node)) {
    textIds(c, into);
  }
  return into;
}

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

describe("[STU-005] [RT-016] layout conformance with Flutter goldens", () => {
  test("the suite has scenes and four goldens per scene and text scale", () => {
    expect(scenes.size).toBeGreaterThanOrEqual(12);
    const textScenes = [...scenes.values()].filter(hasText).length;
    expect(textScenes).toBeGreaterThanOrEqual(6);
    expect(goldens.length).toBe(scenes.size * 4 + textScenes * 4);
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
    const text = textIds(scene.root);
    const tolerance = text.size === 0 ? `${edgeTolerance} px` : `${edgeTolerance} px (text ${textTolerance} px)`;
    test(`${file} matches Flutter within ${tolerance}`, () => {
      const view = layOutScene(scene, golden, engine);
      const actual = sceneGoldenRects(scene, view);
      const baselines = sceneGoldenBaselines(view);
      disposeParagraphs(view);
      expect(compareRects(actual, golden.rects, (id) => (text.has(id) ? textTolerance : edgeTolerance))).toEqual([]);
      expect(compareBaselines(baselines, golden.baselines ?? {}, textTolerance)).toEqual([]);
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
    expect(() => buildNode({ id: "u", type: "Hologram", props: {} }, TextDirection.ltr)).toThrow(
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
    expect(() => buildNode({ id: "t", type: "Text", props: { text: "x" } }, TextDirection.ltr)).toThrow(
      /needs a ParagraphEngine/,
    );
    if (!supported.has("Row")) {
      expect(() => buildNode({ id: "r", type: "Row", props: {} }, TextDirection.ltr)).toThrow(/needs RenderFlex/);
    }
  });
});
