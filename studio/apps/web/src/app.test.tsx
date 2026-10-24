// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, expect, test } from "bun:test";
import { cleanup, render, screen } from "@testing-library/react";
import { App } from "./app.tsx";

afterEach(cleanup);

test("renders the main landmark", () => {
  render(<App />);
  expect(screen.getByRole("main")).toBeDefined();
});
