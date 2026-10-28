// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { Code, ConnectError } from "@connectrpc/connect";
import { createStudioI18n } from "@plux/i18n";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { I18nProvider } from "../../i18n-provider.tsx";
import { loadSourceCatalogue } from "../../test-harness.tsx";
import { StatusChips } from "../shell/status-chips.tsx";
import { ConflictDialog } from "./conflict-dialog.tsx";
import { classifySaveError } from "./document-transport.ts";
import { EditSession, type ServerDocument } from "./edit-session.ts";

/** The test page's shape, for typed access inside edit recipes. */
interface Page {
  kind: string;
  props: { padding: number; title?: string; color?: string };
  children: string[];
}

function page(d: Record<string, unknown>): Page {
  return d as unknown as Page;
}

afterEach(cleanup);

function renderI18n(node: ReactNode) {
  const studio = createStudioI18n({ load: async () => loadSourceCatalogue(), pseudo: false });
  return render(<I18nProvider studio={studio}>{node}</I18nProvider>);
}

const path = "screens/login.page.json";

describe("[SRV-030] D35 conflict dialog and the waiting chip (F2, F4)", () => {
  test("the chip counts edits waiting, online and offline", async () => {
    const view = renderI18n(<StatusChips online stream="live" waiting={2} />);
    expect(await screen.findByText("2 changes waiting")).toBeDefined();
    view.rerender(
      <I18nProvider studio={createStudioI18n({ load: async () => loadSourceCatalogue(), pseudo: false })}>
        <StatusChips online={false} stream="live" waiting={1} />
      </I18nProvider>,
    );
    expect(await screen.findByText("Offline — 1 change waiting")).toBeDefined();
  });

  test("a clash opens D35 with the diff; take the server's closes it", async () => {
    const server: ServerDocument = { path, revision: 2n, content: { kind: "page", props: { padding: 4 } } };
    const session = new EditSession({
      transport: {
        patch: () => Promise.reject(new ConnectError("stale", Code.Aborted)),
        get: () => Promise.resolve(server),
        put: () => Promise.resolve(server),
        remove: () => Promise.resolve(),
        classify: classifySaveError,
      },
      debounceMs: 0,
      maxReplays: 3,
      online: () => true,
    });
    session.open({ path, revision: 1n, content: { kind: "page", props: { padding: 8 } } });
    session.edit(path, (d) => {
      page(d).props.padding = 24;
    });
    session.flushAll();
    renderI18n(<ConflictDialog session={session} />);
    expect(await screen.findByText("This plugin changed on the server")).toBeDefined();
    expect(screen.getAllByText("/props/padding").length).toBeGreaterThan(0);
    fireEvent.click(screen.getByText("Take the server's"));
    await waitFor(() => expect(session.store.getState().conflict === null).toBe(true));
    expect(session.store.getState().docs[path]?.content).toEqual(server.content);
  });
});
