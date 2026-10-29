// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/** Whether this browser can create a WebGL2 context; asked once at start (A5.8). */
export function detectWebGL2(): boolean {
  try {
    return document.createElement("canvas").getContext("webgl2") !== null;
  } catch {
    return false;
  }
}
