import { test } from "@playwright/test";
import { orgA } from "../apps/web/src/test-upstream.ts";
import { mockUpstream } from "./upstream.ts";

const day = 86_400;
const now = Math.floor(Date.now() / 1000);
const apps = [
  { id: "a1", key: "bank", name: "Plux Bank", description: "Retail banking for 2 million customers", tags: ["finance"], activeDevices: 184_220, pluginCount: 14, crashFreeRate: 0.998, crashFreeTrend: 0.002, lastPublishedAt: now - 2 * 3600, lastPublisher: { kind: "user", id: "u1", display: "Ada Lovelace" }, releases: [{ environmentKey: "production", liveSequence: 42 }, { environmentKey: "staging", liveSequence: 44 }] },
  { id: "a2", key: "express", name: "Plux Express", description: "Same-day delivery", tags: ["logistics"], activeDevices: 52_310, pluginCount: 9, crashFreeRate: 0.991, crashFreeTrend: -0.003, lastPublishedAt: now - day, lastPublisher: { kind: "user", id: "u2", display: "Grace Hopper" }, releases: [{ environmentKey: "production", liveSequence: 18 }] },
  { id: "a3", key: "fresh", name: "Fresh", description: "Groceries in 30 minutes", tags: ["retail"], activeDevices: 12_480, pluginCount: 6, crashFreeRate: 0.996, crashFreeTrend: 0.001, lastPublishedAt: now - 3 * day, lastPublisher: { kind: "user", id: "u3", display: "Alan Turing" }, releases: [{ environmentKey: "staging", liveSequence: 7 }] },
  { id: "a4", key: "loans", name: "Loan Desk", description: "Loan calculator and applications", tags: ["finance"], activeDevices: 3_904, pluginCount: 4, crashFreeRate: 0.999, crashFreeTrend: 0, lastPublishedAt: now - 9 * day, lastPublisher: { kind: "user", id: "u1", display: "Ada Lovelace" }, releases: [{ environmentKey: "production", liveSequence: 5 }] },
];
const permissions = ["app.read", "app.manage", "plugin.read", "plugin.write", "audit.read", "members.manage", "organization.manage", "sso.manage", "sessions.manage", "compliance.report"];
const summaries = [
  { id: "p1", key: "home", name: "Home", screens: 4, versions: 3, linksOut: ["loans", "cards"], publishedAt: now - day },
  { id: "p2", key: "loans", name: "Loans", screens: 6, versions: 2, linksOut: ["cards"], unpublishedChanges: true },
  { id: "p3", key: "cards", name: "Cards", screens: 3, versions: 5, linksOut: [] },
  { id: "p4", key: "profile", name: "Profile", screens: 2, versions: 1, linksOut: ["home"] },
  { id: "p5", key: "support", name: "Support", screens: 2, versions: 0, validationErrors: 2, linksOut: ["profile"] },
];
const only = process.env.SHOTS ?? "";

for (const scheme of ["light", "dark"] as const) {
  test(`home ${scheme}`, async ({ browser }) => {
    test.skip(only !== "" && !only.includes("home"));
    const ctx = await browser.newContext({ colorScheme: scheme, viewport: { width: 1440, height: 900 } });
    const p = await ctx.newPage();
    p.on("pageerror", (e) => console.log("PAGEERROR", e.message, (e.stack ?? "").split("\n").slice(0, 8).join(" | ")));
    await mockUpstream(p, { permissions, appsGrid: { apps, calls: [] }, stored: { theme: "system", language: "", density: "", onboardingDismissed: true } });
    await p.goto(`/o/${orgA}`);
    await p.waitForTimeout(2500);
    await p.screenshot({ path: `/tmp/claude-0/shots/r4-home-${scheme}.png` });
    await p.getByRole("heading", { name: "Apps" }).scrollIntoViewIfNeeded();
    await p.mouse.wheel(0, 500);
    await p.waitForTimeout(600);
    await p.getByRole("link", { name: /Plux Bank/ }).first().hover();
    await p.waitForTimeout(1200);
    await p.screenshot({ path: `/tmp/claude-0/shots/r4-hover-${scheme}.png` });
    await ctx.close();
  });
  test(`canvas ${scheme}`, async ({ browser }) => {
    test.skip(only !== "" && !only.includes("canvas"));
    const ctx = await browser.newContext({ colorScheme: scheme, viewport: { width: 1440, height: 900 } });
    const p = await ctx.newPage();
    p.on("pageerror", (e) => console.log("PAGEERROR", e.message, (e.stack ?? "").split("\n").slice(0, 8).join(" | ")));
    p.on("console", (m) => m.type() === "error" && console.log("CONSOLE", m.text().slice(0, 200)));
    await mockUpstream(p, { permissions, summaries, appsGrid: { apps, calls: [] }, stored: { theme: "system", language: "", density: "", onboardingDismissed: true } });
    await p.goto(`/o/${orgA}/apps/a1`);
    await p.waitForTimeout(5000);
    await p.screenshot({ path: `/tmp/claude-0/shots/r4-canvas-${scheme}.png` });
    // Select a box by clicking it in the plugins panel, then drag it on the canvas.
    await p.getByRole("option", { name: "Show Loans on the canvas" }).click();
    await p.waitForTimeout(800);
    await p.screenshot({ path: `/tmp/claude-0/shots/r4-canvas-${scheme}-selected.png` });
    // Loans is now in the middle of the view: drag it down and to the right.
    await p.mouse.move(720, 450);
    await p.mouse.down();
    await p.mouse.move(870, 560, { steps: 12 });
    await p.mouse.up();
    await p.waitForTimeout(600);
    await p.screenshot({ path: `/tmp/claude-0/shots/r4-canvas-${scheme}-dragged.png` });
    const zoomText = async () => (await p.getByRole("group", { name: "Zoom" }).innerText()).replace(/\s+/g, " ");
    console.log(scheme, "zoom before", await zoomText());
    await p.mouse.move(720, 450);
    await p.keyboard.down("Control");
    await p.mouse.wheel(0, -300);
    await p.keyboard.up("Control");
    await p.waitForTimeout(500);
    console.log(scheme, "zoom after ctrl+wheel", await zoomText(), "page zoom", await p.evaluate(() => window.visualViewport?.scale));
    await p.getByRole("button", { name: "Zoom to fit" }).click();
    await p.waitForTimeout(500);
    // Pan with the hand tool.
    await p.keyboard.press("h");
    await p.mouse.move(700, 700);
    await p.mouse.down();
    await p.mouse.move(800, 650, { steps: 8 });
    await p.mouse.up();
    await p.keyboard.press("v");
    await p.waitForTimeout(500);
    await p.screenshot({ path: `/tmp/claude-0/shots/r4-canvas-${scheme}-panned.png` });
    await ctx.close();
  });
}
test("refusals", async ({ browser }) => {
  test.skip(only !== "" && !only.includes("refus"));
  const ctx = await browser.newContext({ colorScheme: "light", viewport: { width: 1440, height: 900 } });
  const p = await ctx.newPage();
  await mockUpstream(p, { permissions, appsGrid: { apps, calls: [] }, admin: { calls: [], refused: { listSessions: [7, "PLX-6200: step-up"], listScimTokens: [7, "PLX-8600: SCIM is not licensed"] } }, stored: { theme: "system", language: "", density: "", onboardingDismissed: true } });
  for (const page of ["sessions", "sso"]) {
    await p.goto(`/o/${orgA}/admin/${page}`);
    await p.waitForTimeout(2000);
    await p.screenshot({ path: `/tmp/claude-0/shots/r4-${page}.png` });
  }
  await ctx.close();
});
