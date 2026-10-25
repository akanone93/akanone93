// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Count, Icon, type IconName, Tabs } from "@plux/ui";
import type { Problem } from "@plux/validator";
import { problemsMessages } from "./problems.messages.ts";
import { countProblems, groupByFile } from "./project-files.ts";
import type { Validation } from "./use-validation.ts";

const icons: Record<Problem["severity"], IconName> = {
  error: "x-circle",
  warning: "warning",
  info: "info",
};

/** Error and warning counts for the action bar. */
export function ProblemCounts({ validation }: { readonly validation: Validation }) {
  const m = problemsMessages(useLingui().i18n);
  if (validation.state !== "done") {
    return null;
  }
  const n = countProblems(validation.problems);
  return (
    <ul aria-label={m.counts} className="flex items-center gap-2">
      <li>
        <Count value={n.error} label={m.errors(n.error)} />
      </li>
      <li>
        <Count value={n.warning} tone="warning" label={m.warnings(n.warning)} />
      </li>
    </ul>
  );
}

/** The drawer with the Problems tab: findings grouped by file, each with its severity icon. */
export function ProblemsDrawer({ validation }: { readonly validation: Validation }) {
  const m = problemsMessages(useLingui().i18n);
  const label: Record<Problem["severity"], string> = {
    error: m.severityError,
    warning: m.severityWarning,
    info: m.severityInfo,
  };
  const body = () => {
    switch (validation.state) {
      case "idle":
      case "running":
        return <p className="text-body text-secondary">{m.validating}</p>;
      case "failed":
        return (
          <p role="alert" className="text-body">
            {m.unavailable}
          </p>
        );
      case "done":
        if (validation.problems.length === 0) {
          return <p className="text-body text-secondary">{m.none}</p>;
        }
        return (
          <div className="flex flex-col gap-3">
            {groupByFile(validation.problems).map(([file, list]) => (
              <section key={file} aria-label={file === "" ? m.project : file}>
                <h3 className="font-mono text-caption text-secondary">{file === "" ? m.project : file}</h3>
                <ul className="mt-1 flex flex-col gap-1">
                  {list.map((p, i) => (
                    // biome-ignore lint/suspicious/noArrayIndexKey: a problem has no identity beyond its place in the list
                    <li key={i} className="flex items-start gap-2 text-body">
                      <Icon name={icons[p.severity]} size="pill" label={label[p.severity]} />
                      <span>
                        {p.message}{" "}
                        <span className="font-mono text-caption text-secondary">
                          {p.code} {p.path}
                        </span>
                      </span>
                    </li>
                  ))}
                </ul>
              </section>
            ))}
          </div>
        );
    }
  };
  return (
    <aside
      aria-label={m.drawer}
      className="absolute inset-x-4 bottom-12 max-h-64 overflow-auto rounded-lg bg-raised p-3 shadow-e1"
    >
      <Tabs defaultValue="problems" tabs={[{ value: "problems", label: m.tab, panel: body() }]} />
    </aside>
  );
}
