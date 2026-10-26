// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { Button, Field, Icon, Input, Select } from "@plux/ui";
import { Handle, type NodeProps, Position } from "@xyflow/react";
import { lazy, Suspense } from "react";
import type { ActionGraphMessages } from "./action-graph.messages.ts";
import { useGraph } from "./graph-context.ts";
import {
  canConnect,
  connectRecipe,
  entryOf,
  exprOf,
  inputAt,
  literalText,
  literalValue,
  makeEntryRecipe,
  type Port,
  portsOf,
  removeStepRecipe,
  type Step,
  type StepField,
  setInputRecipe,
  targetOf,
} from "./graph-model.ts";

const PxlEditor = lazy(() => import("../panels/pxl-editor.tsx"));

const NONE = "-";

function InputRow({ step, field }: { step: Step; field: StepField }) {
  const g = useGraph();
  const value = inputAt(step, field.path);
  const expr = exprOf(value);
  const label = field.name;
  const group = `graph:${step.id}:${field.path.join(".")}`;
  const set = (v: unknown) => g.apply(setInputRecipe(g.location, step.id, field.path, v), group);
  const source = expr === undefined ? undefined : g.pxlSource(step, field);
  const plain = (
    <Input
      value={expr ?? ""}
      readOnly={!g.canEdit}
      aria-label={g.m.pxlNamed(label)}
      onChange={(e) => set(e.target.value === "" ? undefined : { $expr: e.target.value })}
    />
  );
  return (
    <div className="flex flex-col gap-1">
      <Field label={field.required ? `${label} (${g.m.required})` : label} description={field.type}>
        {expr === undefined ? (
          <Input
            value={literalText(value)}
            readOnly={!g.canEdit}
            aria-label={g.m.valueNamed(label)}
            onChange={(e) => set(literalValue(e.target.value, field.type))}
          />
        ) : source === undefined ? (
          plain
        ) : (
          <Suspense fallback={plain}>
            <PxlEditor
              value={expr}
              label={g.m.pxlNamed(label)}
              readOnly={!g.canEdit}
              source={source}
              onChange={(text) => set(text === "" ? undefined : { $expr: text })}
              onBlur={() => undefined}
            />
          </Suspense>
        )}
      </Field>
      <Button
        size="sm"
        variant="ghost"
        aria-pressed={expr !== undefined}
        disabled={!g.canEdit}
        onClick={() => set(expr === undefined ? { $expr: literalText(value) } : literalValue(expr, field.type))}
      >
        <Icon name="pencil-simple" />
        {g.m.pxl}
      </Button>
    </div>
  );
}

function PortRow({ step, port, m }: { step: Step; port: Port; m: ActionGraphMessages }) {
  const g = useGraph();
  const target = targetOf(step, port);
  const options = [
    { value: NONE, label: m.nowhere },
    ...g.steps.filter((s) => canConnect(g.steps, step.id, s.id)).map((s) => ({ value: s.id, label: s.id })),
  ];
  return (
    <div className="relative flex flex-col gap-1">
      <Select
        label={m.continuesTo(port.name)}
        value={target ?? NONE}
        options={options}
        disabled={!g.canEdit}
        onValueChange={(v) => g.apply(connectRecipe(g.location, step.id, port, v === NONE ? undefined : v))}
      />
      {port.type !== "" && <span className="font-mono text-tag text-tertiary">{m.outputType(port.type)}</span>}
      <Handle
        type="source"
        position={Position.Right}
        id={port.name}
        isConnectable={g.canEdit}
        aria-label={m.outgoing(port.name)}
      />
    </div>
  );
}

/** One step of the graph: a small form, so input methods, focus and screen readers work as in any panel (A4.5, STU-011). */
export function StepNode({ id }: NodeProps) {
  const g = useGraph();
  const step = g.steps.find((s) => s.id === id);
  if (step === undefined) {
    return null;
  }
  const entry = entryOf(step, g.catalogue);
  const found = g.problems.get(step.id) ?? [];
  const first = g.steps[0]?.id === step.id;
  return (
    <section
      aria-label={step.id}
      className="flex w-72 flex-col gap-2 rounded-xl border border-hairline bg-card p-3 text-primary"
    >
      <Handle type="target" position={Position.Left} id="in" aria-label={g.m.incoming} isConnectable={g.canEdit} />
      <header className="flex items-center gap-2">
        <span className="min-w-0 flex-1 truncate font-mono text-data">{step.id}</span>
        {first && <span className="text-tag text-secondary">{g.m.entry}</span>}
        {found.length > 0 && (
          <span role="status" className="flex items-center gap-1 text-caption text-danger">
            <Icon name="x-circle" />
            {g.m.stepProblems(found.length)}
          </span>
        )}
      </header>
      <p className="truncate font-mono text-tag text-secondary">
        {entry?.label ?? step.action} · {entry?.category ?? ""}
      </p>
      <div className="nodrag nopan flex flex-col gap-2">
        {(entry?.fields ?? []).map((f) => (
          <InputRow key={f.path.join(".")} step={step} field={f} />
        ))}
        {portsOf(entry, step).map((p) => (
          <PortRow key={p.name} step={step} port={p} m={g.m} />
        ))}
        {g.canEdit && (
          <div className="flex gap-2">
            {!first && (
              <Button size="sm" variant="secondary" onClick={() => g.apply(makeEntryRecipe(g.location, step.id))}>
                {g.m.makeEntry}
              </Button>
            )}
            <Button
              size="sm"
              variant="ghost"
              aria-label={g.m.removeNamed(step.id)}
              onClick={() => g.apply(removeStepRecipe(g.location, step.id))}
            >
              <Icon name="trash" />
              {g.m.removeStep}
            </Button>
          </div>
        )}
      </div>
    </section>
  );
}
