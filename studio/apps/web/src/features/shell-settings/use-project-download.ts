// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { ConnectError, createClient } from "@connectrpc/connect";
import { AppProjectService } from "@plux/api";
import { useCallback, useEffect, useRef, useState } from "react";
import { useApp } from "../../lib/use-app.tsx";
import { download } from "../admin/download.ts";

/** How often a queued or running project job is asked for its status. */
const pollMs = 500;

export type ProjectPhase =
  | { readonly state: "idle" }
  | { readonly state: "working"; readonly status: "queued" | "running" | "downloading" }
  | { readonly state: "done"; readonly filename: string }
  | { readonly state: "failed"; readonly code: string; readonly message: string };

/** What a project job builds: the whole project, or the files that changed since the last generation (GEN-004). */
export type ProjectFormat = "zip" | "patch";

const idle: ProjectPhase = { state: "idle" };

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function refusal(error: unknown): { code: string; message: string } {
  if (error instanceof ConnectError) {
    return { code: "", message: error.rawMessage };
  }
  return { code: "", message: "" };
}

/**
 * Generates the host project of an app as a queued server job (GEN-001, GEN-004, F18): asks for it, follows the job
 * until it succeeds or fails, then hands the zip to the browser. At most one job per app runs at a time and the
 * number per hour is limited by `project.jobsPerHour`; the server's refusal is returned as the failure. A patch needs
 * a project generated before; once a job is done the shell status is read again.
 */
export function useProjectDownload(
  appId: string,
  format: ProjectFormat = "zip",
): {
  readonly phase: ProjectPhase;
  readonly start: () => Promise<void>;
} {
  const { session } = useApp();
  const [phase, setPhase] = useState<ProjectPhase>(idle);
  const alive = useRef(true);
  useEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
    };
  }, []);
  const start = useCallback(async () => {
    const client = createClient(AppProjectService, session.transport);
    const show = (next: ProjectPhase) => {
      if (alive.current) {
        setPhase(next);
      }
    };
    try {
      show({ state: "working", status: "queued" });
      const queued = await client.generateProject({
        appId,
        environmentKey: "production",
        channelKey: "production",
        format,
      });
      const id = queued.job?.id ?? "";
      for (;;) {
        await sleep(pollMs);
        if (!alive.current) {
          return;
        }
        const { job } = await client.getProjectJob({ jobId: id, appId });
        if (job?.status === "failed") {
          show({ state: "failed", code: job.errorCode, message: job.errorMessage });
          return;
        }
        if (job?.status === "succeeded") {
          break;
        }
        show({ state: "working", status: "running" });
      }
      show({ state: "working", status: "downloading" });
      const file = await client.downloadProject({ jobId: id, appId });
      download(file.filename, new Uint8Array(file.archive), "application/zip");
      show({ state: "done", filename: file.filename });
      await session.queryClient.invalidateQueries({ queryKey: ["shell-status", appId] });
    } catch (error) {
      show({ state: "failed", ...refusal(error) });
    }
  }, [appId, format, session]);
  return { phase, start };
}
