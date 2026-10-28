// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { FunctionAdminService } from "@plux/api";
import { Tabs } from "@plux/ui";
import { Link, useNavigate, useParams, useSearch } from "@tanstack/react-router";
import { type ReactElement, useMemo } from "react";
import { useApp } from "../../lib/use-app.tsx";
import { useGuarded } from "../releases/guarded.tsx";
import { useEnvironment, usePermissions, useScope } from "../security/use-security.ts";
import { Frame } from "./frame.tsx";
import {
  AssurancePanel,
  CapabilitiesPanel,
  LogsPanel,
  MetricsPanel,
  StoredTestsPanel,
  VersionsPanel,
} from "./function-panels.tsx";
import { functionsMessages } from "./functions.messages.ts";
import { type FunctionTab, functionTabs } from "./functions.schema.ts";
import { useFunction, useSetAlias } from "./use-functions.ts";

/** A11: one function with its versions, assurance, capabilities, stored tests, metrics and logs (STU-110, FN-035). */
export function FunctionPage(): ReactElement {
  const m = functionsMessages(useLingui().i18n);
  const { session } = useApp();
  const { app, env } = useScope();
  const { org = "", fnId = "" } = useParams({ strict: false }) as { org?: string; fnId?: string };
  const tab = (useSearch({ strict: false }) as { tab?: FunctionTab }).tab ?? "versions";
  const navigate = useNavigate();
  const environment = useEnvironment(app, env);
  const environmentId = environment.environment?.id;
  const fn = useFunction(app, environmentId, fnId);
  const setAlias = useSetAlias(app, environmentId, fnId);
  const { guard, dialog, failed } = useGuarded();
  const canPromote = usePermissions().has("function.deploy");
  const client = useMemo(() => createClient(FunctionAdminService, session.transport), [session.transport]);
  const latest = fn.data?.latest;
  const scope = { app, environmentId: environmentId ?? "", name: fnId };
  const panels: Record<FunctionTab, ReactElement | undefined> = {
    versions: (
      <VersionsPanel
        scope={scope}
        aliases={fn.data?.aliases ?? []}
        canPromote={canPromote}
        onAlias={(alias, version) => guard(() => setAlias(alias, version))}
        failed={failed}
        m={m}
      />
    ),
    assurance: latest && <AssurancePanel v={latest} m={m} />,
    capabilities: latest && <CapabilitiesPanel v={latest} m={m} />,
    tests: <StoredTestsPanel client={client} sourceId={latest?.sourceId} m={m} />,
    metrics: <MetricsPanel scope={scope} m={m} />,
    logs: <LogsPanel scope={scope} m={m} />,
  };
  const labels: Record<FunctionTab, string> = {
    versions: m.tabVersions,
    assurance: m.tabAssurance,
    capabilities: m.tabCapabilities,
    tests: m.tabTests,
    metrics: m.tabMetrics,
    logs: m.tabLogs,
  };
  return (
    <Frame
      id="function-title"
      title={m.detailTitle(fnId)}
      loading={environment.loading || fn.isPending}
      failed={environment.failed || fn.isError}
    >
      <div className="flex gap-4 text-body">
        <Link to="/o/$org/apps/$app/functions" params={{ org, app }} search={{ env }}>
          {m.back}
        </Link>
        <Link to="/o/$org/apps/$app/functions/$fnId/edit" params={{ org, app, fnId } as never} search={{ env }}>
          {m.openEditor}
        </Link>
      </div>
      <Tabs
        value={tab}
        onValueChange={(value) =>
          void navigate({ to: ".", replace: true, search: (s: object) => ({ ...s, tab: value }) } as never)
        }
        tabs={functionTabs.map((value) => ({ value, label: labels[value], panel: panels[value] }))}
      />
      {dialog}
    </Frame>
  );
}
