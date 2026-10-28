// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import type { I18n } from "@lingui/core";
import type { PluginSummary } from "@plux/api";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { appCanvasMessages } from "./app-canvas.messages.ts";
import { changeFrame, type Frame, type FrameChange, MIN_FRAME } from "./app-canvas.model.ts";
import { FrameTools } from "./frame-tools.tsx";
import { PluginDialogs } from "./plugin-dialogs.tsx";

afterEach(cleanup);

const i18n = {
  _: (d: { message: string; values?: Record<string, string> }) =>
    d.message.replace(/\{(\w+)\}/g, (_, k: string) => d.values?.[k] ?? ""),
} as unknown as I18n;
const m = appCanvasMessages(i18n);

const frame: Frame = { id: "f1", label: "Lending", x: 0, y: 0, width: 400, height: 300 };

describe("[STU-035] D20 rename and resize a frame", () => {
  test("changeFrame renames, resizes and keeps a minimum side", () => {
    const out = changeFrame([frame], "f1", { label: " Credit ", width: 10, height: 520.4 });
    expect(out[0]).toMatchObject({ label: "Credit", width: MIN_FRAME, height: 520 });
    expect(changeFrame([frame], "f1", { label: " ", width: Number.NaN, height: 300 })[0]).toMatchObject({
      label: "Lending",
      width: 400,
    });
  });

  test("the dialog hands one change to the editing core", () => {
    const seen: [string, FrameChange][] = [];
    render(<FrameTools frames={[frame]} messages={m} onChange={(id, c) => seen.push([id, c])} />);
    fireEvent.click(screen.getByText("Rename or resize frame Lending"));
    fireEvent.change(screen.getByLabelText("Frame label"), { target: { value: "Credit" } });
    fireEvent.change(screen.getByLabelText("Width"), { target: { value: "640" } });
    fireEvent.click(screen.getByText("Save"));
    expect(seen).toEqual([["f1", { label: "Credit", width: 640, height: 300 }]]);
  });
});

describe("[STU-034] D17 the owning-team picker", () => {
  test("settings show the owning team and save it with the other fields", () => {
    const saved: unknown[][] = [];
    const s = {
      plugin: { id: "p1", key: "loans", name: "Loans", description: "", tags: [], owningTeamId: "t1" },
    } as unknown as PluginSummary;
    render(
      <PluginDialogs
        dialogue={{ kind: "settings" }}
        messages={m}
        summary={s}
        taken={new Set()}
        selectedBoxes={[]}
        teams={[
          { id: "t1", name: "Core" },
          { id: "t2", name: "Risk" },
        ]}
        close={() => {}}
        onCreate={() => {}}
        onSettings={(...a) => saved.push(a)}
        onDuplicate={() => {}}
        onEntry={() => {}}
        onFrame={() => {}}
      />,
    );
    expect(screen.getAllByText("Core").length).toBeGreaterThan(0);
    fireEvent.click(screen.getByText("Save"));
    expect(saved).toEqual([["p1", "Loans", "", [], "t1"]]);
  });
});
