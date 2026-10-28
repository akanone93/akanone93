// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { DevEvent } from "@plux/api";
import type { Node } from "../plugin-canvas/plugin-model.ts";
import { parseDocument } from "../plugin-canvas/plugin-model.ts";
import { childNodes } from "../plugin-canvas/select/tree.ts";
import type { Draft, DraftStep } from "./tests.model.ts";
import { emptyDraft } from "./tests.model.ts";

const TEST_ID = /^[a-z][a-zA-Z0-9_-]*$/;
const ROUTE = /^[a-z][a-zA-Z0-9_-]*$/;
const PAGE_PREFIX = "/pages/";

/** A page of the project, as the device reports node paths against it. */
export interface SessionPage {
  readonly id: string;
  readonly route: string;
  readonly root: (Node & { readonly testId?: string }) | undefined;
}

interface PageJson {
  readonly id?: unknown;
  readonly key?: unknown;
  readonly route?: unknown;
  readonly root?: SessionPage["root"];
}

/** The pages among a project's documents (kind `page`). */
export function pagesOf(documents: readonly { readonly kind: string; readonly content: Uint8Array }[]): SessionPage[] {
  const pages: SessionPage[] = [];
  for (const d of documents) {
    const json = d.kind === "page" ? (parseDocument(d.content) as PageJson | undefined) : undefined;
    if (json !== undefined && typeof json.id === "string") {
      const route = typeof json.route === "string" ? json.route : String(json.key ?? json.id);
      pages.push({ id: json.id, route, root: json.root });
    }
  }
  return pages;
}

/** The page and node a device-reported path (`/pages/<page>/<child ordinals>`, DEV-030) names. */
function resolve(pages: readonly SessionPage[], path: string) {
  if (!path.startsWith(PAGE_PREFIX)) {
    return undefined;
  }
  const [pageId, ...ordinals] = path.slice(PAGE_PREFIX.length).split("/");
  const page = pages.find((p) => p.id === pageId);
  let node = page?.root;
  for (const ordinal of ordinals) {
    node = node === undefined ? undefined : (childNodes(node)[Number(ordinal)] as typeof node);
  }
  return page === undefined || node === undefined ? undefined : { page, node };
}

/** The triggers of a run started by a tap on a widget; the others (lifecycle, timers, commands) are not user input. */
const tapTriggers: ReadonlySet<string> = new Set(["onTap", "tap"]);

/**
 * The steps a device-loop session recorded (TST-001): one `tap` per action run started by a tap on a node that
 * has a `testId`, in the order the runs began. A later event of the same run replaces an earlier one, so a run
 * that was reported as running and then finished counts once.
 */
export function recordedSteps(
  events: readonly DevEvent[],
  pages: readonly SessionPage[],
): { readonly steps: readonly DraftStep[]; readonly page: string } {
  const runs = new Map<string, string>();
  for (const e of events) {
    if (e.payload.case === "actionTrace" && tapTriggers.has(e.payload.value.trigger)) {
      runs.set(e.payload.value.runId, e.payload.value.nodePath);
    }
  }
  const steps: DraftStep[] = [];
  let page = "";
  for (const path of runs.values()) {
    const hit = resolve(pages, path);
    const testId = hit?.node.testId;
    if (hit !== undefined && testId !== undefined && TEST_ID.test(testId)) {
      steps.push({ kind: "tap", testId, text: "" });
      const route = hit.page.route.replace(/^\//, "");
      page = page === "" && ROUTE.test(route) ? route : page;
    }
  }
  return { steps, page };
}

/** A draft that starts from the recorded steps. */
export function draftFromSession(recorded: { readonly steps: readonly DraftStep[]; readonly page: string }): Draft {
  return { ...emptyDraft, page: recorded.page, steps: recorded.steps };
}
