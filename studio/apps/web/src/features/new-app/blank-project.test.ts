// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import {
  type BlankOptions,
  blankProject,
  customTheme,
  hexOf,
  isHexColour,
  monogramOf,
  themePresets,
} from "./blank-project.ts";

const violet = themePresets[0] ?? { id: "violet", name: "Violet", primary: "", dark: "" };
const options: BlankOptions = {
  key: "shop",
  name: "Shop Front",
  description: "Sells things",
  defaultLocale: "de",
  locales: ["en", "de", "ar"],
  theme: violet,
  securityProfile: "strict",
  environments: ["staging", "production"],
};

function counter() {
  let n = 0;
  return () => {
    n += 1;
    return `0192f000-0000-7000-8000-${String(n).padStart(12, "0")}`;
  };
}

const read = (path: string, files = blankProject(options, counter())) => {
  const found = files.find((f) => f.path === path);
  return JSON.parse(new TextDecoder().decode(found?.content)) as Record<string, unknown>;
};

describe("the project of a blank app [STU-024]", () => {
  test("it holds the app, the theme, the default plugin with its entry screen and a translation per language", () => {
    const paths = blankProject(options, counter()).map((f) => f.path);
    expect(paths).toEqual([
      "app.json",
      "theme.json",
      "plugins/main/plugin.json",
      "plugins/main/pages/home.page.json",
      "translations/keys.json",
      "translations/de.json",
      "translations/en.json",
      "translations/ar.json",
    ]);
  });

  test("the app document carries the choices of the wizard", () => {
    const app = read("app.json");
    expect(app).toMatchObject({
      kind: "app",
      key: "shop",
      name: "Shop Front",
      description: "Sells things",
      defaultLocale: "de",
      supportedLocales: ["de", "en", "ar"],
      securityProfile: "strict",
      entryRoute: "home",
      plugins: ["main"],
      environments: [
        { key: "staging", name: "Staging" },
        { key: "production", name: "Production" },
      ],
    });
  });

  test("production is kept even when it was left out, and an empty description is omitted", () => {
    const app = read("app.json", blankProject({ ...options, description: "", environments: [] }, counter()));
    expect(app["environments"]).toEqual([{ key: "production", name: "Production" }]);
    expect("description" in app).toBe(false);
  });

  test("the theme references the app, with the primary colour for light and dark mode", () => {
    const files = blankProject(options, counter());
    const app = read("app.json", files);
    const theme = read("theme.json", files);
    expect(theme["id"]).toBe(app["theme"]);
    expect(theme["tokens"]).toEqual({
      color: { $type: "color", primary: { $value: violet.primary, $extensions: { "dev.plux.dark": violet.dark } } },
    });
  });

  test("the plugin's entry page exists and its title is a translation key with the app's name in every language", () => {
    const files = blankProject(options, counter());
    const plugin = read("plugins/main/plugin.json", files);
    const page = read("plugins/main/pages/home.page.json", files);
    const keys = read("translations/keys.json", files);
    expect(plugin["entryPage"]).toBe(page["id"]);
    expect(plugin["pages"]).toEqual([page["id"]]);
    expect(page).toMatchObject({ key: "home", route: "home", pageKind: "screen", root: { type: "Scaffold" } });
    const keyId = (page["title"] as { $t: string }).$t;
    expect((keys["keys"] as { id: string }[]).map((k) => k.id)).toEqual([keyId]);
    for (const locale of ["de", "en", "ar"]) {
      expect((read(`translations/${locale}.json`, files)["messages"] as Record<string, string>)[keyId]).toBe(
        "Shop Front",
      );
    }
  });

  test("the same options and identifiers give the same files", () => {
    expect(blankProject(options, counter())).toEqual(blankProject(options, counter()));
  });

  test("identifiers are UUIDv7 by default", () => {
    const app = read("app.json", blankProject(options));
    expect(app["id"]).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/);
  });

  test("a monogram is the initials of the first two words, capital letters", () => {
    expect(monogramOf("Shop Front")).toBe("SF");
    expect(monogramOf("plux bank app")).toBe("PB");
    expect(monogramOf("  ")).toBe("A");
    expect(monogramOf("Ärzte")).toBe("Ä");
  });

  test("colours are six hexadecimal digits after a hash sign, and a custom theme uses its colour in both modes", () => {
    expect(isHexColour(hexOf(0x5b3df5))).toBe(true);
    expect(isHexColour("#12345")).toBe(false);
    expect(isHexColour("5B3DF5")).toBe(false);
    expect(hexOf(0xff)).toBe("#0000FF");
    const custom = customTheme(hexOf(0xab12cd).toLowerCase());
    expect(custom.primary).toBe(custom.dark);
    expect(custom.primary).toBe(hexOf(0xab12cd));
  });
});
