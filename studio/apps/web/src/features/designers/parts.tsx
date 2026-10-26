// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { TypeTag } from "@plux/ui";
import type { Problem } from "@plux/validator";
import type { DesignerMessages } from "./designers.messages.ts";
import type { Check } from "./use-document.ts";

/** The one-line status of a designer's draft: checking, invalid, valid, nothing to apply, or the note of the last apply. */
export function Status(props: { check: Check; valid: boolean; changed: boolean; note: string; m: DesignerMessages }) {
  const { check, valid, changed, note, m } = props;
  let text = note;
  if (note === "") {
    if (check.state !== "done") {
      text = check.state === "checking" ? m.checking : m.unavailable;
    } else if (!valid) {
      text = m.invalid;
    } else {
      text = changed ? m.valid : m.nothingToApply;
    }
  }
  return (
    <span role="status" aria-live="polite" className="text-body text-secondary">
      {text}
    </span>
  );
}

/** The validator's findings for the draft. */
export function ProblemList({ problems, m }: { problems: readonly Problem[]; m: DesignerMessages }) {
  return (
    <section aria-label={m.problems} className="max-h-40 overflow-y-auto">
      <h3 className="text-label font-medium">{m.problems}</h3>
      {problems.length === 0 ? (
        <p className="text-body text-secondary">{m.noProblems}</p>
      ) : (
        <ul className="flex flex-col gap-1">
          {problems.map((p) => (
            <li key={`${p.code}${p.path}${p.message}`} className="text-body">
              {p.code !== "" && <TypeTag>{p.code}</TypeTag>} {p.message}
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
