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

createRoot(container).render(
  <StrictMode>
    <I18nProvider studio={createAppI18n()}>
      <App />
    </I18nProvider>
  </StrictMode>,
);
