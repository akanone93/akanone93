// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { create } from "@bufbuild/protobuf";
import { createRouterTransport } from "@connectrpc/connect";
import {
  GetNativeCatalogueResponseSchema,
  HostBuildSchema,
  ListHostBuildsResponseSchema,
  NativeCatalogueService,
} from "@plux/api";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { AppContextProvider } from "../../lib/use-app.tsx";
import { insertWidget } from "../panels/model.ts";
import { stackDropRecipe } from "./select/edits.ts";
import { type Container, resolveDrop } from "./select/reparent.ts";
import { indexNodes } from "./select/tree.ts";
import { useNativeWidgets } from "./use-native-widgets.ts";

afterEach(cleanup);

const text = { id: "t", type: "Text", props: { data: "x" } };
const root = () => ({
  id: "col",
  type: "Column",
  children: [text, { id: "stack", type: "Stack", children: [] }],
});

describe("drop into a Stack [STU-067]", () => {
  const stack: Container = { id: "stack", box: { l: 100, t: 200, r: 400, b: 500 }, axis: "stack", children: [] };

  test("[STU-067] the drop point is rounded and given in the Stack's coordinates", () => {
    const t = resolveDrop(indexNodes([root()] as never), [stack], [], 150.4, 260.6);
    expect(t?.position).toEqual({ left: 50, top: 61 });
    expect(t?.indicator).toEqual({ l: 150, t: 261, r: 150, b: 261 });
  });

  test("[STU-067] a canvas drag wraps the node in a Positioned at the drop point", () => {
    const doc = root();
    stackDropRecipe("t", "stack", { left: 5, top: 7 }, () => "pos")(doc);
    expect(doc.children).toHaveLength(1);
    expect(doc.children[0]).toMatchObject({ id: "stack", children: [{ id: "pos", type: "Positioned" }] });
  });

  test("[STU-067] a Positioned child is moved whole, not double wrapped", () => {
    const doc = {
      id: "col",
      children: [
        {
          id: "a",
          type: "Stack",
          children: [{ id: "p", type: "Positioned", props: { left: 1, top: 1 }, slots: { child: text } }],
        },
        { id: "b", type: "Stack", children: [] },
      ],
    };
    stackDropRecipe("t", "b", { left: 9, top: 8 }, () => "unused")(doc);
    expect(doc.children[0]?.children).toEqual([]);
    expect(doc.children[1]?.children).toEqual([
      { id: "p", type: "Positioned", props: { left: 9, top: 8 }, slots: { child: text } },
    ]);
  });

  test("[STU-067] Insert into a Stack writes a Positioned with left/top and the widget as child", () => {
    const edits: unknown[] = [];
    const ids = ["w", "pos"];
    const d = { type: "Text", props: [] } as never;
    const id = insertWidget(
      d,
      { parent: "stack", index: 0, slot: null, at: { left: 3, top: 4 } },
      (_doc, recipe) => {
        const tree = root();
        (recipe as (x: unknown) => void)(tree);
        edits.push(tree.children[1]);
      },
      () => "doc",
      true,
      () => ids.shift() ?? "x",
    );
    expect(id).toBe("w");
    expect(edits[0]).toMatchObject({
      children: [
        { id: "pos", type: "Positioned", props: { left: 3, top: 4 }, slots: { child: { id: "w", type: "Text" } } },
      ],
    });
  });
});

describe("native widgets [WGT-030]", () => {
  const wrap = (builds: string[]) => {
    const transport = createRouterTransport(({ service }) => {
      service(NativeCatalogueService, {
        listHostBuilds: () =>
          create(ListHostBuildsResponseSchema, {
            hostBuilds: builds.map((b) => create(HostBuildSchema, { appId: "app", hostBuild: b })),
          }),
        getNativeCatalogue: (req) =>
          create(GetNativeCatalogueResponseSchema, {
            catalogue: new TextEncoder().encode(
              JSON.stringify({
                slots: [
                  {
                    type: `Map-${req.hostBuild}`,
                    description: "A map",
                    props: [{ name: "zoom", type: "int", required: true }],
                  },
                ],
              }),
            ),
          }),
      });
    });
    const queryClient = new QueryClient();
    const session = { transport, queryClient } as never;
    return ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={queryClient}>
        <AppContextProvider value={{ session, studio: {} as never }}>{children}</AppContextProvider>
      </QueryClientProvider>
    );
  };

  test("[WGT-030] the newest build's native slots become palette widgets", async () => {
    const { result } = renderHook(() => useNativeWidgets("app"), { wrapper: wrap(["2.0", "1.0"]) });
    await waitFor(() => expect(result.current).toHaveLength(1));
    expect(result.current[0]).toMatchObject({
      type: "Map-2.0",
      category: "native",
      props: [{ name: "zoom", required: true }],
    });
  });

  test("[WGT-030] an app without a build has no native widgets", async () => {
    const { result } = renderHook(() => useNativeWidgets("app"), { wrapper: wrap([]) });
    await new Promise((r) => setTimeout(r, 20));
    expect(result.current).toEqual([]);
  });
});
