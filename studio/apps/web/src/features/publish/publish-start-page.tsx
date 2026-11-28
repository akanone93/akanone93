// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { useQuery } from "@tanstack/react-query";
import { useNavigate, useParams, useSearch } from "@tanstack/react-router";
import { type ReactElement, useEffect } from "react";
import { PageEmpty, PageHeader, PageStatus } from "../../components/page.tsx";
import { useEnvironment, useScope } from "../security/use-security.ts";
import { publishMessages } from "./publish.messages.ts";
import { useAppChanges, useStartRequest, useSummaries } from "./use-publish.ts";

/**
 * A1: resumes an open release request, that of `?env` when the address names one, or creates one with the plugins that
 * have unpublished changes (STU-100).
 */
export function PublishStartPage(): ReactElement {
  const m = publishMessages(useLingui().i18n);
  const { app, env } = useScope();
  const scoped = (useSearch({ strict: false }) as { env?: string }).env !== undefined;
  const org = (useParams({ strict: false }) as { org?: string }).org ?? "";
  const navigate = useNavigate();
  const environment = useEnvironment(app, env);
  const summaries = useSummaries(app);
  const start = useStartRequest();
  const appChanges = useAppChanges(app);
  const environmentId = environment.environment?.id;
  const loaded = summaries.data;
  const appChanged = appChanges.data;
  // A query, not an effect, so that a refusal reaches the page with its reason: publishing needs a fresh
  // security-key confirmation (SEC-103), and the refusal's "confirm" runs the active queries again, this one
  // included. An open request is resumed rather than a second one opened.
  const started = useQuery({
    queryKey: ["publish-start", app, environmentId, scoped],
    enabled: environmentId !== undefined && loaded !== undefined && appChanged !== undefined,
    queryFn: async () => {
      const changed = (loaded ?? [])
        .filter((s) => s.unpublishedChanges)
        .map((s) => ({ pluginKey: s.plugin?.key ?? "", reason: "changed" }));
      return { id: await start(app, environmentId ?? "", changed, appChanged === true, scoped) };
    },
    retry: false,
    staleTime: Number.POSITIVE_INFINITY,
    gcTime: 0,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  });
  const id = started.data?.id;
  useEffect(() => {
    if (id !== undefined) {
      void navigate({ to: "/o/$org/apps/$app/publish/$requestId", params: { org, app, requestId: id }, replace: true });
    }
  }, [app, id, navigate, org]);
  const nothing = started.isSuccess && id === undefined;
  const failed = environment.failed || summaries.isError || appChanges.isError || started.isError;
  return (
    <section className="page-frame" aria-labelledby="publish-title">
      <PageHeader id="publish-title" title={m.title} />
      <div className="page-body">
        <PageStatus
          loading={!nothing && !failed}
          failed={failed}
          error={started.error ?? summaries.error}
          labels={{ loading: m.starting, loadFailed: m.startFailed }}
        />
        {nothing && <PageEmpty icon="paper-plane-tilt" text={m.nothing} />}
      </div>
    </section>
  );
}
