<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: AGPL-3.0-only
-->

# profile feature

Implements: STU-014, STU-012 (U2), SEC-100 (security keys).

Exports: `ProfilePage`, `useAppliedPreferences`.

- `profile-page.tsx`: theme, language, density through PreferencesService with an update mask.
- `use-applied-preferences.ts`: applies stored choices to the document.
- `security-keys.tsx`: the person's security keys and passkeys: add with the WebAuthn ceremony, remove after a
  confirmation (SEC-100, SEC-103).
