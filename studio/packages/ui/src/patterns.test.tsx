// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { cases as countCases } from "./count.cases.tsx";
import { cases as glassCases } from "./glass.cases.tsx";
import { cases as liveCases } from "./live-chip.cases.tsx";
import { cases as meterCases } from "./meter.cases.tsx";
import { cases as metricCases } from "./metric-strip.cases.tsx";
import { cases as sparkCases } from "./sparkline.cases.tsx";
import { cases as pillCases } from "./status-pill.cases.tsx";
import { cases as summaryCases } from "./summary-card.cases.tsx";
import { cases as treeCases } from "./tree.cases.tsx";
import { cases as tagCases } from "./type-tag.cases.tsx";
import { cases as tableCases } from "./virtual-table.cases.tsx";

afterEach(cleanup);

describe("STU-010 STU-011 information patterns (U10)", () => {
  test("every case renders", () => {
    for (const list of [
      pillCases,
      countCases,
      tagCases,
      metricCases,
      meterCases,
      sparkCases,
      liveCases,
      summaryCases,
      glassCases,
    ]) {
      for (const c of list) {
        render(c.element);
        cleanup();
      }
    }
  });

  test("StatusPill shows an icon and a word", () => {
    const { container } = render(pillCases[0]?.element ?? <div />);
    expect(screen.getByText("Passing")).toBeTruthy();
    expect(container.querySelector("svg")?.getAttribute("aria-hidden")).toBe("true");
  });

  test("Count, Meter and Sparkline expose names and values", () => {
    render(
      <>
        {countCases[1]?.element}
        {meterCases[0]?.element}
        {sparkCases[0]?.element}
      </>,
    );
    expect(screen.getByRole("img", { name: "2 warnings" })).toBeTruthy();
    const meter = screen.getByRole("meter", { name: "Bundle size" });
    expect(meter.getAttribute("aria-valuenow")).toBe("256");
    expect(meter.getAttribute("aria-valuetext")).toBe("256 of 512 KB");
    expect(screen.getByRole("img", { name: /Frame time/ }).querySelector("polyline")).toBeTruthy();
  });

  test("LiveChip pulses unless motion is reduced; MetricStrip and SummaryCard are definition lists", () => {
    const { container } = render(
      <>
        {liveCases[0]?.element}
        {metricCases[0]?.element}
        {summaryCases[0]?.element}
      </>,
    );
    expect(screen.getByRole("status").textContent).toContain("41 ms");
    expect(container.querySelector(".motion-reduce\\:animate-none")).toBeTruthy();
    expect(container.querySelectorAll("dl").length).toBe(2);
  });

  test("glass surfaces are named regions and never nest", () => {
    const { container } = render(
      <>
        {glassCases[0]?.element}
        {glassCases[1]?.element}
      </>,
    );
    expect(screen.getByRole("group", { name: "Actions" })).toBeTruthy();
    expect(screen.getByRole("region", { name: "Details" })).toBeTruthy();
    expect(container.querySelector(".glass .glass")).toBeNull();
  });
});

describe("STU-120 STU-011 VirtualTable", () => {
  test("10,000 rows render only the visible ones", () => {
    const { container } = render(tableCases[0]?.element ?? <div />);
    expect(screen.getByRole("table", { name: "Pages" }).getAttribute("aria-rowcount")).toBe("10001");
    const body = container.querySelectorAll('[role="row"]').length;
    expect(body).toBeGreaterThan(1);
    expect(body).toBeLessThan(30);
    expect(screen.getByText("/pages/page-0")).toBeTruthy();
    expect(screen.queryByText("/pages/page-9999")).toBeNull();
  });
});

describe("STU-011 Tree keyboard", () => {
  function setup(index: number) {
    render(treeCases[index]?.element ?? <div />);
    const tree = screen.getByRole("tree");
    const first = screen.getAllByRole("treeitem")[0] as HTMLElement;
    first.focus();
    return { tree, first };
  }
  const key = (el: HTMLElement, k: string) => {
    fireEvent.keyDown(el, { key: k });
    fireEvent.keyUp(document, { key: k });
  };

  test("LTR: Right expands, Left collapses, Down moves", async () => {
    const { first } = setup(0);
    expect(screen.getAllByRole("treeitem").length).toBe(2);
    key(first, "ArrowRight");
    expect(screen.getAllByRole("treeitem").length).toBe(4);
    expect(first.getAttribute("aria-expanded")).toBe("true");
    key(first, "ArrowDown");
    await waitFor(() => expect(document.activeElement?.textContent).toContain("Title"));
    key(document.activeElement as HTMLElement, "ArrowLeft");
    await waitFor(() => expect(document.activeElement?.textContent).toContain("Home"));
    key(document.activeElement as HTMLElement, "ArrowLeft");
    expect(screen.getAllByRole("treeitem").length).toBe(2);
  });

  test("RTL: Left expands, Right collapses", () => {
    const { first } = setup(1);
    key(first, "ArrowRight");
    expect(screen.getAllByRole("treeitem").length).toBe(2);
    key(first, "ArrowLeft");
    expect(screen.getAllByRole("treeitem").length).toBe(4);
    key(first, "ArrowRight");
    expect(screen.getAllByRole("treeitem").length).toBe(2);
  });
});
