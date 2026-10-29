// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { crc32, packArchive, zipSummary } from "./plux-archive.ts";
import { jsonOf, readZip } from "./test-zip.ts";

const encode = (text: string) => new TextEncoder().encode(text);
const app = { key: "shop", name: "Shop", description: "", tags: [] };

describe("the .plux archive writer [STU-024]", () => {
  test("the CRC-32 is the IEEE one ZIP stores", () => {
    expect(crc32(encode("123456789"))).toBe(0xcbf43926);
    expect(crc32(new Uint8Array())).toBe(0);
  });

  test("the manifest comes first, files follow sorted under project/, stored with the ZIP epoch and mode 0644", async () => {
    const bytes = await packArchive(app, [
      { path: "theme.json", content: encode("{}") },
      { path: "app.json", content: encode('{"a":1}') },
      { path: "plugins/main/plugin.json", content: encode("[]") },
    ]);
    const entries = readZip(bytes);
    expect(entries.map((e) => e.name)).toEqual([
      "plux-archive.json",
      "project/app.json",
      "project/plugins/main/plugin.json",
      "project/theme.json",
    ]);
    for (const entry of entries) {
      expect([entry.method, entry.dosTime, entry.dosDate, entry.mode]).toEqual([0, 0, 0x0021, 0o100644]);
    }
  });

  test("the manifest lists every file with its size and SHA-256 and the app's descriptive fields", async () => {
    const bytes = await packArchive({ ...app, description: "d", tags: ["x"] }, [
      { path: "app.json", content: encode("abc") },
    ]);
    const manifest = jsonOf(readZip(bytes), "plux-archive.json");
    expect(manifest).toEqual({
      format: "plux-archive",
      version: 1,
      app: { key: "shop", name: "Shop", description: "d", tags: ["x"] },
      files: [
        {
          path: "app.json",
          size: 3,
          sha256: "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
        },
      ],
    });
  });

  test("the same files give the same bytes, whatever order they come in", async () => {
    const a = { path: "a.json", content: encode("1") };
    const b = { path: "b.json", content: encode("2") };
    expect(await packArchive(app, [a, b])).toEqual(await packArchive(app, [b, a]));
  });

  test("the summary counts the entries and their declared sizes without unzipping", async () => {
    const bytes = await packArchive(app, [
      { path: "a.json", content: encode("12345") },
      { path: "b.json", content: encode("123") },
    ]);
    const manifestSize = readZip(bytes)[0]?.data.length ?? 0;
    expect(zipSummary(bytes)).toEqual({ entries: 3, expandedBytes: manifestSize + 8 });
  });

  test("bytes that are not a ZIP file have no summary", () => {
    expect(zipSummary(encode("not a zip file at all, just text"))).toBeUndefined();
    expect(zipSummary(new Uint8Array())).toBeUndefined();
    const truncated = new Uint8Array(30);
    new DataView(truncated.buffer).setUint32(8, 0x06054b50, true);
    new DataView(truncated.buffer).setUint16(18, 2, true);
    expect(zipSummary(truncated)).toBeUndefined();
  });
});
