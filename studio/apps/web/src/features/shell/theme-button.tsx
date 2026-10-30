// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Icon } from "@plux/ui";
import { type ReactElement, useSyncExternalStore } from "react";
import { isDark } from "../../lib/color-scheme.ts";
import { useCommands } from "../../lib/commands/index.ts";
import { chromeMessages } from "./shell-chrome.messages.ts";

/** Calls `change` whenever the document's theme attribute changes. */
function subscribe(change: () => void): () => void {
  const observer = new MutationObserver(change);
  observer.observe(document.documentElement, { attributes: true, attributeFilter: ["data-theme"] });
  return () => observer.disconnect();
}

/**
 * The top bar's light and dark switch (STU-014): it runs the theme command, so the choice is stored like the profile's,
 * and shows the theme it switches to.
 */
export function ThemeButton(): ReactElement | null {
  const m = chromeMessages(useLingui().i18n);
  const dark = useSyncExternalStore(subscribe, isDark, () => false);
  const toggle = useCommands().states.find((s) => s.command.id === "theme.toggle")?.command;
  if (toggle === undefined) {
    return null;
  }
  return (
    <button
      type="button"
      className="shell-icon"
      aria-label={dark ? m.themeToLight : m.themeToDark}
      onClick={() => void toggle.run()}
    >
      <Icon name={dark ? "sun" : "moon"} size="capsule" />
    </button>
  );
}
