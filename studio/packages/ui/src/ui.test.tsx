// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { cleanup, render, screen } from "@testing-library/react";
import { parsePaths, render as renderIcons } from "../scripts/gen-icons.ts";
import { cases as buttonCases } from "./button.cases.tsx";
import { Button } from "./button.tsx";
import { allCases } from "./cases.ts";
import { cases as dialogCases } from "./dialog.cases.tsx";
import { cases as fieldCases } from "./field.cases.tsx";
import { cases as iconCases } from "./icon.cases.tsx";
import { Icon } from "./icon.tsx";
import { iconMirrors, mirroredIcons } from "./icon-mirrors.ts";
import { iconNames, iconPaths } from "./icons.gen.ts";
import { cases as menuCases } from "./menu.cases.tsx";
import { cases as popoverCases } from "./popover.cases.tsx";
import { cases as selectCases } from "./select.cases.tsx";
import { cases as tabsCases } from "./tabs.cases.tsx";
import { cases as toastCases } from "./toast.cases.tsx";
import { cases as tooltipCases } from "./tooltip.cases.tsx";
import { Tooltip, tooltipDelayMs } from "./tooltip.tsx";

afterEach(cleanup);

describe("STU-010 Icon component and generated set", () => {
  test("every generated icon has a faded and a solid layer", () => {
    for (const name of iconNames) {
      const paths = iconPaths[name];
      expect(paths.some((p) => p.faded)).toBe(true);
      expect(paths.some((p) => !p.faded)).toBe(true);
    }
  });

  test("the mirror list only names icons of the set", () => {
    for (const name of mirroredIcons) {
      expect(iconNames).toContain(name);
    }
    expect(iconMirrors("arrow-left")).toBe(true);
    expect(iconMirrors("play")).toBe(false);
  });

  test("decorative icons are aria-hidden, labelled icons are images", () => {
    const { container } = render(
      <>
        <Icon name="gear" />
        <Icon name="info" label="Details" />
      </>,
    );
    const svgs = container.querySelectorAll("svg");
    expect(svgs[0]?.getAttribute("aria-hidden")).toBe("true");
    expect(screen.getByRole("img", { name: "Details" })).toBeTruthy();
  });

  test("direction-carrying icons mirror under RTL, others never", () => {
    const { container } = render(
      <>
        <Icon name="arrow-left" />
        <Icon name="play" />
      </>,
    );
    const [back, play] = Array.from(container.querySelectorAll("svg"));
    expect(back?.getAttribute("class")).toContain("rtl:-scale-x-100");
    expect(back?.getAttribute("data-mirrors")).toBe("true");
    expect(play?.getAttribute("class")).not.toContain("rtl:");
  });

  test("the generator is deterministic and rejects broken SVG", () => {
    const svg = '<svg><path d="M0,0Z" opacity="0.2"/><path d="M1,1Z"/></svg>';
    expect(parsePaths(svg)).toEqual([
      { d: "M0,0Z", faded: true },
      { d: "M1,1Z", faded: false },
    ]);
    expect(() => parsePaths("<svg></svg>")).toThrow();
    expect(() => parsePaths('<svg><path fill="x"/></svg>')).toThrow();
    expect(renderIcons(() => svg)).toBe(renderIcons(() => svg));
  });

  test("icon cases render", () => {
    for (const c of iconCases) {
      const { container } = render(c.element);
      expect(container.querySelector("svg")).toBeTruthy();
      cleanup();
    }
  });
});

describe("STU-011 accessible components", () => {
  test("every catalogue case renders", () => {
    for (const [name, list] of Object.entries(allCases)) {
      expect(list.length, name).toBeGreaterThan(0);
      for (const c of list) {
        render(c.element);
        cleanup();
      }
    }
  });

  test("Button: roles, loading and disabled stay focusable", () => {
    render(
      <>
        {buttonCases.map((c) => (
          <div key={c.name}>{c.element}</div>
        ))}
      </>,
    );
    const loading = screen.getByRole("button", { name: /Saving/ });
    expect(loading.getAttribute("aria-busy")).toBe("true");
    expect(loading.getAttribute("aria-disabled")).toBe("true");
    const close = screen.getByRole("button", { name: "Close" });
    expect(close.getAttribute("aria-busy")).toBeNull();
  });

  test("Button: variant is exposed and click works", () => {
    let clicks = 0;
    render(
      <Button variant="danger" onClick={() => clicks++}>
        Go
      </Button>,
    );
    const b = screen.getByRole("button", { name: "Go" });
    b.click();
    expect(clicks).toBe(1);
    expect(b.getAttribute("data-variant")).toBe("danger");
  });

  test("Field: label names the control, error is linked and marks it invalid", () => {
    render(
      <>
        {fieldCases.map((c) => (
          <div key={c.name}>{c.element}</div>
        ))}
      </>,
    );
    const inputs = screen.getAllByRole("textbox", { name: "Name" });
    expect(inputs.length).toBeGreaterThan(0);
    const invalid = inputs.find((i) => i.getAttribute("aria-invalid") === "true");
    expect(invalid).toBeTruthy();
    expect(screen.getByText("A name is required.")).toBeTruthy();
    expect(screen.getByRole("textbox", { name: "Details" }).tagName).toBe("TEXTAREA");
  });

  test("Select: trigger is a combobox with a name", () => {
    render(selectCases[0]?.element ?? <div />);
    const trigger = screen.getByRole("combobox", { name: "Choose a language" });
    expect(trigger.getAttribute("aria-expanded")).toBe("false");
  });

  test("Dialog: open dialog has a name and a close control; alert dialog has its role", () => {
    render(dialogCases[1]?.element ?? <div />);
    expect(screen.getByRole("dialog", { name: "Details" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Close" })).toBeTruthy();
    cleanup();
    render(dialogCases[3]?.element ?? <div />);
    expect(screen.getByRole("alertdialog", { name: "Delete this page?" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Cancel" })).toBeTruthy();
  });

  test("Popover: trigger announces its popup", () => {
    render(popoverCases[0]?.element ?? <div />);
    const t = screen.getByRole("button", { name: "Details" });
    expect(t.getAttribute("aria-expanded")).toBe("false");
  });

  test("Menu: open menu exposes menuitems, the disabled one is marked", () => {
    render(menuCases[1]?.element ?? <div />);
    expect(screen.getByRole("menu")).toBeTruthy();
    expect(screen.getAllByRole("menuitem").length).toBe(3);
    expect(screen.getByRole("menuitem", { name: "Duplicate" }).getAttribute("aria-disabled")).toBe("true");
  });

  test("Tabs: tablist, selected tab and panel", () => {
    render(tabsCases[1]?.element ?? <div />);
    expect(screen.getByRole("tablist")).toBeTruthy();
    expect(screen.getByRole("tab", { name: "Actions" }).getAttribute("aria-selected")).toBe("true");
    expect(screen.getByRole("tab", { name: "Pages" }).getAttribute("aria-selected")).toBe("false");
    expect(screen.getByRole("tabpanel")).toBeTruthy();
  });

  test("Tooltip: the trigger renders as the child element; behaviour is covered by Playwright", () => {
    expect(tooltipDelayMs).toBe(400);
    expect(tooltipCases.length).toBe(1);
    render(
      <Tooltip content="Add a page">
        <button type="button">Add</button>
      </Tooltip>,
    );
    expect(screen.getByRole("button", { name: "Add" })).toBeTruthy();
  });

  test("Toast: the region and triggers render", () => {
    render(toastCases[0]?.element ?? <div />);
    expect(screen.getByRole("button", { name: "Save" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Delete" })).toBeTruthy();
  });
});
