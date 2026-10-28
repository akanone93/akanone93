<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: AGPL-3.0-only
-->

# apps feature

Implements: STU-020, STU-021, STU-022, STU-023, STU-025, LIM-005 (Studio part). Route O2 (search `?view&q&team&tag&status&mine&sort`) is the organisation home, which embeds `AppsArea`.

Exports: `AppsArea`, `AppSettingsPage`, `appsSearchSchema`.

- `apps-area.tsx`: the grid and list of apps with search, filters, sort, pins and the card actions.
- `app-card.tsx`, `app-actions.tsx`, `card-visual.tsx`, `limits-usage.tsx`: the card (visual, content, action menu, limits meters).
- `apps-model.ts`: pure filtering, ordering, health and limit levels.
- `use-apps.ts`: summaries in batches of `app.summariesPerCall`, archived apps, "my apps", pins (favourite apps in the preferences).
- `app-settings-page.tsx`: name and description, limits usage table, links to the host fonts and the administration.

The archived apps carry no summary (the server's summaries list active apps only), so their card shows the app and its badge only. The summary has no publisher, so a card shows when the last publish was but not by whom.
