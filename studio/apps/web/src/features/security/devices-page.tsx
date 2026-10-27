// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { timestampDate } from "@bufbuild/protobuf/wkt";
import { useLingui } from "@lingui/react";
import { Field, Input, StatusPill } from "@plux/ui";
import { Link, useNavigate, useParams, useSearch } from "@tanstack/react-router";
import type { ReactElement } from "react";
import { Frame, fmt, Table } from "./parts.tsx";
import { securityMessages } from "./security.messages.ts";
import { useDevices, useEnvironment, useScope } from "./use-security.ts";

/** A31: registered devices of one environment, filtered by `?q`. */
export function DevicesPage(): ReactElement {
  const m = securityMessages(useLingui().i18n);
  const { app, env } = useScope();
  const q = (useSearch({ strict: false }) as { q?: string }).q ?? "";
  const org = (useParams({ strict: false }) as { org?: string }).org ?? "";
  const navigate = useNavigate();
  const environment = useEnvironment(app, env);
  const devices = useDevices(app, environment.environment?.id);
  const needle = q.trim().toLowerCase();
  const shown = (devices.data?.devices ?? []).filter((d) =>
    `${d.id} ${d.platform} ${d.hostBuild}`.toLowerCase().includes(needle),
  );
  return (
    <Frame
      id="devices-title"
      title={m.devicesTitle}
      caption={m.devicesCaption}
      loading={environment.loading || devices.isPending}
      failed={environment.failed || devices.isError}
    >
      <Field label={m.search}>
        <Input
          value={q}
          onChange={(e) =>
            void navigate({
              to: ".",
              replace: true,
              search: (prev: object) => ({ ...prev, q: e.target.value === "" ? undefined : e.target.value }),
            } as never)
          }
        />
      </Field>
      {shown.length === 0 ? (
        <p className="text-body text-secondary">{m.noDevices}</p>
      ) : (
        <Table
          caption={m.devicesTitle}
          heads={[m.colId, m.colPlatform, m.colBuild, m.colAssurance, m.colAttested, m.colState]}
          rows={shown.map((d) => ({
            key: d.id,
            cells: [
              <Link
                key="l"
                to="/o/$org/apps/$app/devices/$deviceId"
                params={{ org, app, deviceId: d.id } as never}
                search={{ env }}
                className="font-mono"
              >
                {d.id}
              </Link>,
              d.platform,
              d.hostBuild,
              d.assuranceLevel,
              fmt(d.attestation?.verifiedAt && timestampDate(d.attestation.verifiedAt), m.never),
              d.revokedAt === undefined ? (
                <StatusPill key="s" status="success" label={m.stateActive} />
              ) : (
                <StatusPill key="s" status="danger" label={m.stateRevoked} />
              ),
            ],
          }))}
        />
      )}
    </Frame>
  );
}
