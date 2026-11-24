// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { AssetService } from "@plux/api";
import { type ReactNode, useMemo, useRef } from "react";
import { useApp } from "../../lib/use-app.tsx";
import { DeviceBay } from "../devices/device-bay.tsx";
import type { ScreenshotStore } from "../devices/screenshots.ts";
import { useLock } from "../editor/editor-provider.tsx";
import { uploadAsset } from "../panels/upload-asset.ts";
import { underAppLock } from "../plugin-canvas/use-assets-tab.ts";

const noInspect = (): void => undefined;

/**
 * The developer console on the app canvas (#282): the plugin canvas's device drawer — Problems, Logs, Process flow,
 * State, Network, Functions, Sync, Performance, Console, Device — for the app's own dev session, so reading a phone's
 * logs or seeing that it refused a bundle (#257) needs no plugin opened. Its Problems tab is the app's (#258); a
 * screenshot is stored as an app asset under the app lock, which this canvas's editor holds. The link's state and the
 * join prompts stay in the canvas's link bar beside it.
 */
export function AppConsole(props: {
  readonly appId: string;
  /** The route the device tools open: the app's entry. */
  readonly route: string | undefined;
  /** The app's Problems pane: its list, and its count once the check is done. */
  readonly problems: { readonly panel: ReactNode; readonly meta: string };
}): ReactNode {
  const { session } = useApp();
  const lock = useLock();
  const lockRef = useRef(lock);
  lockRef.current = lock;
  const store = useMemo<ScreenshotStore>(() => {
    const client = underAppLock(createClient(AssetService, session.transport), () => lockRef.current);
    return {
      upload: (job) => uploadAsset(client, { appId: props.appId, session: "", ...job }, { current: null }, () => {}),
    };
  }, [session.transport, props.appId]);
  return (
    <DeviceBay
      appId={props.appId}
      problems={props.problems.panel}
      problemCount={props.problems.meta === "" ? undefined : Number(props.problems.meta)}
      selectedPath={undefined}
      onInspect={noInspect}
      accepted={0}
      store={store}
      route={props.route ?? ""}
      linkShown={false}
    />
  );
}
