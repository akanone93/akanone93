// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { ScreenStyle } from "@plux/canvas";
import type { PaintTheme } from "@plux/layout";
import { useEffect, useMemo, useRef, useState } from "react";
import { withoutHidden } from "../panels/studio-meta.ts";
import type { MockState, PreviewRender } from "../preview/switches.ts";
import { componentsOf, expandInstances } from "./component-instances.ts";
import { applyConditions, type BranchPick } from "./conditions.ts";
import { designScopes, type FormPreview, stateDefaults } from "./design-scope.ts";
import { expandLists, type ListState, type StateTexts } from "./list-preview.ts";
import { applyLive, emptyLive, type LiveState, withImpliedButtons } from "./live.ts";
import {
  dataSourcesOf,
  localizeTree,
  lookedStyle,
  messagesFor,
  namedMocks,
  pageDataSourcesOf,
  resolveTheme,
} from "./look.ts";
import {
  bindMocks,
  type ComponentScope,
  type DesignScope,
  emptyScope,
  evaluateMockBinding,
  readsItem,
} from "./mock-bindings.ts";
import { withMotion } from "./motion.ts";
import type { Playing } from "./motion-player.ts";
import { type DrawerSide, withOpenDrawers } from "./page-parts.ts";
import { presented, RouteCache } from "./page-routes.ts";
import { paintedStyle, paintThemeOf, themeTokens } from "./paint-theme.ts";
import type { Node } from "./plugin-model.ts";
import { type PageSource, PxlPreview, type ScreenData } from "./pxl-preview.ts";
import { withSnackbar } from "./quick-actions.ts";
import { deviceRoot } from "./responsive.ts";
import type { PlacedScreen } from "./scene.ts";
import { screenStyle } from "./scene.ts";
import { findNode } from "./select/edits.ts";
import { applyTimeline, type Timeline } from "./timelines.ts";

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
  /** The count each plural text is shown with, by node (PC-32). */
  readonly samples?: ReadonlyMap<string, number>;
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

/** A timeline open on the canvas (PC-30): its page and the moment the canvas draws it at. */
export interface Scrubbed {
  readonly page: string;
  readonly timeline: Timeline;
  readonly ms: number;
}

/** A message argument's design-time value: an expression resolved where it can be, a literal as it is. */
function argValue(
  v: unknown,
  mocks: ReadonlyMap<string, unknown>,
  states: Readonly<Record<string, MockState>>,
  scope: DesignScope | undefined,
): unknown {
  const expr = typeof v === "object" && v !== null && "$expr" in v ? (v as { $expr: unknown }).$expr : undefined;
  if (typeof expr !== "string") {
    return v;
  }
  const value = evaluateMockBinding(expr, mocks, states, scope);
  return value === null ? undefined : value;
}

/** What a pass over the screens draws them with. */
interface Pass {
  readonly messages: Record<string, string>;
  readonly mocks: ReadonlyMap<string, unknown>;
  readonly states: Readonly<Record<string, MockState>>;
  readonly components: ReturnType<typeof componentsOf>;
  readonly look: PreviewRender;
  readonly design: DesignPicks;
}

/** A screen's design scope; what the readers do not resolve, the PXL engine evaluates when it can (ADR-0112). */
function scopeOf(
  id: string,
  scopes: ReadonlyMap<string, DesignScope>,
  preview: PxlPreview | undefined,
  evaluation: DesignEvaluation | undefined,
  data: ScreenData,
  components: ReturnType<typeof componentsOf>,
): DesignScope | undefined {
  const found = scopes.get(id);
  const page = evaluation?.pages.get(id);
  if (preview === undefined || page === undefined) {
    return found;
  }
  // A component's own expressions evaluate in its scope, as its file holds them, once per component and screen.
  const kept = new Map<string, ComponentScope | undefined>();
  const componentScope = (component: string): ComponentScope | undefined => {
    if (!kept.has(component)) {
      const source = evaluation?.components?.get(component);
      const state = stateDefaults(components.get(component)?.["state"]);
      kept.set(component, {
        state,
        ...(source === undefined ? {} : { evaluate: preview.forComponent(id, source, data) }),
      });
    }
    return kept.get(component);
  };
  return { ...(found ?? emptyScope), evaluate: preview.forScreen(id, page, data), componentScope };
}

/** A screen's data: the plugin's sources and its page's own, which win on a name both use. */
function screenData(
  id: string,
  sources: ReturnType<typeof dataSourcesOf>,
  own: ReturnType<typeof pageDataSourcesOf>,
  plugin: ScreenData,
  look: PreviewRender,
): ScreenData {
  const mine = own.get(id);
  return mine === undefined ? plugin : namedMocks([...sources, ...mine], look.mockStates);
}

/**
 * A screen's root as the preview draws it: hidden layers left out (STU-061), component instances drawn as their
 * components (PC-37), texts in the previewed locale with their arguments' design-time values (PC-32), conditions and
 * lists from their data (PC-24, PC-25), then every other binding at its design-time value.
 */
function designRoot(root: Node, scope: DesignScope | undefined, pass: Pass): Node | undefined {
  const { mocks, states, look, design } = pass;
  const drawn = expandInstances(withoutHidden(root), pass.components);
  const options = {
    locale: look.locale,
    ...(design.samples === undefined ? {} : { samples: design.samples }),
    arg: (v: unknown) => argValue(v, mocks, states, scope),
  };
  // A text whose arguments read a list item waits for the list's copies, where each has its item's values.
  const deferred = { count: 0 };
  const localized = localizeTree(drawn, pass.messages, look.localize, { ...options, defer: itemArg, deferred });
  const decided = applyConditions(localized, mocks, states, scope, design.branches);
  const listed = expandLists(decided, mocks, states, scope, design.lists, design.listText);
  const texts = deferred.count === 0 ? listed : localizeTree(listed, pass.messages, look.localize, options);
  return bindMocks(texts, mocks, states, scope);
}

/** Whether a message argument is an expression that reads a list item. */
function itemArg(value: unknown): boolean {
  const expr = typeof value === "object" && value !== null ? (value as { $expr?: unknown }).$expr : undefined;
  return typeof expr === "string" && readsItem(expr);
}

/** A page's root drawn at the moment of its open timeline; other pages are kept. */
function scrubbing(page: string, root: Node | undefined, scrubbed: Scrubbed | undefined): Node | undefined {
  return root === undefined || scrubbed === undefined || scrubbed.page !== page
    ? root
    : (applyTimeline(root, scrubbed.timeline, scrubbed.ms) as Node);
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
  /** The PXL engine's evaluator the roots were drawn with; another one draws them again. */
  readonly preview: PxlPreview | undefined;
  /** Each screen's root as last drawn, so the screens the PXL engine answered for are drawn again. */
  readonly roots: Map<string, Node>;
}

/**
 * The PXL engine's design-time evaluation (ADR-0112), kept while the validator's worker lives; when the engine
 * answers, the screens that asked are drawn again.
 */
export interface DesignEvaluation {
  /** Sends a batch to the engine; the same function for as long as the worker lives. */
  readonly evaluate: (request: string) => Promise<unknown>;
  /** Each page's file and document as the canvas draws them, by page id. */
  readonly pages: ReadonlyMap<string, PageSource>;
  /** Each component's file and document, the app's and the plugin's, by component id. */
  readonly components?: ReadonlyMap<string, PageSource>;
}

/**
 * The canvas's evaluator for `evaluation` and the count of its answers: each answer drops the answered screens' drawn
 * roots from `cache`, and the count draws them again.
 */
function usePxlPreview(
  evaluation: DesignEvaluation | undefined,
  cache: { readonly current: LocalizedRoots | undefined },
): { readonly preview: PxlPreview | undefined; readonly answers: number } {
  const [answers, setAnswers] = useState(0);
  const send = evaluation?.evaluate;
  const preview = useMemo(
    () =>
      send === undefined
        ? undefined
        : new PxlPreview(send, (screens) => {
            const kept = cache.current;
            for (const id of screens) {
              const root = kept?.roots.get(id);
              if (root !== undefined) {
                kept?.byRoot.delete(root);
              }
            }
            setAnswers((n) => n + 1);
          }),
    [send, cache],
  );
  return { preview, answers };
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
  scrubbed?: Scrubbed,
  evaluation?: DesignEvaluation,
): { shown: readonly PlacedScreen[]; styled: ScreenStyle; paint: PaintTheme } {
  const everything = useMemo(() => [...(appDocs ?? []), ...(pluginDocs ?? [])], [appDocs, pluginDocs]);
  // The screens are painted in the theme's colours for the previewed mode and brand (STU-005, THM-001–THM-003).
  const tokens = useMemo(() => themeTokens(everything), [everything]);
  const paint = useMemo(
    () => paintThemeOf(tokens, look.theme === "dark", look.brandOverlay, look.platform),
    [tokens, look.theme, look.brandOverlay, look.platform],
  );
  const styled = useMemo(
    // The screens' names are the header bars over the canvas (MR-2): the worker draws no header of its own.
    () => paintedStyle(lookedStyle(screenStyle, resolveTheme(everything, look), look), paint),
    [everything, paint, look],
  );
  const cache = useRef<LocalizedRoots | undefined>(undefined);
  const { preview, answers } = usePxlPreview(evaluation, cache);

  const [opened] = useState(() => new WeakMap<Node, { side: DrawerSide; node: Node }>());
  const [routes] = useState(() => new RouteCache());
  const [implied] = useState(() => new WeakMap<Node, { key: string; node: Node }>());
  const [snacked] = useState(() => new WeakMap<Node, { owner: string; node: Node }>());
  // biome-ignore lint/correctness/useExhaustiveDependencies: the engine's answers draw the screens again by their count
  const shown = useMemo(() => {
    const kept = cache.current;
    const same =
      kept !== undefined &&
      kept.everything === everything &&
      kept.look === look &&
      kept.design === design &&
      kept.preview === preview;
    const byRoot = same ? kept.byRoot : new WeakMap<Node, Node | undefined>();
    const roots = new Map<string, Node>();
    cache.current = { everything, look, design, byRoot, preview, roots };
    const messages = messagesFor(everything, look);
    const sources = dataSourcesOf(everything);
    const own = pageDataSourcesOf(everything);
    const plugin = namedMocks(sources, look.mockStates);
    preview?.begin({ locale: look.locale, messages });
    // Conditions read the `device` root as the frame and the switches set it (PC-33).
    const scopes = designScopes(everything, design.forms, deviceRoot(look));
    const pass: Pass = { messages, ...plugin, components: componentsOf(everything), look, design };
    const drawn = screens.map((s) => {
      if (s.root === undefined) {
        return s;
      }
      roots.set(s.id, s.root);
      if (!byRoot.has(s.root)) {
        const data = screenData(s.id, sources, own, plugin, look);
        const scope = scopeOf(s.id, scopes, preview, evaluation, data, pass.components);
        byRoot.set(s.root, designRoot(s.root, scope, data === plugin ? pass : { ...pass, ...data }));
      }
      const drawer = scrubbing(
        s.id,
        moving(snackbarOn(shownWith(byRoot.get(s.root), drawers, opened), snackOwner, snacked), playing),
        scrubbed,
      );
      return { ...s, root: drawer === undefined ? drawer : livened(drawer, s, live, look.platform, implied) };
    });
    return presentRoutes(drawn, routes);
  }, [
    screens,
    everything,
    look,
    drawers,
    opened,
    routes,
    live,
    implied,
    design,
    snackOwner,
    snacked,
    playing,
    scrubbed,
    preview,
    evaluation,
    answers,
  ]);
  // What the pass left to evaluate goes to the engine after the screens are drawn, in one batch.
  useEffect(() => {
    if (shown !== undefined) {
      preview?.flush();
    }
  }, [preview, shown]);
  return { shown, styled, paint };
}
