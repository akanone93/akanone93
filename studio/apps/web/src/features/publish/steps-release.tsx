// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button, Field, Input, StatusPill, Textarea } from "@plux/ui";
import { type ReactElement, useState } from "react";
import { StepUpDialog } from "../../components/dialogs/index.ts";
import { useStepUp } from "../security/use-security.ts";
import type { PanelProps } from "./panel.ts";
import { publishMessages } from "./publish.messages.ts";

type Messages = ReturnType<typeof publishMessages>;

function diffTone(state: string) {
  return state === "succeeded" ? "success" : state === "failed" ? "danger" : "neutral";
}

function diffLabel(m: Messages, state: string): string {
  const labels: Record<string, string> = {
    queued: m.diffQueued,
    running: m.diffRunning,
    succeeded: m.diffSucceeded,
    failed: m.diffFailed,
  };
  return labels[state] ?? state;
}

/** Step 4: the server-side visual diff of the changed screens; approvers need it finished (GOV-023). */
export function DiffsPanel({ request, diff, editable, busy, api, run }: PanelProps): ReactElement {
  const m = publishMessages(useLingui().i18n);
  const current = diff !== undefined && diff.contentHash === request.contentHash;
  const finished = current && diff.state === "succeeded";
  const again = !current || diff.state === "failed";
  return (
    <div className="flex flex-col gap-3">
      {diff === undefined && <p className="text-body text-secondary">{m.diffNone}</p>}
      {diff !== undefined && <StatusPill status={diffTone(diff.state)} label={diffLabel(m, diff.state)} />}
      <ul className="flex flex-col gap-1">
        {diff?.screens.map((s) => (
          <li key={`${s.pluginKey}/${s.screen}/${s.device}`} className="flex items-center gap-3 text-body">
            <span>{m.diffScreen(s.pluginKey, s.screen, s.device)}</span>
            <span className="text-secondary">
              {s.beforeSha256 === "" ? m.diffNew : s.changed ? m.diffChanged : m.diffSame}
            </span>
          </li>
        ))}
      </ul>
      {editable && (
        <div className="flex gap-2">
          {again && (
            <Button disabled={busy} onClick={() => run(() => api.requestDiff())}>
              {m.requestDiff}
            </Button>
          )}
          {finished && (
            <Button disabled={busy} onClick={() => run(() => api.advance())}>
              {m.continue}
            </Button>
          )}
        </div>
      )}
    </div>
  );
}

/** Step 5: the human notes and the optional label next to the automatic changelog (REL-081). */
export function ChangelogPanel({ request, editable, busy, api, run }: PanelProps): ReactElement {
  const m = publishMessages(useLingui().i18n);
  const [notes, setNotes] = useState(request.changelogNotes);
  const [label, setLabel] = useState(request.label);
  return (
    <div className="flex flex-col gap-3">
      <Field label={m.notes}>
        <Textarea value={notes} readOnly={!editable} onChange={(e) => setNotes(e.target.value)} />
      </Field>
      <Field label={m.label}>
        <Input value={label} readOnly={!editable} onChange={(e) => setLabel(e.target.value)} />
      </Field>
      {editable && (
        <div>
          <Button
            disabled={busy}
            onClick={() => run(() => api.advance({ case: "changelog", value: { notes, label } }))}
          >
            {m.continue}
          </Button>
        </div>
      )}
    </div>
  );
}

/** Step 6: the environment and channel; the build is promoted unchanged (REL-004). */
export function TargetPanel({ request, editable, busy, api, run }: PanelProps): ReactElement {
  const m = publishMessages(useLingui().i18n);
  const [channel, setChannel] = useState(request.channel);
  return (
    <div className="flex flex-col gap-3">
      <p className="text-body">
        {m.environment}: <span className="font-mono">{request.environmentId}</span>
      </p>
      <Field label={m.channel}>
        <Input value={channel} readOnly={!editable} onChange={(e) => setChannel(e.target.value)} />
      </Field>
      <p className="font-mono text-body text-secondary">{m.hash(request.contentHash.slice(0, 12))}</p>
      {editable && (
        <div>
          <Button
            disabled={busy || channel.trim() === ""}
            onClick={() =>
              run(() =>
                api.advance({
                  case: "target",
                  value: { environmentId: request.environmentId, channel: channel.trim() },
                }),
              )
            }
          >
            {m.continue}
          </Button>
        </div>
      )}
    </div>
  );
}

/** Step 7: policies decide who approves; the approval is bound to the content hash (GOV-022). */
export function ApprovalPanel({ request, editable, busy, api, run }: PanelProps): ReactElement {
  const m = publishMessages(useLingui().i18n);
  const waiting = request.hold === "awaiting_approval";
  const approved = request.approvedHash !== "";
  const note =
    request.state === "rejected" ? m.approvalRejected : request.state === "expired" ? m.approvalExpired : undefined;
  return (
    <div className="flex flex-col gap-3">
      {waiting && <StatusPill status="warning" label={m.holdAwaiting} />}
      {waiting && <p className="text-body text-secondary">{m.approvalWaiting}</p>}
      {approved && <p className="text-body">{m.approvalDone(request.approvedHash.slice(0, 12))}</p>}
      {note !== undefined && (
        <p role="alert" className="text-body text-danger">
          {note}
        </p>
      )}
      {editable && !waiting && !approved && (
        <div>
          <Button disabled={busy} onClick={() => run(() => api.requestApproval())}>
            {m.requestApproval}
          </Button>
        </div>
      )}
    </div>
  );
}

function planText(bytes: Uint8Array): string {
  return bytes.length === 0 ? "{}" : new TextDecoder().decode(bytes);
}

function parsePlan(text: string): Uint8Array | undefined {
  try {
    const value: unknown = JSON.parse(text);
    return typeof value === "object" && value !== null && !Array.isArray(value)
      ? new TextEncoder().encode(text)
      : undefined;
  } catch {
    return undefined;
  }
}

/** Step 8: stages, schedule, health gates and activation as the plan JSON the server validates (REL-050 to REL-053). */
export function RolloutPanel({ request, editable, busy, api, run }: PanelProps): ReactElement {
  const m = publishMessages(useLingui().i18n);
  const [text, setText] = useState(planText(request.rolloutPlan));
  const plan = parsePlan(text);
  return (
    <div className="flex flex-col gap-3">
      <Field label={m.rolloutPlan} error={plan === undefined ? m.rolloutInvalid : undefined}>
        <Textarea value={text} readOnly={!editable} onChange={(e) => setText(e.target.value)} />
      </Field>
      {editable && (
        <div>
          <Button
            disabled={busy || plan === undefined}
            onClick={() => plan && run(() => api.advance({ case: "rollout", value: { plan } }))}
          >
            {m.continue}
          </Button>
        </div>
      )}
    </div>
  );
}

/** Step 9: the release itself; the WebAuthn step-up D3 comes first and a cancelled dialog sends nothing (SEC-103). */
export function ReleasePanel({ request, editable, busy, api, run }: PanelProps): ReactElement {
  const m = publishMessages(useLingui().i18n);
  const stepUp = useStepUp();
  const [challenge, setChallenge] = useState<{ requestId: string; options: CredentialRequestOptions }>();
  const [failed, setFailed] = useState(false);
  const begin = () => {
    setFailed(false);
    stepUp.begin().then(setChallenge, () => setFailed(true));
  };
  return (
    <div className="flex flex-col gap-3">
      {request.state === "released" && <p className="text-body">{m.releasedAs(String(request.releaseSequence))}</p>}
      {failed && (
        <p role="alert" className="text-body text-danger">
          {m.stepUpFailed}
        </p>
      )}
      {editable && request.state === "release" && (
        <div>
          <Button disabled={busy} onClick={begin}>
            {m.release}
          </Button>
        </div>
      )}
      {challenge !== undefined && (
        <StepUpDialog
          open
          onOpenChange={(o) => !o && setChallenge(undefined)}
          options={challenge.options}
          onAssertion={(credential) =>
            run(async () => {
              await stepUp.complete(challenge.requestId, credential);
              await api.complete();
            })
          }
        />
      )}
    </div>
  );
}
