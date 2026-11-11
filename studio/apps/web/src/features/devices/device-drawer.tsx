// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { type DevEvent, DevSessionService } from "@plux/api";
import { Button, Icon, type IconName, Select } from "@plux/ui";
import { InputGroup, InputGroupAddon, InputGroupInput } from "@plux/ui/components/input-group";
import { Tabs as ShadcnTabs, TabsContent, TabsList, TabsTrigger } from "@plux/ui/components/tabs";
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
/** How often the drawer reads the session's devices again, in milliseconds. */
const devicesRefresh = 5000;

/** Hands the bytes to the browser as a download of `name`. */
function download(bytes: Uint8Array, name: string): void {
  const url = URL.createObjectURL(new Blob([bytes as BlobPart], { type: "application/gzip" }));
  const link = document.createElement("a");
  link.href = url;
  link.download = name;
  link.click();
  URL.revokeObjectURL(url);
}

const tabIcons: Readonly<Record<DrawerTab, IconName>> = {
  problems: "warning-circle",
  logs: "scroll-text",
  flow: "flow-arrow",
  state: "variable",
  network: "network",
  functions: "code",
  sync: "refresh",
  performance: "gauge",
  console: "terminal",
};

/** The time of an event as the device clock gave it: hours, minutes, seconds and milliseconds. */
function timeOf(e: DevEvent): string {
  const ms = Number(e.timeUnixMicros / 1000n);
  if (ms <= 0) {
    return "";
  }
  const d = new Date(ms);
  const two = (n: number) => String(n).padStart(2, "0");
  return `${two(d.getHours())}:${two(d.getMinutes())}:${two(d.getSeconds())}.${String(d.getMilliseconds()).padStart(3, "0")}`;
}

/** How many events each tab holds, and the problems validation found. */
function countsOf(events: readonly DevEvent[], problems: number | undefined): Map<DrawerTab, number> {
  const counts = new Map<DrawerTab, number>();
  for (const e of events) {
    const t = tabOf(e);
    if (t !== undefined) {
      counts.set(t, (counts.get(t) ?? 0) + 1);
    }
  }
  if (problems !== undefined) {
    counts.set("problems", problems);
  }
  return counts;
}

/** A device tab's rows: the time, the level, what happened and on which device; an empty tab says so. */
function ConsoleRows(props: {
  readonly rows: readonly DevEvent[];
  readonly icon: IconName;
  readonly label: string;
  readonly names: ReadonlyMap<string, string>;
  readonly level: (level: string) => string;
}): ReactNode {
  const m = devicesMessages(useLingui().i18n);
  if (props.rows.length === 0) {
    return (
      <p className="console-empty">
        <Icon name={props.icon} />
        {m.empty}
      </p>
    );
  }
  return (
    <ul aria-label={props.label} className="console-rows">
      {props.rows.map((e) => {
        const level = levelOf(e);
        const source = props.names.get(e.deviceId);
        return (
          <li key={`${e.deviceId}-${e.sequence}`} className="console-row" data-level={level || undefined}>
            <time className="console-time">{timeOf(e)}</time>
            {level !== "" && <strong className="console-level">{props.level(level)}</strong>}
            <span className="console-text">{summarise(e)}</span>
            {source !== undefined && <span className="console-source">{source}</span>}
          </li>
        );
      })}
    </ul>
  );
}

/**
 * The developer console's body (STU-066, DEV-020): nine tabs with their icons and live counts — Problems from
 * validation, the rest fed by the paired devices of the dev session — over the source device, a level filter, search,
 * pause and trace export; rows show the time, the level and the device.
 */
export function DeviceDrawer({
  sessionId,
  problems,
  problemCount,
  tab,
  onTab,
  onEvent,
}: {
  readonly sessionId: string;
  /** The Problems tab's content: validation is not fed by a device. */
  readonly problems: ReactNode;
  readonly problemCount?: number | undefined;
  /** The open tab, when the console owns it; otherwise the drawer keeps its own, starting on Problems. */
  readonly tab?: DrawerTab;
  readonly onTab?: (tab: DrawerTab) => void;
  /** Sees every event as it arrives, before the filters (inspect, screenshots, hot apply). */
  readonly onEvent?: (event: DevEvent) => void;
}): ReactNode {
  const m = devicesMessages(useLingui().i18n);
  const [own, setOwn] = useState<DrawerTab>("problems");
  const current = tab ?? own;
  const choose = onTab ?? setOwn;
  const { session } = useApp();
  const client = useMemo(() => createClient(DevSessionService, session.transport), [session.transport]);
  const [filter, setFilter] = useState<DrawerFilter>({ deviceId: "", level: "all", search: "" });
  // Read again every few seconds: a device pairs, connects and drops while the drawer is open, and the list (with
  // whether each is connected) would otherwise stay as it was when the drawer opened.
  const devices = useQuery({
    queryKey: ["dev-devices", sessionId],
    queryFn: async () => (await client.listDevices({ sessionId })).devices,
    refetchInterval: devicesRefresh,
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
  const names = new Map((devices.data ?? []).map((d) => [d.id, d.name]));
  const counts = countsOf(shown, problemCount);
  const panel = (t: DrawerTab): ReactNode =>
    t === "problems" ? (
      problems
    ) : (
      <ConsoleRows
        rows={shown.filter((e) => tabOf(e) === t)}
        icon={tabIcons[t]}
        label={labels[t]}
        names={names}
        level={lineLevel}
      />
    );
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
    <section aria-label={m.drawer} className="console-drawer">
      <ShadcnTabs value={current} onValueChange={(v) => choose(v as DrawerTab)} className="console-tabs">
        <div className="console-head">
          <TabsList variant="line" className="console-tablist">
            {drawerTabs.map((t) => (
              <TabsTrigger key={t} value={t} className="console-tab">
                <Icon name={tabIcons[t]} size="pill" />
                {labels[t]}
                {(counts.get(t) ?? 0) > 0 && (
                  <span className="console-tab-count" aria-hidden="true">
                    {counts.get(t)}
                  </span>
                )}
              </TabsTrigger>
            ))}
          </TabsList>
          <div className="console-filters">
            <Select
              label={m.source}
              value={filter.deviceId === "" ? allDevices : filter.deviceId}
              options={sources}
              onValueChange={(deviceId) => setFilter({ ...filter, deviceId: deviceId === allDevices ? "" : deviceId })}
            />
            <fieldset aria-label={m.level} className="console-levels">
              {levels.map(([value, label]) => (
                <button
                  key={value}
                  type="button"
                  className="console-level-toggle"
                  aria-pressed={filter.level === value}
                  onClick={() => setFilter({ ...filter, level: value })}
                >
                  {label}
                </button>
              ))}
            </fieldset>
            <InputGroup className="console-search">
              <InputGroupAddon>
                <Icon name="magnifying-glass" />
              </InputGroupAddon>
              <InputGroupInput
                type="search"
                aria-label={m.search}
                placeholder={m.search}
                value={filter.search}
                onChange={(e) => setFilter({ ...filter, search: e.target.value })}
              />
            </InputGroup>
            <Button size="sm" variant="ghost" onClick={paused ? stream.resume : stream.pause}>
              <Icon name={paused ? "play" : "pause"} size="pill" />
              {paused ? m.resume : m.pause}
            </Button>
            <Button size="sm" variant="ghost" loading={exportTrace.isPending} onClick={() => exportTrace.mutate()}>
              <Icon name="download" size="pill" />
              {m.exportTrace}
            </Button>
            {waiting > 0 && (
              <span role="status" className="text-caption text-muted-foreground">
                {m.pausedCount(waiting)}
              </span>
            )}
          </div>
        </div>
        {stream.state === "ended" && <p className="console-note">{m.streamEnded}</p>}
        {exportTrace.isError && (
          <p role="alert" className="console-note">
            {m.failed}
          </p>
        )}
        {drawerTabs.map((t) => (
          <TabsContent key={t} value={t} className="console-body">
            {panel(t)}
          </TabsContent>
        ))}
      </ShadcnTabs>
    </section>
  );
}
