// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { Surface } from "canvaskit-wasm";
import { type Env, type PageDoc, type SceneResult, sceneA, sceneB } from "./scenes";

export type { Env, PageDoc };

/** Runs the named scenes ("A", "B") one after the other on one surface. */
export async function runScenes(env: Env, surface: Surface, names: string[]): Promise<SceneResult[]> {
  const results: SceneResult[] = [];
  if (names.includes("A")) {
    results.push(await sceneA(env, surface));
  }
  if (names.includes("B")) {
    results.push(await sceneB(env, surface));
  }
  return results;
}
