// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The plugin canvas's pointer and keys end to end, in the real canvas (STU-050, STU-067): a plugin with fields in a
 * column, a sized box and a stack is selected, drilled into, reordered, resized, moved, deleted and undone; screens are
 * moved by their name and keep their place after a reload; the menus, the marquee, the tools and the zoom work.
 */
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { expect, type Page, test } from "@playwright/test";
import { orgA } from "../apps/web/src/test-upstream.ts";
import { mockUpstream } from "./upstream.ts";

const root = join(import.meta.dirname, "../../schema/testdata/documents/widgets");
const read = (rel: string) => JSON.parse(readFileSync(join(root, rel), "utf8"));
const P = "01c0c450-6c00-7000-8a99-0000000000";
const id = (n: number) => `${P}${String(n).padStart(2, "0")}`;
const text = (n: number, data: string) => ({ id: id(n), type: "Text", props: { data } });
const field = (n: number, label: string) => ({
  id: id(n),
  type: "TextField",
  props: { decoration: { border: { kind: "outline" }, labelText: label } },
});
const form = {
  schemaVersion: "1.0.0",
  kind: "page",
  pageKind: "screen",
  id: id(1),
  key: "form",
  route: "form",
  title: "Form",
  params: [],
  state: [],
  root: {
    id: id(2),
    type: "Scaffold",
    slots: {
      appBar: { id: id(3), type: "AppBar", slots: { title: text(4, "Form") } },
      body: {
        id: id(5),
        type: "Padding",
        props: { padding: { all: 16 } },
        slots: {
          child: {
            id: id(6),
            type: "Column",
            children: [
              field(10, "Name"),
              field(11, "Email"),
              field(12, "Phone"),
              {
                id: id(13),
                type: "SizedBox",
                props: { width: 120, height: 60 },
                slots: { child: { id: id(14), type: "ColoredBox", props: { color: "#FF8A65" } } },
              },
              {
                id: id(15),
                type: "SizedBox",
                props: { width: 300, height: 160 },
                slots: {
                  child: {
                    id: id(16),
                    type: "Stack",
                    children: [
                      {
                        id: id(17),
                        type: "Positioned",
                        props: { left: 0, top: 0, right: 0, bottom: 0 },
                        slots: { child: { id: id(20), type: "ColoredBox", props: { color: "#BBDEFB" } } },
                      },
                      {
                        id: id(18),
                        type: "Positioned",
                        props: { left: 10, top: 10 },
                        slots: {
                          child: { id: id(19), type: "Container", props: { width: 80, height: 40, color: "#1E88E5" } },
                        },
                      },
                    ],
                  },
                },
              },
            ],
          },
        },
      },
    },
  },
};
const second = {
  schemaVersion: "1.0.0",
  kind: "page",
  pageKind: "screen",
  id: id(30),
  key: "second",
  route: "second",
  title: "Second",
  params: [],
  state: [],
  root: {
    id: id(31),
    type: "Scaffold",
    slots: { body: { id: id(32), type: "Center", slots: { child: text(33, "Second screen") } } },
  },
};
const plugin = read("plugins/gallery/plugin.json");
plugin.pages = [form.id, second.id];
plugin.entryPage = form.id;
plugin.name = "Audit";
const documents = [
  { kind: "app", path: "app.json", json: read("app.json") },
  { kind: "theme", path: "theme.json", json: read("theme.json") },
  { kind: "plugin", path: "plugins/gallery/plugin.json", json: plugin },
  { kind: "page", path: "plugins/gallery/pages/form.page.json", json: form },
  { kind: "page", path: "plugins/gallery/pages/second.page.json", json: second },
];
/** One check of the walk-through; every check runs, and every failure is reported. */
const check = (name: string, ok: boolean, detail = "") =>
  expect.soft(ok, `${name}${detail === "" ? "" : `: ${detail}`}`).toBe(true);

interface Box {
  readonly x: number;
  readonly y: number;
  readonly w: number;
  readonly h: number;
}

const near = (a: number, b: number, d = 2) => Math.abs(a - b) < d;
const canvas = (p: Page) => p.locator("canvas[role=application]");

async function selectedText(p: Page): Promise<string> {
  const bar = (await p.locator(".plugin-topbar-end").textContent()) ?? "";
  return bar.match(/(Nothing selected|[^·]*? selected)/)?.[0]?.trim() ?? "-";
}

/** The overlay's box of the selection (or another kind), in page pixels. */
async function overlayBox(p: Page, kind = "selected"): Promise<Box> {
  const box = await p.evaluate((k) => {
    const el = document.querySelector<HTMLElement>(`.pc-outline[data-kind="${k}"]`);
    const c = document.querySelector("canvas[role=application]")?.getBoundingClientRect();
    if (!el || !c) {
      return null;
    }
    const v = (n: string) => Number.parseFloat(el.style.getPropertyValue(n));
    return { x: c.left + v("--x"), y: c.top + v("--y"), w: v("--w"), h: v("--h") };
  }, kind);
  if (box === null) {
    throw new Error(`no ${kind} outline`);
  }
  return box;
}

async function layerOrder(p: Page): Promise<string[]> {
  return p.evaluate(() =>
    [...document.querySelectorAll<HTMLElement>("[data-item-id]")].map((e) => e.dataset["itemId"] ?? ""),
  );
}

async function pickLayer(p: Page, nodeId: string): Promise<void> {
  await p.locator(`[data-item-id="${nodeId}"]`).first().click();
  await p.waitForTimeout(400);
}

async function drag(p: Page, from: { x: number; y: number }, to: { x: number; y: number }, drop = true) {
  await p.mouse.move(from.x, from.y);
  await p.mouse.down();
  await p.mouse.move((from.x + to.x) / 2, (from.y + to.y) / 2, { steps: 4 });
  await p.mouse.move(to.x, to.y, { steps: 4 });
  await p.waitForTimeout(300);
  if (drop) {
    await p.mouse.up();
    await p.waitForTimeout(1200);
  }
}

const centre = (b: Box) => ({ x: b.x + b.w / 2, y: b.y + b.h / 2 });

async function open(p: Page, errors: string[]): Promise<void> {
  p.on("pageerror", (e) => errors.push(e.message));
  await p.addInitScript(() => localStorage.setItem("plux.dock.end.folded", ""));
  await mockUpstream(p, {
    permissions: ["app.read", "app.manage", "plugin.read", "plugin.write", "release.publish"],
    summaries: [
      {
        id: "01c0c450-6c00-7000-8000-000000000010",
        key: "gallery",
        name: "Audit",
        screens: 2,
        versions: 1,
        linksOut: [],
      },
    ],
    documents,
    stored: { theme: "system", language: "", density: "", onboardingDismissed: true },
  });
  await p.goto(`/o/${orgA}/apps/01c0c450-6c00-7000-8000-000000000001/plugins/01c0c450-6c00-7000-8000-000000000010`);
  await showLayers(p);
}

async function showLayers(p: Page): Promise<void> {
  await expect(p.getByRole("tab", { name: "Layers" })).toBeVisible({ timeout: 30_000 });
  await p.waitForTimeout(3000);
  await p.getByRole("tab", { name: "Layers" }).click();
  await p.getByRole("button", { name: "Expand all" }).click();
  await p.waitForTimeout(500);
}

async function ctrlClick(p: Page, at: { x: number; y: number }): Promise<void> {
  await p.keyboard.down("Control");
  await p.mouse.click(at.x, at.y);
  await p.keyboard.up("Control");
  await p.waitForTimeout(300);
}

async function selecting(p: Page): Promise<void> {
  await pickLayer(p, id(10));
  check("the bar names the selection", (await selectedText(p)).startsWith("TextField"), await selectedText(p));
  const f1 = centre(await overlayBox(p));
  await p.keyboard.press("Escape");
  await p.keyboard.press("Escape");
  await p.keyboard.press("Escape");
  await p.mouse.click(f1.x, f1.y);
  await p.waitForTimeout(300);
  check("a click selects the top level", (await selectedText(p)).startsWith("Padding"), await selectedText(p));
  await p.mouse.dblclick(f1.x, f1.y);
  await p.waitForTimeout(300);
  check("a double-click drills in", (await selectedText(p)).startsWith("Column"), await selectedText(p));
  await p.mouse.dblclick(f1.x, f1.y);
  await p.waitForTimeout(300);
  check("and again", (await selectedText(p)).startsWith("TextField"), await selectedText(p));
  await p.keyboard.press("Escape");
  await p.waitForTimeout(200);
  check("Esc goes to the parent", (await selectedText(p)).startsWith("Column"), await selectedText(p));
  await ctrlClick(p, f1);
  check("Ctrl-click selects the innermost", (await selectedText(p)).startsWith("TextField"), await selectedText(p));
}

async function reordering(p: Page): Promise<void> {
  await pickLayer(p, id(10));
  const name = await overlayBox(p);
  // Phone is two fields below Name: picked on the canvas, so no panel pans the view between the measures.
  const phone = { x: name.x, y: name.y + 2 * name.h, w: name.w, h: name.h };
  await ctrlClick(p, centre(phone));
  await drag(p, centre(phone), { x: centre(name).x, y: name.y + 4 });
  const order = (await layerOrder(p)).filter((x) => [id(10), id(11), id(12)].includes(x));
  check("a drag reorders within the column", order.join() === [id(12), id(10), id(11)].join(), order.join());
}

async function resizing(p: Page): Promise<void> {
  await pickLayer(p, id(13));
  const before = await overlayBox(p);
  await drag(
    p,
    { x: before.x + before.w, y: before.y + before.h },
    { x: before.x + before.w + 30, y: before.y + before.h + 20 },
  );
  const after = await overlayBox(p);
  check(
    "a handle resizes",
    near(after.w, before.w + 30) && near(after.h, before.h + 20),
    `${before.w}x${before.h} -> ${after.w}x${after.h}`,
  );
  check("the size label", (await p.locator(".pc-size").textContent()) === "150 × 80");
}

async function movingInStack(p: Page): Promise<void> {
  await pickLayer(p, id(18));
  const before = await overlayBox(p);
  await drag(p, centre(before), { x: centre(before).x + 40, y: centre(before).y + 30 });
  const after = await overlayBox(p);
  check(
    "a stack child moves by its offset",
    near(after.x - before.x, 40) && near(after.y - before.y, 30),
    `${after.x - before.x},${after.y - before.y}`,
  );
  await canvas(p).focus();
  for (let i = 0; i < 5; i++) {
    await p.keyboard.press("ArrowRight");
  }
  await p.keyboard.press("Shift+ArrowDown");
  await p.waitForTimeout(800);
  const nudged = await overlayBox(p);
  check("the arrows nudge it by 1 and Shift by 10", near(nudged.x - after.x, 5) && near(nudged.y - after.y, 10));
}

async function deleting(p: Page): Promise<void> {
  await pickLayer(p, id(11));
  const field = centre(await overlayBox(p));
  await p.mouse.click(field.x, field.y, { button: "right" });
  await p.getByRole("menuitem", { name: "Delete" }).click();
  await p.waitForTimeout(1000);
  check(
    "deleting from the menu with Layers open does not crash",
    (await p.getByText("Something went wrong").count()) === 0,
  );
  check("the deleted row leaves the tree", !(await layerOrder(p)).includes(id(11)));
  await canvas(p).focus();
  await p.keyboard.press("Control+z");
  await p.waitForTimeout(1000);
  check("undo brings it back", (await layerOrder(p)).includes(id(11)));
  await pickLayer(p, id(11));
  await canvas(p).focus();
  await p.keyboard.press("Delete");
  await p.waitForTimeout(800);
  check("the Delete key deletes", !(await layerOrder(p)).includes(id(11)));
  await p.keyboard.press("Control+z");
  await p.waitForTimeout(800);
}

/** Moves the second screen by its name; returns how far it moved. */
async function movingScreens(p: Page): Promise<void> {
  await pickLayer(p, id(31));
  await p.mouse.move(800, 500);
  await p.mouse.wheel(0, -200);
  await p.waitForTimeout(600);
  const before = await overlayBox(p, "screen");
  await drag(p, { x: before.x + 60, y: before.y - 20 }, { x: before.x + 180, y: before.y + 70 });
  const after = await overlayBox(p, "screen");
  check("a screen moves by its name", near(after.x - before.x, 120, 8) && near(after.y - before.y, 90, 8));
  await p.waitForTimeout(2500);
  await p.reload();
  await showLayers(p);
  await pickLayer(p, id(2));
  await p.keyboard.down("Shift");
  await pickLayer(p, id(31));
  await p.keyboard.up("Shift");
  const frames = await p.evaluate(() =>
    [...document.querySelectorAll<HTMLElement>('.pc-outline[data-kind="screen"]')].map((el) => ({
      x: Number.parseFloat(el.style.getPropertyValue("--x")),
      y: Number.parseFloat(el.style.getPropertyValue("--y")),
      w: Number.parseFloat(el.style.getPropertyValue("--w")),
    })),
  );
  const [a, b] = [...frames].sort((u, v) => u.x - v.x);
  const z = (a?.w ?? 390) / 390;
  const off = { x: ((b?.x ?? 0) - (a?.x ?? 0)) / z, y: ((b?.y ?? 0) - (a?.y ?? 0)) / z };
  check("its place survives a reload", near(off.x, 550 + 120, 4) && near(off.y, 90, 4), JSON.stringify(off));
}

async function viewing(p: Page): Promise<void> {
  await p.keyboard.press("Escape");
  await p.keyboard.press("Escape");
  await p.getByRole("button", { name: "Zoom to fit" }).click();
  await p.waitForTimeout(600);
  await p.mouse.move(800, 500);
  await p.keyboard.down("Control");
  await p.mouse.wheel(0, 250);
  await p.keyboard.up("Control");
  await p.waitForTimeout(800);
  // From the empty canvas above the console, left of the inspector, to the top left: both screens are inside.
  await drag(p, { x: 1180, y: 925 }, { x: 400, y: 75 });
  check("a marquee selects both screens", (await selectedText(p)).startsWith("2 "), await selectedText(p));
  const before = p.url();
  await canvas(p).focus();
  await p.keyboard.press("h");
  await drag(p, { x: 700, y: 500 }, { x: 600, y: 400 });
  check("the hand tool pans", p.url() !== before);
  await p.keyboard.press("v");
}

async function inserting(p: Page): Promise<void> {
  await pickLayer(p, id(10));
  const target = await overlayBox(p);
  const c = await canvas(p).boundingBox();
  const rows = (await layerOrder(p)).length;
  await p.getByRole("tab", { name: "Insert" }).click();
  await p
    .locator('[draggable="true"][aria-label="Insert Text"]')
    .first()
    .dragTo(canvas(p), {
      targetPosition: { x: target.x - (c?.x ?? 0) + target.w / 2, y: target.y - (c?.y ?? 0) + target.h - 2 },
    });
  await p.waitForTimeout(1200);
  await p.getByRole("tab", { name: "Layers" }).click();
  await p.waitForTimeout(500);
  check("a widget dragged from Insert shows in Layers at once", (await layerOrder(p)).length === rows + 1);
}

for (const scheme of ["light", "dark"] as const) {
  test(`[STU-050] [STU-067] select, drill, reorder, resize, move, delete, undo and move screens (${scheme})`, async ({
    browser,
  }) => {
    test.setTimeout(300_000);
    const ctx = await browser.newContext({ colorScheme: scheme, viewport: { width: 1600, height: 1000 } });
    const p = await ctx.newPage();
    const errors: string[] = [];
    await open(p, errors);
    await selecting(p);
    await reordering(p);
    await resizing(p);
    await movingInStack(p);
    await deleting(p);
    await movingScreens(p);
    await viewing(p);
    await inserting(p);
    check("no page errors", errors.length === 0, errors.slice(0, 3).join(" | "));
    await ctx.close();
  });
}

async function menus(p: Page): Promise<void> {
  await pickLayer(p, id(10));
  const name = await overlayBox(p);
  await p.keyboard.press("Escape");
  await p.keyboard.press("Escape");
  await p.keyboard.press("Escape");
  await p.mouse.move(name.x + 30, name.y + 20);
  await p.waitForTimeout(300);
  check("hover outlines what a click selects", (await p.locator('.pc-outline[data-kind="hover"]').count()) === 1);
  await p.mouse.click(1150, 900, { button: "right" });
  await p.waitForTimeout(400);
  const items = await p.getByRole("menuitem").allTextContents();
  check("the empty canvas's menu", items.includes("New screen") && items.includes("Fit all screens"), items.join("|"));
  await p.keyboard.press("Escape");
}

async function structure(p: Page): Promise<void> {
  await pickLayer(p, id(11));
  const field = centre(await overlayBox(p));
  await p.mouse.click(field.x, field.y, { button: "right" });
  await p.getByRole("menuitem", { name: "Wrap selection in a column" }).click();
  await p.waitForTimeout(800);
  const order = await layerOrder(p);
  const before = order[order.indexOf(id(11)) - 1] ?? "";
  check("wrap puts a new column around the field", before !== id(10) && !before.startsWith(P), before);
  await canvas(p).focus();
  await p.keyboard.press("Control+z");
  await p.waitForTimeout(600);
  const rows = (await layerOrder(p)).length;
  await pickLayer(p, id(11));
  const again = centre(await overlayBox(p));
  await p.mouse.click(again.x, again.y, { button: "right" });
  await p.getByRole("menuitem", { name: "Copy" }).click();
  await p.waitForTimeout(300);
  await p.mouse.click(again.x, again.y, { button: "right" });
  await p.getByRole("menuitem", { name: "Paste" }).click();
  await p.waitForTimeout(1200);
  check("copy and paste add a widget", (await layerOrder(p)).length === rows + 1);
}

async function screenActions(p: Page): Promise<void> {
  await pickLayer(p, id(31));
  const s0 = await overlayBox(p, "screen");
  await p.mouse.click(s0.x + 40, s0.y - 20, { button: "right" });
  await p.waitForTimeout(300);
  const items = await p.getByRole("menuitem").allTextContents();
  check("a screen's menu", items.some((t) => t.includes("Rename")) && items.some((t) => t.includes("Delete screen")));
  await p.keyboard.press("Escape");
  await p.mouse.dblclick(s0.x + 40, s0.y - 20);
  await p.waitForTimeout(500);
  check("a double-click on a screen's name opens Rename", (await p.getByRole("dialog").count()) === 1);
  await p.keyboard.press("Escape");
  await p.waitForTimeout(300);
  await canvas(p).focus();
  await p.keyboard.press("Delete");
  await p.waitForTimeout(500);
  check(
    "Delete on a screen asks first",
    (await p.getByRole("alertdialog").count()) + (await p.getByRole("dialog").count()) >= 1,
  );
  await p.keyboard.press("Escape");
  await pickLayer(p, id(31));
  const s1 = await overlayBox(p, "screen");
  await drag(p, { x: s1.x + 40, y: s1.y - 20 }, { x: s1.x + 140, y: s1.y + 30 });
  const moved = await overlayBox(p, "screen");
  await canvas(p).focus();
  await p.keyboard.press("Control+z");
  await p.waitForTimeout(1200);
  const back = await overlayBox(p, "screen");
  check(
    "undo puts a moved screen back",
    near(moved.x - s1.x, 100, 3) && near(back.x, s1.x, 3),
    `${s1.x} ${moved.x} ${back.x}`,
  );
}

async function rightToLeft(p: Page): Promise<void> {
  await p.getByRole("button", { name: "Preview" }).click();
  await p.getByRole("combobox", { name: "Locale" }).click();
  await p
    .getByRole("option", { name: /ar|Arabic|العربية/ })
    .first()
    .click();
  await p.keyboard.press("Escape");
  await p.waitForTimeout(2500);
  check("Studio's panels keep their side", (await p.locator(".plugin-canvas").getAttribute("dir")) === null);
  await pickLayer(p, id(10));
  check("the selection is outlined in a mirrored screen", (await overlayBox(p)).w > 100);
  const before = p.url();
  await p.keyboard.down(" ");
  await drag(p, { x: 900, y: 600 }, { x: 800, y: 500 });
  await p.keyboard.up(" ");
  check("Space-drag pans", p.url() !== before);
}

test("[STU-067] hover, menus, nudge, wrap, copy and paste, screen actions, undo, right to left and Space", async ({
  browser,
}) => {
  test.setTimeout(300_000);
  const ctx = await browser.newContext({
    viewport: { width: 1600, height: 1000 },
    permissions: ["clipboard-read", "clipboard-write"],
  });
  const p = await ctx.newPage();
  const errors: string[] = [];
  await open(p, errors);
  await menus(p);
  await structure(p);
  await screenActions(p);
  await rightToLeft(p);
  check("no page errors", errors.length === 0, errors.slice(0, 3).join(" | "));
  await ctx.close();
});
