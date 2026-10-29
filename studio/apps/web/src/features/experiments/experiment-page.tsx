// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button, Field, Input, StatusPill } from "@plux/ui";
import { useParams } from "@tanstack/react-router";
import { type ReactElement, useState } from "react";
import { Frame } from "../releases/frame.tsx";
import { useGuarded } from "../releases/guarded.tsx";
import { Table } from "../security/parts.tsx";
import { useEnvironment, usePermissions, useScope } from "../security/use-security.ts";
import { type ExperimentsMessages, experimentsMessages, stateNames } from "./experiments.messages.ts";
import { parseDefinition } from "./experiments.schema.ts";
import { useExperiment, useExperimentActions, useResults } from "./use-experiments.ts";

type Metric = NonNullable<ReturnType<typeof useResults>["data"]>["primary"];

const fixed = (n: number) => (Number.isFinite(n) ? n.toFixed(4) : "");

function verdict(m: ExperimentsMessages, c: { guardrailTripped: boolean; improved: boolean; worsened: boolean }) {
  if (c.guardrailTripped) {
    return m.guardrailTripped;
  }
  if (c.improved) {
    return m.improved;
  }
  return c.worsened ? m.worsened : m.inconclusive;
}

function MetricTable({
  m,
  title,
  metric,
}: {
  readonly m: ExperimentsMessages;
  readonly title: string;
  readonly metric: Metric;
}): ReactElement {
  return (
    <>
      <h3 className="text-heading">{`${title}: ${metric?.event ?? ""}`}</h3>
      <Table
        caption={title}
        heads={[m.colVariant, m.colSubjects, m.colMean, m.colDiff, m.colInterval, m.colP, m.colVerdict]}
        rows={(metric?.means ?? []).map((mean) => {
          const c = metric?.comparisons.find((x) => x.variant === mean.variant);
          return {
            key: mean.variant,
            cells: [
              mean.variant,
              String(mean.subjects),
              fixed(mean.mean),
              c === undefined ? "" : fixed(c.diff),
              c === undefined ? "" : `${fixed(c.lower)} to ${fixed(c.upper)}`,
              c === undefined ? "" : fixed(c.pValue),
              c === undefined ? "" : verdict(m, c),
            ],
          };
        })}
      />
    </>
  );
}

interface DecisionProps {
  readonly m: ExperimentsMessages;
  readonly id: string;
  readonly state: number;
  readonly variants: readonly { name: string; control: boolean }[];
  readonly allowed: boolean;
  readonly actions: ReturnType<typeof useExperimentActions>;
  readonly guard: (action: () => Promise<unknown>) => void;
  readonly onShipped: () => void;
}

/** D48: start a draft; stop a running experiment or ship a variant, both behind the step-up. */
function DecisionPanel({ m, id, state, variants, allowed, actions, guard, onShipped }: DecisionProps): ReactElement {
  const [label, setLabel] = useState("");
  return (
    <>
      <h2 className="text-heading">{m.decision}</h2>
      {state === 1 && (
        <Button disabled={!allowed} onClick={() => void actions.start(id)}>
          {m.start}
        </Button>
      )}
      {state === 2 && (
        <Button variant="danger" disabled={!allowed} onClick={() => guard(() => actions.stop(id))}>
          {m.stop}
        </Button>
      )}
      {state >= 2 && (
        <>
          <Field label={m.shipLabel}>
            <Input value={label} onChange={(e) => setLabel(e.target.value)} />
          </Field>
          <ul className="flex gap-2">
            {variants
              .filter((v) => !v.control)
              .map((v) => (
                <li key={v.name}>
                  <Button
                    disabled={!allowed || label.trim() === ""}
                    aria-label={m.shipNamed(v.name)}
                    onClick={() => guard(() => actions.ship(id, v.name, label.trim()).then(() => onShipped()))}
                  >
                    {m.ship}
                  </Button>
                </li>
              ))}
          </ul>
        </>
      )}
    </>
  );
}

type Results = ReturnType<typeof useResults>["data"];

/** The sample split and the comparison of every primary metric and guardrail. */
function ResultsPanel({ m, r }: { readonly m: ExperimentsMessages; readonly r: Results }): ReactElement {
  if (r === undefined) {
    return <p className="text-body text-secondary">{m.noResults}</p>;
  }
  return (
    <>
      {r.sampleRatioMismatch && (
        <p role="alert" className="text-body text-danger">
          {m.mismatch}
        </p>
      )}
      <Table
        caption={m.results}
        heads={[m.colVariant, m.colSubjects, m.colShare]}
        rows={r.variants.map((v) => ({ key: v.name, cells: [v.name, String(v.subjects), `${fixed(v.share)}`] }))}
      />
      {(r.primaries.length > 0 ? r.primaries : [r.primary]).map((p) => (
        <MetricTable key={p?.event ?? ""} m={m} title={m.primaryMetric} metric={p} />
      ))}
      {r.guardrails.map((g) => (
        <MetricTable key={g.event} m={m} title={m.guardrails} metric={g} />
      ))}
    </>
  );
}

/** A8: definition, live results and the decision (stop or ship, D48) of one experiment. */
export function ExperimentPage(): ReactElement {
  const m = experimentsMessages(useLingui().i18n);
  const { app, env } = useScope();
  const id = (useParams({ strict: false }) as { expId?: string }).expId ?? "";
  const environment = useEnvironment(app, env);
  const experiment = useExperiment(id);
  const results = useResults(id);
  const actions = useExperimentActions(app, environment.environment?.id ?? "");
  const { guard, dialog, failed } = useGuarded();
  const allowed = usePermissions().has("experiments.manage");
  const [shipped, setShipped] = useState(false);
  const x = experiment.data?.experiment;
  const def = parseDefinition(x?.definitionJson ?? "");
  const running = x?.state === 2;
  const r = results.data;
  return (
    <Frame
      id="experiment-title"
      title={m.forKey(x?.key ?? "")}
      loading={environment.loading || experiment.isPending}
      failed={environment.failed || experiment.isError}
    >
      <StatusPill status={running ? "success" : "neutral"} label={stateNames(m)[x?.state ?? 0] ?? ""} />
      <h2 className="text-heading">{m.definition}</h2>
      <Table
        caption={m.definition}
        heads={[m.colVariant, m.colShare]}
        rows={def.variants.map((v) => ({
          key: v.name,
          cells: [v.control ? `${v.name} (${m.variantControl})` : v.name, `${v.weight}%`],
        }))}
      />
      <h2 className="text-heading">{m.results}</h2>
      <ResultsPanel m={m} r={r} />
      <DecisionPanel
        m={m}
        id={id}
        state={x?.state ?? 0}
        variants={def.variants}
        allowed={allowed}
        actions={actions}
        guard={guard}
        onShipped={() => setShipped(true)}
      />
      {shipped && (
        <p role="status" className="text-body">
          {m.shipped}
        </p>
      )}
      {failed && (
        <p role="alert" className="text-body text-danger">
          {m.actionFailed}
        </p>
      )}
      {dialog}
    </Frame>
  );
}
