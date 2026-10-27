// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import type { DevEvent } from "@plux/api";
import { Button } from "@plux/ui";
import { type ReactNode, useCallback, useEffect, useRef, useState } from "react";
import { type Command, useScopedCommands } from "../../lib/commands/index.ts";
import { DeviceControls } from "./device-controls.tsx";
import { DeviceDrawer } from "./device-drawer.tsx";
import { devicesMessages } from "./devices.messages.ts";
import { createLatencyMeter } from "./latency.ts";
import { PairingDialog } from "./pairing-dialog.tsx";
import { type Capture, ScreenshotPanel } from "./screenshot-panel.tsx";
import type { ScreenshotStore } from "./screenshots.ts";
import { SendToDeviceDialog } from "./send-dialog.tsx";
import { useDevClient, useStudioDevSession } from "./use-dev-session.ts";

/** What the page gives the bay: the canvas selection and the autosave clock. */
export interface DeviceBayProps {
  readonly appId: string;
  /** The Problems tab's content. */
  readonly problems: ReactNode;
  /** What stands under the toolbar until the device drawer is opened: the Problems drawer. */
  readonly collapsed: ReactNode;
  /** The document path of the node selected in Studio, if exactly one is (STU-131). */
  readonly selectedPath: string | undefined;
  /** A device tap in inspect mode: select the node at this document path in Studio (STU-131). */
  readonly onInspect: (path: string) => void;
  /** Counts the autosaves the server accepted; each increment starts an edit-to-device measurement (NFR-030). */
  readonly accepted: number;
  readonly store: ScreenshotStore;
  readonly route: string;
  readonly now?: () => number;
}

type Dialog = "pair" | "send" | null;

/** Pairing and send-to-device as toolbar buttons and palette commands (S6.5 moves them into the action bar). */
function useDeviceCommands(open: (d: Dialog) => void): void {
  const m = devicesMessages(useLingui().i18n);
  const list: readonly Command[] = [
    { id: "device.pair", title: m.pairAction, group: "navigate", permission: null, run: () => open("pair") },
    { id: "device.send", title: m.sendAction, group: "navigate", permission: null, run: () => open("send") },
  ];
  useScopedCommands("devices", list);
}

/**
 * The device loop under the canvas (STU-131 to STU-134, WGT-031, NFR-030, DEV-020): the toolbar, the drawer, the
 * device controls, the screenshot and the measured time from an accepted autosave to the device applying it.
 */
export function DeviceBay(props: DeviceBayProps): ReactNode {
  const m = devicesMessages(useLingui().i18n);
  const [shown, setShown] = useState(false);
  const [dialog, setDialog] = useState<Dialog>(null);
  const [highlight, setHighlight] = useState(false);
  const [capture, setCapture] = useState<Capture | undefined>(undefined);
  const [latency, setLatency] = useState<number | undefined>(undefined);
  const sessionId = useStudioDevSession(props.appId, shown || dialog !== null);
  const client = useDevClient();
  const meter = useRef(createLatencyMeter(props.now ?? Date.now));
  const open = useCallback((d: Dialog) => setDialog(d), []);
  useDeviceCommands(open);
  const { accepted, selectedPath, onInspect } = props;
  const first = useRef(accepted);
  useEffect(() => {
    if (accepted !== first.current) {
      meter.current.accepted();
    }
  }, [accepted]);
  useEffect(() => {
    if (sessionId !== undefined && highlight && selectedPath !== undefined) {
      void client.sendCommand({
        sessionId,
        deviceId: "",
        command: { case: "inspectSelect", value: { nodePath: selectedPath } },
      });
    }
  }, [client, sessionId, highlight, selectedPath]);
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
      } else if (body.case === "hotApply") {
        setLatency(meter.current.applied());
      }
    },
    [onInspect],
  );
  const takeShot = () => {
    if (sessionId !== undefined) {
      void client.sendCommand({ sessionId, deviceId: "", command: { case: "captureScreenshot", value: {} } });
    }
  };
  return (
    <div className="absolute inset-x-4 bottom-12 flex flex-col gap-2">
      <div className="flex flex-wrap items-center gap-2">
        <Button size="sm" variant="secondary" aria-expanded={shown} onClick={() => setShown(!shown)}>
          {m.bayToggle}
        </Button>
        <Button size="sm" variant="ghost" onClick={() => setDialog("pair")}>
          {m.pairAction}
        </Button>
        <Button size="sm" variant="ghost" onClick={() => setDialog("send")}>
          {m.sendAction}
        </Button>
        {latency !== undefined && (
          <span role="status" className="text-caption text-secondary">
            {m.latency(Math.round(latency))}
          </span>
        )}
      </div>
      {!shown && props.collapsed}
      {shown && sessionId !== undefined && (
        <>
          <div className="flex flex-wrap items-center gap-2 rounded-lg bg-raised p-2 shadow-e1">
            <Button
              size="sm"
              variant={highlight ? "primary" : "ghost"}
              aria-pressed={highlight}
              onClick={() => setHighlight(!highlight)}
            >
              {m.highlight}
            </Button>
            <DeviceControls sessionId={sessionId} deviceId="" initialRoute={props.route} />
            <Button size="sm" variant="secondary" onClick={takeShot}>
              {m.screenshotCapture}
            </Button>
          </div>
          {capture !== undefined && (
            <ScreenshotPanel capture={capture} store={props.store} defaultTarget={props.route} />
          )}
          <DeviceDrawer sessionId={sessionId} problems={props.problems} onEvent={onEvent} />
        </>
      )}
      {dialog === "pair" && sessionId !== undefined && (
        <PairingDialog sessionId={sessionId} onClose={() => setDialog(null)} />
      )}
      {dialog === "send" && sessionId !== undefined && (
        <SendToDeviceDialog sessionId={sessionId} initialRoute={props.route} onClose={() => setDialog(null)} />
      )}
    </div>
  );
}
