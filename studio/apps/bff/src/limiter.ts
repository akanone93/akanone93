// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/** Counts hits per key in a sliding minute. S0.4b can supply a Valkey-backed implementation. */
export interface RateLimiter {
  allow(key: string): boolean;
}

export class MemoryRateLimiter implements RateLimiter {
  private readonly hits = new Map<string, number[]>();
  constructor(
    private readonly perMinute: number,
    private readonly now: () => number = Date.now,
  ) {}

  allow(key: string): boolean {
    const t = this.now();
    const recent = (this.hits.get(key) ?? []).filter((x) => t - x < 60000);
    if (recent.length >= this.perMinute) {
      this.hits.set(key, recent);
      return false;
    }
    recent.push(t);
    this.hits.set(key, recent);
    if (this.hits.size > 10000) {
      this.sweep(t);
    }
    return true;
  }

  private sweep(t: number): void {
    for (const [k, v] of this.hits) {
      if (v.every((x) => t - x >= 60000)) {
        this.hits.delete(k);
      }
    }
  }
}

/** Tracks open streams per session and in total. */
export class StreamCounter {
  private readonly perKey = new Map<string, number>();
  private total = 0;
  constructor(
    private readonly perSession: number,
    private readonly overall: number,
  ) {}

  acquire(key: string): (() => void) | undefined {
    const n = this.perKey.get(key) ?? 0;
    if (n >= this.perSession || this.total >= this.overall) {
      return undefined;
    }
    this.perKey.set(key, n + 1);
    this.total += 1;
    let done = false;
    return () => {
      if (done) {
        return;
      }
      done = true;
      this.total -= 1;
      const left = (this.perKey.get(key) ?? 1) - 1;
      if (left <= 0) {
        this.perKey.delete(key);
      } else {
        this.perKey.set(key, left);
      }
    };
  }

  get open(): number {
    return this.total;
  }
}

export async function sha256Hex(text: string): Promise<string> {
  const d = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(text));
  return Array.from(new Uint8Array(d), (b) => b.toString(16).padStart(2, "0")).join("");
}
