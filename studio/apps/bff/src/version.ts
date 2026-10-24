// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/** Parses "1.2.3" or "v1.2.3-rc" into [major, minor]; undefined when not a version. */
export function majorMinor(v: string): [number, number] | undefined {
  const m = /^v?(\d+)\.(\d+)/.exec(v);
  return m === null ? undefined : [Number(m[1]), Number(m[2])];
}

export function isOlder(version: string, minimum: string): boolean {
  const a = majorMinor(version);
  const b = majorMinor(minimum);
  if (a === undefined || b === undefined) {
    return false;
  }
  return a[0] !== b[0] ? a[0] < b[0] : a[1] < b[1];
}

/** Reads the upstream's version from its /version endpoint; undefined when it does not report one. */
export async function upstreamVersion(upstream: string, timeoutMs: number): Promise<string | undefined> {
  try {
    const r = await fetch(new URL("/version", upstream), { signal: AbortSignal.timeout(timeoutMs) });
    if (!r.ok) {
      await r.body?.cancel();
      return undefined;
    }
    const body = (await r.json()) as { version?: unknown };
    return typeof body.version === "string" ? body.version : undefined;
  } catch {
    return undefined;
  }
}
