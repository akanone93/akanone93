// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { type DevEvent, DevSessionService } from "@plux/api";
import { Button, Field, Input, Select, Tabs } from "@plux/ui";
import { useMutation, useQuery } from "@tanstack/react-query";
import { type ReactNode, useMemo, useState } from "react";
import { useApp } from "../../lib/use-app.tsx";
import { devicesMessages } from "./devices.messages.ts";
import {
  type DrawerFilter,
  type DrawerTab,
  drawerTabs,
  type LevelFilter,
  levelOf,
  matches,
  pending,
  summarise,
  tabOf,
  visible,
} from "./drawer-model.ts";
import { useDeviceEvents } from "./use-device-events.ts";

/** The source picker's value for every device; a Select option needs a non-empty value. */
const allDevices = "*";

/** Hands the bytes to the browser as a download of `name`. */
function download(bytes: Uint8Array, name: string): void {
  const url = URL.createObjectURL(new Blob([bytes as BlobPart], { type: "application/gzip" }));
  const link = document.createElement("a");
  link.href = url;
  link.download = name;
  link.click();
  URL.revokeObjectURL(url);
}

/**
 * The bottom drawer (STU-066, DEV-020): nine tabs, the first validation, the rest fed by the paired devices
 * of the dev session, with a source device, a level filter, search, pause and trace export.
 */
export function DeviceDrawer({
  sessionId,
  problems,
  onEvent,
}: {
  readonly sessionId: string;
  /** The Problems tab's content: validation is not fed by a device. */
  readonly problems: ReactNode;
  /** Sees every event as it arrives, before the filters (inspect, screenshots, hot apply). */
  readonly onEvent?: (event: DevEvent) => void;
}): ReactNode {
  const m = devicesMessages(useLingui().i18n);
  const { session } = useApp();
  const client = useMemo(() => createClient(DevSessionService, session.transport), [session.transport]);
  const [filter, setFilter] = useState<DrawerFilter>({ deviceId: "", level: "all", search: "" });
  const devices = useQuery({
    queryKey: ["dev-devices", sessionId],
    queryFn: async () => (await client.listDevices({ sessionId })).devices,
  });
  const stream = useDeviceEvents(sessionId, filter.deviceId === "" ? [] : [filter.deviceId], onEvent);
  const paused = stream.buffer.frozenAt !== undefined;
  const exportTrace = useMutation({
    mutationFn: async () => {
      const res = await client.exportTrace({ sessionId, deviceIds: filter.deviceId === "" ? [] : [filter.deviceId] });
      download(res.trace, `plux-trace-${sessionId}.bin.gz`);
    },
  });
  const shown = useMemo(() => visible(stream.buffer).filter((e) => matches(e, filter)), [stream.buffer, filter]);
  const labels: Record<DrawerTab, string> = {
    problems: m.tabProblems,
    logs: m.tabLogs,
    flow: m.tabFlow,
    state: m.tabState,
    network: m.tabNetwork,
    functions: m.tabFunctions,
    sync: m.tabSync,
    performance: m.tabPerformance,
    console: m.tabConsole,
  };
  const lineLevel = (level: string): string =>
    ({ debug: m.lineDebug, info: m.lineInfo, warning: m.lineWarning, error: m.lineError })[level] ?? level;
  const panel = (tab: DrawerTab): ReactNode => {
    if (tab === "problems") {
      return problems;
    }
    const rows = shown.filter((e) => tabOf(e) === tab);
    return rows.length === 0 ? (
      <p className="text-body text-secondary">{m.empty}</p>
    ) : (
      <ul aria-label={labels[tab]} className="flex flex-col gap-1 font-mono text-caption">
        {rows.map((e) => (
          <li key={`${e.deviceId}-${e.sequence}`}>
            {tab === "console" && levelOf(e) !== "" && <strong className="me-1">{lineLevel(levelOf(e))}</strong>}
            {summarise(e)}
          </li>
        ))}
      </ul>
    );
  };
  const sources = [
    { value: allDevices, label: m.allDevices },
    ...(devices.data ?? []).map((d) => ({ value: d.id, label: m.deviceName(d.name, d.connected) })),
  ];
  const levels: readonly [LevelFilter, string][] = [
    ["all", m.levelAll],
    ["warning", m.levelWarning],
    ["error", m.levelError],
  ];
  const waiting = pending(stream.buffer);
  return (
    <aside
      aria-label={m.drawer}
      className="flex max-h-80 flex-col gap-2 overflow-auto rounded-lg bg-raised p-3 shadow-e1"
    >
      <div className="flex flex-wrap items-end gap-2">
        <Select
          label={m.source}
          value={filter.deviceId}
          options={sources}
          onValueChange={(deviceId) => setFilter({ ...filter, deviceId })}
        />
        <fieldset aria-label={m.level} className="flex items-center gap-1">
          {levels.map(([value, label]) => (
            <Button
              key={value}
              size="sm"
              variant={filter.level === value ? "primary" : "ghost"}
              aria-pressed={filter.level === value}
              onClick={() => setFilter({ ...filter, level: value })}
            >
              {label}
            </Button>
          ))}
        </fieldset>
        <Field label={m.search}>
          <Input
            type="search"
            value={filter.search}
            onChange={(e) => setFilter({ ...filter, search: e.target.value })}
          />
        </Field>
        <Button size="sm" variant="secondary" onClick={paused ? stream.resume : stream.pause}>
          {paused ? m.resume : m.pause}
        </Button>
        <Button size="sm" variant="secondary" loading={exportTrace.isPending} onClick={() => exportTrace.mutate()}>
          {m.exportTrace}
        </Button>
        {waiting > 0 && (
          <span role="status" className="text-caption text-secondary">
            {m.pausedCount(waiting)}
          </span>
        )}
      </div>
      {stream.state === "ended" && <p className="text-caption text-secondary">{m.streamEnded}</p>}
      {exportTrace.isError && (
        <p role="alert" className="text-body">
          {m.failed}
        </p>
      )}
      <Tabs
        defaultValue="problems"
        tabs={drawerTabs.map((value) => ({ value, label: labels[value], panel: panel(value) }))}
      />
    </aside>
  );
}
