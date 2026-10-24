// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { CaseDefinition } from "@plux/ui";
import type { ReactElement } from "react";

const pageTitle = "Catalogue";

/** Every `*.cases.tsx` of the UI package, discovered at build time. This module is reachable only in development. */
const modules = import.meta.glob<{ cases: readonly CaseDefinition[] }>("../../../packages/ui/src/*.cases.tsx", {
  eager: true,
});

const sections = Object.entries(modules)
  .map(([path, mod]) => ({ name: path.slice(path.lastIndexOf("/") + 1, -".cases.tsx".length), cases: mod.cases }))
  .sort((a, b) => a.name.localeCompare(b.name));

/** Sections whose cases open an overlay, which makes the rest of the page inert; they render alone with `?only=<name>`. */
const overlaySections: ReadonlySet<string> = new Set(["dialog", "menu", "popover"]);

/** Development-only page that renders every component state; Playwright runs axe on it (STU-011). */
export function Catalogue(): ReactElement {
  const only = new URLSearchParams(window.location.search).get("only");
  const shown = sections.filter((s) => (only === null ? !overlaySections.has(s.name) : s.name === only));
  return (
    <main className="mx-auto flex max-w-dialog-lg flex-col gap-6 p-6">
      <h1 className="font-serif text-title">{pageTitle}</h1>
      {shown.map((section) => (
        <section key={section.name} aria-labelledby={`c-${section.name}`} className="flex flex-col gap-3">
          <h2 id={`c-${section.name}`} className="font-serif text-section">
            {section.name}
          </h2>
          {section.cases.map((c) => (
            <div key={c.name} data-case={`${section.name}/${c.name}`} className="flex flex-col gap-1">
              <h3 className="text-caption text-secondary">{c.name}</h3>
              <div>{c.element}</div>
            </div>
          ))}
        </section>
      ))}
    </main>
  );
}

/** Applies `?theme=light|dark|contrast`, `?dir=rtl` and `?lang=` so the e2e runs can cover every theme and direction. */
export function applyCatalogueParams(search: string, root: HTMLElement): void {
  const params = new URLSearchParams(search);
  const theme = params.get("theme");
  if (theme !== null) {
    root.setAttribute("data-theme", theme);
  }
  const lang = params.get("lang");
  if (lang !== null) {
    root.setAttribute("lang", lang);
  }
  root.setAttribute("dir", params.get("dir") === "rtl" ? "rtl" : "ltr");
}
