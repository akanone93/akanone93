// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { Problem } from "@plux/validator";

/** A one-click way past a finding; the Problems tab offers the ones that apply. */
export type ProblemFix =
  /** Upload a file under the name the asset list expects: the entry keeps its identifier, so its uses work again. */
  | { readonly kind: "uploadAsset"; readonly file: string }
  /** List a plugin directory in app.json's `plugins`. */
  | { readonly kind: "listPlugin"; readonly plugin: string }
  /** Take a key that has no plugin directory out of app.json's `plugins`. */
  | { readonly kind: "unlistPlugin"; readonly plugin: string }
  /** Open the file in the JSON editor at the place the finding names. */
  | { readonly kind: "openJson"; readonly file: string };

/** What a finding means in plain words, and what can be done about it. */
export interface ProblemHelp {
  readonly why: "assetMissing" | "pluginNotListed" | "pluginNoDirectory" | "duplicateRoute" | "unresolved" | undefined;
  readonly fixes: readonly ProblemFix[];
}

const ASSET_MISSING = /^asset file assets\/(\S+) is missing$/;
const NOT_LISTED = /^plugin directory "([^"]+)" is not listed in app\.json plugins$/;
const NO_DIRECTORY = /^plugin "([^"]+)" has no directory plugins\//;

/**
 * The help for one finding (P1): known findings get their meaning and a fix; every finding in a file the JSON editor
 * can open gets "Open in JSON editor". `editable` says which files the editor can open here.
 */
export function helpFor(p: Problem, editable: (file: string) => boolean): ProblemHelp {
  const open: ProblemFix[] = p.file !== "" && editable(p.file) ? [{ kind: "openJson", file: p.file }] : [];
  if (p.code === "PLX-1020") {
    const missing = ASSET_MISSING.exec(p.message);
    if (missing?.[1] !== undefined) {
      return { why: "assetMissing", fixes: [{ kind: "uploadAsset", file: missing[1] }, ...open] };
    }
    const unlisted = NOT_LISTED.exec(p.message);
    if (unlisted?.[1] !== undefined) {
      return { why: "pluginNotListed", fixes: [{ kind: "listPlugin", plugin: unlisted[1] }, ...open] };
    }
    const orphan = NO_DIRECTORY.exec(p.message);
    if (orphan?.[1] !== undefined) {
      return { why: "pluginNoDirectory", fixes: [{ kind: "unlistPlugin", plugin: orphan[1] }, ...open] };
    }
  }
  if (p.code === "PLX-1103") {
    return { why: "duplicateRoute", fixes: open };
  }
  if (p.code === "PLX-1102") {
    return { why: "unresolved", fixes: open };
  }
  return { why: undefined, fixes: open };
}

/** app.json's plugins list with `plugin` added at the end, or the list unchanged when it is there. */
export function withPlugin(list: unknown, plugin: string): string[] {
  const keys = Array.isArray(list) ? list.filter((k): k is string => typeof k === "string") : [];
  return keys.includes(plugin) ? keys : [...keys, plugin];
}

/** app.json's plugins list without `plugin`. */
export function withoutPlugin(list: unknown, plugin: string): string[] {
  const keys = Array.isArray(list) ? list.filter((k): k is string => typeof k === "string") : [];
  return keys.filter((k) => k !== plugin);
}
