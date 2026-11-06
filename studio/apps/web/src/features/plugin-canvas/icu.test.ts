// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { format, pluralArgs } from "./icu.ts";

const items = "{count, plural, =0 {No items} one {# item} other {# items}}";

describe("PC-32 ICU messages on the canvas [I18N-001]", () => {
  test("fills simple arguments and keeps the ones the canvas has no value for", () => {
    expect(format("Hello {name}", { name: "Ada" }, "en")).toBe("Hello Ada");
    expect(format("Hello {name}", {}, "en")).toBe("Hello {name}");
  });

  test("plural picks an exact match first, then the locale's category, and # shows the count in the locale", () => {
    expect(format(items, { count: 0 }, "en")).toBe("No items");
    expect(format(items, { count: 1 }, "en")).toBe("1 item");
    expect(format(items, { count: 1234 }, "de-DE")).toBe("1.234 items");
    const ar = "{n, plural, zero {لا عناصر} one {عنصر واحد} two {عنصران} few {# عناصر} many {# عنصرًا} other {# عنصر}}";
    // Latin digits are the default on the device (I18N-003), whatever the browser's own choice for the locale.
    expect(format(ar, { n: 3 }, "ar")).toBe("3 عناصر");
    expect(format(ar, { n: 11 }, "ar")).toBe("11 عنصرًا");
  });

  test("plural offset, selectordinal, select and nesting", () => {
    const guests =
      "{count, plural, offset:1 =0 {Nobody} =1 {{host}} one {{host} and # other} other {{host} and # others}}";
    expect(format(guests, { count: 3, host: "Ada" }, "en")).toBe("Ada and 2 others");
    expect(format(guests, { count: 1, host: "Ada" }, "en")).toBe("Ada");
    const place = "{n, selectordinal, one {#st} two {#nd} few {#rd} other {#th}}";
    expect([1, 2, 3, 4, 22].map((n) => format(place, { n }, "en"))).toEqual(["1st", "2nd", "3rd", "4th", "22nd"]);
    expect(format("{g, select, female {She} male {He} other {They}} paid", { g: "female" }, "en")).toBe("She paid");
    expect(format("{g, select, female {She} other {They}} paid", {}, "en")).toBe("They paid");
  });

  test("apostrophes quote braces and themselves", () => {
    expect(format("Use '{name}' as is, it''s fine", { name: "x" }, "en")).toBe("Use {name} as is, it's fine");
  });

  test("number, date and time arguments take the locale's formats and their styles", () => {
    expect(format("{n, number}", { n: 1234.5 }, "de-DE")).toBe("1.234,5");
    expect(format("{n, number, percent}", { n: 0.25 }, "en")).toBe("25%");
    expect(format("{n, number, integer}", { n: 2.6 }, "en")).toBe("3");
    expect(format("{d, date, long}", { d: "2026-03-01T10:30:00Z" }, "de-DE")).toBe("1. März 2026");
    expect(format("{d, time, short}", { d: "2026-03-01T10:30:00Z" }, "en")).toBe("10:30 AM");
    expect(format("{d, date}", { d: "not a date" }, "en")).toBe("{d}");
  });

  test("a plural without its count shows the other branch with the argument's name for #", () => {
    expect(format(items, {}, "en")).toBe("{count} items");
  });

  test("lists the plural and selectordinal arguments a sample can drive", () => {
    expect(pluralArgs(`${items} {p, selectordinal, other {#}} {g, select, other {x}}`)).toEqual(["count", "p"]);
  });
});
