// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { loadGhostImages, sha256Hex, syncThumbnails, thumbnailName } from "./thumbnails.ts";

const png = (n: number) => new Uint8Array([137, 80, 78, 71, n]);

describe("STU-052 thumbnails upload only when changed (A5.6)", () => {
  const make = (stored: { name: string; sha256: string }[]) => {
    const uploads: string[] = [];
    let captures = 0;
    const deps = {
      capture: async () => {
        captures++;
        return png(captures > 100 ? 0 : 1);
      },
      list: async () => stored,
      upload: async (job: { name: string }) => {
        uploads.push(job.name);
      },
    };
    return { deps, uploads };
  };

  test("uploads a new thumbnail, then skips it while the hash is unchanged", async () => {
    const { deps, uploads } = make([]);
    const known = new Map<string, string>();
    expect(await syncThumbnails(deps, ["a"], known)).toEqual(["a"]);
    expect(await syncThumbnails(deps, ["a"], known)).toEqual([]);
    expect(uploads).toEqual([thumbnailName("a")]);
  });

  test("skips an upload when the server already holds the same hash", async () => {
    const hash = await sha256Hex(png(1));
    const { deps, uploads } = make([{ name: thumbnailName("a"), sha256: hash }]);
    expect(await syncThumbnails(deps, ["a"], new Map())).toEqual([]);
    expect(uploads).toEqual([]);
  });

  test("uploads again when the picture changed", async () => {
    const { deps, uploads } = make([{ name: thumbnailName("a"), sha256: "old" }]);
    expect(await syncThumbnails(deps, ["a"], new Map())).toEqual(["a"]);
    expect(uploads).toHaveLength(1);
  });

  test("a screen the worker cannot render is skipped", async () => {
    const { deps, uploads } = make([]);
    expect(await syncThumbnails({ ...deps, capture: async () => null }, ["a"], new Map())).toEqual([]);
    expect(uploads).toEqual([]);
  });
});

describe("ghost images", () => {
  const asset = (id: string, sha256: string, url?: string) => ({
    name: thumbnailName(id),
    sha256,
    variants: url === undefined ? [] : [{ artifact: { urls: [url] } }],
  });

  test("fetches thumbnails the worker lacks and names those it holds", async () => {
    const fetched: string[] = [];
    const images = await loadGhostImages(
      [asset("a", "h1", "u1"), asset("b", "h2", "u2"), asset("c", "h3")],
      ["a", "b", "c", "d"],
      async (url) => {
        fetched.push(url);
        return png(2);
      },
      new Set(["h2"]),
    );
    expect(fetched).toEqual(["u1"]);
    expect(images.map((i) => [i.screenId, i.key, i.bytes === null])).toEqual([
      ["a", "h1", false],
      ["b", "h2", true],
    ]);
  });

  test("a failed fetch leaves the ghost without a thumbnail", async () => {
    const images = await loadGhostImages(
      [asset("a", "h1", "u1")],
      ["a"],
      async () => {
        throw new Error("down");
      },
      new Set(),
    );
    expect(images).toEqual([]);
  });
});
