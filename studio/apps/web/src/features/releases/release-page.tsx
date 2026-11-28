// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button, Field, Select, StatusPill } from "@plux/ui";
import { useParams } from "@tanstack/react-router";
import { type ReactElement, useState } from "react";
import { PageEmpty } from "../../components/page.tsx";
import { firstError, plxCode, reasonOf, withReason } from "../../components/refusal.tsx";
import { useEnvironments } from "../functions/use-functions.ts";
import { Table } from "../security/parts.tsx";
import { useEnvironment, usePermissions, useScope } from "../security/use-security.ts";
import { Frame } from "./frame.tsx";
import { useGuarded } from "./guarded.tsx";
import { releasesMessages, stateNames } from "./releases.messages.ts";
import { type Check, defaultChannel, parseCheck } from "./releases.schema.ts";
import { pillOf } from "./releases-page.tsx";
import { RolloutControls } from "./rollout-controls.tsx";
import { useChangelog, useRelease, useReleaseActions, useRollouts } from "./use-releases.ts";

function HealthTable({ check }: { readonly check: Check }): ReactElement {
  const m = releasesMessages(useLingui().i18n);
  const row = (key: string, name: string, a: number, b: number) => ({
    key,
    cells: [name, String(a), String(b)],
  });
  return (
    <>
      <Table
        caption={m.health}
        heads={[m.colMetric, m.colCandidate, m.colPrevious]}
        rows={[
          row("s", m.sessions, check.candidate.Sessions, check.previous.Sessions),
          row("c", m.crashed, check.candidate.Crashed, check.previous.Crashed),
          row("e", m.errors, check.candidate.Errors, check.previous.Errors),
        ]}
      />
      {(check.regressions ?? []).length > 0 && (
        <p role="status" className="text-body text-danger">
          {m.regressions((check.regressions ?? []).map((r) => r.metric).join(", "))}
        </p>
      )}
    </>
  );
}

/** D45: rolling back makes a new release with this content, after the step-up D3 (REL-006, SEC-103). */
function RollbackButton({
  app,
  environmentId,
  channel,
  sequence,
}: {
  readonly app: string;
  readonly environmentId: string;
  readonly channel: string;
  readonly sequence: bigint;
}): ReactElement {
  const m = releasesMessages(useLingui().i18n);
  const actions = useReleaseActions(app);
  const { guard, dialog, failed, error } = useGuarded();
  const mayPromote = usePermissions().has("release.promote");
  return (
    <div className="flex flex-col gap-1">
      <Button
        variant="danger"
        disabled={!mayPromote || environmentId === ""}
        onClick={() => guard(() => actions.rollback(environmentId, channel, sequence))}
      >
        {m.rollback}
      </Button>
      <p className="text-caption text-muted-foreground">{m.rollbackHint}</p>
      {failed && (
        <p role="alert" className="text-body text-danger">
          {withReason(m.actionFailed, error)}
        </p>
      )}
      {dialog}
    </div>
  );
}

/**
 * Promotes this release, unchanged, to another environment's channel (REL-004), after the step-up D3 (SEC-103). The
 * server applies the target's approval policy: a promotion it holds opens the approval request, and the panel says so
 * with the server's reason (GOV-020, SRV-006).
 */
function PromotePanel({
  app,
  currentEnv,
  channel,
  sequence,
}: {
  readonly app: string;
  readonly currentEnv: string;
  readonly channel: string;
  readonly sequence: bigint;
}): ReactElement {
  const m = releasesMessages(useLingui().i18n);
  const actions = useReleaseActions(app);
  const { guard, dialog, failed, error } = useGuarded();
  const mayPromote = usePermissions().has("release.promote");
  const targets = (useEnvironments(app).data?.environments ?? []).filter((e) => e.key !== currentEnv);
  const [chosen, setChosen] = useState("");
  const [promoted, setPromoted] = useState("");
  const target = targets.find((e) => e.key === chosen) ?? targets[0];
  const awaitsApproval = failed && plxCode(error) === "PLX-8001";
  return (
    <section className="flex flex-col gap-2" aria-labelledby="promote-title">
      <h2 id="promote-title" className="text-heading">
        {m.promoteTitle}
      </h2>
      <p className="text-caption text-muted-foreground">{m.promoteHint}</p>
      {target === undefined ? (
        <PageEmpty icon="rocket-launch" text={m.promoteNoTarget} />
      ) : (
        <>
          <Field label={m.promoteTarget}>
            <Select
              options={targets.map((e) => ({ value: e.key, label: e.name || e.key }))}
              value={target.key}
              onValueChange={setChosen}
            />
          </Field>
          <div>
            <Button
              disabled={!mayPromote}
              onClick={() => {
                const { id, key, name } = target;
                setPromoted("");
                guard(() => actions.promote(id, channel, sequence).then(() => setPromoted(name || key)));
              }}
            >
              {m.promote}
            </Button>
          </div>
        </>
      )}
      {promoted !== "" && (
        <p role="status" className="text-body">
          {m.promoted(String(sequence), promoted)}
        </p>
      )}
      {awaitsApproval && (
        <p role="status" className="text-body">
          {m.promoteAwaitsApproval} <span className="text-muted-foreground">{reasonOf(error)}</span>
        </p>
      )}
      {failed && !awaitsApproval && (
        <p role="alert" className="text-body text-danger">
          {withReason(m.actionFailed, error)}
        </p>
      )}
      {dialog}
    </section>
  );
}

/** A4: the content, changelog, adoption and health of one release against the previous one, its rollout controls D44 and the roll back D45. */
export function ReleasePage(): ReactElement {
  const m = releasesMessages(useLingui().i18n);
  const { app, env } = useScope();
  const sequence = BigInt((useParams({ strict: false }) as { releaseId?: string }).releaseId ?? "0");
  const environment = useEnvironment(app, env);
  const release = useRelease(app, sequence);
  const changelog = useChangelog(app, sequence);
  const rollouts = useRollouts(app);
  const allowed = usePermissions().has("rollout.manage");
  const rollout = (rollouts.data?.rollouts ?? []).find(
    (r) => r.candidateSequence === sequence && r.environmentKey === env,
  );
  const check = rollout === undefined ? undefined : parseCheck(rollout.lastCheck);
  const environmentId = environment.environment?.id ?? "";
  const channel = rollout?.channel ?? defaultChannel;
  return (
    <Frame
      id="release-title"
      title={m.detailTitle(String(sequence))}
      loading={environment.loading || release.isPending || rollouts.isPending}
      failed={environment.failed || release.isError || rollouts.isError}
      error={firstError(release.error, rollouts.error)}
    >
      <h2 className="text-heading">{m.content}</h2>
      <Table
        caption={m.content}
        heads={[m.colRelease, m.bundle]}
        rows={(release.data?.versions ?? []).map((v) => ({
          key: v.id,
          cells: [`${v.pluginId} ${String(v.version)}`, ""],
        }))}
      />
      <p className="text-body">
        {m.bundle}: <span className="font-mono">{release.data?.release?.appBundleSha256 ?? ""}</span>
      </p>
      <h2 className="text-heading">{m.changelog}</h2>
      {(changelog.data?.entries ?? []).length === 0 ? (
        <PageEmpty icon="rocket-launch" text={m.noChanges} />
      ) : (
        <ul className="text-body">
          {(changelog.data?.entries ?? []).map((e) => (
            <li key={`${e.kind}:${e.pluginKey}:${e.name}`}>{`${e.change} ${e.kind} ${e.pluginKey} ${e.name}`}</li>
          ))}
        </ul>
      )}
      {rollout !== undefined && (
        <>
          <h2 className="text-heading">{m.adoption}</h2>
          <p className="text-body">
            <StatusPill status={pillOf(rollout.state)} label={stateNames(m)[rollout.state] ?? ""} />{" "}
            <span className="font-mono">{rollout.basisPoints / 100}%</span>{" "}
            {m.stage(String(rollout.stageIndex + 1), String(rollout.stages.length))}
          </p>
          <h2 className="text-heading">{m.health}</h2>
          {check === undefined ? <PageEmpty icon="rocket-launch" text={m.noHealth} /> : <HealthTable check={check} />}
          <RolloutControls app={app} rollout={rollout} allowed={allowed} />
        </>
      )}
      <PromotePanel app={app} currentEnv={env} channel={channel} sequence={sequence} />
      <RollbackButton app={app} environmentId={environmentId} channel={channel} sequence={sequence} />
    </Frame>
  );
}
