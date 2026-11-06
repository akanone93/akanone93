// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { ScreenStyle } from "@plux/canvas";
import type { PaintTheme } from "@plux/layout";
import { useMemo, useRef, useState } from "react";
import { useSceneTheme } from "../app-canvas/scene-theme.ts";
import { withoutHidden } from "../panels/studio-meta.ts";
import type { PreviewRender } from "../preview/switches.ts";
import { applyConditions, type BranchPick } from "./conditions.ts";
import { designScopes, type FormPreview } from "./design-scope.ts";
import { expandLists, type ListState, type StateTexts } from "./list-preview.ts";
import { applyLive, emptyLive, type LiveState, withImpliedButtons } from "./live.ts";
import { dataSourcesOf, localizeTree, lookedStyle, messagesFor, namedMocks, resolveTheme } from "./look.ts";
import { bindMocks } from "./mock-bindings.ts";
import { withMotion } from "./motion.ts";
import type { Playing } from "./motion-player.ts";
import { type DrawerSide, withOpenDrawers } from "./page-parts.ts";
import { presented, RouteCache } from "./page-routes.ts";
import { paintedStyle, paintThemeOf, themeTokens } from "./paint-theme.ts";
import type { Node } from "./plugin-model.ts";
import { withSnackbar } from "./quick-actions.ts";
import type { PlacedScreen } from "./scene.ts";
import { screenStyle } from "./scene.ts";
import { findNode } from "./select/edits.ts";

interface DocLike {
  readonly kind: string;
  readonly content: Uint8Array;
}

const NO_DRAWERS: ReadonlyMap<string, DrawerSide> = new Map();

/**
 * What the inspector picks for the canvas to draw (PC-24–PC-26): each list's state, each condition's branch, each
 * form's preview mode, and the words of the default views and the validators.
 */
export interface DesignPicks {
  readonly lists: ReadonlyMap<string, ListState>;
  readonly branches: ReadonlyMap<string, BranchPick>;
  readonly forms?: FormPreview;
  readonly listText?: StateTexts;
}

const NO_PICKS: DesignPicks = { lists: new Map(), branches: new Map() };

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

/**
 * A screen's root as the device shows it (PC-15): the buttons its app bar implies (kept per root, so an unchanged
 * screen is not laid out again) and the live surface's values.
 */
function livened(
  root: Node,
  s: PlacedScreen,
  live: LiveState,
  platform: "material" | "cupertino",
  cache: WeakMap<Node, { key: string; node: Node }>,
): Node {
  const leading = s.kind === "fullscreenDialog" ? "close" : s.back === true ? "back" : undefined;
  const key = `${leading ?? ""}:${platform}`;
  let kept = cache.get(root);
  if (kept?.key !== key) {
    kept = { key, node: withImpliedButtons(root, leading, platform) };
    cache.set(root, kept);
  }
  return applyLive(kept.node, live);
}

/** A page's root with the transition being played drawn on its widget (PC-29); other roots are kept. */
function moving(root: Node | undefined, playing: Playing | undefined): Node | undefined {
  return root === undefined || playing === undefined || findNode(root, playing.id) === null
    ? root
    : (withMotion(root, playing.id, playing.frame) as Node);
}

/** A page's root with the snackbar the selected widget shows (PC-27), kept per root and owner. */
function snackbarOn(
  root: Node | undefined,
  owner: string | undefined,
  cache: WeakMap<Node, { owner: string; node: Node }>,
): Node | undefined {
  if (root === undefined || owner === undefined) {
    return root;
  }
  const kept = cache.get(root);
  if (kept?.owner === owner) {
    return kept.node;
  }
  const node = withSnackbar(root, owner) as Node;
  cache.set(root, { owner, node });
  return node;
}

/** Dialog and sheet pages wrapped in their routes over the pages that open them (PC-13). */
function presentRoutes(drawn: readonly PlacedScreen[], routes: RouteCache): PlacedScreen[] {
  const byId = new Map(drawn.map((s) => [s.id, s]));
  return drawn.map((s) => {
    if (s.root === undefined || !presented(s.kind)) {
      return s;
    }
    const opener = s.opener === undefined ? undefined : byId.get(s.opener)?.root;
    return { ...s, root: routes.wrap(s.kind, s.root, opener) };
  });
}

interface LocalizedRoots {
  readonly everything: readonly DocLike[];
  readonly look: PreviewRender;
  readonly design: DesignPicks;
  readonly byRoot: WeakMap<Node, Node | undefined>;
}

/** What the preview switches draw (STU-058, DAT-080): the screens with translated texts, data bound to the selected mock states, and the style with theme, scale and direction. */
export function usePreviewScene(
  screens: readonly PlacedScreen[],
  appDocs: readonly DocLike[] | undefined,
  pluginDocs: readonly DocLike[] | undefined,
  look: PreviewRender,
  drawers: ReadonlyMap<string, DrawerSide> = NO_DRAWERS,
  live: LiveState = emptyLive,
  design: DesignPicks = NO_PICKS,
  snackOwner?: string,
  playing?: Playing,
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
  const [routes] = useState(() => new RouteCache());
  const [implied] = useState(() => new WeakMap<Node, { key: string; node: Node }>());
  const [snacked] = useState(() => new WeakMap<Node, { owner: string; node: Node }>());
  const shown = useMemo(() => {
    const kept = cache.current;
    const byRoot =
      kept !== undefined && kept.everything === everything && kept.look === look && kept.design === design
        ? kept.byRoot
        : new WeakMap<Node, Node | undefined>();
    cache.current = { everything, look, design, byRoot };
    const messages = messagesFor(everything, look);
    const { mocks, states } = namedMocks(dataSourcesOf(everything), look.mockStates);
    const scopes = designScopes(everything, design.forms);
    const drawn = screens.map((s) => {
      if (s.root === undefined) {
        return s;
      }
      if (!byRoot.has(s.root)) {
        // A layer hidden in Layers is left out of what the canvas draws (STU-061); the device still shows it.
        const drawn = withoutHidden(s.root);
        // Conditions pick their branches (PC-25) and lists are drawn from their data (PC-24) first; then every other
        // binding takes its design-time value.
        const scope = scopes.get(s.id);
        const decided = applyConditions(
          localizeTree(drawn, messages, look.localize),
          mocks,
          states,
          scope,
          design.branches,
        );
        const listed = expandLists(decided, mocks, states, scope, design.lists, design.listText);
        byRoot.set(s.root, bindMocks(listed, mocks, states, scope));
      }
      const drawer = moving(snackbarOn(shownWith(byRoot.get(s.root), drawers, opened), snackOwner, snacked), playing);
      return { ...s, root: drawer === undefined ? drawer : livened(drawer, s, live, look.platform, implied) };
    });
    return presentRoutes(drawn, routes);
  }, [screens, everything, look, drawers, opened, routes, live, implied, design, snackOwner, snacked, playing]);
  return { shown, styled, paint };
}
