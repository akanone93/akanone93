// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { useState } from "react";
import { DockTabs } from "./dock-tabs.tsx";

afterEach(cleanup);

function Dock() {
  const [folded, setFolded] = useState(false);
  return (
    <DockTabs
      panes={[{ value: "a", label: "Alpha", icon: "stack", panel: <p>Alpha pane</p> }]}
      defaultValue="a"
      mark="squares-four"
      head={<h2>Dock</h2>}
      folded={folded}
      onFold={setFolded}
      foldLabel="Fold the panel"
      unfoldLabel="Open the panel"
    />
  );
}

describe("dock folding (ADR-0103)", () => {
  test("the header folds the dock and the button outside the folded dock opens it again", () => {
    render(<Dock />);
    expect(screen.queryByRole("button", { name: "Open the panel" })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Fold the panel" }));
    expect(screen.getByText("Alpha pane").closest("[hidden]")).not.toBeNull();
    const open = screen.getByRole("button", { name: "Open the panel" });
    expect(open.getAttribute("data-side")).toBe("start");
    fireEvent.click(open);
    expect(screen.getByText("Alpha pane").closest("[hidden]")).toBeNull();
    expect(screen.queryByRole("button", { name: "Open the panel" })).toBeNull();
  });
});
