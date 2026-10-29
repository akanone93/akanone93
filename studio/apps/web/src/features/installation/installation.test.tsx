// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { cleanup, screen } from "@testing-library/react";
import { renderApp, type Upstream } from "../../test-harness.tsx";

afterEach(cleanup);

const open = (path: string, state: Upstream = {}) =>
  renderApp(`/installation${path}`, { installationAdmin: true, ...state });

describe("[STU-116] installation administration", () => {
  test("I1 shows each health check and its result", async () => {
    open("", {
      admin: {
        installation: {
          checks: [
            { name: "cache", healthy: true },
            { name: "database", healthy: false },
          ],
        },
      },
    });
    await screen.findByText("cache");
    expect(screen.getByText("Healthy")).toBeTruthy();
    expect(screen.getByText("Failing")).toBeTruthy();
    expect(screen.getByRole("link", { name: "Audit chain" })).toBeTruthy();
  });

  test("I2 lists every organisation of the installation with its counts", async () => {
    open("/organisations", {
      admin: {
        installation: {
          organizations: [
            { id: "o1", name: "Acme", createdAtMs: 1_700_000_000_000n, memberCount: 12n, appCount: 3n },
            { id: "o2", name: "Globex", createdAtMs: 1_700_000_100_000n, memberCount: 4n, appCount: 0n },
          ],
        },
      },
    });
    await screen.findByText("Acme");
    expect(screen.getByText("Globex")).toBeTruthy();
    expect(screen.getByText("12")).toBeTruthy();
    expect(screen.getByRole("columnheader", { name: "Apps" })).toBeTruthy();
  });

  test("I3 shows the database size", async () => {
    open("/storage", { admin: { installation: { storage: [{ area: "database", bytes: 5n * 1024n * 1024n }] } } });
    await screen.findByText("5.0 MiB");
  });

  test("I4 shows the queue depths", async () => {
    open("/jobs", { admin: { installation: { jobs: [{ queue: "publish", state: "waiting", count: 7n }] } } });
    await screen.findByText("publish");
    expect(screen.getByText("7")).toBeTruthy();
  });

  test("I5 says plainly that the server records no backups", async () => {
    open("/backups");
    await screen.findByText(/records no backups yet/);
  });

  test("I6 lists the trusted CI issuers", async () => {
    open("/issuers", {
      admin: {
        installation: {
          issuers: [{ issuer: "https://ci.example.org", audience: "plux", subjectPattern: "repo:acme/*" }],
        },
      },
    });
    await screen.findByText("https://ci.example.org");
    expect(screen.getByText("repo:acme/*")).toBeTruthy();
  });

  test("I7 shows the installation audit chain, newest first", async () => {
    open("/audit", {
      admin: {
        audit: [
          { id: "a1", action: "user.signed_in", actor: "Ada", targetKind: "user", targetId: "u1", atMs: 1 },
          { id: "a2", action: "license.state_changed", actor: "Ada", targetKind: "license", targetId: "l1", atMs: 2 },
        ],
      },
    });
    await screen.findByText("license.state_changed");
    const rows = screen.getAllByRole("row").map((r) => r.textContent ?? "");
    expect(rows[1]).toContain("license.state_changed");
    expect(rows[2]).toContain("user.signed_in");
  });

  test("a read that fails shows the error state", async () => {
    open("/issuers", { admin: { failing: true } });
    await screen.findByText("Nothing to show.");
  });

  test("a person who is not an installation administrator is refused", async () => {
    renderApp("/installation", { installationAdmin: false });
    await screen.findByText("Your role does not include the permission this page needs.");
  });
});
