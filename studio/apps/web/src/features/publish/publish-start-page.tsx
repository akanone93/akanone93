// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { useNavigate, useParams } from "@tanstack/react-router";
import { type ReactElement, useEffect, useRef, useState } from "react";
import { PageEmpty, PageHeader, PageStatus } from "../../components/page.tsx";
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
  const loaded = summaries.data;
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
    <section className="page-frame" aria-labelledby="publish-title">
      <PageHeader id="publish-title" title={m.title} />
      <div className="page-body">
        <PageStatus
          loading={state === "working" && !failed}
          failed={failed}
          error={summaries.error}
          labels={{ loading: m.starting, loadFailed: m.startFailed }}
        />
        {state === "nothing" && <PageEmpty icon="paper-plane-tilt" text={m.nothing} />}
      </div>
    </section>
  );
}
