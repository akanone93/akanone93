// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import type { I18n } from "@lingui/core";
import { createStudioI18n } from "@plux/i18n";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { I18nProvider } from "../../i18n-provider.tsx";
import { loadSourceCatalogue } from "../../test-harness.tsx";
import { FoldCard, FoldInline, FoldList, foldOpen } from "./fold.tsx";
import { panelMessages } from "./panels.messages.ts";
import { propGroupOf } from "./prop-groups.ts";
import { valueSummary } from "./prop-row.tsx";
import { ValueEditor } from "./right-dock.tsx";

afterEach(() => {
  cleanup();
  localStorage.clear();
});

const m = panelMessages({ _: (d: { message: string }) => d.message } as unknown as I18n);

const cards = (first: string | undefined, all = false) => (
  <FoldList scope="test" first={first} all={all}>
    <FoldCard id="content" icon="text-t" title="Content" note={3}>
      <p>content body</p>
    </FoldCard>
    <FoldCard id="layout" icon="grid-four" title="Layout">
      <p>layout body</p>
    </FoldCard>
    <FoldCard id="motion" icon="sparkle" title="Motion">
      <p>motion body</p>
    </FoldCard>
  </FoldList>
);
const head = (name: RegExp) => screen.getByRole("button", { name });

describe("[STU-061] [STU-062] MR-3 sections fold: the first open, the rest closed", () => {
  test("only the list's first section is open at first; each head says what it holds", () => {
    render(cards("content"));
    expect(screen.getByText("content body")).toBeTruthy();
    expect(screen.queryByText("layout body")).toBeNull();
    expect(screen.queryByText("motion body")).toBeNull();
    expect(head(/^Content/).hasAttribute("data-panel-open")).toBe(true);
    expect(head(/^Layout/).hasAttribute("data-panel-open")).toBe(false);
    expect(head(/^Content/).textContent).toContain("3");
    expect(screen.getByRole("region", { name: "Motion" })).toBeTruthy();
  });

  test("what the person opens and closes is kept for the next time, by section", () => {
    render(cards("content"));
    fireEvent.click(head(/^Motion/));
    fireEvent.click(head(/^Content/));
    expect(screen.getByText("motion body")).toBeTruthy();
    expect(screen.queryByText("content body")).toBeNull();
    cleanup();
    // Another selection whose first section is Layout: Motion stays open, Content closed, Layout opens as the first.
    render(cards("layout"));
    expect(screen.getByText("motion body")).toBeTruthy();
    expect(screen.getByText("layout body")).toBeTruthy();
    expect(screen.queryByText("content body")).toBeNull();
    expect(foldOpen({ motion: true }, "content", "content")).toBe(true);
    expect(foldOpen({ content: false }, "content", "content")).toBe(false);
    expect(foldOpen({}, "layout", "content")).toBe(false);
  });

  test("a search opens every section, so nothing it finds is hidden", () => {
    render(cards("content", true));
    for (const body of ["content body", "layout body", "motion body"]) {
      expect(screen.getByText(body)).toBeTruthy();
    }
  });

  test("inside another section a card is a plain subsection, always shown", () => {
    render(
      <FoldInline>
        <FoldCard id="auto-layout" icon="rows" title="Auto layout" note="Column">
          <p>auto body</p>
        </FoldCard>
      </FoldInline>,
    );
    expect(screen.getByRole("heading", { name: /Auto layout/ })).toBeTruthy();
    expect(screen.getByText("auto body")).toBeTruthy();
    expect(screen.queryByRole("button")).toBeNull();
  });
});

describe("[STU-062] MR-3 composite values fold into rows with a summary", () => {
  test("a summary names a token, an expression, a list's length or an object's first plain values", () => {
    expect(valueSummary(undefined, "Not set")).toBe("Not set");
    expect(valueSummary({ $token: "textTheme.bodyLarge" }, "Not set")).toBe("bodyLarge");
    expect(valueSummary({ $expr: "page.style" }, "Not set")).toBe("page.style");
    expect(valueSummary(["a", "b"], "Not set")).toBe("[2]");
    expect(valueSummary({ fontSize: 16, fontWeight: "w600", color: "#1D1B20", italic: true }, "")).toBe(
      "16 · w600 · #1D1B20",
    );
    expect(valueSummary({ color: { $token: "color.primary" } }, "")).toBe("primary");
  });

  test("an object's plain fields come first on the grid; its composite fields fold, the first open", async () => {
    render(
      <I18nProvider studio={createStudioI18n({ load: async () => loadSourceCatalogue(), pseudo: false })}>
        <ValueEditor
          type="InputDecoration"
          name="decoration"
          value={{ labelText: "Name", labelStyle: { fontSize: 12 } }}
          mixed={false}
          unit=""
          readOnly={false}
          sources={{ tokens: [], keys: [], check: undefined }}
          m={m}
          onChange={() => undefined}
          onEnd={() => undefined}
        />
      </I18nProvider>,
    );
    await screen.findAllByText("Label text");
    const grid = document.querySelector(".prop-grid-nested") as HTMLElement;
    const children = [...grid.children];
    const firstFold = children.findIndex((c) => c.classList.contains("fold-row"));
    // Every plain row stands before the first folded one.
    expect(children.slice(firstFold).every((c) => c.classList.contains("fold-row"))).toBe(true);
    expect(children.slice(0, firstFold).some((c) => c.textContent?.includes("Label text"))).toBe(true);
    const folds = [...grid.querySelectorAll<HTMLElement>(".fold-row-trigger")];
    expect(folds.length).toBeGreaterThan(2);
    expect(folds.filter((f) => f.hasAttribute("data-panel-open")).length).toBe(1);
    expect(folds[0]?.hasAttribute("data-panel-open")).toBe(true);
    const label = folds.find((f) => f.textContent?.startsWith("Label style"));
    expect(label?.textContent).toContain("12");
    expect(label?.closest(".fold-row")?.hasAttribute("data-set")).toBe(true);
  });
});

describe("[STU-062] MR-3 a text field's keyboard and input props have their own section", () => {
  test("keyboard, autocorrect, length and cursor size go to Keyboard & input; the cursor colour stays a style", () => {
    for (const name of [
      "keyboardType",
      "textInputAction",
      "textCapitalization",
      "autocorrect",
      "obscureText",
      "maxLength",
      "cursorWidth",
      "autofillHints",
    ]) {
      expect(propGroupOf(name)).toBe("input");
    }
    expect(propGroupOf("cursorColor")).toBe("appearance");
    expect(propGroupOf("cursorRadius")).toBe("appearance");
    expect(propGroupOf("textAlign")).toBe("content");
    expect(propGroupOf("maxLines")).toBe("behaviour");
    expect(propGroupOf("textDirection")).toBe("layout");
    expect(propGroupOf("softWrap")).toBe("content");
  });
});
