// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { ReleaseRequest, VisualDiffJob } from "@plux/api";
import type { ReleaseApi } from "./use-publish.ts";

/** What every step panel receives; `editable` is false for other people's requests, past steps and ended requests. */
export interface PanelProps {
  readonly request: ReleaseRequest;
  readonly diff: VisualDiffJob | undefined;
  readonly api: ReleaseApi;
  readonly editable: boolean;
  readonly busy: boolean;
  /** Plugin key to plugin id, for the calls that name a plugin. */
  readonly pluginIds: ReadonlyMap<string, string>;
  readonly run: (fn: () => Promise<unknown>) => void;
}

/**
 * The visual diff is parked (maintainer decision 2026-11-26, ADR-0113, build log row 300): the diffs step says it is
 * coming soon, offers no render and moves on without one, as the server's `diffsParked` lets it. The step still shows
 * the JSON diff of the changed documents.
 */
export const visualDiffParked = true;
