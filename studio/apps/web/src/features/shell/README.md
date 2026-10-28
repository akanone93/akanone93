<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: AGPL-3.0-only
-->

# shell feature

Implements: STU-013, STU-012 (layouts, guards' pages, top bar).

Exports: layouts, error pages, `ShellLayout`.

- `layouts.tsx`: public, canvas (empty), page (empty).
- `shell-layout.tsx`: top bar, organisation switcher, profile menu.
- `create-organisation-dialog.tsx`: creates an organisation (installation administrators) from the top bar and the empty page, then switches to it.
- `app-nav.tsx`: navigation between the areas of the app in the URL, filtered by the permissions the route guards need.
- `error-pages.tsx`: 404, 403 with Explain access, licence page.
