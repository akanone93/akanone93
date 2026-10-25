// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { createStudioI18n } from "@plux/i18n";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { I18nProvider } from "../../i18n-provider.tsx";
import { loadSourceCatalogue } from "../../test-harness.tsx";
import { EditSession } from "./edit-session.ts";
import { LOCK_TTL_MS, LockController, type LockInfo, type LockTransport } from "./lock-controller.ts";
import { LockBanner, LockLostDialog } from "./lock-ui.tsx";

afterEach(cleanup);

function renderI18n(node: ReactNode) {
  const studio = createStudioI18n({ load: async () => loadSourceCatalogue(), pseudo: false });
  return render(<I18nProvider studio={studio}>{node}</I18nProvider>);
}

const never: Scheduler = { setTimeout: () => 0, clearTimeout: () => undefined };
type Scheduler = { setTimeout(fn: () => void, ms: number): unknown; clearTimeout(h: unknown): void };

function stub(other: LockInfo | null, calls: string[]): LockTransport {
  return {
    acquire: async (_s, force) => {
      calls.push(`acquire ${force}`);
      throw new Error("held");
    },
    renew: async () => {
      throw new Error("held");
    },
    release: async () => undefined,
    request: async () => {
      calls.push("request");
      return other as LockInfo;
    },
    get: async () => other,
    classify: () => "refused",
  };
}

function lockOf(now: number, allowedIn: number): LockInfo {
  return {
    holder: "Ben",
    session: "b-1",
    acquiredAt: new Date(now - 60_000),
    expiresAt: new Date(now + LOCK_TTL_MS),
    lastEditAt: new Date(now),
    takeoverAllowedAt: new Date(now + allowedIn),
    requestedBy: [],
  };
}

function controller(transport: LockTransport, now: number) {
  return new LockController({
    transport,
    session: "tab-1",
    idleReleaseMs: 600_000,
    takeoverIdleMs: 300_000,
    scheduler: never,
    ticker: { every: () => () => undefined },
    now: () => now,
  });
}

describe("STU-075 lock banner, D36 and D37 (F1, L-1)", () => {
  test("the read-only banner names the holder; Take over waits for L-1; Request edit asks", async () => {
    const now = Date.now();
    const calls: string[] = [];
    const lock = controller(stub(lockOf(now, 300_000), calls), now);
    await lock.refresh();
    renderI18n(<LockBanner lock={lock} now={() => now} />);
    expect(await screen.findByText(/Ben has been editing this plugin/)).toBeDefined();
    expect(screen.getByText(/Take over is possible from/)).toBeDefined();
    // Disabled with a reason, not hidden (F1.6); the button stays focusable.
    const takeOver = screen.getByRole("button", { name: "Take over" });
    expect(takeOver.getAttribute("aria-disabled")).toBe("true");
    fireEvent.click(screen.getByRole("button", { name: "Request edit" }));
    await waitFor(() => expect(calls).toContain("request"));
    expect(await screen.findByText("Ben was asked to hand over the plugin.")).toBeDefined();
  });

  test("D36 confirms the takeover with the holder and their last edit", async () => {
    const now = Date.now();
    const calls: string[] = [];
    const lock = controller(stub(lockOf(now, -1), calls), now);
    await lock.refresh();
    renderI18n(<LockBanner lock={lock} now={() => now} />);
    const takeOver = await screen.findByRole("button", { name: "Take over" });
    expect(takeOver.getAttribute("aria-disabled")).not.toBe("true");
    fireEvent.click(takeOver);
    expect(await screen.findByText(/Ben holds the lock; their last edit was/)).toBeDefined();
    const confirm = screen.getAllByRole("button", { name: "Take over" }).at(-1);
    if (confirm === undefined) {
      throw new Error("no confirm button");
    }
    fireEvent.click(confirm);
    await waitFor(() => expect(calls).toContain("acquire true"));
  });

  test("D37 shows the unsaved changes and offers re-take, compare and discard", async () => {
    const now = Date.now();
    const lock = controller(stub(null, []), now);
    lock.store.setState({ status: "lost", lost: "expired", lock: null });
    const session = new EditSession({
      transport: {
        patch: () => new Promise(() => undefined),
        get: () => new Promise(() => undefined),
        classify: () => "transient",
      },
      debounceMs: 1000,
      maxReplays: 1,
      scheduler: never,
    });
    session.open({ path: "a.page.json", revision: 1n, content: { kind: "page" } });
    session.edit("a.page.json", (d) => {
      (d as { title?: string }).title = "x";
    });
    renderI18n(<LockLostDialog lock={lock} session={session} />);
    expect(await screen.findByText("You no longer hold the editing lock")).toBeDefined();
    expect(screen.getByText("1 unsaved change")).toBeDefined();
    fireEvent.click(screen.getByRole("button", { name: "Compare" }));
    expect(await screen.findByText("a.page.json (1)")).toBeDefined();
    fireEvent.click(screen.getByRole("button", { name: "Discard" }));
    await waitFor(() => expect(lock.store.getState().status).toBe("free"));
  });
});
