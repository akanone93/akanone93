// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { createClient, createRouterTransport } from "@connectrpc/connect";
import { AppService, DevPhoneService } from "@plux/api";
import { createSession } from "./session.ts";

/** A server that records the organisation header of each call. */
function recording() {
  const seen: (string | null)[] = [];
  const transport = createRouterTransport(({ service }) => {
    service(DevPhoneService, {
      listPhones: (_req, ctx) => {
        seen.push(ctx.requestHeader.get("X-Plux-Organization"));
        return { phones: [] };
      },
    });
    service(AppService, {
      listApps: (_req, ctx) => {
        seen.push(ctx.requestHeader.get("X-Plux-Organization"));
        return {};
      },
    });
  });
  return { seen, transport };
}

describe("the organisation a call names", () => {
  test("a phone call on a page outside any organisation names the home one (DEV-002, ADR-0104)", async () => {
    const { seen, transport } = recording();
    const session = createSession("/api", transport);
    session.setHome("org-home");
    await createClient(DevPhoneService, session.transport).listPhones({});
    // A page's own call names no organisation there.
    await createClient(AppService, session.transport).listApps({});
    expect(seen).toEqual(["org-home", null]);
  });

  test("inside an organisation every call names it", async () => {
    const { seen, transport } = recording();
    const session = createSession("/api", transport);
    session.setHome("org-home");
    session.setOrganization("org-b");
    await createClient(DevPhoneService, session.transport).listPhones({});
    await createClient(AppService, session.transport).listApps({});
    expect(seen).toEqual(["org-b", "org-b"]);
  });
});
