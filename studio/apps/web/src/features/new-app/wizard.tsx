// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { ConnectError } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { Button, Dialog } from "@plux/ui";
import { useQueryClient } from "@tanstack/react-query";
import { useRouter } from "@tanstack/react-router";
import { type ReactElement, useState } from "react";
import { dialogMessages } from "../../components/dialogs/dialogs.messages.ts";
import { DoneStep } from "./done-step.tsx";
import { type Draft, initialDraft, problemOf, type StepId, stepsOf } from "./draft.ts";
import { newAppMessages } from "./new-app.messages.ts";
import {
  DetailsStep,
  EnvironmentsStep,
  LocalesStep,
  SecurityStep,
  SourceStep,
  type StepProps,
  ThemeStep,
} from "./steps.tsx";
import { type CreatedApp, type CreateStage, useCreateApp } from "./use-new-app.ts";

const stepViews: Readonly<Record<Exclude<StepId, "done">, (props: StepProps) => ReactElement>> = {
  source: SourceStep,
  details: DetailsStep,
  locales: LocalesStep,
  theme: ThemeStep,
  security: SecurityStep,
  environments: EnvironmentsStep,
};

export interface NewAppWizardProps {
  readonly open: boolean;
  readonly onOpenChange: (open: boolean) => void;
  /** The organisation's immutable ID, which the requests carry. */
  readonly organizationId: string;
  /** The `:org` URL parameter, which the canvas path continues. */
  readonly org: string;
}

/**
 * O3: the new-app wizard (STU-024). It starts from a blank app, a template or a `.plux` archive; a blank app then
 * asks for languages, theme, security profile and environments, and every new app starts with a default plugin and
 * its entry screen. The last step offers the generated Flutter project for download (GEN-001).
 */
export function NewAppWizard({ open, onOpenChange, organizationId, org }: NewAppWizardProps): ReactElement {
  const m = newAppMessages(useLingui().i18n);
  const d = dialogMessages(useLingui().i18n);
  const queryClient = useQueryClient();
  const router = useRouter();
  const createApp = useCreateApp(organizationId);
  const [draft, setDraft] = useState<Draft>(initialDraft);
  const [at, setAt] = useState(0);
  const [attempted, setAttempted] = useState(false);
  const [stage, setStage] = useState<CreateStage | undefined>(undefined);
  const [refusal, setRefusal] = useState<string | undefined>(undefined);
  const [created, setCreated] = useState<CreatedApp | undefined>(undefined);
  const steps = stepsOf(draft.source);
  const step = steps[Math.min(at, steps.length - 1)] ?? "source";
  const names: Readonly<Record<StepId, string>> = {
    source: m.stepSource,
    details: m.stepDetails,
    locales: m.stepLocales,
    theme: m.stepTheme,
    security: m.stepSecurity,
    environments: m.stepEnvironments,
    done: m.stepDone,
  };
  const reset = () => {
    setDraft(initialDraft);
    setAt(0);
    setAttempted(false);
    setStage(undefined);
    setRefusal(undefined);
    setCreated(undefined);
  };
  const close = (next: boolean) => {
    if (!next) {
      reset();
    }
    onOpenChange(next);
  };
  const update = (patch: Partial<Draft>) => setDraft((current) => ({ ...current, ...patch }));
  const problem = problemOf(draft, step);
  const last = steps[at + 1] === "done";
  const working = stage !== undefined;
  const create = async () => {
    setStage("preparing");
    setRefusal(undefined);
    try {
      const made = await createApp(draft, setStage);
      await queryClient.invalidateQueries({ queryKey: ["apps"] });
      setCreated(made);
      setAt(at + 1);
    } catch (error) {
      const raw = error instanceof ConnectError ? error.rawMessage : "";
      setRefusal(raw === "" ? d.createFailed : raw);
    } finally {
      setStage(undefined);
    }
  };
  const next = () => {
    setAttempted(true);
    if (problem !== undefined || working) {
      return;
    }
    setAttempted(false);
    if (last) {
      void create();
    } else {
      setAt(at + 1);
    }
  };
  const openApp = (id: string) => {
    close(false);
    router.history.push(`/o/${org}/apps/${id}`);
  };
  const View = step === "done" ? undefined : stepViews[step];
  return (
    <Dialog open={open} onOpenChange={close} size="lg" title={m.title} labels={{ close: d.close }}>
      <ol aria-label={m.stepsLabel} className="flex flex-wrap gap-2 text-caption text-secondary">
        {steps.map((id, index) => (
          <li
            key={id}
            {...(index === at && { "aria-current": "step" as const })}
            className={index === at ? "font-medium text-primary" : undefined}
          >
            <span className="font-mono">{index + 1}</span> {names[id]}
          </li>
        ))}
      </ol>
      <h2 className="text-section">{m.stepOf(at + 1, steps.length, names[step])}</h2>
      {created !== undefined && step === "done" ? (
        <DoneStep created={created} onOpen={() => openApp(created.id)} onClose={() => close(false)} />
      ) : (
        <form
          noValidate
          onSubmit={(e) => {
            e.preventDefault();
            next();
          }}
          className="flex flex-col gap-3"
        >
          {View !== undefined && <View draft={draft} update={update} attempted={attempted} problem={problem} />}
          {stage !== undefined && (
            <div className="flex flex-col gap-1">
              <p role="status" className="text-body text-secondary">
                {stage === "preparing" ? m.progressPreparing : m.progressCreating}
              </p>
              <progress aria-label={m.progressLabel} max={2} value={stage === "preparing" ? 1 : 2} className="w-full" />
            </div>
          )}
          {refusal !== undefined && (
            <p role="alert" className="text-body text-danger">
              {refusal}
            </p>
          )}
          <div className="flex justify-end gap-2">
            <Button type="button" variant="secondary" onClick={() => close(false)}>
              {d.cancel}
            </Button>
            {at > 0 && (
              <Button
                type="button"
                variant="secondary"
                disabled={working}
                onClick={() => {
                  setAt(at - 1);
                  setAttempted(false);
                  setRefusal(undefined);
                }}
              >
                {m.back}
              </Button>
            )}
            <Button type="submit" loading={working} loadingLabel={m.creating}>
              {last ? m.title : m.next}
            </Button>
          </div>
        </form>
      )}
    </Dialog>
  );
}
