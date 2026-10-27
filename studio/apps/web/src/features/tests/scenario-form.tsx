// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { Button, Field, Input, Select } from "@plux/ui";
import type { ReactElement } from "react";
import type { TestsMessages } from "./tests.messages.ts";
import {
  type Draft,
  type DraftExpect,
  type DraftProblem,
  type DraftStep,
  type ExpectKind,
  expectKinds,
  type StepKind,
  stepKinds,
} from "./tests.model.ts";

interface Props {
  readonly m: TestsMessages;
  readonly draft: Draft;
  readonly problems: readonly DraftProblem[];
  readonly onChange: (draft: Draft) => void;
  readonly onSave: () => void;
  readonly busy: boolean;
}

const replaceAt = <T,>(list: readonly T[], index: number, value: T): T[] =>
  list.map((v, i) => (i === index ? value : v));

function stepNames(m: TestsMessages): Record<StepKind, string> {
  return { tap: m.stepTap, enterText: m.stepEnterText, waitFor: m.stepWaitFor, scroll: m.stepScroll };
}

function expectNames(m: TestsMessages): Record<ExpectKind, string> {
  return {
    visible: m.expectVisible,
    notVisible: m.expectNotVisible,
    textEquals: m.expectTextEquals,
    navigatedTo: m.expectNavigatedTo,
  };
}

function problemText(m: TestsMessages): Record<DraftProblem, string> {
  return {
    file: m.problemFile,
    name: m.problemName,
    page: m.problemPage,
    step: m.problemStep,
    expect: m.problemExpect,
  };
}

function StepRow(props: {
  readonly m: TestsMessages;
  readonly step: DraftStep;
  readonly index: number;
  readonly onChange: (step: DraftStep) => void;
}): ReactElement {
  const { m, step, index, onChange } = props;
  const names = stepNames(m);
  return (
    <li className="flex items-end gap-3">
      <Field label={`${m.stepsHeading} ${index + 1} · ${m.kindLabel}`}>
        <Select
          options={stepKinds.map((k) => ({ value: k, label: names[k] }))}
          value={step.kind}
          onValueChange={(v) => onChange({ ...step, kind: v as StepKind })}
        />
      </Field>
      <Field label={`${m.stepsHeading} ${index + 1} · ${m.testIdLabel}`}>
        <Input value={step.testId} onChange={(e) => onChange({ ...step, testId: e.target.value })} />
      </Field>
      {step.kind === "enterText" && (
        <Field label={`${m.stepsHeading} ${index + 1} · ${m.textLabel}`}>
          <Input value={step.text} onChange={(e) => onChange({ ...step, text: e.target.value })} />
        </Field>
      )}
    </li>
  );
}

function ExpectRow(props: {
  readonly m: TestsMessages;
  readonly expect: DraftExpect;
  readonly index: number;
  readonly onChange: (expect: DraftExpect) => void;
}): ReactElement {
  const { m, expect, index, onChange } = props;
  const names = expectNames(m);
  const valueLabel = expect.kind === "navigatedTo" ? m.routeLabel : m.testIdLabel;
  return (
    <li className="flex items-end gap-3">
      <Field label={`${m.expectsHeading} ${index + 1} · ${m.kindLabel}`}>
        <Select
          options={expectKinds.map((k) => ({ value: k, label: names[k] }))}
          value={expect.kind}
          onValueChange={(v) => onChange({ ...expect, kind: v as ExpectKind })}
        />
      </Field>
      <Field label={`${m.expectsHeading} ${index + 1} · ${valueLabel}`}>
        <Input value={expect.value} onChange={(e) => onChange({ ...expect, value: e.target.value })} />
      </Field>
      {expect.kind === "textEquals" && (
        <Field label={`${m.expectsHeading} ${index + 1} · ${m.textLabel}`}>
          <Input value={expect.text} onChange={(e) => onChange({ ...expect, text: e.target.value })} />
        </Field>
      )}
    </li>
  );
}

/** The authoring form of one scenario (TST-001): given page, steps, expectations. */
export function ScenarioForm({ m, draft, problems, onChange, onSave, busy }: Props): ReactElement {
  const texts = problemText(m);
  return (
    <form
      className="flex flex-col gap-3"
      aria-label={m.newTitle}
      onSubmit={(e) => {
        e.preventDefault();
        onSave();
      }}
    >
      <h2 className="text-label font-medium">{m.newTitle}</h2>
      <div className="flex gap-3">
        <Field label={m.fileLabel}>
          <Input value={draft.file} onChange={(e) => onChange({ ...draft, file: e.target.value })} />
        </Field>
        <Field label={m.nameLabel}>
          <Input value={draft.name} onChange={(e) => onChange({ ...draft, name: e.target.value })} />
        </Field>
        <Field label={m.pageLabel}>
          <Input value={draft.page} onChange={(e) => onChange({ ...draft, page: e.target.value })} />
        </Field>
      </div>
      <h3 className="text-label font-medium">{m.stepsHeading}</h3>
      <ul className="flex flex-col gap-2">
        {draft.steps.map((s, i) => (
          <StepRow
            // biome-ignore lint/suspicious/noArrayIndexKey: rows are positional and have no identity
            key={i}
            m={m}
            step={s}
            index={i}
            onChange={(v) => onChange({ ...draft, steps: replaceAt(draft.steps, i, v) })}
          />
        ))}
      </ul>
      <div>
        <Button
          variant="secondary"
          type="button"
          onClick={() => onChange({ ...draft, steps: [...draft.steps, { kind: "tap", testId: "", text: "" }] })}
        >
          {m.addStep}
        </Button>
      </div>
      <h3 className="text-label font-medium">{m.expectsHeading}</h3>
      <ul className="flex flex-col gap-2">
        {draft.expects.map((x, i) => (
          <ExpectRow
            // biome-ignore lint/suspicious/noArrayIndexKey: rows are positional and have no identity
            key={i}
            m={m}
            expect={x}
            index={i}
            onChange={(v) => onChange({ ...draft, expects: replaceAt(draft.expects, i, v) })}
          />
        ))}
      </ul>
      <div>
        <Button
          variant="secondary"
          type="button"
          onClick={() => onChange({ ...draft, expects: [...draft.expects, { kind: "visible", value: "", text: "" }] })}
        >
          {m.addExpect}
        </Button>
      </div>
      {problems.map((p) => (
        <p key={p} role="note" className="text-body text-secondary">
          {texts[p]}
        </p>
      ))}
      <div>
        <Button type="submit" disabled={problems.length > 0 || busy}>
          {m.save}
        </Button>
      </div>
    </form>
  );
}
