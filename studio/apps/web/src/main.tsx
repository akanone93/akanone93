// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { App } from "./app.tsx";
import { createAppI18n, I18nProvider } from "./i18n-provider.tsx";
import "./styles/app.css";

const container = document.getElementById("root");
if (container === null) {
  throw new Error("index.html has no #root element");
}

/** Development-only route: `import.meta.env.DEV` is false in production builds, so the catalogue code is dropped. */
const catalogueRoute = "/__catalogue";

async function bootCatalogue(root: HTMLElement): Promise<void> {
  const { Catalogue, applyCatalogueParams } = await import("./catalogue.tsx");
  applyCatalogueParams(window.location.search, document.documentElement);
  createRoot(root).render(
    <StrictMode>
      <Catalogue />
    </StrictMode>,
  );
}

if (import.meta.env.DEV && window.location.pathname === catalogueRoute) {
  void bootCatalogue(container);
} else {
  createRoot(container).render(
    <StrictMode>
      <I18nProvider studio={createAppI18n()}>
        <App />
      </I18nProvider>
    </StrictMode>,
  );
}
