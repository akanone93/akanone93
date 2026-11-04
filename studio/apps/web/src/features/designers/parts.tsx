// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { Icon, type IconName, TypeTag } from "@plux/ui";
import type { Problem } from "@plux/validator";
import type { DesignerMessages } from "./designers.messages.ts";
import type { Check } from "./use-document.ts";

/** What the status line says, and its tone: the last apply's note, else how the check of the draft went. */
function statusOf(check: Check, valid: boolean, changed: boolean, note: string, m: DesignerMessages) {
  if (note !== "") {
    return { text: note, tone: "ok" };
  }
  if (check.state !== "done") {
    return check.state === "checking" ? { text: m.checking, tone: "busy" } : { text: m.unavailable, tone: "idle" };
  }
  if (!valid) {
    return { text: m.invalid, tone: "error" };
  }
  return changed ? { text: m.valid, tone: "ok" } : { text: m.nothingToApply, tone: "idle" };
}

/** The one-line status of a designer's draft: checking, invalid, valid, nothing to apply, or the note of the last apply. */
export function Status(props: { check: Check; valid: boolean; changed: boolean; note: string; m: DesignerMessages }) {
  const { text, tone } = statusOf(props.check, props.valid, props.changed, props.note, props.m);
  return (
    <span role="status" aria-live="polite" className="sheet-status" data-tone={tone}>
      <span className="sheet-status-dot" aria-hidden="true" />
      {text}
    </span>
  );
}

const severityIcon: Readonly<Record<string, IconName>> = { error: "x-circle", warning: "warning", info: "info" };

/** The validator's findings for the draft, with their severity, at the foot of the sheet. */
export function ProblemList({ problems, m }: { problems: readonly Problem[]; m: DesignerMessages }) {
  return (
    <section aria-label={m.problems} className="flex flex-col gap-1.5">
      <h3 className="flex items-center gap-2 text-xs font-semibold">
        {m.problems}
        <span className="sheet-count">{problems.length}</span>
      </h3>
      {problems.length === 0 ? (
        <p className="flex items-center gap-1.5 text-xs text-muted-foreground">
          <Icon name="check-circle" size="pill" />
          {m.noProblems}
        </p>
      ) : (
        <ul className="flex flex-col gap-1">
          {problems.map((p) => (
            <li key={`${p.code}${p.path}${p.message}`} className="sheet-problem" data-severity={p.severity}>
              <Icon name={severityIcon[p.severity] ?? "info"} size="pill" />
              {p.code !== "" && <TypeTag>{p.code}</TypeTag>}
              <span className="min-w-0 flex-1">{p.message}</span>
              {p.path !== "" && <code className="text-tag text-muted-foreground">{p.path}</code>}
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
