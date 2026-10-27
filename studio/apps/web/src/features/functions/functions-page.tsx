// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { timestampDate } from "@bufbuild/protobuf/wkt";
import { useLingui } from "@lingui/react";
import { StatusPill } from "@plux/ui";
import { Link, useParams } from "@tanstack/react-router";
import type { ReactElement } from "react";
import { Table } from "../security/parts.tsx";
import { useEnvironment, useScope } from "../security/use-security.ts";
import { Frame } from "./frame.tsx";
import { functionsMessages } from "./functions.messages.ts";
import { useFunctions } from "./use-functions.ts";

/** A10: the functions of one environment with their latest version, placement and test state (STU-110). */
export function FunctionsPage(): ReactElement {
  const m = functionsMessages(useLingui().i18n);
  const { app, env } = useScope();
  const org = (useParams({ strict: false }) as { org?: string }).org ?? "";
  const environment = useEnvironment(app, env);
  const functions = useFunctions(app, environment.environment?.id);
  const list = functions.data?.functions ?? [];
  return (
    <Frame
      id="functions-title"
      title={m.listTitle}
      loading={environment.loading || functions.isPending}
      failed={environment.failed || functions.isError}
    >
      <p className="text-body text-secondary">{m.listCaption}</p>
      {list.length === 0 ? (
        <p className="text-body text-secondary">{m.noFunctions}</p>
      ) : (
        <Table
          caption={m.listTitle}
          heads={[m.colName, m.colVersion, m.colPlacement, m.colTests, m.colCreated]}
          rows={list.map((f) => ({
            key: f.name,
            cells: [
              <Link
                key="n"
                to="/o/$org/apps/$app/functions/$fnId"
                params={{ org, app, fnId: f.name } as never}
                search={{ env }}
              >
                {f.name}
              </Link>,
              String(f.version),
              f.placement,
              <StatusPill
                key="t"
                status={f.testsPassed ? "success" : "danger"}
                label={f.testsPassed ? m.testsPassedYes : m.testsPassedNo}
              />,
              f.createdAt === undefined ? "" : timestampDate(f.createdAt).toLocaleString(),
            ],
          }))}
        />
      )}
    </Frame>
  );
}
