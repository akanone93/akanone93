// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import type { DevEvent } from "@plux/api";
import { Button, Icon } from "@plux/ui";
import { Tabs as ShadcnTabs, TabsContent, TabsList, TabsTrigger } from "@plux/ui/components/tabs";
import {
  type ReactNode,
  type PointerEvent as ReactPointerEvent,
  type RefObject,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { type Command, useScopedCommands } from "../../lib/commands/index.ts";
import {
  type ConsoleStatus,
  DevLinkPill,
  DevLinkPrompts,
  DevLinkProvider,
  useDevLink,
  useDevLinkEvents,
} from "./dev-link.tsx";
import { DeviceControls } from "./device-controls.tsx";
import { type ConsoleTab, DeviceDrawer } from "./device-drawer.tsx";
import { devicesMessages } from "./devices.messages.ts";
import type { DrawerTab } from "./drawer-model.ts";
import { createLatencyMeter, type LatencyMeter } from "./latency.ts";
import { PairingDialog } from "./pairing-dialog.tsx";
import { type Capture, ScreenshotPanel } from "./screenshot-panel.tsx";
import type { ScreenshotStore } from "./screenshots.ts";
import { SendToDeviceDialog } from "./send-dialog.tsx";
import { useDevClient } from "./use-dev-session.ts";

export { type ConsoleStatus, consoleStatus } from "./dev-link.tsx";

/** What the page gives the bay: the canvas selection and the autosave clock. */
export interface DeviceBayProps {
  readonly appId: string;
  /** The Problems tab's content. */
  readonly problems: ReactNode;
  /** How many problems validation found, for the console's bar and its Problems tab. */
  readonly problemCount?: number | undefined;
  /** Opens the console on a tab from elsewhere (the top bar's problem counts); a new `n` opens it again. */
  readonly request?: { readonly tab: DrawerTab; readonly n: number } | undefined;
  /** The document path of the node selected in Studio, if exactly one is (STU-131). */
  readonly selectedPath: string | undefined;
  /** A device tap in inspect mode: select the node at this document path in Studio (STU-131). */
  readonly onInspect: (path: string) => void;
  /** Counts the autosaves the server accepted; each increment starts an edit-to-device measurement (NFR-030). */
  readonly accepted: number;
  readonly store: ScreenshotStore;
  readonly route: string;
  /** The canvas's drawing of a screen, to compare a device screenshot with (PC-38). */
  readonly canvasImage?: ((screen: string) => Promise<Uint8Array | null>) | undefined;
  readonly now?: () => number;
  /**
   * Starts the dev session as soon as the editor opens (for a person who may create one): a tap on the device selects
   * in Studio (STU-131) and the autosave time is measured (NFR-030) whether or not the console is open. Under the
   * app's link the link starts it.
   */
  readonly live?: boolean;
}

type Dialog = "pair" | "send" | null;

/** Pairing and send-to-device as palette commands (the plugin canvas action bar runs them (STU-060)). */
function useDeviceCommands(open: (d: Dialog) => void): void {
  const m = devicesMessages(useLingui().i18n);
  const list: readonly Command[] = [
    { id: "device.pair", title: m.pairAction, group: "navigate", permission: null, run: () => open("pair") },
    { id: "device.send", title: m.sendAction, group: "navigate", permission: null, run: () => open("send") },
  ];
  useScopedCommands("devices", list);
}

const MIN_HEIGHT = 220;
const STEP = 32;

const clamp = (h: number) => Math.round(Math.max(MIN_HEIGHT, Math.min(window.innerHeight * 0.7, h)));

/** The handle on the open console's top edge: drag it, or focus it and press the arrow keys, to change the height. */
function ConsoleGrip({ height, onHeight }: { readonly height: number; readonly onHeight: (h: number) => void }) {
  const m = devicesMessages(useLingui().i18n);
  const drag = (e: ReactPointerEvent<HTMLDivElement>) => {
    const from = e.clientY;
    const move = (ev: PointerEvent) => onHeight(clamp(height + from - ev.clientY));
    const up = () => {
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerup", up);
    };
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", up);
  };
  return (
    // biome-ignore lint/a11y/useSemanticElements: a focusable splitter (WAI-ARIA window splitter), not a rule
    <div
      role="separator"
      aria-orientation="horizontal"
      aria-label={m.consoleResize}
      aria-valuenow={height}
      aria-valuemin={MIN_HEIGHT}
      tabIndex={0}
      className="dev-console-grip"
      onPointerDown={drag}
      onKeyDown={(e) => {
        if (e.key === "ArrowUp" || e.key === "ArrowDown") {
          e.preventDefault();
          onHeight(clamp(height + (e.key === "ArrowUp" ? STEP : -STEP)));
        }
      }}
    />
  );
}

/** The console's bar: its toggle, whether it streams, the problems, the autosave time and, live, the device tools. */
function ConsoleBar(props: {
  readonly shown: boolean;
  readonly status: ConsoleStatus;
  readonly live: boolean;
  readonly problemCount: number | undefined;
  readonly latency: number | undefined;
  readonly highlight: boolean;
  readonly onToggle: () => void;
  readonly onProblems: () => void;
  readonly onHighlight: () => void;
  readonly onShot: () => void;
}) {
  const m = devicesMessages(useLingui().i18n);
  return (
    <div className="dev-console-bar">
      <button type="button" className="dev-console-toggle" aria-expanded={props.shown} onClick={props.onToggle}>
        <Icon name="terminal" size="pill" />
        {m.bayToggle}
        <Icon name={props.shown ? "caret-down" : "caret-up"} size="pill" />
      </button>
      <DevLinkPill status={props.status} />
      <button
        type="button"
        className="dev-console-chip"
        data-alert={(props.problemCount ?? 0) > 0 || undefined}
        onClick={props.onProblems}
      >
        <Icon name="warning-circle" size="pill" />
        {m.tabProblems}
        {props.problemCount !== undefined && (
          <span className="console-tab-count" aria-hidden="true">
            {props.problemCount}
          </span>
        )}
      </button>
      {props.latency !== undefined && (
        <span role="status" className="dev-console-meta">
          <Icon name="lightning" size="pill" />
          {m.latency(Math.round(props.latency))}
        </span>
      )}
      {props.live && (
        <span className="dev-console-tools">
          {/* Icons with their names as labels and tips, so the bar stays one line. */}
          <Button
            size="sm"
            variant={props.highlight ? "primary" : "ghost"}
            aria-pressed={props.highlight}
            aria-label={m.highlight}
            title={m.highlight}
            onClick={props.onHighlight}
          >
            <Icon name="crosshair" size="pill" />
          </Button>
          <Button
            size="sm"
            variant="ghost"
            aria-label={m.screenshotCapture}
            title={m.screenshotCapture}
            onClick={props.onShot}
          >
            <Icon name="camera" size="pill" />
          </Button>
        </span>
      )}
    </div>
  );
}

/** The open console without a dev session: the Problems tab alone, and why the device tabs are missing. */
function ProblemsOnly({ problems }: { readonly problems: ReactNode }) {
  const m = devicesMessages(useLingui().i18n);
  return (
    <ShadcnTabs value="problems" className="console-tabs console-tabs-single">
      <div className="console-head">
        <TabsList variant="line" className="console-tablist">
          <TabsTrigger value="problems" className="console-tab">
            <Icon name="warning-circle" size="pill" />
            {m.tabProblems}
          </TabsTrigger>
        </TabsList>
        <span className="console-note">{m.consoleProblemsOnly}</span>
      </div>
      <TabsContent value="problems" className="console-body">
        <div className="console-scroll">{problems}</div>
      </TabsContent>
    </ShadcnTabs>
  );
}

/** Tells the canvas stage the console's height while it is open, so the page lifts its zoom above it. */
function useConsoleStage(box: RefObject<HTMLElement | null>, shown: boolean, height: number): void {
  useEffect(() => {
    const stage = box.current?.closest<HTMLElement>(".canvas-stage");
    if (stage === null || stage === undefined) {
      return undefined;
    }
    stage.toggleAttribute("data-console-open", shown);
    stage.style.setProperty("--console-block", String(height));
    return () => stage.removeAttribute("data-console-open");
  }, [box, shown, height]);
}

/** What the device stream tells the bay: a tap to inspect, a screenshot, and when an autosave was applied. */
function useBayEvents(onInspect: (path: string) => void, meter: LatencyMeter, onCapture: () => void) {
  const [capture, setCapture] = useState<Capture | undefined>(undefined);
  const [latency, setLatency] = useState<number | undefined>(undefined);
  const onEvent = useCallback(
    (event: DevEvent) => {
      const body = event.payload;
      if (body.case === "inspect") {
        onInspect(body.value.nodePath);
      } else if (body.case === "screenshot") {
        setCapture({
          png: new Uint8Array(body.value.png),
          platformViewsNotCaptured: body.value.platformViewsNotCaptured,
        });
        onCapture();
      } else if (body.case === "hotApply") {
        setLatency(meter.applied());
      }
    },
    [onInspect, meter, onCapture],
  );
  useDevLinkEvents(onEvent);
  return { capture, latency };
}

/**
 * The developer console under the canvas (STU-066, STU-131 to STU-134, WGT-031, NFR-030, DEV-020): always there as a
 * slim bar between the docks — its name, the app's link state, the problems and the autosave time — and open, a
 * drawer that resizes from its top edge, with a rail of tabs down its side: the nine of the drawer, the device's
 * controls and its last screenshot. It follows the app's link with its devices; without one around it (a plugin
 * opened alone) it opens its own when the console opens.
 */
export function DeviceBay(props: DeviceBayProps): ReactNode {
  const link = useDevLink();
  const [asked, setAsked] = useState(false);
  const ask = useCallback(() => setAsked(true), []);
  if (link !== undefined && link.appId === props.appId) {
    return <Bay {...props} onAsk={ask} />;
  }
  return (
    <DevLinkProvider appId={props.appId} enabled={props.live === true || asked}>
      <Bay {...props} onAsk={ask} />
    </DevLinkProvider>
  );
}

function Bay(props: DeviceBayProps & { readonly onAsk: () => void }): ReactNode {
  const link = useDevLink();
  const [shown, setShown] = useState(false);
  const [tab, setTab] = useState<ConsoleTab>("problems");
  const [height, setHeight] = useState(360);
  const [dialog, setDialog] = useState<Dialog>(null);
  const [highlight, setHighlight] = useState(false);
  const { canvasImage, route, onAsk } = props;
  // The canvas's drawing of the screen the device shows, kept while the screen is the same (PC-38).
  const compareCanvas = useMemo(
    () => (canvasImage === undefined ? undefined : () => canvasImage(route)),
    [canvasImage, route],
  );
  useEffect(() => {
    if (shown || dialog !== null) {
      onAsk();
    }
  }, [shown, dialog, onAsk]);
  const sessionId = link?.sessionId;
  const client = useDevClient();
  const [meter] = useState(() => createLatencyMeter(props.now ?? Date.now));
  const open = useCallback((d: Dialog) => setDialog(d), []);
  useDeviceCommands(open);
  const { accepted, selectedPath, onInspect, request } = props;
  const showShot = useCallback(() => {
    setShown(true);
    setTab("screenshot");
  }, []);
  const { capture, latency } = useBayEvents(onInspect, meter, showShot);
  const status = link?.status ?? { kind: "idle" };
  const first = useRef(accepted);
  useEffect(() => {
    if (accepted !== first.current) {
      meter.accepted();
    }
  }, [accepted, meter]);
  useEffect(() => {
    if (request !== undefined) {
      setShown(true);
      setTab(request.tab);
    }
  }, [request]);
  useEffect(() => {
    if (sessionId !== undefined && highlight && selectedPath !== undefined) {
      void client.sendCommand({
        sessionId,
        deviceId: "",
        command: { case: "inspectSelect", value: { nodePath: selectedPath } },
      });
    }
  }, [client, sessionId, highlight, selectedPath]);
  const takeShot = () => {
    if (sessionId !== undefined) {
      void client.sendCommand({ sessionId, deviceId: "", command: { case: "captureScreenshot", value: {} } });
    }
  };
  const box = useRef<HTMLElement>(null);
  useConsoleStage(box, shown, height);
  const m = devicesMessages(useLingui().i18n);
  return (
    <section
      ref={box}
      aria-label={m.bayToggle}
      className="dev-console absolute bottom-3 z-10"
      data-open={shown || undefined}
    >
      <DevLinkPrompts />
      <div className="canvas-panel glass dev-console-panel" style={shown ? { blockSize: height } : undefined}>
        {shown && <ConsoleGrip height={height} onHeight={setHeight} />}
        <ConsoleBar
          shown={shown}
          status={status}
          live={status.kind === "live"}
          problemCount={props.problemCount}
          latency={latency}
          highlight={highlight}
          onToggle={() => setShown(!shown)}
          onProblems={() => {
            setShown(true);
            setTab("problems");
          }}
          onHighlight={() => setHighlight(!highlight)}
          onShot={takeShot}
        />
        {shown && (
          <div className="dev-console-body">
            {sessionId === undefined ? (
              <ProblemsOnly problems={props.problems} />
            ) : (
              <DeviceDrawer
                sessionId={sessionId}
                problems={props.problems}
                problemCount={props.problemCount}
                tab={tab}
                onTab={setTab}
                device={
                  <div className="console-device">
                    <DeviceControls sessionId={sessionId} deviceId="" initialRoute={props.route} />
                  </div>
                }
                screenshot={
                  capture === undefined ? undefined : (
                    <ScreenshotPanel
                      capture={capture}
                      store={props.store}
                      defaultTarget={props.route}
                      canvas={compareCanvas}
                    />
                  )
                }
              />
            )}
          </div>
        )}
      </div>
      {dialog === "pair" && sessionId !== undefined && (
        <PairingDialog sessionId={sessionId} onClose={() => setDialog(null)} />
      )}
      {dialog === "send" && sessionId !== undefined && (
        <SendToDeviceDialog sessionId={sessionId} initialRoute={props.route} onClose={() => setDialog(null)} />
      )}
    </section>
  );
}
