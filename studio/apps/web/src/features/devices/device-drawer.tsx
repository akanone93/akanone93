// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { type DevDevice, type DevEvent, DevSessionService } from "@plux/api";
import { Button, Icon, type IconName, Select } from "@plux/ui";
import { InputGroup, InputGroupAddon, InputGroupInput } from "@plux/ui/components/input-group";
import { Tabs as ShadcnTabs, TabsContent, TabsList, TabsTrigger } from "@plux/ui/components/tabs";
import { useMutation, useQuery } from "@tanstack/react-query";
import { type ReactNode, useMemo, useState } from "react";
import { useApp } from "../../lib/use-app.tsx";
import type { StreamState } from "../shell/studio-events.ts";
import { devicesRefresh, useDevLink } from "./dev-link.tsx";
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

/** Where the drawer's events come from: the app's link when it follows this session, else a stream of its own. */
function useDrawerStream(
  client: ReturnType<typeof createClient<typeof DevSessionService>>,
  sessionId: string,
  onEvent: ((event: DevEvent) => void) | undefined,
  onStreamState: ((state: StreamState) => void) | undefined,
) {
  const link = useDevLink();
  const linked = link !== undefined && link.sessionId === sessionId;
  // Read again every few seconds: a device pairs, connects and drops while the drawer is open, and the list (with
  // whether each is connected) would otherwise stay as it was when the drawer opened.
  const devices = useQuery({
    queryKey: ["dev-devices", sessionId],
    enabled: !linked,
    queryFn: async () => (await client.listDevices({ sessionId })).devices,
    refetchInterval: devicesRefresh,
  });
  const own = useDeviceEvents(linked ? undefined : sessionId, [], onEvent, onStreamState);
  if (linked) {
    return link;
  }
  return { buffer: own.buffer, pause: own.pause, resume: own.resume, stream: own.state, devices: devices.data ?? [] };
}

/** The filters above a device tab's rows: the source device, the level, search, pause and trace export. */
function ConsoleFilters(props: {
  readonly filter: DrawerFilter;
  readonly onFilter: (filter: DrawerFilter) => void;
  readonly devices: readonly DevDevice[];
  readonly paused: boolean;
  readonly waiting: number;
  readonly onPause: () => void;
  readonly exporting: boolean;
  readonly onExport: () => void;
}): ReactNode {
  const m = devicesMessages(useLingui().i18n);
  const { filter, onFilter } = props;
  const sources = [
    { value: allDevices, label: m.allDevices },
    ...props.devices.map((d) => ({ value: d.id, label: m.deviceName(d.name, d.connected) })),
  ];
  const levels: readonly [LevelFilter, string][] = [
    ["all", m.levelAll],
    ["warning", m.levelWarning],
    ["error", m.levelError],
  ];
  return (
    <div className="console-filters">
      <Select
        label={m.source}
        value={filter.deviceId === "" ? allDevices : filter.deviceId}
        options={sources}
        onValueChange={(deviceId) => onFilter({ ...filter, deviceId: deviceId === allDevices ? "" : deviceId })}
      />
      <fieldset aria-label={m.level} className="console-levels">
        {levels.map(([value, label]) => (
          <button
            key={value}
            type="button"
            className="console-level-toggle"
            aria-pressed={filter.level === value}
            onClick={() => onFilter({ ...filter, level: value })}
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
          onChange={(e) => onFilter({ ...filter, search: e.target.value })}
        />
      </InputGroup>
      <span className="console-filters-end">
        {props.waiting > 0 && (
          <span role="status" className="text-caption text-muted-foreground">
            {m.pausedCount(props.waiting)}
          </span>
        )}
        <Button size="sm" variant="ghost" onClick={props.onPause}>
          <Icon name={props.paused ? "play" : "pause"} size="pill" />
          {props.paused ? m.resume : m.pause}
        </Button>
        <Button size="sm" variant="ghost" loading={props.exporting} onClick={props.onExport}>
          <Icon name="download" size="pill" />
          {m.exportTrace}
        </Button>
      </span>
    </div>
  );
}

/** The console's tabs: the nine of the drawer, then the device's controls and its last screenshot. */
export type ConsoleTab = DrawerTab | "device" | "screenshot";

/** A tab of the rail: its icon, its name and how many entries it holds. */
function RailTab({
  value,
  icon,
  label,
  count,
}: {
  readonly value: ConsoleTab;
  readonly icon: IconName;
  readonly label: string;
  readonly count?: number | undefined;
}): ReactNode {
  return (
    <TabsTrigger value={value} className="console-rail-tab">
      <Icon name={icon} size="pill" />
      <span className="console-rail-label">{label}</span>
      {(count ?? 0) > 0 && (
        <span className="console-tab-count" aria-hidden="true">
          {count}
        </span>
      )}
    </TabsTrigger>
  );
}

/**
 * The developer console's body (STU-066, DEV-020): a rail of tabs down its side — the nine of the drawer with their
 * icons and live counts, then the device's controls and its last screenshot — and the open tab beside it, each
 * scrolling on its own so everything stays in reach at any height. Problems come from validation; the device tabs
 * from the paired devices of the dev session, with the source device, a level filter, search, pause and trace export
 * above their rows. The app's link (one stream for the app) feeds it when there is one; otherwise it streams the
 * session itself.
 */
export function DeviceDrawer({
  sessionId,
  problems,
  problemCount,
  tab,
  onTab,
  onEvent,
  onStreamState,
  device,
  screenshot,
}: {
  readonly sessionId: string;
  /** The Problems tab's content: validation is not fed by a device. */
  readonly problems: ReactNode;
  readonly problemCount?: number | undefined;
  /** The open tab, when the console owns it; otherwise the drawer keeps its own, starting on Problems. */
  readonly tab?: ConsoleTab;
  readonly onTab?: (tab: ConsoleTab) => void;
  /** Sees every event as it arrives, before the filters, when the drawer streams the session itself. */
  readonly onEvent?: (event: DevEvent) => void;
  /** Hears where the drawer's own event stream is: connecting, live, reconnecting or ended. */
  readonly onStreamState?: (state: StreamState) => void;
  /** The device's controls (open a screen, language, theme, text scale), in a tab of their own. */
  readonly device?: ReactNode;
  /** The device's last screenshot, in a tab of its own once there is one. */
  readonly screenshot?: ReactNode;
}): ReactNode {
  const m = devicesMessages(useLingui().i18n);
  const [own, setOwn] = useState<ConsoleTab>("problems");
  const current = tab ?? own;
  const choose = onTab ?? setOwn;
  const { session } = useApp();
  const client = useMemo(() => createClient(DevSessionService, session.transport), [session.transport]);
  const [filter, setFilter] = useState<DrawerFilter>({ deviceId: "", level: "all", search: "" });
  const stream = useDrawerStream(client, sessionId, onEvent, onStreamState);
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
  const list = stream.devices;
  const names = new Map(list.map((d) => [d.id, d.name]));
  const counts = countsOf(shown, problemCount);
  const tools = (
    <ConsoleFilters
      filter={filter}
      onFilter={setFilter}
      devices={list}
      paused={paused}
      waiting={pending(stream.buffer)}
      onPause={paused ? stream.resume : stream.pause}
      exporting={exportTrace.isPending}
      onExport={() => exportTrace.mutate()}
    />
  );
  const panel = (t: DrawerTab): ReactNode =>
    t === "problems" ? (
      <div className="console-scroll">{problems}</div>
    ) : (
      <>
        {tools}
        {stream.stream === "ended" && <p className="console-note">{m.streamEnded}</p>}
        {exportTrace.isError && (
          <p role="alert" className="console-note">
            {m.failed}
          </p>
        )}
        <div className="console-scroll">
          <ConsoleRows
            rows={shown.filter((e) => tabOf(e) === t)}
            icon={tabIcons[t]}
            label={labels[t]}
            names={names}
            level={lineLevel}
          />
        </div>
      </>
    );
  return (
    <section aria-label={m.drawer} className="console-drawer">
      <ShadcnTabs
        value={current}
        onValueChange={(v) => choose(v as ConsoleTab)}
        orientation="vertical"
        className="console-tabs"
      >
        <TabsList aria-label={m.drawer} className="console-rail">
          {drawerTabs.map((t) => (
            <RailTab key={t} value={t} icon={tabIcons[t]} label={labels[t]} count={counts.get(t)} />
          ))}
          {(device !== undefined || screenshot !== undefined) && (
            <span className="console-rail-split" aria-hidden="true" />
          )}
          {device !== undefined && <RailTab value="device" icon="device-mobile" label={m.tabDevice} />}
          {screenshot !== undefined && <RailTab value="screenshot" icon="camera" label={m.tabScreenshot} />}
        </TabsList>
        {drawerTabs.map((t) => (
          <TabsContent key={t} value={t} className="console-body">
            {panel(t)}
          </TabsContent>
        ))}
        {device !== undefined && (
          <TabsContent value="device" className="console-body">
            <div className="console-scroll">{device}</div>
          </TabsContent>
        )}
        {screenshot !== undefined && (
          <TabsContent value="screenshot" className="console-body">
            <div className="console-scroll">{screenshot}</div>
          </TabsContent>
        )}
      </ShadcnTabs>
    </section>
  );
}
