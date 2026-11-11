// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import type { BuildRef, PublishJob } from "@plux/api";
import { Button, StatusPill } from "@plux/ui";
import { type ReactElement, useState } from "react";
import { bundleName } from "./bundle-name.ts";
import { evidenceMessages } from "./evidence.messages.ts";
import { PluginReport } from "./evidence.tsx";
import type { PanelProps } from "./panel.ts";
import { publishMessages } from "./publish.messages.ts";
import { type PluginEvidence, useEvidence, type WarningDetail } from "./use-evidence.ts";
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
            <span className="font-mono">{bundleName(p.pluginKey, m.appBundle)}</span>
            <span className="text-muted-foreground">
              {p.reason === "dependency" ? m.reasonDependency : m.reasonChanged}
            </span>
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
  // Why a build failed: the job's own diagnostics, as the server reported them (code, message, where).
  const reasons = state === "failed" || state === "cancelled" ? (job?.diagnostics ?? []) : [];
  return (
    <li className="flex flex-col gap-1 text-body">
      <span className="flex items-center gap-3">
        <span className="font-mono">{bundleName(pluginKey, m.appBundle)}</span>
        <StatusPill status={jobKind(state)} label={jobLabel(m, state)} />
        {state === "running" && (
          <span className="text-muted-foreground">{m.buildProgress(String(job?.percent ?? 0), job?.stage ?? "")}</span>
        )}
        {state === "succeeded" && (
          <span className="text-muted-foreground">{m.buildVersion(String(job?.version ?? 0n))}</span>
        )}
      </span>
      {reasons.length > 0 && (
        <ul aria-label={m.buildFailed} className="flex flex-col gap-1 text-sm text-danger">
          {reasons.map((d, i) => (
            // biome-ignore lint/suspicious/noArrayIndexKey: diagnostics have no identity of their own
            <li key={i}>
              <span className="font-mono">{d.code}</span> {d.message}
              {d.location?.file ? <span className="text-muted-foreground"> ({d.location.file})</span> : null}
            </li>
          ))}
        </ul>
      )}
    </li>
  );
}

/** Step 2: starts one publish job per plugin (idempotent, so a reload re-attaches) and shows their progress (SRV-050). */
export function BuildPanel({ request, editable, busy, api, run, pluginIds }: PanelProps): ReactElement {
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
      <p className="text-body text-muted-foreground">{m.buildsHint}</p>
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
export function ReportsPanel({ request, editable, busy, api, run, pluginIds }: PanelProps): ReactElement {
  const m = publishMessages(useLingui().i18n);
  const e = evidenceMessages(useLingui().i18n);
  const evidence = useEvidence(request, pluginIds);
  const [open, setOpen] = useState(false);
  const warnings = [...new Set(request.builds.flatMap((b) => b.warnings))];
  const blocked = request.hold === "blocked" || request.builds.some((b) => b.errors > 0);
  const missing = warnings.filter((w) => !request.acknowledged.includes(w));
  const pass = (acknowledged: string[]) =>
    run(() => api.advance({ case: "reports", value: { acknowledgedWarnings: acknowledged } }));
  return (
    <div className="flex flex-col gap-3">
      <p className="text-body text-muted-foreground">
        {m.reportValidation} · {m.reportSize} · {m.reportAccessibility}
      </p>
      <ul className="flex flex-col gap-2">
        {request.builds.map((b) => (
          <li key={b.pluginKey} className="flex flex-wrap items-center gap-3 text-body">
            <span className="font-mono">{bundleName(b.pluginKey, m.appBundle)}</span>
            <span className="font-mono text-muted-foreground">{m.reportBundle(b.bundleSha256.slice(0, 8))}</span>
            {b.errors > 0 && <StatusPill status="danger" label={m.reportErrors(String(b.errors))} />}
            {b.warnings.length > 0 && (
              <StatusPill status="warning" label={m.reportWarnings(String(b.warnings.length))} />
            )}
            {b.errors === 0 && b.warnings.length === 0 && <span className="text-muted-foreground">{m.reportOk}</span>}
          </li>
        ))}
      </ul>
      {evidence.isPending && request.builds.length > 0 && pluginIds.size > 0 && (
        <p role="status" className="text-body text-muted-foreground">
          {e.loading}
        </p>
      )}
      {evidence.isError && (
        <p role="alert" className="text-body text-danger">
          {e.failed}
        </p>
      )}
      {evidence.data !== undefined && (
        <ul className="flex flex-col gap-2">
          {evidence.data.map((x) => (
            <PluginReport key={x.pluginKey} evidence={x} />
          ))}
        </ul>
      )}
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
          details={warningDetails(evidence.data)}
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

/** The builds' warnings by their `code@file` name, for the acknowledgement (SRV-051). */
function warningDetails(
  evidence: readonly PluginEvidence[] | undefined,
): ReadonlyMap<string, readonly WarningDetail[]> {
  const out = new Map<string, WarningDetail[]>();
  for (const e of evidence ?? []) {
    for (const w of e.warnings) {
      out.set(w.id, [...(out.get(w.id) ?? []), w]);
    }
  }
  return out;
}
