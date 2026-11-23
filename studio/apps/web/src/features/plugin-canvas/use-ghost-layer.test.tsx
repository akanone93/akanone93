// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { create } from "@bufbuild/protobuf";
import { createRouterTransport } from "@connectrpc/connect";
import { AssetService, ListAssetsResponseSchema } from "@plux/api";
import type { SceneOp } from "@plux/canvas";
import { createStudioI18n } from "@plux/i18n";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { I18nProvider } from "../../i18n-provider.tsx";
import { AppContextProvider } from "../../lib/use-app.tsx";
import { loadSourceCatalogue } from "../../test-harness.tsx";
import { ghostItem, type OwnScreen } from "./ghosts.ts";
import type { PlacedScreen } from "./scene.ts";
import type { CanvasSurface } from "./surface.ts";
import { useGhostLayer } from "./use-ghost-layer.ts";

afterEach(cleanup);

/** A surface that records the patches sent to it; `replace()` stands for a layout that replaced the scene. */
function recordingSurface() {
  const patches: SceneOp[][] = [];
  let replaced = 1;
  const surface: CanvasSurface = {
    layoutScreens: async () => [],
    setViewport: () => undefined,
    pan: () => undefined,
    zoomAbout: () => undefined,
    patch: (ops) => patches.push([...ops]),
    viewport: { x: 0, y: 0, zoom: 1 },
    get scenesReplaced() {
      return replaced;
    },
    dispose: () => undefined,
  };
  return {
    surface,
    patches,
    replace: () => {
      replaced++;
    },
  };
}

function wrapper({ children }: { children: ReactNode }) {
  const transport = createRouterTransport(({ service }) => {
    service(AssetService, { listAssets: () => create(ListAssetsResponseSchema, { assets: [] }) } as never);
  });
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const studio = createStudioI18n({ load: async () => loadSourceCatalogue(), pseudo: false });
  return (
    <QueryClientProvider client={queryClient}>
      <AppContextProvider value={{ session: { transport, queryClient } as never, studio }}>
        <I18nProvider studio={studio}>{children}</I18nProvider>
      </AppContextProvider>
    </QueryClientProvider>
  );
}

const home: PlacedScreen = { id: "home", title: "Home", at: { x: 0, y: 0 }, root: undefined };
const own: readonly OwnScreen[] = [{ id: "home", route: "/", title: "Home", targets: ["/pay"] }];
const nativeRoutes = ["/pay"];
const NONE: readonly never[] = [];

/** The ids a patch adds or replaces that draw the target box of the native route `/pay`. */
const box = ghostItem("native:/pay");
const boxOps = (ops: readonly SceneOp[]) =>
  ops.flatMap((op) => ((op.op === "add" || op.op === "replace") && op.item.id.startsWith(box) ? [op.item.id] : []));

describe("the plugin canvas's target boxes across a scene replaced by a layout (#289)", () => {
  test("a layout that replaced the scene gets the boxes sent again; one that kept it sends nothing new", async () => {
    const s = recordingSurface();
    const input = (heights: ReadonlyMap<string, number>) => ({
      appId: "app",
      pluginId: "pl",
      canEdit: false,
      surface: () => s.surface,
      screens: [home],
      own,
      heights,
      appScreens: NONE,
      nativeRoutes,
    });
    const { rerender } = renderHook((h: ReadonlyMap<string, number>) => useGhostLayer(input(h)), {
      wrapper,
      initialProps: new Map([["home", 300]]),
    });
    await waitFor(() => expect(s.patches.some((p) => boxOps(p).length > 0)).toBe(true));
    const drawn = s.patches.flatMap(boxOps);
    expect(drawn.length).toBeGreaterThan(0);

    // A layout that kept the scene (new heights, the same scene): the box is not sent again.
    s.patches.length = 0;
    act(() => rerender(new Map([["home", 300]])));
    expect(s.patches.flatMap(boxOps)).toEqual([]);

    // A layout that replaced the scene took the box with it: it is sent again, whole.
    s.patches.length = 0;
    s.replace();
    act(() => rerender(new Map([["home", 300]])));
    expect(new Set(s.patches.flatMap(boxOps))).toEqual(new Set(drawn));
  });
});
