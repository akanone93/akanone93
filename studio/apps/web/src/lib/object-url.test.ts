// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { objectUrl } from "./object-url.ts";

describe("[SEC-109] object URLs are read same-origin", () => {
  test("the server's object URLs go through the BFF; others are left alone", () => {
    expect(objectUrl("https://localhost:8080/v1/objects/assets/0e/0e7d")).toBe("/api/v1/objects/assets/0e/0e7d");
    expect(objectUrl("https://plux.example/v1/objects/bundles/ab/abcd?v=1")).toBe(
      "/api/v1/objects/bundles/ab/abcd?v=1",
    );
    expect(objectUrl("/v1/objects/assets/aa/aabb")).toBe("/api/v1/objects/assets/aa/aabb");
    expect(objectUrl("https://s3.example/bucket/assets/aa?X-Amz-Signature=1")).toBe(
      "https://s3.example/bucket/assets/aa?X-Amz-Signature=1",
    );
    expect(objectUrl("https://cdn.example/assets/aa")).toBe("https://cdn.example/assets/aa");
    expect(objectUrl("")).toBe("");
  });
});
