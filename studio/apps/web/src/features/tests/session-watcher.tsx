// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { DevEvent } from "@plux/api";
import { useDeviceEvents } from "../devices/use-device-events.ts";

/** Streams a device-loop session's events into `onEvent` while mounted; it draws nothing. */
export function SessionWatcher(props: {
  readonly sessionId: string;
  readonly onEvent: (event: DevEvent) => void;
}): null {
  useDeviceEvents(props.sessionId, [], props.onEvent);
  return null;
}
