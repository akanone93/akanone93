// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The project of a blank app (STU-024): the app and theme documents the wizard's choices fill in, one default plugin
 * and its entry screen, in the SCH-006 layout. The wizard packs it as a `.plux` archive and imports it, so the
 * server validates and creates the whole app in one step (ADR-0102).
 */

import { uuidv7 } from "../plugin-canvas/screens.ts";
import type { ArchiveFile } from "./plux-archive.ts";

export type SecurityProfileChoice = "standard" | "strict" | "maximum";
export type EnvironmentChoice = "development" | "staging" | "production";

/** The environments an app may declare, in the order the wizard lists them; production is always kept. */
export const environmentChoices: readonly EnvironmentChoice[] = ["development", "staging", "production"];

export const securityProfiles: readonly SecurityProfileChoice[] = ["standard", "strict", "maximum"];

export interface ThemeChoice {
  /** The theme's display name. */
  readonly name: string;
  /** The primary colour, `#RRGGBB`. */
  readonly primary: string;
  /** The primary colour in dark mode, `#RRGGBB`. */
  readonly dark: string;
}

/** `#RRGGBB` of a 24-bit colour number. */
export function hexOf(rgb: number): string {
  return `#${rgb.toString(16).toUpperCase().padStart(6, "0")}`;
}

export type ThemePresetId = "violet" | "blue" | "green" | "slate";

/** The ready-made themes of the wizard (primary colour, then the same in dark mode); a custom primary colour is the other choice. */
export const themePresets: readonly (ThemeChoice & { readonly id: ThemePresetId })[] = [
  { id: "violet", name: "Violet", primary: hexOf(0x5b3df5), dark: hexOf(0x8b78ff) },
  { id: "blue", name: "Blue", primary: hexOf(0x1f6feb), dark: hexOf(0x6ca6ff) },
  { id: "green", name: "Green", primary: hexOf(0x1a7f4b), dark: hexOf(0x4cc38a) },
  { id: "slate", name: "Slate", primary: hexOf(0x3d4a5c), dark: hexOf(0x9fb0c7) },
];

/** The dark-mode primary colour of a custom primary colour: the colour itself, so a custom theme never claims a contrast it has not measured. */
export function customTheme(primary: string): ThemeChoice {
  return { name: "Custom", primary: primary.toUpperCase(), dark: primary.toUpperCase() };
}

export interface BlankOptions {
  readonly key: string;
  readonly name: string;
  readonly description: string;
  readonly defaultLocale: string;
  /** Every supported locale, the default included. */
  readonly locales: readonly string[];
  readonly theme: ThemeChoice;
  readonly securityProfile: SecurityProfileChoice;
  readonly environments: readonly EnvironmentChoice[];
}

const hexColour = /^#[0-9A-Fa-f]{6}$/;

/** True for a `#RRGGBB` colour. */
export function isHexColour(value: string): boolean {
  return hexColour.test(value);
}

/** The locales the wizard offers: BCP 47 tags in the languages Studio and the runtime ship text for. */
export const localeChoices: readonly string[] = [
  "en",
  "de",
  "fr",
  "es",
  "it",
  "pt",
  "nl",
  "pl",
  "tr",
  "ru",
  "ja",
  "ko",
  "zh",
  "ar",
  "he",
  "hi",
];

const envNames: Readonly<Record<EnvironmentChoice, string>> = {
  development: "Development",
  staging: "Staging",
  production: "Production",
};

/** One to three capital letters for the monogram icon: the initials of the first words of the name. */
export function monogramOf(name: string): string {
  const letters = name
    .trim()
    .split(/\s+/)
    .map((word) => Array.from(word)[0] ?? "")
    .join("")
    .toUpperCase();
  const text = Array.from(letters).slice(0, 2).join("");
  return text === "" ? "A" : text;
}

function json(value: unknown): Uint8Array {
  return new TextEncoder().encode(`${JSON.stringify(value, null, 2)}\n`);
}

/** The documents of a blank app. Identifiers are new UUIDv7s from `newId`; the same options give the same layout. */
export function blankProject(options: BlankOptions, newId: () => string = uuidv7): ArchiveFile[] {
  const themeId = newId();
  const pluginId = newId();
  const pageId = newId();
  const rootId = newId();
  const titleKeyId = newId();
  const locales = [options.defaultLocale, ...options.locales.filter((l) => l !== options.defaultLocale)];
  const environments = environmentChoices
    .filter((e) => e === "production" || options.environments.includes(e))
    .map((e) => ({ key: e, name: envNames[e] }));
  const icon = { monogram: { text: monogramOf(options.name), background: options.theme.primary.toUpperCase() } };
  const files: ArchiveFile[] = [
    {
      path: "app.json",
      content: json({
        schemaVersion: "1.0.0",
        kind: "app",
        id: newId(),
        key: options.key,
        name: options.name,
        ...(options.description !== "" && { description: options.description }),
        icon,
        defaultLocale: options.defaultLocale,
        supportedLocales: locales,
        theme: themeId,
        entryRoute: "home",
        plugins: ["main"],
        environments,
        securityProfile: options.securityProfile,
        sync: { activation: "atSafePoint", startup: { mode: "useCacheThenSync" } },
        minRuntimeVersion: "0.1.0",
      }),
    },
    {
      path: "theme.json",
      content: json({
        schemaVersion: "1.0.0",
        kind: "theme",
        id: themeId,
        key: options.key,
        name: options.theme.name,
        tokens: {
          color: {
            $type: "color",
            primary: { $value: options.theme.primary, $extensions: { "dev.plux.dark": options.theme.dark } },
          },
        },
      }),
    },
    {
      path: "plugins/main/plugin.json",
      content: json({
        schemaVersion: "1.0.0",
        kind: "plugin",
        id: pluginId,
        key: "main",
        name: "Main",
        team: options.key,
        entryPage: pageId,
        pages: [pageId],
        icon,
      }),
    },
    {
      path: "plugins/main/pages/home.page.json",
      content: json({
        schemaVersion: "1.0.0",
        kind: "page",
        id: pageId,
        key: "home",
        route: "home",
        pageKind: "screen",
        title: { $t: titleKeyId },
        root: { id: rootId, type: "Scaffold" },
      }),
    },
  ];
  files.push({
    path: "translations/keys.json",
    content: json({
      schemaVersion: "1.0.0",
      kind: "translationKeys",
      id: newId(),
      keys: [{ id: titleKeyId, key: "home.title", description: "Title of the entry screen." }],
    }),
  });
  // The app's name is the first text in every language; translators change it per locale.
  for (const locale of locales) {
    files.push({
      path: `translations/${locale}.json`,
      content: json({
        schemaVersion: "1.0.0",
        kind: "translations",
        id: newId(),
        locale,
        messages: { [titleKeyId]: options.name },
      }),
    });
  }
  return files;
}
