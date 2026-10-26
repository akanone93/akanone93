// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { readFileSync } from "node:fs";
import type { Messages } from "@lingui/core";
import { createStudioI18n } from "@plux/i18n";
import { createMemoryHistory } from "@tanstack/react-router";
import { render } from "@testing-library/react";
import { App } from "./app.tsx";
import type { EditSession } from "./features/editor/edit-session.ts";
import type { SurfaceFactory } from "./features/plugin-canvas/surface.ts";
import type { ValidatorFactory } from "./features/problems/validator-client.ts";
import { I18nProvider } from "./i18n-provider.tsx";
import { createSession } from "./lib/session.ts";
import { configureZod } from "./lib/zod.ts";
import { createAppRouter } from "./router.tsx";
import { createUpstream, type UpstreamData } from "./test-upstream.ts";

export { createUpstream, orgA, orgB } from "./test-upstream.ts";

export interface Upstream extends UpstreamData {
  surface?: SurfaceFactory;
  validator?: ValidatorFactory;
  /** Receives the plugin canvas's edit pipeline. */
  editor?: (edit: EditSession) => void;
}

/** Renders the whole app at `path` against the in-memory upstream. */
export function renderApp(path: string, state: Upstream = {}) {
  configureZod();
  const session = createSession("/api", createUpstream(state));
  const studio = createStudioI18n({ load: async () => loadSourceCatalogue(), pseudo: false });
  const history = createMemoryHistory({ initialEntries: [path] });
  const router = createAppRouter(
    {
      session,
      ...(state.surface !== undefined && { createSurface: state.surface }),
      ...(state.editor !== undefined && { observeEditor: state.editor }),
      createValidator:
        state.validator ?? (() => ({ load: async () => [], validatePage: async () => [], dispose: () => undefined })),
    },
    history,
  );
  const utils = render(
    <I18nProvider studio={studio}>
      <App session={session} router={router} />
    </I18nProvider>,
  );
  return { ...utils, router, session };
}

/** The English source catalogue, parsed from the extracted PO file, in Lingui's compiled form. */
export function loadSourceCatalogue(): Messages {
  const text = readFileSync(new URL("./locales/en/messages.po", import.meta.url), "utf-8");
  const out: Messages = {};
  for (const m of text.matchAll(/msgid "([^"]+)"\nmsgstr "([^"]*)"/g)) {
    const id = m[1];
    const message = m[2];
    if (id !== undefined && message !== undefined) {
      const parts = message.split(/\{(\w+)\}/);
      out[id] =
        parts.length === 1
          ? message
          : (parts.map((part, i) => (i % 2 === 1 ? [part] : part)).filter((p) => p !== "") as Messages[string]);
    }
  }
  return out;
}
