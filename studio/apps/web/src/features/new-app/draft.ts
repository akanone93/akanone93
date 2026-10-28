// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { limit, limitDefault } from "@plux/schema";
import { isKey, isName } from "../../lib/keys.ts";
import {
  type BlankOptions,
  customTheme,
  type EnvironmentChoice,
  environmentChoices,
  isHexColour,
  type SecurityProfileChoice,
  type ThemePresetId,
  themePresets,
} from "./blank-project.ts";
import { zipSummary } from "./plux-archive.ts";

export type Source = "blank" | "template" | "import";
export type StepId = "source" | "details" | "locales" | "theme" | "security" | "environments" | "done";

/** Why a chosen archive cannot be imported, found before it is sent. */
export type ArchiveIssue =
  | { readonly kind: "extension" }
  | { readonly kind: "unreadable" }
  | { readonly kind: "bytes"; readonly size: number; readonly max: number }
  | { readonly kind: "entries"; readonly count: number; readonly max: number }
  | { readonly kind: "expanded"; readonly size: number; readonly max: number };

export interface ChosenArchive {
  readonly name: string;
  readonly bytes: Uint8Array;
  readonly issue?: ArchiveIssue;
}

export interface Draft {
  readonly source: Source;
  readonly sampleId: string;
  readonly archive?: ChosenArchive | undefined;
  readonly name: string;
  readonly key: string;
  readonly keyEdited: boolean;
  readonly description: string;
  readonly defaultLocale: string;
  readonly locales: readonly string[];
  readonly themeId: ThemePresetId | "custom";
  readonly customPrimary: string;
  readonly profile: SecurityProfileChoice;
  readonly environments: readonly EnvironmentChoice[];
}

export const initialDraft: Draft = {
  source: "blank",
  sampleId: "",
  name: "",
  key: "",
  keyEdited: false,
  description: "",
  defaultLocale: "en",
  locales: ["en"],
  themeId: "violet",
  customPrimary: "",
  profile: "standard",
  environments: environmentChoices,
};

/** The steps of each way to start; a template and an archive bring their own languages, theme and security. */
export function stepsOf(source: Source): readonly StepId[] {
  return source === "blank"
    ? ["source", "details", "locales", "theme", "security", "environments", "done"]
    : ["source", "details", "done"];
}

/** The theme a draft chooses. */
export function themeOf(draft: Draft): BlankOptions["theme"] {
  const preset = themePresets.find((p) => p.id === draft.themeId);
  return preset ?? customTheme(draft.customPrimary);
}

/** The options of the blank project a draft describes. */
export function blankOptions(draft: Draft): BlankOptions {
  return {
    key: draft.key,
    name: draft.name.trim(),
    description: draft.description.trim(),
    defaultLocale: draft.defaultLocale,
    locales: draft.locales,
    theme: themeOf(draft),
    securityProfile: draft.profile,
    environments: draft.environments,
  };
}

/** The most languages one app may support: the registry default of `app.locales`. */
export const maxLocales = limitDefault("app.locales");

/**
 * Looks at a chosen file before it is read by the server. The registry maxima are the largest values any
 * organisation can be given, so a file past them can never be imported; below them the server decides with the
 * organisation's own limit and says why it refuses.
 */
export function inspectArchive(name: string, bytes: Uint8Array): ArchiveIssue | undefined {
  if (!name.toLowerCase().endsWith(".plux")) {
    return { kind: "extension" };
  }
  const maxBytes = limit("archive.bytes").max;
  if (bytes.length > maxBytes) {
    return { kind: "bytes", size: bytes.length, max: maxBytes };
  }
  const summary = zipSummary(bytes);
  if (summary === undefined) {
    return { kind: "unreadable" };
  }
  const maxEntries = limit("archive.entries").max;
  if (summary.entries > maxEntries) {
    return { kind: "entries", count: summary.entries, max: maxEntries };
  }
  const maxExpanded = limit("archive.expandedBytes").max;
  if (summary.expandedBytes > maxExpanded) {
    return { kind: "expanded", size: summary.expandedBytes, max: maxExpanded };
  }
  return undefined;
}

export type StepProblem = "template" | "archive" | "name" | "key" | "locales" | "tooManyLocales" | "colour";

function sourceProblem(draft: Draft): StepProblem | undefined {
  if (draft.source === "template") {
    return draft.sampleId === "" ? "template" : undefined;
  }
  if (draft.source === "import") {
    return draft.archive === undefined || draft.archive.issue !== undefined ? "archive" : undefined;
  }
  return undefined;
}

function detailsProblem(draft: Draft): StepProblem | undefined {
  // An import keeps the archive's own name and key when these are left empty.
  const optional = draft.source === "import";
  if (!(optional && draft.name.trim() === "") && !isName(draft.name)) {
    return "name";
  }
  if (optional && draft.key === "") {
    return undefined;
  }
  return isKey(draft.key) ? undefined : "key";
}

function localesProblem(draft: Draft): StepProblem | undefined {
  if (draft.locales.length < 1 || !draft.locales.includes(draft.defaultLocale)) {
    return "locales";
  }
  return draft.locales.length > maxLocales ? "tooManyLocales" : undefined;
}

/** What stops a step from being finished; `undefined` when it can be. */
export function problemOf(draft: Draft, step: StepId): StepProblem | undefined {
  switch (step) {
    case "source":
      return sourceProblem(draft);
    case "details":
      return detailsProblem(draft);
    case "locales":
      return localesProblem(draft);
    case "theme":
      return draft.themeId === "custom" && !isHexColour(draft.customPrimary) ? "colour" : undefined;
    default:
      return undefined;
  }
}
