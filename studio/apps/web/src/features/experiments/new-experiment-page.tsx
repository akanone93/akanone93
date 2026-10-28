// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button, Field, Input, Select, Textarea } from "@plux/ui";
import { useNavigate, useParams, useSearch } from "@tanstack/react-router";
import { type ReactElement, useState } from "react";
import { Frame } from "../releases/frame.tsx";
import { useEnvironment, useScope } from "../security/use-security.ts";
import { type ExperimentsMessages, experimentsMessages } from "./experiments.messages.ts";
import {
  buildDocument,
  type Draft,
  type DraftVariant,
  initialDraft,
  problems,
  type Scope,
  scopes,
  type WizardStep,
  wizardSteps,
} from "./experiments.schema.ts";
import { useExperimentActions } from "./use-experiments.ts";

interface PanelProps {
  readonly m: ExperimentsMessages;
  readonly draft: Draft;
  readonly set: (patch: Partial<Draft>) => void;
}

const numberOf = (raw: string) => (raw === "" || Number.isNaN(Number(raw)) ? 0 : Number(raw));

function scopeNames(m: ExperimentsMessages): Readonly<Record<Scope, string>> {
  return {
    page: m.scopePage,
    subtree: m.scopeSubtree,
    prop: m.scopeProp,
    flow: m.scopeFlow,
    pluginVersion: m.scopePluginVersion,
  };
}

function TargetPanel({ m, draft, set }: PanelProps): ReactElement {
  const names = scopeNames(m);
  return (
    <>
      <Field label={m.keyLabel}>
        <Input value={draft.key} onChange={(e) => set({ key: e.target.value })} />
      </Field>
      <Field label={m.scopeLabel}>
        <Select
          options={scopes.map((s) => ({ value: s, label: names[s] }))}
          value={draft.scope}
          onValueChange={(v) => set({ scope: v as Scope })}
        />
      </Field>
      <Field label={m.targetLabel}>
        <Input value={draft.target} onChange={(e) => set({ target: e.target.value })} />
      </Field>
    </>
  );
}

function VariantRow({
  m,
  v,
  index,
  change,
  remove,
  weight,
}: {
  readonly m: ExperimentsMessages;
  readonly v: DraftVariant;
  readonly index: number;
  readonly change: (patch: Partial<DraftVariant>) => void;
  readonly remove: () => void;
  readonly weight: boolean;
}): ReactElement {
  return (
    <li key={index} className="flex items-end gap-3">
      <Field label={m.variantName}>
        <Input value={v.name} onChange={(e) => change({ name: e.target.value })} />
      </Field>
      {weight && (
        <Field label={m.weightLabel}>
          <Input
            type="number"
            value={String(v.weight)}
            onChange={(e) => change({ weight: numberOf(e.target.value) })}
          />
        </Field>
      )}
      {!weight && (
        <>
          <label className="flex items-center gap-2 text-body">
            <input type="radio" checked={v.control} onChange={() => change({ control: true })} />
            {m.variantControl}
          </label>
          <Button variant="secondary" aria-label={m.removeNamed(v.name)} onClick={remove}>
            {m.removeVariant}
          </Button>
        </>
      )}
    </li>
  );
}

/** Edits one list of variants; the control radio sits on the variants step, the shares on the allocation step. */
function VariantsPanel({ m, draft, set, weight }: PanelProps & { readonly weight: boolean }): ReactElement {
  const edit = (index: number, patch: Partial<DraftVariant>) =>
    set({
      variants: draft.variants.map((v, i) =>
        i === index ? { ...v, ...patch } : patch.control === true ? { ...v, control: false } : v,
      ),
    });
  return (
    <>
      <ul className="flex flex-col gap-2">
        {draft.variants.map((v, i) => (
          <VariantRow
            key={v.id}
            m={m}
            v={v}
            index={i}
            weight={weight}
            change={(patch) => edit(i, patch)}
            remove={() => set({ variants: draft.variants.filter((_, j) => j !== i) })}
          />
        ))}
      </ul>
      {!weight && (
        <Button
          variant="secondary"
          onClick={() =>
            set({ variants: [...draft.variants, { id: crypto.randomUUID(), name: "", control: false, weight: 0 }] })
          }
        >
          {m.addVariant}
        </Button>
      )}
      {weight && (
        <Field label={m.holdoutLabel}>
          <Input
            type="number"
            value={String(draft.holdout)}
            onChange={(e) => set({ holdout: numberOf(e.target.value) })}
          />
        </Field>
      )}
    </>
  );
}

function MetricsPanel({ m, draft, set }: PanelProps): ReactElement {
  return (
    <>
      <Field label={m.primaryEvent}>
        <Input value={draft.event} onChange={(e) => set({ event: e.target.value })} />
      </Field>
      <Field label={m.primaryKind}>
        <Select
          options={[
            { value: "proportion", label: m.kindProportion },
            { value: "mean", label: m.kindMean },
          ]}
          value={draft.kind}
          onValueChange={(v) => set({ kind: v as Draft["kind"] })}
        />
      </Field>
      <Field label={m.directionLabel}>
        <Select
          options={[
            { value: "increase", label: m.directionIncrease },
            { value: "decrease", label: m.directionDecrease },
          ]}
          value={draft.direction}
          onValueChange={(v) => set({ direction: v as Draft["direction"] })}
        />
      </Field>
      <Field label={m.primariesLabel}>
        <Textarea rows={2} value={draft.primaries} onChange={(e) => set({ primaries: e.target.value })} />
        <span className="text-caption text-secondary">{m.primariesHelp}</span>
      </Field>
      <Field label={m.guardrailEvent}>
        <Textarea rows={2} value={draft.guardrails} onChange={(e) => set({ guardrails: e.target.value })} />
      </Field>
      <Field label={m.minimumSample}>
        <Input
          type="number"
          value={String(draft.minimumSample)}
          onChange={(e) => set({ minimumSample: numberOf(e.target.value) })}
        />
      </Field>
    </>
  );
}

function Panel({ step, ...p }: PanelProps & { readonly step: WizardStep }): ReactElement {
  switch (step) {
    case "target":
      return <TargetPanel {...p} />;
    case "variants":
      return <VariantsPanel {...p} weight={false} />;
    case "allocation":
      return <VariantsPanel {...p} weight />;
    case "targeting":
      return (
        <>
          <Field label={p.m.segmentLabel}>
            <Input value={p.draft.segments} onChange={(e) => p.set({ segments: e.target.value })} />
            <span className="text-caption text-secondary">{p.m.segmentHelp}</span>
          </Field>
          <Field label={p.m.attributesLabel}>
            <Textarea rows={3} value={p.draft.attributes} onChange={(e) => p.set({ attributes: e.target.value })} />
            <span className="text-caption text-secondary">{p.m.attributesHelp}</span>
          </Field>
        </>
      );
    case "metrics":
      return <MetricsPanel {...p} />;
    default:
      return (
        <>
          <Field label={p.m.startLabel}>
            <Input type="datetime-local" value={p.draft.start} onChange={(e) => p.set({ start: e.target.value })} />
          </Field>
          <Field label={p.m.endLabel}>
            <Input type="datetime-local" value={p.draft.end} onChange={(e) => p.set({ end: e.target.value })} />
          </Field>
        </>
      );
  }
}

function problemText(m: ExperimentsMessages, found: readonly string[]): string[] {
  const text: Record<string, string> = {
    weights: m.weightsSum,
    control: m.needControl,
    key: m.invalidForm,
    event: m.invalidForm,
    targeting: m.invalidTargeting,
    metrics: m.invalidMetrics,
    limits: m.limitsExceeded,
  };
  return [...new Set(found.map((p) => text[p] ?? ""))];
}

/** A7: the wizard that creates an experiment; `?target` is the canvas selection that is the variant subject. */
export function NewExperimentPage(): ReactElement {
  const m = experimentsMessages(useLingui().i18n);
  const { app, env } = useScope();
  const search = useSearch({ strict: false }) as { target?: string; step?: WizardStep };
  const org = (useParams({ strict: false }) as { org?: string }).org ?? "";
  const navigate = useNavigate();
  const environment = useEnvironment(app, env);
  const actions = useExperimentActions(app, environment.environment?.id ?? "");
  const [draft, setDraft] = useState(() => initialDraft(search.target));
  const [failed, setFailed] = useState(false);
  const step = search.step ?? "target";
  const index = wizardSteps.indexOf(step);
  const found = problems(draft);
  const go = (to: WizardStep) =>
    void navigate({ to: ".", replace: true, search: (s: object) => ({ ...s, step: to }) } as never);
  const stepNames: Record<WizardStep, string> = {
    target: m.stepTarget,
    variants: m.stepVariants,
    allocation: m.stepAllocation,
    targeting: m.stepTargeting,
    metrics: m.stepMetrics,
    schedule: m.stepSchedule,
  };
  const submit = () => {
    setFailed(false);
    actions.create(buildDocument(draft, crypto.randomUUID())).then(
      (r) =>
        void navigate({
          to: "/o/$org/apps/$app/experiments/$expId",
          params: { org, app, expId: r.experiment?.id ?? "" } as never,
          search: { env },
        } as never),
      () => setFailed(true),
    );
  };
  return (
    <Frame
      id="new-experiment-title"
      title={m.wizardTitle}
      caption={m.wizardCaption}
      loading={environment.loading}
      failed={environment.failed}
    >
      <h2 className="text-heading">{stepNames[step]}</h2>
      <Panel step={step} m={m} draft={draft} set={(patch) => setDraft((d) => ({ ...d, ...patch }))} />
      {step === "schedule" && found.length > 0 && (
        <div role="alert" className="text-body text-danger">
          {problemText(m, found).map((t) => (
            <p key={t}>{t}</p>
          ))}
        </div>
      )}
      {failed && (
        <p role="alert" className="text-body text-danger">
          {m.actionFailed}
        </p>
      )}
      <div className="flex gap-2">
        <Button variant="secondary" disabled={index === 0} onClick={() => go(wizardSteps[index - 1] ?? "target")}>
          {m.back}
        </Button>
        {step === "schedule" ? (
          <Button disabled={found.length > 0} onClick={submit}>
            {m.create}
          </Button>
        ) : (
          <Button onClick={() => go(wizardSteps[index + 1] ?? "schedule")}>{m.next}</Button>
        )}
      </div>
    </Frame>
  );
}
