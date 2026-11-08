// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import "./monaco-features.ts";

/** Monaco's registry of the services it creates for the editors (an internal module, without types). */
const extensions = "monaco-editor/platform/instantiation/common/extensions.js";

describe("Monaco's editor features load before the first editor [STU-063]", () => {
  test("the services the editor features need are registered with them", async () => {
    const { getSingletonServiceDescriptors } = (await import(extensions)) as {
      getSingletonServiceDescriptors(): [unknown, unknown][];
    };
    const registered = new Set(getSingletonServiceDescriptors().map(([id]) => String(id)));
    for (const service of [
      "ICodeLensCache",
      "IInlayHintsCache",
      "ISuggestMemories",
      "treeViewsDndService",
      "actionWidgetService",
    ]) {
      expect(registered.has(service)).toBe(true);
    }
  });
});
