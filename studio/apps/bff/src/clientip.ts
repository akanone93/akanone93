// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { BlockList, isIPv4 } from "node:net";

/** Client address resolution: X-Forwarded-For counts only when the peer is a trusted proxy. */
export function makeClientIp(trustedProxies: string[], socketIp: (req: Request) => string): (req: Request) => string {
  const trusted = new BlockList();
  for (const cidr of trustedProxies) {
    const [addr, bits] = cidr.split("/");
    if (addr !== undefined) {
      trusted.addSubnet(addr, Number(bits ?? (isIPv4(addr) ? 32 : 128)), isIPv4(addr) ? "ipv4" : "ipv6");
    }
  }
  const isTrusted = (a: string) => trustedProxies.length > 0 && trusted.check(a, isIPv4(a) ? "ipv4" : "ipv6");
  /** X-Forwarded-For is honoured only from a trusted proxy; the rightmost untrusted hop is the client. */
  const ip = (req: Request): string => {
    const peer = socketIp(req);
    const xff = req.headers.get("x-forwarded-for");
    if (xff === null || !isTrusted(peer)) {
      return peer;
    }
    const hops = xff.split(",").map((x) => x.trim());
    for (let i = hops.length - 1; i >= 0; i--) {
      const h = hops[i] ?? "";
      if (h !== "" && !isTrusted(h)) {
        return h;
      }
    }
    return peer;
  };
  return ip;
}
