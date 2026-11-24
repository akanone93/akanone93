// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Icon } from "@plux/ui";
import { Button } from "@plux/ui/components/button";
import { InputGroup, InputGroupAddon, InputGroupInput } from "@plux/ui/components/input-group";
import { type ReactNode, useMemo, useState } from "react";
import { type PageInfo, routesOf } from "../safety/index.ts";
import { appCanvasMessages } from "./app-canvas.messages.ts";
import type { AppGraph } from "./app-canvas.scene.ts";

/**
 * Every route of the app in one list (#268): each plugin's screens by their route names, the native routes, and the
 * routes links name that nothing has (#259). The entry route is marked, a name several screens share is flagged, and
 * each screen opens its plugin at it — the direct answer to "which screen does `home` open?" (#250).
 */

/** A screen or a link's source, as the list opens it. */
export interface RouteScreen {
  readonly pluginId: string;
  readonly pageId: string;
  readonly title: string;
}

export interface AppRoute {
  readonly route: string;
  readonly kind: "screen" | "native" | "missing";
  /** The screens the name names; more than one is a clash (PLX-1103). */
  readonly screens: readonly RouteScreen[];
  /** The screens that navigate to it, each once. */
  readonly from: readonly RouteScreen[];
  readonly entry: boolean;
}

const screenOf = (p: PageInfo): RouteScreen => ({ pluginId: p.pluginId, pageId: p.id, title: p.title });

/** The app's routes: the entry route first, then by name. */
export function appRoutes(
  graph: AppGraph,
  nativeRoutes: readonly string[],
  entryRoute: string | undefined,
): AppRoute[] {
  const from = new Map<string, Map<string, RouteScreen>>();
  const source = (route: string, pageId: string) => {
    const page = graph.pages.get(pageId);
    if (page === undefined) {
      return;
    }
    const list = from.get(route) ?? new Map<string, RouteScreen>();
    from.set(route, list);
    list.set(page.id, screenOf(page));
  };
  for (const l of graph.links) {
    source(l.route, l.sourcePage);
  }
  for (const b of graph.broken) {
    source(b.route, b.sourcePage);
  }
  const sourcesOf = (route: string) => [...(from.get(route)?.values() ?? [])];
  const byRoute = routesOf(graph.pages.values());
  const out: AppRoute[] = [...byRoute].map(([route, pages]) => ({
    route,
    kind: "screen",
    screens: pages.map(screenOf),
    from: sourcesOf(route),
    entry: route === entryRoute,
  }));
  const native = new Set(nativeRoutes);
  for (const route of native) {
    if (!byRoute.has(route)) {
      out.push({ route, kind: "native", screens: [], from: sourcesOf(route), entry: route === entryRoute });
    }
  }
  const missing = new Set(graph.broken.map((b) => b.route));
  // An entry route nothing has opens nothing on a device: it heads the list, flagged, though no link names it (#248).
  if (entryRoute !== undefined && entryRoute !== "" && !byRoute.has(entryRoute) && !native.has(entryRoute)) {
    missing.add(entryRoute);
  }
  for (const route of missing) {
    out.push({ route, kind: "missing", screens: [], from: sourcesOf(route), entry: route === entryRoute });
  }
  return out.sort((a, b) => Number(b.entry) - Number(a.entry) || a.route.localeCompare(b.route));
}

/** The routes a search keeps: by route name, screen title or plugin name. */
export function searchRoutes(
  routes: readonly AppRoute[],
  query: string,
  pluginName: (id: string) => string,
): AppRoute[] {
  const q = query.trim().toLowerCase();
  if (q === "") {
    return [...routes];
  }
  const words = (r: AppRoute) =>
    [r.route, ...r.screens.flatMap((s) => [s.title, pluginName(s.pluginId)])].join(" ").toLowerCase();
  return routes.filter((r) => words(r).includes(q));
}

function ScreenButton(props: {
  readonly screen: RouteScreen;
  readonly plugin: string;
  readonly onOpen: (screen: RouteScreen) => void;
  /** Offered for a screen of the route, not for a link's source: moves it to another plugin (#287). */
  readonly onMove?: ((screen: RouteScreen) => void) | undefined;
}): ReactNode {
  const { i18n } = useLingui();
  const m = appCanvasMessages(i18n);
  const where = `${props.plugin} › ${props.screen.title}`;
  const { onMove } = props;
  return (
    <div className="flex min-w-0 items-center">
      <Button
        size="sm"
        variant="ghost"
        className="h-auto min-w-0 flex-1 justify-start px-1 py-0.5 text-start"
        aria-label={m.openRoute(where)}
        onClick={() => props.onOpen(props.screen)}
      >
        <span className="truncate text-muted-foreground">{props.plugin} ›</span>
        <span className="truncate">{props.screen.title}</span>
      </Button>
      {onMove !== undefined && (
        <Button
          size="sm"
          variant="ghost"
          className="h-auto px-1 py-0.5"
          aria-label={m.moveScreenOf(where)}
          title={m.moveScreenOf(where)}
          onClick={() => onMove(props.screen)}
        >
          <Icon name="arrows-up-down" size="pill" />
        </Button>
      )}
    </div>
  );
}

function RouteRow(props: {
  readonly route: AppRoute;
  readonly pluginName: (id: string) => string;
  readonly onOpen: (screen: RouteScreen) => void;
  readonly onMove?: ((screen: RouteScreen) => void) | undefined;
}): ReactNode {
  const { i18n } = useLingui();
  const m = appCanvasMessages(i18n);
  const r = props.route;
  const shared = r.screens.length > 1;
  return (
    <li className="flex flex-col gap-0.5 border-b border-border pb-2" data-kind={r.kind}>
      <div className="flex items-center gap-1.5">
        {r.entry && <Icon name="house" size="pill" label={m.entryRoute} />}
        <span className="min-w-0 flex-1 truncate font-mono text-label">{r.route}</span>
        {r.kind === "native" && <span className="app-dock-chip">{m.native}</span>}
        {shared && <Icon name="warning" size="pill" label={m.sharedRoute} />}
        {r.kind === "missing" && <Icon name="x-circle" size="pill" label={m.missingRoute} />}
      </div>
      {(shared || r.kind === "missing") && (
        <p className="text-caption text-muted-foreground">{shared ? m.sharedRoute : m.missingRoute}</p>
      )}
      {r.screens.map((s) => (
        <ScreenButton
          key={s.pageId}
          screen={s}
          plugin={props.pluginName(s.pluginId)}
          onOpen={props.onOpen}
          onMove={props.onMove}
        />
      ))}
      {r.kind !== "screen" && r.from.length > 0 && (
        <>
          <span className="text-caption text-muted-foreground">{m.linkedFrom}</span>
          {r.from.map((s) => (
            <ScreenButton key={s.pageId} screen={s} plugin={props.pluginName(s.pluginId)} onOpen={props.onOpen} />
          ))}
        </>
      )}
    </li>
  );
}

/** The Routes tab of the app canvas's dock. */
export function AppRoutes(props: {
  readonly routes: readonly AppRoute[];
  readonly pluginName: (id: string) => string;
  readonly onOpen: (screen: RouteScreen) => void;
  /** Moves a screen to another plugin (#287); absent where it cannot be offered. */
  readonly onMove?: ((screen: RouteScreen) => void) | undefined;
}): ReactNode {
  const { i18n } = useLingui();
  const m = appCanvasMessages(i18n);
  const [query, setQuery] = useState("");
  const shown = searchRoutes(props.routes, query, props.pluginName);
  if (props.routes.length === 0) {
    return <p className="dock-note">{m.noRoutes}</p>;
  }
  return (
    <div className="flex flex-col gap-2">
      <InputGroup>
        <InputGroupAddon>
          <Icon name="magnifying-glass" />
        </InputGroupAddon>
        <InputGroupInput
          aria-label={m.searchRoutes}
          placeholder={m.searchRoutes}
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
      </InputGroup>
      {shown.length === 0 ? (
        <p className="dock-note">{m.noRouteMatches}</p>
      ) : (
        <ul className="flex flex-col gap-2" aria-label={m.routesTab}>
          {shown.map((r) => (
            <RouteRow
              key={`${r.kind}:${r.route}`}
              route={r}
              pluginName={props.pluginName}
              onOpen={props.onOpen}
              onMove={props.onMove}
            />
          ))}
        </ul>
      )}
    </div>
  );
}

/**
 * The Routes pane of the app canvas's dock: its label, its count of routes — of problems when names clash or links are
 * broken — and the list.
 */
export function useRoutesPane(
  graph: AppGraph,
  nativeRoutes: readonly string[],
  entryRoute: string | undefined,
  pluginName: (id: string) => string,
  onOpen: (screen: RouteScreen) => void,
  onMove?: (screen: RouteScreen) => void,
): { readonly label: string; readonly meta: string; readonly panel: ReactNode } {
  const { i18n } = useLingui();
  const routes = useMemo(() => appRoutes(graph, nativeRoutes, entryRoute), [graph, nativeRoutes, entryRoute]);
  return {
    label: appCanvasMessages(i18n).routesTab,
    meta: String(routes.length),
    panel: <AppRoutes routes={routes} pluginName={pluginName} onOpen={onOpen} onMove={onMove} />,
  };
}
