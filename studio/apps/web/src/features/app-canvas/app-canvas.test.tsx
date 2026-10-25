// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import type { I18n } from "@lingui/core";
import type { PluginSummary } from "@plux/api";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { appCanvasMessages } from "./app-canvas.messages.ts";
import { PluginBoxCard } from "./app-canvas-page.tsx";

afterEach(cleanup);

// A catalogue-free translator: the message with its values appended.
const i18n = {
  _: (d: { message: string; values?: Record<string, unknown> }) =>
    `${d.message}${d.values === undefined ? "" : ` ${JSON.stringify(d.values)}`}`,
} as unknown as I18n;
const m = appCanvasMessages(i18n);

const summary = (over: Record<string, unknown>) =>
  ({
    plugin: { id: "p1", key: "loans", name: "Loans", archived: false, tags: [] },
    lockHolder: "",
    unpublishedChanges: false,
    validationErrors: 0n,
    killed: false,
    linksOut: [],
    linksIn: [],
    functionsUsed: [],
    ...over,
  }) as unknown as PluginSummary;

const renderCard = (s: PluginSummary, entry = false, onOpen = () => {}) =>
  render(
    <PluginBoxCard
      box={{ id: "p1", key: "loans", x: 10, y: 20 }}
      summary={s}
      selected={false}
      entry={entry}
      messages={m}
      onPointerDown={() => {}}
      onOpen={onOpen}
      onHover={() => {}}
    />,
  );

describe("plugin box badges (STU-030)", () => {
  test("a clean plugin shows Valid and no warnings", () => {
    renderCard(summary({}));
    expect(screen.getByText(m.valid)).toBeTruthy();
    expect(screen.queryByText(m.unpublished)).toBeNull();
    expect(screen.queryByText(m.killed)).toBeNull();
  });

  test("lock, unpublished changes, validation errors, kill switch and entry are all shown", () => {
    renderCard(summary({ lockHolder: "Ada", unpublishedChanges: true, validationErrors: 3n, killed: true }), true);
    expect(screen.getByText(m.lockedBy("Ada"))).toBeTruthy();
    expect(screen.getByText(m.unpublished)).toBeTruthy();
    expect(screen.getByText(m.validationErrors(3))).toBeTruthy();
    expect(screen.getByText(m.killed)).toBeTruthy();
    expect(screen.getByText(m.entryBadge)).toBeTruthy();
  });

  test("Enter and double click open the plugin canvas", () => {
    let opened = 0;
    renderCard(summary({}), false, () => opened++);
    const box = screen.getByRole("button", { name: m.plugin("Loans", "loans") });
    fireEvent.keyDown(box, { key: "Enter" });
    fireEvent.doubleClick(box);
    expect(opened).toBe(2);
  });
});
