// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { Code, ConnectError, createRouterTransport, createClient } from "@connectrpc/connect";
import { create } from "@bufbuild/protobuf";
import { AssetSchema, AssetService } from "@plux/api";
import { createStudioI18n } from "@plux/i18n";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { I18nProvider } from "../../i18n-provider.tsx";
import { loadSourceCatalogue } from "../../test-harness.tsx";
import { AssetsTab } from "./assets-tab.tsx";

afterEach(cleanup);

interface Server {
  assets: { id: string; name: string }[];
  held: Uint8Array;
  declared: Uint8Array;
  failNextChunk: boolean;
  calls: string[];
  deleted: string[];
}

function stub(server: Server) {
  const transport = createRouterTransport(({ service }) => {
    service(AssetService, {
      listAssets: () => ({
        assets: server.assets.map((a) => create(AssetSchema, { ...a, mediaType: "image/png", size: 2048n })),
      }),
      deleteAsset: (req) => {
        server.deleted.push(req.id);
        server.assets = server.assets.filter((a) => a.id !== req.id);
        return {};
      },
      beginAssetUpload: (req) => {
        server.calls.push("begin");
        server.declared = req.sha256;
        return { uploadId: "u1", chunkSize: 4n };
      },
      getAssetUpload: () => {
        server.calls.push("get");
        return { size: 10n, received: BigInt(server.held.length) };
      },
      uploadAssetChunk: (req) => {
        server.calls.push(`chunk@${req.offset}`);
        if (server.failNextChunk && req.offset > 0n) {
          server.failNextChunk = false;
          throw new ConnectError("down", Code.Unavailable);
        }
        if (BigInt(server.held.length) !== req.offset) {
          throw new ConnectError("order", Code.OutOfRange);
        }
        server.held = new Uint8Array([...server.held, ...req.chunk]);
        return { received: BigInt(server.held.length) };
      },
      finishAssetUpload: () => {
        server.calls.push("finish");
        server.assets.push({ id: "new", name: "images/logo.png" });
        return { asset: create(AssetSchema, { id: "new", name: "images/logo.png" }) };
      },
    });
  });
  return createClient(AssetService, transport);
}

function renderTab(server: Server) {
  const studio = createStudioI18n({ load: async () => loadSourceCatalogue(), pseudo: false });
  return render(
    <I18nProvider studio={studio}>
      <AssetsTab client={stub(server)} appId="app1" session="tab-1" />
    </I18nProvider>,
  );
}

const fresh = (): Server => ({
  assets: [{ id: "a1", name: "images/old.png" }],
  held: new Uint8Array(),
  declared: new Uint8Array(),
  failNextChunk: false,
  calls: [],
  deleted: [],
});

describe("S2.6 the Assets tab (SRV-060)", () => {
  test("lists the app's assets", async () => {
    renderTab(fresh());
    expect(await screen.findByText("images/old.png")).toBeDefined();
  });

  test("uploads in chunks with the file's SHA-256, then lists the new asset", async () => {
    const server = fresh();
    renderTab(server);
    await screen.findByText("images/old.png");
    const file = new File([new Uint8Array([1, 2, 3, 4, 5, 6, 7, 8, 9, 10])], "Logo.png", { type: "image/png" });
    fireEvent.change(screen.getByLabelText("Upload file"), { target: { files: [file] } });
    await waitFor(() => expect(server.calls).toContain("finish"));
    expect(server.calls).toEqual(["begin", "chunk@0", "chunk@4", "chunk@8", "finish"]);
    const digest = new Uint8Array(await crypto.subtle.digest("SHA-256", await file.arrayBuffer()));
    expect(Array.from(server.declared)).toEqual(Array.from(digest));
    expect(await screen.findByText("images/logo.png")).toBeDefined();
  });

  test("a failed chunk offers Resume, which continues at the server's offset", async () => {
    const server = fresh();
    server.failNextChunk = true;
    renderTab(server);
    await screen.findByText("images/old.png");
    const file = new File([new Uint8Array([1, 2, 3, 4, 5, 6, 7, 8, 9, 10])], "logo.png");
    fireEvent.change(screen.getByLabelText("Upload file"), { target: { files: [file] } });
    fireEvent.click(await screen.findByRole("button", { name: "Resume" }));
    await waitFor(() => expect(server.calls).toContain("finish"));
    expect(server.calls).toEqual(["begin", "chunk@0", "chunk@4", "get", "chunk@4", "chunk@8", "finish"]);
    expect(Array.from(server.held)).toEqual([1, 2, 3, 4, 5, 6, 7, 8, 9, 10]);
  });

  test("delete asks for confirmation first", async () => {
    const server = fresh();
    renderTab(server);
    fireEvent.click(await screen.findByRole("button", { name: "Delete images/old.png" }));
    expect(server.deleted).toEqual([]);
    fireEvent.click(await screen.findByRole("button", { name: "Delete" }));
    await waitFor(() => expect(server.deleted).toEqual(["a1"]));
    expect(await screen.findByText("No assets yet.")).toBeDefined();
  });
});
