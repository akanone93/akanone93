// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button, StatusPill } from "@plux/ui";
import { useQueryClient } from "@tanstack/react-query";
import { useNavigate, useParams, useSearch } from "@tanstack/react-router";
import { type ReactElement, useState } from "react";
import { usePermissions } from "../security/use-security.ts";
import { CancelDialog } from "./cancel-dialog.tsx";
import type { PanelProps } from "./panel.ts";
import { publishMessages } from "./publish.messages.ts";
import { isEnded, type Step, stepOf, steps } from "./publish.schema.ts";
import { BuildPanel, ReportsPanel, SelectPanel } from "./steps-build.tsx";
import {
  ApprovalPanel,
  ChangelogPanel,
  DiffsPanel,
  ReleasePanel,
  RolloutPanel,
  TargetPanel,
} from "./steps-release.tsx";
import { codeOf, useReleaseApi, useReleaseRequest, useSummaries, useUserId } from "./use-publish.ts";

type Messages = ReturnType<typeof publishMessages>;

function stepName(m: Messages, step: Step): string {
  const names: Record<Step, string> = {
    select: m.stepSelect,
    build: m.stepBuild,
    reports: m.stepReports,
    diffs: m.stepDiffs,
    changelog: m.stepChangelog,
    target: m.stepTarget,
    approval: m.stepApproval,
    rollout: m.stepRollout,
    release: m.stepRelease,
  };
  return names[step];
}

function errorText(m: Messages, code: string): string {
  const texts: Record<string, string> = { "PLX-8060": m.err8060, "PLX-8061": m.err8061, "PLX-8062": m.err8062 };
  return texts[code] ?? m.errGeneric;
}

function endedText(m: Messages, state: string): string {
  const texts: Record<string, string> = {
    released: m.ended_released,
    rejected: m.ended_rejected,
    expired: m.ended_expired,
    cancelled: m.ended_cancelled,
  };
  return texts[state] ?? state;
}

function holdText(m: Messages, hold: string): string {
  const texts: Record<string, string> = {
    blocked: m.holdBlocked,
    failed: m.holdFailed,
    awaiting_approval: m.holdAwaiting,
  };
  return texts[hold] ?? hold;
}

function Panel({
  step,
  props,
  pluginIds,
}: {
  readonly step: Step;
  readonly props: PanelProps;
  readonly pluginIds: ReadonlyMap<string, string>;
}): ReactElement {
  switch (step) {
    case "select":
      return <SelectPanel {...props} />;
    case "build":
      return <BuildPanel {...props} pluginIds={pluginIds} />;
    case "reports":
      return <ReportsPanel {...props} />;
    case "diffs":
      return <DiffsPanel {...props} />;
    case "changelog":
      return <ChangelogPanel {...props} />;
    case "target":
      return <TargetPanel {...props} />;
    case "approval":
      return <ApprovalPanel {...props} />;
    case "rollout":
      return <RolloutPanel {...props} />;
    default:
      return <ReleasePanel {...props} />;
  }
}

/** A2: the publish flow of one release request; the steps mirror the server state, so a reload resumes where the request is (STU-100, F11). */
export function PublishPage(): ReactElement {
  const m = publishMessages(useLingui().i18n);
  const { app, requestId } = useParams({ strict: false }) as { app: string; requestId: string };
  const search = useSearch({ strict: false }) as { step?: Step };
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const query = useReleaseRequest(app, requestId);
  const summaries = useSummaries(app);
  const can = usePermissions().has("release.publish");
  const me = useUserId();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [cancelling, setCancelling] = useState(false);
  const request = query.data?.request;
  const api = useReleaseApi(app, request ?? { id: requestId, state: "", revision: 0n, environmentId: "" });
  if (request === undefined) {
    return (
      <section className="flex flex-col gap-4" aria-labelledby="publish-title">
        <h1 id="publish-title" className="text-screen">
          {m.title}
        </h1>
        {query.isError ? (
          <p role="alert" className="text-body text-danger">
            {m.loadFailed}
          </p>
        ) : (
          <p role="status" className="text-body text-secondary">
            {m.loading}
          </p>
        )}
      </section>
    );
  }
  const ended = isEnded(request.state);
  const current = stepOf(request.state);
  const viewed = search.step ?? current;
  const own = request.createdBy === me;
  const editable = can && own && !ended && viewed === current && request.hold !== "awaiting_approval";
  const run = (fn: () => Promise<unknown>) => {
    setBusy(true);
    setError("");
    fn()
      .catch((err: unknown) => setError(errorText(m, codeOf(err))))
      .finally(() => {
        setBusy(false);
        void queryClient.invalidateQueries({ queryKey: ["release-request", app, requestId] });
      });
  };
  const pluginIds = new Map((summaries.data?.summaries ?? []).map((s) => [s.plugin?.key ?? "", s.plugin?.id ?? ""]));
  const props: PanelProps = { request, diff: query.data?.visualDiff, api, editable, busy, run };
  return (
    <section className="flex flex-col gap-4" aria-labelledby="publish-title">
      <h1 id="publish-title" className="text-screen">
        {m.title}
      </h1>
      {!own && <p className="text-body text-secondary">{m.othersPublishing(request.createdByName)}</p>}
      {own && !can && <p className="text-body text-secondary">{m.viewOnly}</p>}
      {ended && (
        <StatusPill status={request.state === "released" ? "success" : "neutral"} label={endedText(m, request.state)} />
      )}
      {request.hold !== "" && (
        <StatusPill status="warning" label={holdText(m, request.hold)} value={request.holdReason} />
      )}
      <ol aria-label={m.stepsLabel} className="flex flex-wrap gap-2">
        {steps.map((s, i) => (
          <li key={s}>
            <Button
              size="sm"
              variant={s === viewed ? "primary" : "secondary"}
              aria-current={s === viewed ? "step" : undefined}
              disabled={i > steps.indexOf(current)}
              onClick={() => void navigate({ to: ".", search: (prev: object) => ({ ...prev, step: s }) })}
            >
              {i + 1}. {stepName(m, s)}
            </Button>
          </li>
        ))}
      </ol>
      <h2 className="text-section">{stepName(m, viewed)}</h2>
      {error !== "" && (
        <p role="alert" className="text-body text-danger">
          {error}
        </p>
      )}
      <Panel step={viewed} props={props} pluginIds={pluginIds} />
      {can && own && !ended && (
        <div>
          <Button variant="secondary" disabled={busy} onClick={() => setCancelling(true)}>
            {m.cancelRequest}
          </Button>
        </div>
      )}
      {cancelling && (
        <CancelDialog
          onClose={() => setCancelling(false)}
          onConfirm={() => {
            setCancelling(false);
            run(() => api.cancel());
          }}
        />
      )}
    </section>
  );
}
