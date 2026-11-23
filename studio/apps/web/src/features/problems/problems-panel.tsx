// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Icon, type IconName, Tooltip } from "@plux/ui";
import type { Problem } from "@plux/validator";
import { type ReactNode, useRef } from "react";
import { problemsMessages } from "./problems.messages.ts";
import { countProblems, groupByFile, splitNotes } from "./project-files.ts";
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
    <Tooltip content={m.counts}>
      <button type="button" className="problem-counts" data-busy={busy ? "" : undefined} onClick={onOpen}>
        {pills}
      </button>
    </Tooltip>
  );
}

/** A list of findings grouped by file, each with its severity icon. */
function ProblemGroups(props: {
  readonly problems: readonly Problem[];
  readonly label: Record<Problem["severity"], string>;
  readonly project: string;
  readonly help?: ((p: Problem) => ReactNode) | undefined;
}) {
  return (
    <div className="flex flex-col gap-3">
      {groupByFile(props.problems).map(([file, list]) => (
        <section key={file} aria-label={file === "" ? props.project : file}>
          <h3 className="font-mono text-caption text-muted-foreground">{file === "" ? props.project : file}</h3>
          <ul className="mt-1 flex flex-col gap-1">
            {list.map((p, i) => (
              // biome-ignore lint/suspicious/noArrayIndexKey: a problem has no identity beyond its place in the list
              <li key={i} className="flex items-start gap-2 text-body">
                <Icon name={icons[p.severity]} size="pill" label={props.label[p.severity]} />
                <span className="flex min-w-0 flex-col gap-1">
                  <span>
                    {p.message}{" "}
                    <span className="font-mono text-caption text-muted-foreground">
                      {p.code} {p.path}
                    </span>
                  </span>
                  {props.help?.(p)}
                </span>
              </li>
            ))}
          </ul>
        </section>
      ))}
    </div>
  );
}

/**
 * The body of the Problems tab: the errors and warnings the chip counts, grouped by file; then the notes, folded under
 * their own count (`splitNotes`, #226), so the chip's figure is the number of problems listed.
 */
export function ProblemsContent({
  validation,
  help,
}: {
  readonly validation: Validation;
  /** What a finding means and the buttons that fix it (P1); absent where no fix can run. */
  readonly help?: ((p: Problem) => ReactNode) | undefined;
}) {
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
      case "done": {
        const { issues, notes } = splitNotes(validation.problems);
        return (
          <div className="flex flex-col gap-3">
            {issues.length === 0 ? (
              <p className="text-body text-muted-foreground">{m.none}</p>
            ) : (
              <ProblemGroups problems={issues} label={label} project={m.project} help={help} />
            )}
            {notes.length > 0 && (
              <details className="border-t border-border pt-2">
                <summary className="cursor-pointer select-none text-caption font-medium text-muted-foreground">
                  {m.notes(notes.length)}
                </summary>
                <div className="mt-2">
                  <ProblemGroups problems={notes} label={label} project={m.project} help={help} />
                </div>
              </details>
            )}
          </div>
        );
      }
    }
  };
  return body();
}
