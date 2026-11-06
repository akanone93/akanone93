// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, mock, test } from "bun:test";
import { createStudioI18n } from "@plux/i18n";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { I18nProvider } from "../../i18n-provider.tsx";
import { loadSourceCatalogue } from "../../test-harness.tsx";
import { animatableProps, TimelineStrip } from "./timeline-strip.tsx";

afterEach(cleanup);

describe("PC-30 the timeline strip [ANI-002]", () => {
  test("it scrubs, goes to a keyframe, removes one with Shift and keys the selected widget at the playhead", async () => {
    const setMs = mock((_: number) => undefined);
    const removeKeyframe = mock((_n: string | undefined, _p: string, _a: number) => undefined);
    const putKeyframe = mock((_n: string, _p: string, _v: unknown) => undefined);
    render(
      <I18nProvider studio={createStudioI18n({ load: async () => loadSourceCatalogue(), pseudo: false })}>
        <TimelineStrip
          timeline={{
            id: "t",
            name: "intro",
            durationMs: 1000,
            tracks: [
              {
                node: "box",
                prop: "opacity",
                keyframes: [
                  { atMs: 0, value: 0 },
                  { atMs: 600, value: 1 },
                ],
              },
            ],
          }}
          ms={300}
          setMs={setMs}
          canEdit
          nameOf={() => "Opacity"}
          selected={{ id: "box", type: "Opacity", props: { opacity: 1 } }}
          scrollViews={[]}
          set={() => undefined}
          putKeyframe={putKeyframe}
          removeKeyframe={removeKeyframe}
          onClose={() => undefined}
        />
      </I18nProvider>,
    );
    fireEvent.change(await screen.findByRole("slider", { name: "Playhead" }), { target: { value: "450" } });
    expect(setMs).toHaveBeenLastCalledWith(450);
    fireEvent.click(screen.getByRole("button", { name: /opacity 0.60 s/ }));
    expect(setMs).toHaveBeenLastCalledWith(600);
    fireEvent.click(screen.getByRole("button", { name: /opacity 0.00 s/ }), { shiftKey: true });
    expect(removeKeyframe).toHaveBeenLastCalledWith("box", "opacity", 0);
    fireEvent.click(screen.getByRole("button", { name: /Add keyframe at 0.30 s/ }));
    expect(putKeyframe.mock.calls[0]?.[0]).toBe("box");
    expect(putKeyframe.mock.calls[0]?.[1]).toBe("opacity");
    expect(animatableProps("Opacity")).toContain("opacity");
  });
});
