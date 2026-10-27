// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The edit-to-device measurement (NFR-030, DEV-010): from the moment the server accepted an autosave to the
 * moment the device reported it applied. The clock is injected so a test drives it.
 */
export interface LatencyMeter {
  /** Marks the server accepting an autosave. */
  accepted(): void;
  /** Marks a device reporting the apply; returns the milliseconds since the oldest unanswered autosave. */
  applied(): number | undefined;
}

export function createLatencyMeter(now: () => number): LatencyMeter {
  const pending: number[] = [];
  return {
    accepted: () => {
      pending.push(now());
    },
    applied: () => {
      const start = pending.shift();
      return start === undefined ? undefined : Math.max(0, now() - start);
    },
  };
}
