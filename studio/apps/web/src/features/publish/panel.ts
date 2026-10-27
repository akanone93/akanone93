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
  readonly run: (fn: () => Promise<unknown>) => void;
}
