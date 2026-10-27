// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { type Rollout, RolloutState } from "@plux/api";
import { Button, Field, Input } from "@plux/ui";
import { type ReactElement, useState } from "react";
import { useGuarded } from "./guarded.tsx";
import { releasesMessages } from "./releases.messages.ts";
import { useReleaseActions } from "./use-releases.ts";

/** D44: pause, resume, advance and reschedule act at once; D45 abort is destructive and goes through the step-up D3 (SEC-103). */
export function RolloutControls({
  app,
  rollout,
  allowed,
}: {
  readonly app: string;
  readonly rollout: Rollout;
  readonly allowed: boolean;
}): ReactElement {
  const m = releasesMessages(useLingui().i18n);
  const actions = useReleaseActions(app);
  const { guard, dialog, failed } = useGuarded();
  const [end, setEnd] = useState("");
  const [error, setError] = useState(false);
  const run = (call: () => Promise<unknown>) => {
    setError(false);
    call().catch(() => setError(true));
  };
  const open = rollout.state === RolloutState.ACTIVE || rollout.state === RolloutState.PAUSED;
  const moving = rollout.state === RolloutState.ACTIVE;
  const end2 = end === "" ? undefined : new Date(end);
  return (
    <section className="flex flex-col gap-3" aria-label={m.controls}>
      <h3 className="text-heading">{m.controls}</h3>
      <div className="flex flex-wrap gap-2">
        <Button variant="secondary" disabled={!allowed || !moving} onClick={() => run(() => actions.pause(rollout.id))}>
          {m.pause}
        </Button>
        <Button
          variant="secondary"
          disabled={!allowed || rollout.state !== RolloutState.PAUSED}
          onClick={() => run(() => actions.resume(rollout.id))}
        >
          {m.resume}
        </Button>
        <Button
          variant="secondary"
          disabled={!allowed || !moving}
          onClick={() => run(() => actions.advance(rollout.id))}
        >
          {m.advance}
        </Button>
      </div>
      <div className="flex flex-wrap items-end gap-2">
        <Field label={m.endLabel}>
          <Input type="datetime-local" value={end} onChange={(e) => setEnd(e.target.value)} />
        </Field>
        <Button
          variant="secondary"
          disabled={!allowed || !open || end2 === undefined || Number.isNaN(end2.getTime())}
          onClick={() => run(() => actions.reschedule(rollout.id, end2))}
        >
          {m.reschedule}
        </Button>
        <Button
          variant="ghost"
          disabled={!allowed || !open}
          onClick={() => run(() => actions.reschedule(rollout.id, undefined))}
        >
          {m.clearEnd}
        </Button>
      </div>
      <div className="flex flex-col gap-1">
        <Button variant="danger" disabled={!allowed || !open} onClick={() => guard(() => actions.abort(rollout.id))}>
          {m.abort}
        </Button>
        <p className="text-caption text-secondary">{m.abortHint}</p>
      </div>
      {(error || failed) && (
        <p role="alert" className="text-body text-danger">
          {m.actionFailed}
        </p>
      )}
      {dialog}
    </section>
  );
}
