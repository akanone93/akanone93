// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import type { BuildRef, PublishJob } from "@plux/api";
import { Button, StatusPill } from "@plux/ui";
import { type ReactElement, useState } from "react";
import type { PanelProps } from "./panel.ts";
import { publishMessages } from "./publish.messages.ts";
import { useJobs } from "./use-publish.ts";
import { WarningsDialog } from "./warnings-dialog.tsx";

/** Step 1: the plugins in the request, each with the reason it is there (REL-003). */
export function SelectPanel({ request, editable, busy, api, run }: PanelProps): ReactElement {
  const m = publishMessages(useLingui().i18n);
  return (
    <div className="flex flex-col gap-3">
      <ul className="flex flex-col gap-2">
        {request.plugins.map((p) => (
          <li key={p.pluginKey} className="flex items-center gap-3 text-body">
            <span className="font-mono">{p.pluginKey}</span>
            <span className="text-secondary">{p.reason === "dependency" ? m.reasonDependency : m.reasonChanged}</span>
          </li>
        ))}
      </ul>
      {editable && (
        <div>
          <Button
            disabled={busy}
            onClick={() => run(() => api.advance({ case: "selection", value: { plugins: request.plugins } }))}
          >
            {m.continue}
          </Button>
        </div>
      )}
    </div>
  );
}

function jobLabel(m: ReturnType<typeof publishMessages>, state: string): string {
  const labels: Record<string, string> = {
    queued: m.buildQueued,
    running: m.buildRunning,
    succeeded: m.buildSucceeded,
    failed: m.buildFailed,
    cancelled: m.buildCancelled,
  };
  return labels[state] ?? state;
}

function jobKind(state: string) {
  return state === "succeeded" ? "success" : state === "failed" || state === "cancelled" ? "danger" : "neutral";
}

function JobRow({
  pluginKey,
  job,
}: {
  readonly pluginKey: string;
  readonly job: PublishJob | undefined;
}): ReactElement {
  const m = publishMessages(useLingui().i18n);
  const state = job?.state ?? "queued";
  return (
    <li className="flex items-center gap-3 text-body">
      <span className="font-mono">{pluginKey}</span>
      <StatusPill status={jobKind(state)} label={jobLabel(m, state)} />
      {state === "running" && (
        <span className="text-secondary">{m.buildProgress(String(job?.percent ?? 0), job?.stage ?? "")}</span>
      )}
      {state === "succeeded" && <span className="text-secondary">{m.buildVersion(String(job?.version ?? 0n))}</span>}
    </li>
  );
}

/** Step 2: starts one publish job per plugin (idempotent, so a reload re-attaches) and shows their progress (SRV-050). */
export function BuildPanel({
  request,
  editable,
  busy,
  api,
  run,
  pluginIds,
}: PanelProps & { readonly pluginIds: ReadonlyMap<string, string> }): ReactElement {
  const m = publishMessages(useLingui().i18n);
  const [refs, setRefs] = useState<readonly { pluginKey: string; jobId: string }[]>([]);
  const jobs = useJobs(refs.map((r) => r.jobId));
  const states = jobs.data ?? [];
  const done = refs.length > 0 && states.length === refs.length && states.every((j) => j?.state === "succeeded");
  const failed = request.hold === "failed" || states.some((j) => j?.state === "failed" || j?.state === "cancelled");
  const shown =
    request.builds.length > 0
      ? request.builds.map((b) => ({ pluginKey: b.pluginKey, job: undefined, ref: b }))
      : refs.map((r, i) => ({ pluginKey: r.pluginKey, job: states[i], ref: undefined }));
  return (
    <div className="flex flex-col gap-3">
      <p className="text-body text-secondary">{m.buildsHint}</p>
      <ul className="flex flex-col gap-2">
        {shown.map((s) => (
          <JobRow key={s.pluginKey} pluginKey={s.pluginKey} job={s.job ?? buildAsJob(s.ref)} />
        ))}
      </ul>
      {editable && (
        <div className="flex gap-2">
          {refs.length === 0 && (
            <Button
              disabled={busy}
              onClick={() => run(async () => setRefs(await api.startBuilds(request.plugins, pluginIds)))}
            >
              {m.startBuild}
            </Button>
          )}
          {failed && (
            <Button
              disabled={busy}
              onClick={() =>
                run(async () => {
                  setRefs([]);
                  await api.refreeze();
                })
              }
            >
              {m.refreeze}
            </Button>
          )}
          {done && (
            <Button
              disabled={busy}
              onClick={() => run(() => api.advance({ case: "builds", value: { builds: [...refs] } }))}
            >
              {m.continue}
            </Button>
          )}
        </div>
      )}
    </div>
  );
}

function buildAsJob(b: BuildRef | undefined): PublishJob | undefined {
  return b === undefined ? undefined : ({ state: b.state, version: b.version, percent: 100, stage: "" } as PublishJob);
}

/** Step 3: the validation, size and accessibility findings of each build; errors block, warnings are acknowledged (D42, SRV-051). */
export function ReportsPanel({ request, editable, busy, api, run }: PanelProps): ReactElement {
  const m = publishMessages(useLingui().i18n);
  const [open, setOpen] = useState(false);
  const warnings = [...new Set(request.builds.flatMap((b) => b.warnings))];
  const blocked = request.hold === "blocked" || request.builds.some((b) => b.errors > 0);
  const missing = warnings.filter((w) => !request.acknowledged.includes(w));
  const pass = (acknowledged: string[]) =>
    run(() => api.advance({ case: "reports", value: { acknowledgedWarnings: acknowledged } }));
  return (
    <div className="flex flex-col gap-3">
      <p className="text-body text-secondary">
        {m.reportValidation} · {m.reportSize} · {m.reportAccessibility}
      </p>
      <ul className="flex flex-col gap-2">
        {request.builds.map((b) => (
          <li key={b.pluginKey} className="flex flex-wrap items-center gap-3 text-body">
            <span className="font-mono">{b.pluginKey}</span>
            <span className="font-mono text-secondary">{m.reportBundle(b.bundleSha256.slice(0, 8))}</span>
            {b.errors > 0 && <StatusPill status="danger" label={m.reportErrors(String(b.errors))} />}
            {b.warnings.length > 0 && (
              <StatusPill status="warning" label={m.reportWarnings(String(b.warnings.length))} />
            )}
            {b.errors === 0 && b.warnings.length === 0 && <span className="text-secondary">{m.reportOk}</span>}
          </li>
        ))}
      </ul>
      {blocked && (
        <p role="alert" className="text-body text-danger">
          {m.reportsBlocked}
        </p>
      )}
      {request.acknowledged.length > 0 && (
        <p className="text-body">{m.acknowledged(String(request.acknowledged.length))}</p>
      )}
      {editable && !blocked && (
        <div className="flex gap-2">
          {missing.length > 0 ? (
            <Button disabled={busy} onClick={() => setOpen(true)}>
              {m.acknowledge}
            </Button>
          ) : (
            <Button disabled={busy} onClick={() => pass([...request.acknowledged])}>
              {m.continue}
            </Button>
          )}
        </div>
      )}
      {open && (
        <WarningsDialog
          warnings={missing}
          onClose={() => setOpen(false)}
          onConfirm={(a) => {
            setOpen(false);
            pass([...request.acknowledged, ...a]);
          }}
        />
      )}
    </div>
  );
}
