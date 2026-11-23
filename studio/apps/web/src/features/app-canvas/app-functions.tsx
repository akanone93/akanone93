// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import type { FunctionVersion, PluginSummary } from "@plux/api";
import { Icon } from "@plux/ui";
import { Button } from "@plux/ui/components/button";
import { Link } from "@tanstack/react-router";
import { type ReactNode, useMemo } from "react";
import { useFunctions } from "../functions/use-functions.ts";
import { defaultEnvironment } from "../security/security.schema.ts";
import { useEnvironment } from "../security/use-security.ts";
import { appCanvasMessages } from "./app-canvas.messages.ts";

/**
 * The app's functions on the app canvas (#266): which plugin calls which function, and each function's latest version
 * in the development environment — the one the dev session runs against — so a call to a function that is not deployed
 * there, or whose tests fail, shows before a device does.
 */

export interface FunctionUse {
  readonly name: string;
  /** The plugins that call it, by name. */
  readonly plugins: readonly { readonly id: string; readonly name: string }[];
  /** Its latest version in the environment; undefined when it has none there. */
  readonly deployed?: { readonly version: number; readonly placement: string; readonly testsPassed: boolean };
}

/** Every function a plugin calls or the environment has, by name, with its callers and its latest version there. */
export function functionUses(summaries: readonly PluginSummary[], deployed: readonly FunctionVersion[]): FunctionUse[] {
  const callers = new Map<string, { id: string; name: string }[]>();
  for (const s of summaries) {
    if (s.plugin === undefined) {
      continue;
    }
    for (const fn of new Set(s.functionsUsed)) {
      const list = callers.get(fn) ?? [];
      callers.set(fn, list);
      list.push({ id: s.plugin.id, name: s.plugin.name });
    }
  }
  const latest = new Map(deployed.map((f) => [f.name, f]));
  const names = [...new Set([...callers.keys(), ...latest.keys()])].sort((a, b) => a.localeCompare(b));
  return names.map((name) => {
    const f = latest.get(name);
    const plugins = (callers.get(name) ?? []).sort((a, b) => a.name.localeCompare(b.name));
    return f === undefined
      ? { name, plugins }
      : { name, plugins, deployed: { version: f.version, placement: f.placement, testsPassed: f.testsPassed } };
  });
}

function FunctionRow(props: {
  readonly use: FunctionUse;
  readonly known: boolean;
  readonly org: string;
  readonly app: string;
  readonly onShowPlugin: (id: string) => void;
}): ReactNode {
  const { i18n } = useLingui();
  const m = appCanvasMessages(i18n);
  const { use: f } = props;
  return (
    <li className="flex flex-col gap-1 border-b border-border pb-2">
      <div className="flex items-center gap-1.5">
        <Icon name="lightning" size="pill" className="text-muted-foreground" />
        <Link
          className="min-w-0 flex-1 truncate font-mono text-label underline-offset-2 hover:underline"
          to="/o/$org/apps/$app/functions/$fnId"
          params={{ org: props.org, app: props.app, fnId: f.name } as never}
          search={{ env: defaultEnvironment } as never}
        >
          {f.name}
        </Link>
        {f.deployed !== undefined && (
          <span className="app-dock-chip">{m.functionVersion(f.deployed.version, f.deployed.placement)}</span>
        )}
      </div>
      {props.known && f.deployed === undefined && f.plugins.length > 0 && (
        <p className="flex items-center gap-1 text-caption text-destructive">
          <Icon name="x-circle" size="pill" />
          {m.functionNotDeployed(defaultEnvironment)}
        </p>
      )}
      {f.deployed?.testsPassed === false && (
        <p className="flex items-center gap-1 text-caption text-muted-foreground">
          <Icon name="warning" size="pill" />
          {m.functionTestsFail}
        </p>
      )}
      {f.plugins.length === 0 ? (
        <p className="text-caption text-muted-foreground">{m.functionUnused}</p>
      ) : (
        <div className="flex flex-wrap items-center gap-1">
          <span className="text-caption text-muted-foreground">{m.functionCalledBy}</span>
          {f.plugins.map((p) => (
            <Button
              key={p.id}
              size="sm"
              variant="ghost"
              className="h-auto px-1 py-0.5"
              onClick={() => props.onShowPlugin(p.id)}
            >
              {p.name}
            </Button>
          ))}
        </div>
      )}
    </li>
  );
}

/** Whether the environment's functions were read; until they are, or when they cannot be, nothing is said of deployment. */
export type DeployedState = "loading" | "failed" | "done";

/** The Functions tab of the app canvas's dock. */
export function AppFunctions(props: {
  readonly uses: readonly FunctionUse[];
  readonly deployed: DeployedState;
  readonly org: string;
  readonly app: string;
  readonly onShowPlugin: (id: string) => void;
}): ReactNode {
  const { i18n } = useLingui();
  const m = appCanvasMessages(i18n);
  const note =
    props.deployed === "loading" ? m.checkingFunctions : props.deployed === "failed" ? m.functionsUnavailable : null;
  if (props.uses.length === 0) {
    return <p className="dock-note">{note ?? m.noFunctions}</p>;
  }
  return (
    <div className="flex flex-col gap-2">
      {note !== null && <p className="dock-note">{note}</p>}
      <ul className="flex flex-col gap-2" aria-label={m.functionsTab}>
        {props.uses.map((f) => (
          <FunctionRow
            key={f.name}
            use={f}
            known={props.deployed === "done"}
            org={props.org}
            app={props.app}
            onShowPlugin={props.onShowPlugin}
          />
        ))}
      </ul>
    </div>
  );
}

/** The Functions pane of the app canvas's dock: its label, its count of functions, and the list. */
export function useFunctionsPane(
  org: string,
  app: string,
  summaries: readonly PluginSummary[],
  onShowPlugin: (id: string) => void,
): { readonly label: string; readonly meta: string; readonly panel: ReactNode } {
  const { i18n } = useLingui();
  const environment = useEnvironment(app, defaultEnvironment);
  const functions = useFunctions(app, environment.environment?.id);
  const deployed = functions.data?.functions;
  const uses = useMemo(() => functionUses(summaries, deployed ?? []), [summaries, deployed]);
  const state: DeployedState =
    deployed !== undefined ? "done" : environment.failed || functions.isError ? "failed" : "loading";
  return {
    label: appCanvasMessages(i18n).functionsTab,
    meta: String(uses.length),
    panel: <AppFunctions uses={uses} deployed={state} org={org} app={app} onShowPlugin={onShowPlugin} />,
  };
}
