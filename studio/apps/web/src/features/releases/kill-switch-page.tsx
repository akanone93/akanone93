// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button, Field, Input } from "@plux/ui";
import { type ReactElement, useState } from "react";
import { useEnvironment, usePermissions, useScope } from "../security/use-security.ts";
import { Frame } from "./frame.tsx";
import { useGuarded } from "./guarded.tsx";
import { releasesMessages } from "./releases.messages.ts";
import { defaultChannel } from "./releases.schema.ts";
import { useControl, useReleaseActions } from "./use-releases.ts";

/** A5 and D46: the plugins switched off in one environment; both switching off and on go through the step-up D3 (REL-060, SEC-103). */
export function KillSwitchPage(): ReactElement {
  const m = releasesMessages(useLingui().i18n);
  const { app, env } = useScope();
  const environment = useEnvironment(app, env);
  const environmentId = environment.environment?.id;
  const control = useControl(app, environmentId, defaultChannel);
  const actions = useReleaseActions(app);
  const { guard, dialog, failed } = useGuarded();
  const allowed = usePermissions().has("release.promote");
  const [target, setTarget] = useState("");
  const off = control.data?.control?.killSwitchPlugins ?? [];
  const id = environmentId ?? "";
  return (
    <Frame
      id="kill-title"
      title={m.killTitle}
      caption={m.killCaption}
      loading={environment.loading || control.isPending}
      failed={environment.failed || control.isError}
    >
      {control.data?.control?.appKillSwitch === true && (
        <p role="status" className="text-body text-danger">
          {m.killAppWide}
        </p>
      )}
      <h2 className="text-heading">{m.killActive}</h2>
      {off.length === 0 ? (
        <p className="text-body text-secondary">{m.killNone}</p>
      ) : (
        <ul className="flex flex-col gap-2">
          {off.map((key) => (
            <li key={key} className="flex items-center gap-3">
              <span className="font-mono">{key}</span>
              <Button
                variant="secondary"
                disabled={!allowed}
                aria-label={`${m.killClear} ${key}`}
                onClick={() => guard(() => actions.clearKillSwitch(id, defaultChannel, key))}
              >
                {m.killClear}
              </Button>
            </li>
          ))}
        </ul>
      )}
      <div className="flex items-end gap-2">
        <Field label={m.pluginLabel}>
          <Input value={target} onChange={(e) => setTarget(e.target.value)} />
        </Field>
        <Button
          variant="danger"
          disabled={!allowed || target.trim() === ""}
          onClick={() => guard(() => actions.setKillSwitch(id, defaultChannel, target.trim()))}
        >
          {m.killSet}
        </Button>
      </div>
      {failed && (
        <p role="alert" className="text-body text-danger">
          {m.actionFailed}
        </p>
      )}
      {dialog}
    </Frame>
  );
}
