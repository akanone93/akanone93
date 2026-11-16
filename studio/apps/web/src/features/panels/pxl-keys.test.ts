// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { referencedKeys } from "../localisation/l10n-model.ts";
import { keyCalls, keyLiteralAt, matchingKeys } from "./pxl-keys.ts";

const ID = "01a0c450-6c00-7002-8000-000000003dde";

describe("[I18N-007] translation keys in PXL, by identifier", () => {
  test("every t(<id>) of an expression is found with where its identifier sits", () => {
    const text = `t("${ID}") + " · " + t('${ID}', {"n": "2"}) + t("home.title")`;
    const calls = keyCalls(text);
    expect(calls.map((c) => c.id)).toEqual([ID, ID]);
    expect(text.slice(calls[0]?.from, calls[0]?.to)).toBe(ID);
    expect(text.slice(calls[1]?.from, calls[1]?.to)).toBe(ID);
  });

  test('inside t("…") the editor offers keys; elsewhere it does not', () => {
    expect(keyLiteralAt('t("ho', 5)).toEqual({ from: 3, to: 5 });
    expect(keyLiteralAt('t("ho")', 5)).toEqual({ from: 3, to: 5 });
    expect(keyLiteralAt('upper("ho', 9)).toBeUndefined();
    expect(keyLiteralAt("t(name", 6)).toBeUndefined();
    const keys = [
      { id: ID, key: "home.title" },
      { id: "01a0c450-6c00-7002-8000-000000003ddf", key: "account.name" },
    ];
    expect(matchingKeys(keys, "home").map((k) => k.key)).toEqual(["home.title"]);
    expect(matchingKeys(keys, "").map((k) => k.key)).toEqual(["account.name", "home.title"]);
  });

  test("a key read by an expression counts as used, so it is not deleted while in use (STU-072)", () => {
    const page = {
      root: { id: "n1", type: "Text", props: { data: { $expr: `t("${ID}")` } } },
    };
    expect(referencedKeys(page)).toEqual([{ id: ID, nodeId: "n1" }]);
  });
});
