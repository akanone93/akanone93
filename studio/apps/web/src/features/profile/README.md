<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: AGPL-3.0-only
-->

# profile feature

Implements: STU-014, STU-012 (U2).

Exports: `ProfilePage`, `useAppliedPreferences`.

- `profile-page.tsx`: theme, language, density through PreferencesService with an update mask.
- `use-applied-preferences.ts`: applies stored choices to the document.
