// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { useNavigate, useParams } from "@tanstack/react-router";
import { type ReactElement, useEffect, useRef, useState } from "react";
import { useEnvironment, useScope } from "../security/use-security.ts";
import { publishMessages } from "./publish.messages.ts";
import { useStartRequest, useSummaries } from "./use-publish.ts";

/** A1: resumes the open release request or creates one with the plugins that have unpublished changes (STU-100). */
export function PublishStartPage(): ReactElement {
  const m = publishMessages(useLingui().i18n);
  const { app, env } = useScope();
  const org = (useParams({ strict: false }) as { org?: string }).org ?? "";
  const navigate = useNavigate();
  const environment = useEnvironment(app, env);
  const summaries = useSummaries(app);
  const start = useStartRequest();
  const started = useRef(false);
  const [state, setState] = useState<"working" | "nothing" | "failed">("working");
  const environmentId = environment.environment?.id;
  const loaded = summaries.data?.summaries;
  useEffect(() => {
    if (started.current || environmentId === undefined || loaded === undefined) {
      return;
    }
    started.current = true;
    const changed = loaded
      .filter((s) => s.unpublishedChanges)
      .map((s) => ({ pluginKey: s.plugin?.key ?? "", reason: "changed" }));
    start(app, environmentId, changed).then(
      (id) =>
        id === undefined
          ? setState("nothing")
          : void navigate({
              to: "/o/$org/apps/$app/publish/$requestId",
              params: { org, app, requestId: id },
              replace: true,
            }),
      () => setState("failed"),
    );
  }, [app, environmentId, loaded, navigate, org, start]);
  const failed = environment.failed || summaries.isError || state === "failed";
  return (
    <section className="flex flex-col gap-4" aria-labelledby="publish-title">
      <h1 id="publish-title" className="text-screen">
        {m.title}
      </h1>
      {state === "working" && !failed && (
        <p role="status" className="text-body text-secondary">
          {m.starting}
        </p>
      )}
      {state === "nothing" && <p className="text-body text-secondary">{m.nothing}</p>}
      {failed && (
        <p role="alert" className="text-body text-danger">
          {m.startFailed}
        </p>
      )}
    </section>
  );
}
