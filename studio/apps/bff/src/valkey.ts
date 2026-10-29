// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { RedisClient } from "bun";
import { MemoryRateLimiter, type RateLimiter } from "./limiter.ts";

export type CounterStore = {
  incr(key: string): Promise<number>;
  expire(key: string, seconds: number): Promise<unknown>;
};

/** Fixed-window limiter on Valkey INCR+EXPIRE shared by BFF replicas; falls back to local counting on error. */
export class ValkeyRateLimiter implements RateLimiter {
  private readonly local: MemoryRateLimiter;
  constructor(
    private readonly store: CounterStore,
    private readonly perMinute: number,
    private readonly now: () => number = Date.now,
    private readonly scope = "rl",
  ) {
    this.local = new MemoryRateLimiter(perMinute, now);
  }

  async allow(key: string): Promise<boolean> {
    try {
      const k = `plux:bff:${this.scope}:${key}:${Math.floor(this.now() / 60000)}`;
      const n = await this.store.incr(k);
      if (n === 1) {
        await this.store.expire(k, 120);
      }
      return n <= this.perMinute;
    } catch {
      return this.local.allow(key);
    }
  }
}

/** A limiter on Valkey; scope keeps the counters of different limits apart. */
export function connectValkey(url: string, perMinute: number, scope = "rl"): RateLimiter {
  return new ValkeyRateLimiter(new RedisClient(url), perMinute, Date.now, scope);
}
