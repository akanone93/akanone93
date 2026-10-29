// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button } from "@plux/ui";
import type { ReactElement } from "react";
import { shellSettingsMessages } from "./shell-settings.messages.ts";
import { type ProjectFormat, useProjectDownload } from "./use-project-download.ts";

/**
 * Generates the host project of an app and downloads it (GEN-001, F18): one button, the job's progress while it
 * queues and runs, and the server's explanation when it fails. `label` names the button for its context; `format` is the whole project or the patch since the last generation.
 */
export function ProjectDownload({
  appId,
  label,
  format = "zip",
  disabled = false,
}: {
  readonly appId: string;
  readonly label: string;
  readonly format?: ProjectFormat;
  readonly disabled?: boolean;
}): ReactElement {
  const m = shellSettingsMessages(useLingui().i18n);
  const { phase, start } = useProjectDownload(appId, format);
  const progress =
    phase.state === "working"
      ? { queued: m.projectQueued, running: m.projectRunning, downloading: m.projectDownloading }[phase.status]
      : undefined;
  return (
    <div className="flex flex-col items-start gap-2">
      <Button
        variant="secondary"
        disabled={disabled}
        loading={phase.state === "working"}
        {...(progress !== undefined && { loadingLabel: progress })}
        onClick={() => void start()}
      >
        {label}
      </Button>
      {progress !== undefined && (
        <div className="flex w-full flex-col gap-1">
          <p role="status" className="text-body text-secondary">
            {progress}
          </p>
          <progress aria-label={m.projectProgress} className="w-full" />
        </div>
      )}
      {phase.state === "done" && (
        <p role="status" className="text-body text-secondary">
          {m.projectDone(phase.filename)}
        </p>
      )}
      {phase.state === "failed" && (
        <p role="alert" className="text-body text-danger">
          {phase.message === "" ? m.projectFailedPlain : m.projectFailed(phase.message)}
        </p>
      )}
    </div>
  );
}
