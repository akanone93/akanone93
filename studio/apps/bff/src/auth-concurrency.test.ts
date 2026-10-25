// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterAll, beforeAll, describe, expect, test } from "bun:test";
import { createLoginClient } from "./auth.ts";

let upstream: ReturnType<typeof Bun.serve>;

beforeAll(() => {
  upstream = Bun.serve({
    port: 0,
    async fetch(req) {
      // The OIDC code names the session; "slow" sends its headers at once and its body later,
      // so a second completion can start while the first is still reading its response.
      const code = new TextDecoder().decode(new Uint8Array(await req.arrayBuffer())).includes("slow") ? "slow" : "fast";
      const headers = new Headers({ "Content-Type": "application/proto" });
      headers.append("Set-Cookie", `__Host-plux_session=${code}; Path=/; HttpOnly; Secure`);
      // An unknown varint field (number 1000, value 0), split so the headers and first byte go
      // out at once and the rest later; the client skips unknown fields.
      const body = new ReadableStream<Uint8Array>({
        async start(controller) {
          controller.enqueue(new Uint8Array([0xc0, 0x3e]));
          if (code === "slow") {
            await Bun.sleep(150);
          }
          controller.enqueue(new Uint8Array([0x00]));
          controller.close();
        },
      });
      return new Response(body, { headers });
    },
  });
});

afterAll(() => {
  upstream.stop(true);
});

describe("SEC-101 STU-002 sign-in completions are request-scoped", () => {
  test("overlapping completions each get their own session cookie", async () => {
    const login = createLoginClient(`http://127.0.0.1:${upstream.port}`);
    const slow = login.oidc("slow", "s1");
    await Bun.sleep(40);
    const fast = login.oidc("fast", "s2");
    const [a, b] = await Promise.all([slow, fast]);
    expect(a).toEqual({ ok: true, cookies: ["__Host-plux_session=slow; Path=/; HttpOnly; Secure"] });
    expect(b).toEqual({ ok: true, cookies: ["__Host-plux_session=fast; Path=/; HttpOnly; Secure"] });
  });
});
