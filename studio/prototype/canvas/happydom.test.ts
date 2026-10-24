// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Does Base UI run under happy-dom? Renders six components, opens each with
// user events and asserts the role and aria state. Both published names of
// Base UI are tried. The suite runs only from this directory (`bun test` here),
// so a `bun test` from the Studio workspace root skips it instead of
// registering happy-dom globals for the other tests.

import { afterAll, beforeAll, describe, expect, test } from "bun:test";

type BaseUi = typeof import("@base-ui/react");
type Rtl = typeof import("@testing-library/react");
type UserEvent = typeof import("@testing-library/user-event");
type React = typeof import("react");

const active = process.cwd() === import.meta.dir;
const suite = active ? describe : describe.skip;
const packages = ["@base-ui-components/react", "@base-ui/react"] as const;

let unregister: (() => Promise<void>) | undefined;

// happy-dom must exist before Testing Library is imported (it reads `document`
// and registers its own hooks at import time), so both happen at module level.
if (active) {
  const { GlobalRegistrator } = await import("@happy-dom/global-registrator");
  GlobalRegistrator.register({ url: "http://localhost:3000", width: 1280, height: 800 });
  (globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
  unregister = () => GlobalRegistrator.unregister();
}
const rtl = (active ? await import("@testing-library/react") : undefined) as Rtl;

afterAll(async () => {
  await unregister?.();
});

for (const name of packages) {
  suite(name, () => {
    let ui: BaseUi;
    let userEvent: UserEvent["default"];
    let h: React["createElement"];

    beforeAll(async () => {
      ui = (await import(name)) as BaseUi;
      userEvent = (await import("@testing-library/user-event")).default;
      h = (await import("react")).createElement;
    });

    const mount = (element: ReturnType<React["createElement"]>) => {
      rtl.render(element);
      return { user: userEvent.setup(), screen: rtl.screen };
    };

    test("Dialog opens, labels itself and closes on Escape", async () => {
      const { Dialog } = ui;
      const { user, screen } = mount(
        h(
          Dialog.Root,
          null,
          h(Dialog.Trigger, null, "Open dialog"),
          h(
            Dialog.Portal,
            null,
            h(Dialog.Backdrop),
            h(Dialog.Popup, null, h(Dialog.Title, null, "Settings"), h(Dialog.Close, null, "Close")),
          ),
        ),
      );
      const trigger = screen.getByRole("button", { name: "Open dialog" });
      expect(trigger.getAttribute("aria-expanded")).not.toBe("true");
      await user.click(trigger);
      const dialog = await screen.findByRole("dialog");
      expect(dialog.getAttribute("aria-labelledby")).toBe(screen.getByText("Settings").id);
      expect(trigger.getAttribute("aria-expanded")).toBe("true");
      await user.keyboard("{Escape}");
      await rtl.waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    });

    test("Popover opens as a dialog and the trigger reports expanded", async () => {
      const { Popover } = ui;
      const { user, screen } = mount(
        h(
          Popover.Root,
          null,
          h(Popover.Trigger, null, "Open popover"),
          h(
            Popover.Portal,
            null,
            h(Popover.Positioner, null, h(Popover.Popup, null, h(Popover.Title, null, "Details"), "Body")),
          ),
        ),
      );
      const trigger = screen.getByRole("button", { name: "Open popover" });
      await user.click(trigger);
      await screen.findByRole("dialog");
      expect(trigger.getAttribute("aria-expanded")).toBe("true");
      await user.keyboard("{Escape}");
      await rtl.waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    });

    test("Menu opens with menuitems and closes after a selection", async () => {
      const { Menu } = ui;
      const { user, screen } = mount(
        h(
          Menu.Root,
          null,
          h(Menu.Trigger, null, "Open menu"),
          h(
            Menu.Portal,
            null,
            h(
              Menu.Positioner,
              null,
              h(Menu.Popup, null, h(Menu.Item, null, "Copy"), h(Menu.Item, null, "Paste")),
            ),
          ),
        ),
      );
      const trigger = screen.getByRole("button", { name: "Open menu" });
      expect(trigger.getAttribute("aria-haspopup")).toBe("menu");
      await user.click(trigger);
      await screen.findByRole("menu");
      expect(screen.getAllByRole("menuitem").map((item) => item.textContent)).toEqual(["Copy", "Paste"]);
      expect(trigger.getAttribute("aria-expanded")).toBe("true");
      await user.click(screen.getByRole("menuitem", { name: "Paste" }));
      await rtl.waitFor(() => expect(screen.queryByRole("menu")).toBeNull());
    });

    test("Select opens a listbox and selecting an option updates the trigger", async () => {
      const { Select } = ui;
      const { user, screen } = mount(
        h(
          Select.Root,
          { defaultValue: "a" },
          h(Select.Trigger, null, h(Select.Value)),
          h(
            Select.Portal,
            null,
            h(
              Select.Positioner,
              null,
              h(
                Select.Popup,
                null,
                h(Select.Item, { value: "a" }, h(Select.ItemText, null, "Alpha")),
                h(Select.Item, { value: "b" }, h(Select.ItemText, null, "Beta")),
              ),
            ),
          ),
        ),
      );
      const trigger = screen.getByRole("combobox");
      await user.click(trigger);
      await screen.findByRole("listbox");
      const options = screen.getAllByRole("option");
      expect(options).toHaveLength(2);
      expect(options[0]?.getAttribute("aria-selected")).toBe("true");
      await user.click(screen.getByRole("option", { name: "Beta" }));
      await rtl.waitFor(() => expect(screen.queryByRole("listbox")).toBeNull());
      expect(trigger.textContent).toContain("b");
    });

    test("Tabs expose tablist, tab and tabpanel roles and switch panels", async () => {
      const { Tabs } = ui;
      const { user, screen } = mount(
        h(
          Tabs.Root,
          { defaultValue: "one" },
          h(Tabs.List, null, h(Tabs.Tab, { value: "one" }, "One"), h(Tabs.Tab, { value: "two" }, "Two")),
          h(Tabs.Panel, { value: "one" }, "First panel"),
          h(Tabs.Panel, { value: "two" }, "Second panel"),
        ),
      );
      expect(screen.getByRole("tablist")).toBeTruthy();
      const one = screen.getByRole("tab", { name: "One" });
      const two = screen.getByRole("tab", { name: "Two" });
      expect(one.getAttribute("aria-selected")).toBe("true");
      expect(two.getAttribute("aria-selected")).toBe("false");
      await user.click(two);
      await rtl.waitFor(() => expect(two.getAttribute("aria-selected")).toBe("true"));
      expect(one.getAttribute("aria-selected")).toBe("false");
      expect(screen.getByRole("tabpanel").textContent).toBe("Second panel");
    });

    test("Tooltip shows on hover with the tooltip role", async () => {
      const { Tooltip } = ui;
      const { user, screen } = mount(
        h(
          Tooltip.Provider,
          { delay: 0 },
          h(
            Tooltip.Root,
            null,
            h(Tooltip.Trigger, null, "Hover me"),
            h(Tooltip.Portal, null, h(Tooltip.Positioner, null, h(Tooltip.Popup, null, "Helpful text"))),
          ),
        ),
      );
      await user.hover(screen.getByRole("button", { name: "Hover me" }));
      const tip = await screen.findByText("Helpful text");
      expect(tip.getAttribute("role")).toBe("tooltip");
    });
  });
}
