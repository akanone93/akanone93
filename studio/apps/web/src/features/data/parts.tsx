// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { Button, Field, Textarea } from "@plux/ui";
import { useEffect, useState } from "react";
import type { designerMessages } from "../designers/designers.messages.ts";
import { ProblemList, Status } from "../designers/parts.tsx";
import { isObject } from "../designers/use-document.ts";
import type { DataMessages } from "./data.messages.ts";
import type { useCommit } from "./use-draft.ts";

/** A JSON text field: `onValid` gets the parsed value (`undefined` for empty text when empty is allowed); invalid text is reported and not passed on. */
export function JsonField(props: {
  readonly label: string;
  readonly help?: string;
  readonly error: string;
  readonly value: unknown;
  readonly seedKey: string;
  readonly disabled: boolean;
  readonly objectOnly: boolean;
  readonly allowEmpty: boolean;
  readonly rows?: number;
  readonly onValid: (value: unknown) => void;
}) {
  const { value, seedKey, objectOnly, allowEmpty, onValid } = props;
  const [text, setText] = useState("");
  const [bad, setBad] = useState(false);
  // biome-ignore lint/correctness/useExhaustiveDependencies: the text is re-seeded only when another source is shown
  useEffect(() => {
    setText(value === undefined ? "" : JSON.stringify(value, null, 2));
    setBad(false);
  }, [seedKey]);
  const change = (next: string) => {
    setText(next);
    if (next.trim() === "" && allowEmpty) {
      setBad(false);
      onValid(undefined);
      return;
    }
    try {
      const parsed: unknown = JSON.parse(next);
      if (objectOnly && !isObject(parsed)) {
        throw new Error("object");
      }
      setBad(false);
      onValid(parsed);
    } catch {
      setBad(true);
    }
  };
  return (
    <Field label={props.label} description={props.help} error={bad ? props.error : undefined}>
      <Textarea
        rows={props.rows ?? 6}
        value={text}
        disabled={props.disabled}
        onChange={(e) => change(e.target.value)}
      />
    </Field>
  );
}

/** Apply, the validator's verdict and its findings for a draft. */
export function CommitBar(props: {
  readonly state: ReturnType<typeof useCommit>;
  readonly m: DataMessages;
  readonly d: ReturnType<typeof designerMessages>;
  readonly label?: string;
}) {
  const { state, m, d } = props;
  return (
    <div className="flex flex-col gap-2">
      <div className="flex flex-wrap items-center gap-3">
        <Button disabled={!state.canApply} onClick={state.commit}>
          {props.label ?? m.apply}
        </Button>
        <Status
          check={state.check}
          valid={state.check.state === "done" && state.check.problems.every((p) => p.severity !== "error")}
          changed={state.changed}
          note={state.applied ? m.applied : ""}
          m={d}
        />
      </div>
      {!state.canEdit && <p className="text-caption text-secondary">{m.readOnly}</p>}
      <ProblemList problems={state.check.state === "done" ? state.check.problems : []} m={d} />
    </div>
  );
}
