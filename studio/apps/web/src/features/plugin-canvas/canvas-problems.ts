// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { ScreenHeight } from "@plux/canvas";
import type { Problem } from "@plux/validator";
import { pointerOf } from "../panels/properties-model.ts";
import type { Validation } from "../problems/use-validation.ts";

/**
 * What the canvas's own layout found, as problems (#80): each widget it could not build as Flutter does — drawn as a
 * placeholder or an approximation instead — at its node in its page, so a blank or odd screen is explained. The code
 * is the runtime's for a node that fails to build (`PLX-4001`); the severity is a warning, as the canvas's scene is an
 * approximation of the device's and the device reports its own failures.
 */

/** A widget the canvas could not build, on its screen. */
interface Unbuilt {
  readonly screen: string;
  readonly id: string;
  readonly type: string;
  readonly reason: string;
}

/** The widgets the layout could not build, as one sorted key: an unchanged layout keeps the same key. */
export function unbuiltKeyOf(laidOut: readonly ScreenHeight[]): string {
  const found: Unbuilt[] = laidOut.flatMap((h) =>
    (h.unbuilt ?? []).map((u) => ({ screen: h.id, id: u.id, type: u.type, reason: u.reason })),
  );
  return found.length === 0
    ? ""
    : JSON.stringify(found.sort((a, b) => `${a.screen}/${a.id}`.localeCompare(`${b.screen}/${b.id}`)));
}

/** The page documents the canvas draws, as `usePageChecks` takes them. */
interface PageDoc {
  readonly path: string;
  readonly kind: string;
  readonly parsed: unknown;
}

/** The problems of `key` (`unbuiltKeyOf`), each at its node's pointer in the page whose id is its screen's. */
export function unbuiltProblems(
  key: string,
  documents: readonly PageDoc[],
  message: (type: string, reason: string) => string,
): Problem[] {
  if (key === "") {
    return [];
  }
  const pages = new Map<string, PageDoc>();
  for (const d of documents) {
    const id =
      d.kind === "page" && typeof d.parsed === "object" && d.parsed !== null
        ? (d.parsed as { id?: unknown }).id
        : undefined;
    if (typeof id === "string") {
      pages.set(id, d);
    }
  }
  return (JSON.parse(key) as Unbuilt[]).flatMap((u) => {
    const page = pages.get(u.screen);
    if (page === undefined) {
      return [];
    }
    return [
      {
        code: "PLX-4001",
        severity: "warning" as const,
        file: page.path,
        // A node inside a component instance has no place of its own in the page: the page's root stands for it.
        path: pointerOf(page.parsed, u.id) ?? "/root",
        message: message(u.type, u.reason),
      },
    ];
  });
}

/** `validation` with `extra` among its problems, once it has any. */
export function withProblems(validation: Validation, extra: readonly Problem[]): Validation {
  return validation.state !== "done" || extra.length === 0
    ? validation
    : { ...validation, problems: [...validation.problems, ...extra] };
}
