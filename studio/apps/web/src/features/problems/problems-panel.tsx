// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Icon, type IconName } from "@plux/ui";
import type { Problem } from "@plux/validator";
import { useRef } from "react";
import { problemsMessages } from "./problems.messages.ts";
import { countProblems, groupByFile } from "./project-files.ts";
import type { Validation } from "./use-validation.ts";

const icons: Record<Problem["severity"], IconName> = {
  error: "x-circle",
  warning: "warning",
  info: "info",
};

/**
 * Error and warning counts for a canvas's top bar. They keep their place and their last figures while the validator
 * runs again, so the bar never jumps; with `onOpen` they are a button that opens the Problems tab.
 */
export function ProblemCounts({
  validation,
  onOpen,
}: {
  readonly validation: Validation;
  readonly onOpen?: (() => void) | undefined;
}) {
  const m = problemsMessages(useLingui().i18n);
  const last = useRef<{ readonly error: number; readonly warning: number } | null>(null);
  if (validation.state === "done") {
    last.current = countProblems(validation.problems);
  }
  const n = last.current;
  const busy = validation.state === "idle" || validation.state === "running";
  const pills = (
    <>
      <span
        className="problem-count"
        data-tone={n !== null && n.error > 0 ? "error" : undefined}
        role="img"
        aria-label={n === null ? m.validating : m.errors(n.error)}
      >
        <Icon name="x-circle" size="pill" />
        {n === null ? "–" : n.error}
      </span>
      <span
        className="problem-count"
        data-tone={n !== null && n.warning > 0 ? "warning" : undefined}
        {...(n === null ? { "aria-hidden": true } : { role: "img", "aria-label": m.warnings(n.warning) })}
      >
        <Icon name="warning" size="pill" />
        {n === null ? "–" : n.warning}
      </span>
    </>
  );
  return onOpen === undefined ? (
    // biome-ignore lint/a11y/useSemanticElements: a labelled group of two counts, not a form's fieldset
    <span aria-label={m.counts} role="group" className="problem-counts" data-busy={busy ? "" : undefined}>
      {pills}
    </span>
  ) : (
    <button
      type="button"
      title={m.counts}
      className="problem-counts"
      data-busy={busy ? "" : undefined}
      onClick={onOpen}
    >
      {pills}
    </button>
  );
}

/** The findings grouped by file, each with its severity icon: the body of the Problems tab. */
export function ProblemsContent({ validation }: { readonly validation: Validation }) {
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
        return <p className="text-body text-muted-foreground">{m.validating}</p>;
      case "failed":
        return (
          <p role="alert" className="text-body">
            {m.unavailable}
          </p>
        );
      case "done":
        if (validation.problems.length === 0) {
          return <p className="text-body text-muted-foreground">{m.none}</p>;
        }
        return (
          <div className="flex flex-col gap-3">
            {groupByFile(validation.problems).map(([file, list]) => (
              <section key={file} aria-label={file === "" ? m.project : file}>
                <h3 className="font-mono text-caption text-muted-foreground">{file === "" ? m.project : file}</h3>
                <ul className="mt-1 flex flex-col gap-1">
                  {list.map((p, i) => (
                    // biome-ignore lint/suspicious/noArrayIndexKey: a problem has no identity beyond its place in the list
                    <li key={i} className="flex items-start gap-2 text-body">
                      <Icon name={icons[p.severity]} size="pill" label={label[p.severity]} />
                      <span>
                        {p.message}{" "}
                        <span className="font-mono text-caption text-muted-foreground">
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
  return body();
}
