// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import type { UnaryRequest } from "@connectrpc/connect";
import { AppService } from "./index.ts";
import { headerInterceptor } from "./transport.ts";

async function run(csrf: string | undefined, org: string | undefined): Promise<Headers> {
  const seen: Headers[] = [];
  const next = (async (req: UnaryRequest) => {
    seen.push(req.header);
    return {} as never;
  }) as never;
  const req = { header: new Headers() } as UnaryRequest;
  await headerInterceptor({ csrfToken: () => csrf, organization: () => org })(next)(req);
  return seen[0] as Headers;
}

describe("STU-001 API clients", () => {
  test("interceptor adds the CSRF and organisation headers when present", async () => {
    const h = await run("tok", "org-1");
    expect(h.get("X-CSRF-Token")).toBe("tok");
    expect(h.get("X-Plux-Organization")).toBe("org-1");
  });

  test("interceptor omits the headers when undefined", async () => {
    const h = await run(undefined, undefined);
    expect(h.has("X-CSRF-Token")).toBe(false);
    expect(h.has("X-Plux-Organization")).toBe(false);
  });

  test("generated descriptor has the expected type name", () => {
    expect(AppService.typeName).toBe("plux.v1.AppService");
  });
});
