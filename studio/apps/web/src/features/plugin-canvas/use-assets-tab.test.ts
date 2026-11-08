// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import type { Client } from "@connectrpc/connect";
import type { AssetService } from "@plux/api";
import { type LockController, LockUnavailable } from "../editor/lock-controller.ts";
import { underAppLock } from "./use-assets-tab.ts";

type AssetClient = Client<typeof AssetService>;

/** A client that records the session of each write. */
function recording(): { client: AssetClient; sessions: string[] } {
  const sessions: string[] = [];
  const client = {
    beginAssetUpload: async (req: { session?: string }) => {
      sessions.push(req.session ?? "");
      return { uploadId: "u", chunkSize: 1n };
    },
    deleteAsset: async (req: { session?: string }) => {
      sessions.push(req.session ?? "");
      return {};
    },
    listAssets: async () => ({ assets: [] }),
  } as unknown as AssetClient;
  return { client, sessions };
}

const lock = (session: string) => ({ whenHeld: async () => session }) as unknown as LockController;

describe("assets under the app's editing lock [SRV-040] [SRV-060]", () => {
  test("an upload and a delete are sent under the app lock's session, not the caller's", async () => {
    const { client, sessions } = recording();
    const locked = underAppLock(client, () => lock("app-lock"));
    await locked.beginAssetUpload({ appId: "a", name: "images/x.png", session: "editor-tab" });
    await locked.deleteAsset({ id: "x", session: "editor-tab" });
    expect(sessions).toEqual(["app-lock", "app-lock"]);
  });

  test("a write before the app session is ready is refused, and nothing is sent", async () => {
    const { client, sessions } = recording();
    const locked = underAppLock(client, () => undefined);
    await expect(locked.beginAssetUpload({ appId: "a", name: "images/x.png" })).rejects.toBeInstanceOf(LockUnavailable);
    expect(sessions).toEqual([]);
  });

  test("reads pass through", async () => {
    const { client } = recording();
    expect((await underAppLock(client, () => undefined).listAssets({ appId: "a" })).assets).toEqual([]);
  });
});
