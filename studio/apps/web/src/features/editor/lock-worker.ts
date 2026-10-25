// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// The lock heartbeat's clock (F1.2): timers in a worker are not throttled like a background tab's.
const scope = self as unknown as {
  addEventListener(type: "message", fn: (e: MessageEvent<{ every?: number }>) => void): void;
  postMessage(message: "tick"): void;
};
let handle: ReturnType<typeof setInterval> | undefined;
scope.addEventListener("message", (e) => {
  clearInterval(handle);
  if (typeof e.data.every === "number") {
    handle = setInterval(() => scope.postMessage("tick"), e.data.every);
  }
});
