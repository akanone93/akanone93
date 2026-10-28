// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, mock, test } from "bun:test";
import { createStudioI18n } from "@plux/i18n";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { I18nProvider } from "../../../i18n-provider.tsx";
import { loadSourceCatalogue } from "../../../test-harness.tsx";
import type { ReportItem, Resolution, TargetHas } from "./envelope.ts";
import { ClipboardReportDialog } from "./report-dialog.tsx";

afterEach(cleanup);

const renderI18n = (node: ReactNode) =>
  render(
    <I18nProvider studio={createStudioI18n({ load: async () => loadSourceCatalogue(), pseudo: false })}>
      {node}
    </I18nProvider>,
  );

const has = (ids: string[]): TargetHas => ({
  functions: new Set(ids),
  translations: new Set(ids),
  assets: new Set(ids),
  dataSources: new Set(ids),
});

const items: readonly ReportItem[] = [
  { kind: "assets", id: "logo", options: ["map", "copy", "drop"], preset: { action: "copy" } },
  { kind: "translations", id: "greet", options: ["map", "drop"], preset: { action: "drop" } },
  { kind: "functions", id: "<img src=x onerror=1>", options: ["drop"], preset: { action: "drop" } },
];

const open = (onConfirm: (c: ReadonlyMap<string, Resolution>) => void, onClose: () => void) =>
  renderI18n(
    <ClipboardReportDialog
      items={items}
      candidates={has(["brand", "other"])}
      refusal={undefined}
      onConfirm={onConfirm}
      onClose={onClose}
    />,
  );

describe("[STU-067] the paste dependency report (D34, F7)", () => {
  test("lists every missing item as text and confirms the presets", async () => {
    const onConfirm = mock((_: ReadonlyMap<string, Resolution>) => undefined);
    open(onConfirm, () => undefined);
    expect(await screen.findByText("Functions")).toBeTruthy();
    expect(screen.getByText("<img src=x onerror=1>")).toBeTruthy();
    expect(document.querySelector("img")).toBeNull();
    expect(screen.queryByText("Data sources")).toBeNull();
    fireEvent.click(screen.getByText("Paste with these choices"));
    const choices = onConfirm.mock.calls[0]?.[0];
    expect(choices?.get("assets:logo")).toEqual({ action: "copy" });
    expect(choices?.get("translations:greet")).toEqual({ action: "drop" });
  });

  test("map picks an existing item, copy and drop are switchable", async () => {
    const onConfirm = mock((_: ReadonlyMap<string, Resolution>) => undefined);
    open(onConfirm, () => undefined);
    const action = await screen.findByLabelText("What to do with greet");
    fireEvent.change(action, { target: { value: "map" } });
    fireEvent.change(screen.getByLabelText("Existing item to use for greet"), { target: { value: "other" } });
    fireEvent.change(screen.getByLabelText("What to do with logo"), { target: { value: "drop" } });
    fireEvent.click(screen.getByText("Paste with these choices"));
    const choices = onConfirm.mock.calls[0]?.[0];
    expect(choices?.get("translations:greet")).toEqual({ action: "map", to: "other" });
    expect(choices?.get("assets:logo")).toEqual({ action: "drop" });
    expect(choices?.get("functions:<img src=x onerror=1>")).toEqual({ action: "drop" });
  });

  test("copy is chosen again after a drop", async () => {
    const onConfirm = mock((_: ReadonlyMap<string, Resolution>) => undefined);
    open(onConfirm, () => undefined);
    const logo = await screen.findByLabelText("What to do with logo");
    fireEvent.change(logo, { target: { value: "drop" } });
    fireEvent.change(logo, { target: { value: "copy" } });
    fireEvent.click(screen.getByText("Paste with these choices"));
    expect(onConfirm.mock.calls[0]?.[0].get("assets:logo")).toEqual({ action: "copy" });
  });

  test("cancel applies nothing", async () => {
    const onConfirm = mock((_: ReadonlyMap<string, Resolution>) => undefined);
    const onClose = mock(() => undefined);
    open(onConfirm, onClose);
    fireEvent.click(await screen.findByText("Cancel"));
    expect(onClose).toHaveBeenCalled();
    expect(onConfirm).not.toHaveBeenCalled();
  });

  test("a refusal shows its reason and no paste button", async () => {
    renderI18n(
      <ClipboardReportDialog
        items={undefined}
        candidates={has([])}
        refusal="too-large"
        onConfirm={() => undefined}
        onClose={() => undefined}
      />,
    );
    expect(await screen.findByText("The clipboard holds more than the size limit allows.")).toBeTruthy();
    expect(screen.queryByText("Paste with these choices")).toBeNull();
  });
});
