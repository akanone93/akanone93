// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { orgA, renderApp } from "../../test-harness.tsx";
import { readSegment, ruleOf, ruleText, segmentDocument, segmentKeyOf } from "./segments.ts";

afterEach(cleanup);

const beta = {
  schemaVersion: "1.0.0",
  kind: "segment",
  id: "01a1ff23-0000-7000-8000-000000000001",
  key: "beta",
  name: "Beta testers",
  rule: { attribute: "user.segment", operator: "eq", value: "beta" },
};
const grouped = {
  ...beta,
  id: "01a1ff23-0000-7000-8000-000000000002",
  key: "mixed",
  name: "Mixed",
  rule: { any: [beta.rule] },
};

describe("[ABT-006] segment documents", () => {
  test("reads segment paths and rules, and writes a rule from its lines", () => {
    expect(segmentKeyOf("segments/beta.segment.json")).toBe("beta");
    expect(segmentKeyOf("segments/1x.segment.json")).toBeUndefined();
    expect(segmentKeyOf("app.json")).toBeUndefined();
    expect(readSegment("theme.json", 1n, beta)).toBeUndefined();
    expect(readSegment("segments/beta.segment.json", 3n, beta)?.name).toBe("Beta testers");
    expect(ruleText(beta.rule)).toBe("user.segment eq beta");
    expect(ruleText({ all: [beta.rule, { attribute: "region", operator: "in", value: ["DE", "AT"] }] })).toBe(
      "user.segment eq beta\nregion in DE, AT",
    );
    expect(ruleText(grouped.rule)).toBeUndefined();
    expect(ruleOf("user.segment eq beta")).toEqual(beta.rule);
    expect(ruleOf("user.segment eq beta\nregion in DE, AT")).toEqual({
      all: [beta.rule, { attribute: "region", operator: "in", value: ["DE", "AT"] }],
    });
    expect(ruleOf("")).toBeUndefined();
    expect(ruleOf("region is DE")).toBeUndefined();
  });

  test("builds a document only from a valid form and keeps an edited segment's id", () => {
    const form = {
      key: "beta",
      name: " Beta testers ",
      description: "",
      conditions: "user.segment eq beta",
      existing: undefined,
    };
    expect(segmentDocument(form, () => "new-id")).toEqual({ ...beta, id: "new-id" });
    const existing = readSegment("segments/beta.segment.json", 2n, beta);
    expect(segmentDocument({ ...form, existing }, () => "new-id")?.["id"]).toBe(beta.id);
    expect(segmentDocument({ ...form, key: "Beta" }, () => "x")).toBeUndefined();
    expect(segmentDocument({ ...form, name: " " }, () => "x")).toBeUndefined();
    expect(segmentDocument({ ...form, conditions: "nonsense" }, () => "x")).toBeUndefined();
  });
});

describe("[STU-103] segments page", () => {
  const path = `/o/${orgA}/apps/app1/experiments/segments`;
  const docs = [
    { kind: "segment", path: "segments/beta.segment.json", json: beta },
    { kind: "segment", path: "segments/mixed.segment.json", json: grouped },
    { kind: "app", path: "app.json", json: {} },
  ];

  test("lists the app's segments and creates one", async () => {
    const createdDocuments: string[] = [];
    renderApp(path, { permissions: ["app.read", "app.manage"], appDocuments: docs, createdDocuments });
    await screen.findByText("Beta testers");
    expect(screen.getByText("user.segment eq beta")).toBeTruthy();
    expect(screen.getByText("A rule with groups: edit it in the app's JSON editor.")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Edit mixed" }).getAttribute("aria-disabled")).toBe("true");
    fireEvent.change(screen.getByLabelText("Segment key"), { target: { value: "germans" } });
    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Germans" } });
    fireEvent.change(screen.getByLabelText("Conditions"), { target: { value: "region in DE, AT" } });
    fireEvent.click(screen.getByRole("button", { name: "Save segment" }));
    await waitFor(() => expect(createdDocuments).toEqual(["segments/germans.segment.json"]));
  });

  test("refuses an invalid key or condition, and deletes a segment", async () => {
    const deletedDocuments: string[] = [];
    renderApp(path, { permissions: ["app.read", "app.manage"], appDocuments: docs, deletedDocuments });
    await screen.findByText("Beta testers");
    fireEvent.change(screen.getByLabelText("Segment key"), { target: { value: "Beta" } });
    await screen.findByText("A segment key is lower case letters, digits and hyphens, and starts with a letter.");
    fireEvent.change(screen.getByLabelText("Conditions"), { target: { value: "region is DE" } });
    await screen.findByText(/Each line must be an attribute/);
    // An attribute neither a device nor the user context has is refused here, not only by the server (row 320).
    fireEvent.change(screen.getByLabelText("Conditions"), { target: { value: "beta eq true" } });
    await screen.findByText(/Each line must be an attribute/);
    expect(screen.getByRole("button", { name: "Save segment" }).getAttribute("aria-disabled")).toBe("true");
    fireEvent.click(screen.getByRole("button", { name: "Delete beta" }));
    await waitFor(() => expect(deletedDocuments).toEqual(["segments/beta.segment.json"]));
  });

  test("a person without app.manage reads but cannot change segments", async () => {
    renderApp(path, { permissions: ["app.read"], appDocuments: docs });
    await screen.findByText("Beta testers");
    expect(screen.getByRole("button", { name: "Delete beta" }).getAttribute("aria-disabled")).toBe("true");
    expect(screen.getByRole("button", { name: "Save segment" }).getAttribute("aria-disabled")).toBe("true");
  });
});
