// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { ScreenStyle } from "@plux/canvas";
import type { PaintTheme } from "@plux/layout";
import { useMemo, useRef, useState } from "react";
import { useSceneTheme } from "../app-canvas/scene-theme.ts";
import { withoutHidden } from "../panels/studio-meta.ts";
import type { PreviewRender } from "../preview/switches.ts";
import { dataSourcesOf, localizeTree, lookedStyle, messagesFor, resolveTheme } from "./look.ts";
import { bindMocks } from "./mock-bindings.ts";
import { type DrawerSide, withOpenDrawers } from "./page-parts.ts";
import { paintedStyle, paintThemeOf, themeTokens } from "./paint-theme.ts";
import type { Node } from "./plugin-model.ts";
import type { PlacedScreen } from "./scene.ts";
import { screenStyle } from "./scene.ts";

interface DocLike {
  readonly kind: string;
  readonly content: Uint8Array;
}

const NO_DRAWERS: ReadonlyMap<string, DrawerSide> = new Map();

/**
 * A page's root as shown with its drawer open when `drawers` names its scaffold (PC-12): a shallow copy marked
 * `x-studio.open`, kept per root and side so an unchanged screen keeps its identity and is not laid out again.
 */
function shownWith(
  root: Node | undefined,
  drawers: ReadonlyMap<string, DrawerSide>,
  opened: WeakMap<Node, { side: DrawerSide; node: Node }>,
): Node | undefined {
  const side = root === undefined ? undefined : drawers.get(root.id);
  if (root === undefined || side === undefined) {
    return root;
  }
  const kept = opened.get(root);
  if (kept?.side === side) {
    return kept.node;
  }
  const node = withOpenDrawers(root, new Map([[root.id, side]]));
  opened.set(root, { side, node });
  return node;
}

interface LocalizedRoots {
  readonly everything: readonly DocLike[];
  readonly look: PreviewRender;
  readonly byRoot: WeakMap<Node, Node | undefined>;
}

/** What the preview switches draw (STU-058, DAT-080): the screens with translated texts, data bound to the selected mock states, and the style with theme, scale and direction. */
export function usePreviewScene(
  screens: readonly PlacedScreen[],
  appDocs: readonly DocLike[] | undefined,
  pluginDocs: readonly DocLike[] | undefined,
  look: PreviewRender,
  drawers: ReadonlyMap<string, DrawerSide> = NO_DRAWERS,
): { shown: readonly PlacedScreen[]; styled: ScreenStyle; paint: PaintTheme } {
  const everything = useMemo(() => [...(appDocs ?? []), ...(pluginDocs ?? [])], [appDocs, pluginDocs]);
  // Each screen's name above its frame takes the canvas theme's label colour, so it reads on a light and a dark canvas.
  const title = useSceneTheme().arrows.labelText;
  // The screens are painted in the theme's colours for the previewed mode and brand (STU-005, THM-001–THM-003).
  const tokens = useMemo(() => themeTokens(everything), [everything]);
  const paint = useMemo(
    () => paintThemeOf(tokens, look.theme === "dark", look.brandOverlay, look.platform),
    [tokens, look.theme, look.brandOverlay, look.platform],
  );
  const styled = useMemo(
    () => ({ ...paintedStyle(lookedStyle(screenStyle, resolveTheme(everything, look), look), paint), title }),
    [everything, paint, look, title],
  );
  const cache = useRef<LocalizedRoots | undefined>(undefined);
  const [opened] = useState(() => new WeakMap<Node, { side: DrawerSide; node: Node }>());
  const shown = useMemo(() => {
    const kept = cache.current;
    const byRoot =
      kept !== undefined && kept.everything === everything && kept.look === look
        ? kept.byRoot
        : new WeakMap<Node, Node | undefined>();
    cache.current = { everything, look, byRoot };
    const messages = messagesFor(everything, look);
    const mocks = new Map(dataSourcesOf(everything).map((src) => [src.id, src.mock]));
    return screens.map((s) => {
      if (s.root === undefined) {
        return s;
      }
      if (!byRoot.has(s.root)) {
        // A layer hidden in Layers is left out of what the canvas draws (STU-061); the device still shows it.
        const drawn = withoutHidden(s.root);
        byRoot.set(s.root, bindMocks(localizeTree(drawn, messages, look.localize), mocks, look.mockStates));
      }
      return { ...s, root: shownWith(byRoot.get(s.root), drawers, opened) };
    });
  }, [screens, everything, look, drawers, opened]);
  return { shown, styled, paint };
}
