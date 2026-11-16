// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { readFileSync } from "node:fs";
import type { Messages } from "@lingui/core";
import { createStudioI18n } from "@plux/i18n";
import { createMemoryHistory } from "@tanstack/react-router";
import { render } from "@testing-library/react";
import { MotionGlobalConfig } from "motion/react";
import { App } from "./app.tsx";
import type { EditSession } from "./features/editor/edit-session.ts";
import type { SurfaceFactory } from "./features/plugin-canvas/surface.ts";
import type { ValidatorFactory } from "./features/problems/validator-client.ts";
import { I18nProvider } from "./i18n-provider.tsx";
import { createSession } from "./lib/session.ts";
import { configureZod } from "./lib/zod.ts";
import { createAppRouter } from "./router.tsx";
import { createUpstream, type UpstreamData } from "./test-upstream.ts";

export { createUpstream, orgA, orgB, orgNew } from "./test-upstream.ts";

// Tests read the end state: Motion jumps there at once instead of running the Web Animations
// that happy-dom cancels between tests (U13).
MotionGlobalConfig.skipAnimations = true;

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

type Compiled = Messages[string];
type Token = string | [string] | [string, string, Record<string, Compiled>];

/**
 * An ICU message in Lingui's compiled form: text, `{name}` placeholders, and `plural`/`select` choices whose branches
 * hold `#` as a token of its own and `=0` as the key `0`, as Lingui's own compiler writes them.
 */
function compileIcu(text: string): Compiled {
  let at = 0;
  const branch = (inChoice: boolean): Token[] => {
    const out: Token[] = [];
    let buf = "";
    const flush = () => {
      if (buf !== "") {
        out.push(buf);
        buf = "";
      }
    };
    while (at < text.length) {
      const c = text[at];
      if (c === "}") {
        break;
      }
      if (c === "#" && inChoice) {
        flush();
        out.push("#");
        at += 1;
      } else if (c === "{") {
        flush();
        at += 1;
        out.push(argument());
      } else {
        buf += c;
        at += 1;
      }
    }
    flush();
    return out;
  };
  const argument = (): Token => {
    const close = text.indexOf("}", at);
    const comma = text.indexOf(",", at);
    if (comma < 0 || comma > close) {
      const name = text.slice(at, close).trim();
      at = close + 1;
      return [name];
    }
    const name = text.slice(at, comma).trim();
    const next = text.indexOf(",", comma + 1);
    const kind = text.slice(comma + 1, next).trim();
    at = next + 1;
    const cases: Record<string, Compiled> = {};
    for (;;) {
      while (text[at] === " ") {
        at += 1;
      }
      if (text[at] === "}") {
        at += 1;
        return [name, kind, cases];
      }
      const open = text.indexOf("{", at);
      const key = text.slice(at, open).trim();
      at = open + 1;
      cases[key.startsWith("=") ? key.slice(1) : key] = branch(kind !== "select") as Compiled;
      at += 1;
    }
  };
  const tokens = branch(false);
  return (tokens.length === 1 && typeof tokens[0] === "string" ? tokens[0] : tokens) as Compiled;
}

/** The English source catalogue, parsed from the extracted PO file, in Lingui's compiled form. */
export function loadSourceCatalogue(): Messages {
  const text = readFileSync(new URL("./locales/en/messages.po", import.meta.url), "utf-8");
  const out: Messages = {};
  for (const m of text.matchAll(/msgid "([^"]+)"\nmsgstr "((?:[^"\\]|\\.)*)"/g)) {
    const id = m[1];
    const message = m[2]?.replaceAll('\\"', '"');
    if (id !== undefined && message !== undefined) {
      out[id] = compileIcu(message);
    }
  }
  return out;
}
