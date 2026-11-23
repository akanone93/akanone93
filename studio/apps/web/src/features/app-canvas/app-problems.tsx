// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { type Diagnostic, DocumentService, Severity } from "@plux/api";
import { Icon, type IconName } from "@plux/ui";
import { Button } from "@plux/ui/components/button";
import { useQuery } from "@tanstack/react-query";
import { type ReactNode, useMemo } from "react";
import { useApp } from "../../lib/use-app.tsx";
import { nodeOfPointer } from "../problems/problem-nodes.ts";
import { problemsMessages } from "../problems/problems.messages.ts";
import type { LoadedDoc } from "../safety/index.ts";
import { appCanvasMessages } from "./app-canvas.messages.ts";

/**
 * The app's problems on the app canvas (#258): every error and warning of the whole app as the server checks its
 * stored drafts — the check each card's count comes from — grouped by plugin and file, each opening its plugin at the
 * screen and widget it is about. Notes are folded under their count, as in a plugin's Problems tab.
 */

export interface AppProblem {
  readonly code: string;
  readonly severity: "error" | "warning" | "info";
  readonly file: string;
  readonly path: string;
  readonly message: string;
}

/** A plugin as the list names it. */
export interface ProblemPlugin {
  readonly id: string;
  readonly key: string;
  readonly name: string;
}

/** Where a problem opens: its plugin, and the screen and widget it is about when the file is a page. */
export interface ProblemTarget {
  readonly pluginId: string;
  readonly focus?: string;
  readonly node?: string;
}

const severities: Record<number, AppProblem["severity"]> = {
  [Severity.ERROR]: "error",
  [Severity.WARNING]: "warning",
  [Severity.INFO]: "info",
};

/** A diagnostic of the server's check as the list shows it; an unknown severity counts as an error. */
export function problemOf(d: Diagnostic): AppProblem {
  return {
    code: d.code,
    severity: severities[d.severity] ?? "error",
    file: d.location?.file ?? "",
    path: d.location?.path ?? "",
    message: d.message,
  };
}

/** The plugin key a project path belongs to, `plugins/<key>/…`; undefined for the app's own files. */
export function pluginKeyOf(file: string): string | undefined {
  const m = /^plugins\/([^/]+)\//.exec(file);
  return m?.[1];
}

/** A plugin's problems by file, the app's own files first, then the plugins in the order given. */
export interface ProblemGroup {
  readonly plugin: ProblemPlugin | undefined;
  readonly files: readonly (readonly [string, readonly AppProblem[]])[];
}

export function groupByPlugin(problems: readonly AppProblem[], plugins: readonly ProblemPlugin[]): ProblemGroup[] {
  const byKey = new Map(plugins.map((p) => [p.key, p]));
  const groups = new Map<string, Map<string, AppProblem[]>>();
  for (const p of problems) {
    const key = pluginKeyOf(p.file) ?? "";
    const files = groups.get(key) ?? new Map<string, AppProblem[]>();
    groups.set(key, files);
    const list = files.get(p.file) ?? [];
    files.set(p.file, list);
    list.push(p);
  }
  const order = ["", ...plugins.map((p) => p.key)];
  const known = [...groups.keys()].sort((a, b) => {
    const ia = order.indexOf(a);
    const ib = order.indexOf(b);
    return (ia < 0 ? order.length : ia) - (ib < 0 ? order.length : ib) || a.localeCompare(b);
  });
  return known.map((key) => ({
    plugin: key === "" ? undefined : (byKey.get(key) ?? { id: "", key, name: key }),
    files: [...(groups.get(key) ?? new Map()).entries()].sort(([a], [b]) => a.localeCompare(b)),
  }));
}

/** Where a problem of a plugin's file opens: the plugin, and in a page the page and the widget its pointer reaches. */
export function targetOf(
  problem: AppProblem,
  plugin: ProblemPlugin | undefined,
  docs: readonly LoadedDoc[],
): ProblemTarget | undefined {
  if (plugin === undefined || plugin.id === "") {
    return undefined;
  }
  const doc = docs.find((d) => d.path === problem.file);
  if (doc === undefined || doc.kind !== "page") {
    return { pluginId: plugin.id };
  }
  const json = doc.json as Record<string, unknown> | null;
  const focus = typeof json?.["id"] === "string" ? json["id"] : undefined;
  const node = nodeOfPointer(doc.json, problem.path);
  return {
    pluginId: plugin.id,
    ...(focus === undefined ? {} : { focus }),
    ...(node === undefined ? {} : { node }),
  };
}

/** The server's check of the whole app, again whenever the app's documents were read again (`revision`). */
export function useAppProblems(appId: string, revision: number) {
  const { session } = useApp();
  const documents = useMemo(() => createClient(DocumentService, session.transport), [session.transport]);
  return useQuery({
    queryKey: ["app-problems", appId, revision],
    enabled: appId !== "",
    staleTime: Number.POSITIVE_INFINITY,
    placeholderData: (previous) => previous,
    queryFn: async () => (await documents.validateDraft({ appId, pluginId: "" })).diagnostics.map(problemOf),
  });
}

const icons: Record<AppProblem["severity"], IconName> = {
  error: "x-circle",
  warning: "warning",
  info: "info",
};

function Groups(props: {
  readonly problems: readonly AppProblem[];
  readonly plugins: readonly ProblemPlugin[];
  readonly docs: readonly LoadedDoc[];
  readonly onOpen: (target: ProblemTarget) => void;
}): ReactNode {
  const { i18n } = useLingui();
  const pm = problemsMessages(i18n);
  const m = appCanvasMessages(i18n);
  const label = { error: pm.severityError, warning: pm.severityWarning, info: pm.severityInfo };
  return groupByPlugin(props.problems, props.plugins).map((g) => (
    <section key={g.plugin?.key ?? ""} className="app-problems-group" aria-label={g.plugin?.name ?? pm.project}>
      <h3 className="text-label font-medium">{g.plugin?.name ?? pm.project}</h3>
      {g.files.map(([file, list]) => (
        <div key={file} className="mt-1">
          <p className="font-mono text-caption text-muted-foreground">{file === "" ? pm.project : file}</p>
          <ul className="mt-1 flex flex-col gap-1">
            {list.map((p, i) => {
              const target = targetOf(p, g.plugin, props.docs);
              return (
                // biome-ignore lint/suspicious/noArrayIndexKey: a problem has no identity beyond its place in the list
                <li key={i} className="flex items-start gap-2 text-body">
                  <Icon name={icons[p.severity]} size="pill" label={label[p.severity]} />
                  <span className="flex min-w-0 flex-1 flex-col">
                    <span>{p.message}</span>
                    <span className="font-mono text-caption text-muted-foreground">
                      {p.code} {p.path}
                    </span>
                  </span>
                  {target !== undefined && (
                    <Button size="sm" variant="ghost" onClick={() => props.onOpen(target)}>
                      {m.openProblem}
                    </Button>
                  )}
                </li>
              );
            })}
          </ul>
        </div>
      ))}
    </section>
  ));
}

/** The Problems tab of the app canvas's dock. */
export function AppProblems(props: {
  readonly state: "loading" | "failed" | "done";
  readonly problems: readonly AppProblem[];
  readonly plugins: readonly ProblemPlugin[];
  readonly docs: readonly LoadedDoc[];
  readonly onOpen: (target: ProblemTarget) => void;
}): ReactNode {
  const { i18n } = useLingui();
  const pm = problemsMessages(i18n);
  const m = appCanvasMessages(i18n);
  if (props.state === "loading") {
    return <p className="dock-note">{m.checkingApp}</p>;
  }
  if (props.state === "failed") {
    return (
      <p role="alert" className="dock-note">
        {pm.unavailable}
      </p>
    );
  }
  const issues = props.problems.filter((p) => p.severity !== "info");
  const notes = props.problems.filter((p) => p.severity === "info");
  return (
    <div className="app-problems flex flex-col gap-3">
      {issues.length === 0 ? (
        <p className="dock-note">{pm.none}</p>
      ) : (
        <Groups problems={issues} plugins={props.plugins} docs={props.docs} onOpen={props.onOpen} />
      )}
      {notes.length > 0 && (
        <details className="border-t border-border pt-2">
          <summary className="cursor-pointer select-none text-caption font-medium text-muted-foreground">
            {pm.notes(notes.length)}
          </summary>
          <div className="mt-2 flex flex-col gap-3">
            <Groups problems={notes} plugins={props.plugins} docs={props.docs} onOpen={props.onOpen} />
          </div>
        </details>
      )}
    </div>
  );
}

/** The plugin canvas's address of a problem: its screen and widget, when known. */
export function problemSearch(t: ProblemTarget): { focus?: string; node?: string } {
  return { ...(t.focus === undefined ? {} : { focus: t.focus }), ...(t.node === undefined ? {} : { node: t.node }) };
}

/** The Problems pane of the app canvas's dock: its label, its count of errors and warnings, and the list. */
export function useProblemsPane(
  appId: string,
  revision: number,
  summaries: readonly {
    readonly plugin?: { readonly id: string; readonly key: string; readonly name: string } | undefined;
  }[],
  docs: readonly LoadedDoc[],
  onOpen: (target: ProblemTarget) => void,
): { readonly label: string; readonly meta: string; readonly panel: ReactNode } {
  const { i18n } = useLingui();
  const checked = useAppProblems(appId, revision);
  const plugins = useMemo(
    () =>
      summaries.flatMap((s) =>
        s.plugin === undefined ? [] : [{ id: s.plugin.id, key: s.plugin.key, name: s.plugin.name }],
      ),
    [summaries],
  );
  const problems = checked.data ?? [];
  const state = checked.isError ? "failed" : checked.data === undefined ? "loading" : "done";
  return {
    label: problemsMessages(i18n).tab,
    meta: checked.data === undefined ? "" : String(problems.filter((p) => p.severity !== "info").length),
    panel: <AppProblems state={state} problems={problems} plugins={plugins} docs={docs} onOpen={onOpen} />,
  };
}
