// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, mock, test } from "bun:test";
import { createStudioI18n } from "@plux/i18n";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { I18nProvider } from "../../../i18n-provider.tsx";
import { loadSourceCatalogue } from "../../../test-harness.tsx";
import { ClipboardReportDialog } from "./report-dialog.tsx";

afterEach(cleanup);

const renderI18n = (node: ReactNode) =>
  render(
    <I18nProvider studio={createStudioI18n({ load: async () => loadSourceCatalogue(), pseudo: false })}>
      {node}
    </I18nProvider>,
  );

describe("STU-055 the paste dependency report (D34)", () => {
  test("lists what the target lacks with what Studio does, and confirms or cancels", async () => {
    const onConfirm = mock(() => undefined);
    const onClose = mock(() => undefined);
    renderI18n(
      <ClipboardReportDialog
        report={{ functions: ["<img src=x onerror=1>"], translations: [], assets: ["logo"], dataSources: [] }}
        refusal={undefined}
        onConfirm={onConfirm}
        onClose={onClose}
      />,
    );
    expect(await screen.findByText("Functions")).toBeTruthy();
    expect(screen.getByText("<img src=x onerror=1>")).toBeTruthy();
    expect(document.querySelector("img")).toBeNull();
    expect(screen.getByText("Assets")).toBeTruthy();
    expect(screen.queryByText("Translation keys")).toBeNull();
    fireEvent.click(screen.getByText("Paste anyway"));
    expect(onConfirm).toHaveBeenCalledTimes(1);
    fireEvent.click(screen.getByText("Cancel"));
    expect(onClose).toHaveBeenCalled();
  });

  test("a refusal shows its reason and no paste button", async () => {
    renderI18n(
      <ClipboardReportDialog
        report={undefined}
        refusal="too-large"
        onConfirm={() => undefined}
        onClose={() => undefined}
      />,
    );
    expect(await screen.findByText("The clipboard holds more than the size limit allows.")).toBeTruthy();
    expect(screen.queryByText("Paste anyway")).toBeNull();
  });
});
