// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { formatter } from "@lingui/format-po";

/** Lingui configuration (ADR-0094): ICU MessageFormat, PO catalogues per locale, pseudo-locales generated at load. */
export default {
  sourceLocale: "en",
  locales: ["en", "de", "ar"],
  catalogs: [
    {
      path: "<rootDir>/apps/web/src/locales/{locale}/messages",
      include: ["<rootDir>/apps/web/src"],
    },
  ],
  format: formatter({ lineNumbers: false }),
};
