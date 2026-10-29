// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Dependency management for the Rust editor (FN-033): the `[dependencies]` table of the function's `Cargo.toml`.
 * Any crate that compiles for `wasm32-wasip1` may be declared (FN-003); the builder works from vendored sources and a
 * lock file only (FN-016), so a crate added here takes effect when the source is uploaded again with its `vendor/`.
 */

export interface Dependency {
  name: string;
  /** The Cargo version requirement, such as `1.0` or `^0.8.2`. */
  version: string;
}

const CRATE_NAME = /^[A-Za-z][A-Za-z0-9_-]{0,63}$/;
const VERSION_REQ = /^(?:[~^=<>]=?\s*)?\d+(?:\.\d+){0,2}(?:-[0-9A-Za-z.-]+)?(?:\.\*)?$/;
const TABLE = /^\[dependencies\]\s*$/m;
const ANY_TABLE = /^\[/m;

/** The crate-name and version-requirement rules the editor applies before it edits the manifest. */
export function checkDependency(dep: Dependency): string | undefined {
  if (!CRATE_NAME.test(dep.name)) {
    return "crate names start with a letter and use letters, digits, - and _";
  }
  if (!VERSION_REQ.test(dep.version)) {
    return "use a version requirement such as 1.0 or ^0.8.2";
  }
  return undefined;
}

/** The span of the body of `[dependencies]`, or undefined when the manifest has none. */
function body(manifest: string): { start: number; end: number } | undefined {
  const header = TABLE.exec(manifest);
  if (header === null) {
    return undefined;
  }
  const start = header.index + header[0].length;
  const rest = manifest.slice(start);
  const next = ANY_TABLE.exec(rest);
  return { start, end: next === null ? manifest.length : start + next.index };
}

/** The simple `name = "version"` and `name = { version = "..", ... }` entries; other forms are kept but not listed. */
export function listDependencies(manifest: string): Dependency[] {
  const span = body(manifest);
  if (span === undefined) {
    return [];
  }
  const out: Dependency[] = [];
  for (const line of manifest.slice(span.start, span.end).split("\n")) {
    const simple = /^\s*([A-Za-z][A-Za-z0-9_-]*)\s*=\s*"([^"]*)"\s*(?:#.*)?$/.exec(line);
    const table = /^\s*([A-Za-z][A-Za-z0-9_-]*)\s*=\s*\{[^}]*\bversion\s*=\s*"([^"]*)"/.exec(line);
    const hit = simple ?? table;
    if (hit !== null) {
      out.push({ name: hit[1] as string, version: hit[2] as string });
    }
  }
  return out;
}

/** Adds the dependency, or sets its version when the crate is already declared as a simple entry. */
export function setDependency(manifest: string, dep: Dependency): string {
  const problem = checkDependency(dep);
  if (problem !== undefined) {
    throw new Error(problem);
  }
  const entry = `${dep.name} = "${dep.version}"`;
  const span = body(manifest);
  if (span === undefined) {
    return `${manifest.replace(/\n*$/, "\n")}\n[dependencies]\n${entry}\n`;
  }
  const lines = manifest.slice(span.start, span.end).split("\n");
  const at = lines.findIndex((l) => new RegExp(`^\\s*${dep.name}\\s*=`).test(l));
  if (at >= 0) {
    lines[at] = entry;
  } else {
    // Insert after the last entry, before the blank lines that separate the next table.
    let last = lines.length - 1;
    while (last > 0 && (lines[last] ?? "").trim() === "") {
      last -= 1;
    }
    lines.splice(last + 1, 0, entry);
  }
  return manifest.slice(0, span.start) + lines.join("\n") + manifest.slice(span.end);
}

/** Removes the crate's entry; the manifest is returned unchanged when it is not declared. */
export function removeDependency(manifest: string, name: string): string {
  const span = body(manifest);
  if (span === undefined) {
    return manifest;
  }
  const kept = manifest
    .slice(span.start, span.end)
    .split("\n")
    .filter((l) => {
      const at = l.indexOf("=");
      return at < 0 || l.slice(0, at).trim() !== name;
    });
  return manifest.slice(0, span.start) + kept.join("\n") + manifest.slice(span.end);
}
