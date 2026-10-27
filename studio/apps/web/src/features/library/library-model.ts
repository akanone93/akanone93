// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { BinaryReader } from "@bufbuild/protobuf/wire";
import { ConnectError } from "@connectrpc/connect";
import type { LibraryItem } from "@plux/api";
import type { LibrarySearch } from "./library.schema.ts";

export const VISIBILITIES = ["private", "organization", "public"] as const;
export type Visibility = (typeof VISIBILITIES)[number];

export interface Finding {
  readonly pointer: string;
  readonly category: string;
}

/**
 * The filters the server does not apply (STU-086): the owner matches the author's name or id, the category the
 * template kind (page or flow) and a tag is a `#tag` word in the description.
 */
export function filterItems(items: readonly LibraryItem[], search: LibrarySearch): LibraryItem[] {
  const owner = search.owner?.trim().toLowerCase();
  const tag = search.tag?.trim().replace(/^#/, "").toLowerCase();
  return items.filter((i) => {
    if (owner && ![i.createdBy, i.createdByName].some((v) => v.toLowerCase().includes(owner))) {
      return false;
    }
    if (search.category && i.templateKind !== search.category) {
      return false;
    }
    return !tag || i.description.toLowerCase().split(/\s+/).includes(`#${tag}`);
  });
}

export function decodeMetadata(info: Uint8Array): Map<string, string> {
  const out = new Map<string, string>();
  const reader = new BinaryReader(info);
  while (reader.pos < reader.len) {
    const [field, wire] = reader.tag();
    if (field !== 3) {
      reader.skip(wire);
      continue;
    }
    const entry = new BinaryReader(reader.bytes());
    let key = "";
    let value = "";
    while (entry.pos < entry.len) {
      const [f, w] = entry.tag();
      if (f === 1) {
        key = entry.string();
      } else if (f === 2) {
        value = entry.string();
      } else {
        entry.skip(w);
      }
    }
    out.set(key, value);
  }
  return out;
}

function parseList(json: string): Finding[] {
  try {
    const list: unknown = JSON.parse(json);
    if (!Array.isArray(list)) {
      return [];
    }
    return list.flatMap((f: { pointer?: unknown; category?: unknown }) =>
      typeof f.pointer === "string" && typeof f.category === "string"
        ? [{ pointer: f.pointer === "" ? "/" : f.pointer, category: f.category }]
        : [],
    );
  } catch {
    return [];
  }
}

/** The ErrorInfo detail's `findings` entry, or the "pointer (category)" list in the message. Never a value. */
export function scanFindings(err: unknown): Finding[] {
  if (!(err instanceof ConnectError)) {
    return [];
  }
  for (const d of err.details) {
    if ("value" in d && d.value instanceof Uint8Array) {
      const json = decodeMetadata(d.value).get("findings");
      if (json !== undefined) {
        return parseList(json);
      }
    }
  }
  return [...err.rawMessage.matchAll(/(\/\S*) \((\w+)\)/g)].map((m) => ({
    pointer: m[1] ?? "/",
    category: m[2] ?? "",
  }));
}

export function isScanFailure(err: unknown): boolean {
  return err instanceof ConnectError && (err.rawMessage.includes("cannot be public") || scanFindings(err).length > 0);
}

/** Hands a file to the browser's download; the bytes never leave the page. */
export function saveFile(name: string, bytes: Uint8Array): void {
  const url = URL.createObjectURL(new Blob([bytes as BlobPart]));
  const a = document.createElement("a");
  a.href = url;
  a.download = name;
  a.click();
  URL.revokeObjectURL(url);
}

export async function readFile(file: File): Promise<Uint8Array> {
  return new Uint8Array(await file.arrayBuffer());
}

export interface InterfaceDiff {
  readonly addedProps: string[];
  readonly removedProps: string[];
  readonly addedSlots: string[];
  readonly removedSlots: string[];
}

function namesOf(content: Uint8Array | undefined, field: "props" | "slots"): Set<string> {
  if (content === undefined) {
    return new Set();
  }
  try {
    const doc: unknown = JSON.parse(new TextDecoder().decode(content));
    const part = typeof doc === "object" && doc !== null ? (doc as Record<string, unknown>)[field] : undefined;
    if (Array.isArray(part)) {
      return new Set(part.flatMap((p: { name?: unknown }) => (typeof p.name === "string" ? [p.name] : [])));
    }
    return new Set(typeof part === "object" && part !== null ? Object.keys(part) : []);
  } catch {
    return new Set();
  }
}

/** D32: what changes in a component's props and slots between the adopted version and the one on offer. */
export function interfaceDiff(from: Uint8Array | undefined, to: Uint8Array | undefined): InterfaceDiff {
  const delta = (field: "props" | "slots") => {
    const a = namesOf(from, field);
    const b = namesOf(to, field);
    return {
      added: [...b].filter((n) => !a.has(n)).sort(),
      removed: [...a].filter((n) => !b.has(n)).sort(),
    };
  };
  const props = delta("props");
  const slots = delta("slots");
  return { addedProps: props.added, removedProps: props.removed, addedSlots: slots.added, removedSlots: slots.removed };
}

/** The screen a use site is on: the page document's file name without its extension. */
export function screenOfFile(file: string): string {
  const name = file.slice(file.lastIndexOf("/") + 1);
  return name.replace(/\.page\.json$|\.json$/, "");
}

/** The prop and slot names a component document exposes, with the declared prop types. */
export function interfaceOf(content: Uint8Array | undefined): {
  readonly props: ReadonlyMap<string, string>;
  readonly slots: readonly string[];
} {
  const types = new Map<string, string>();
  try {
    const doc = JSON.parse(new TextDecoder().decode(content)) as { props?: unknown };
    const part = doc.props;
    for (const [k, v] of Object.entries(Array.isArray(part) ? {} : (part ?? {}))) {
      types.set(k, typeof v === "object" && v !== null ? String((v as { type?: unknown }).type ?? "string") : "string");
    }
  } catch {
    // an unreadable document exposes nothing
  }
  return { props: types, slots: [...namesOf(content, "slots")] };
}
