<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: AGPL-3.0-only
-->

# auth feature

Implements: STU-013, STU-012 (P1 sign-in, P4 invitation), CLI-002 (device sign-in page).

Exports: `SignInPage`, `InvitePage`, `DeviceLoginPage` (CLI-002).

- `sign-in-page.tsx`: password, TOTP/passkey step, provider buttons.
- `invite-page.tsx`: accept an invitation.
- `device-login-page.tsx`: `/device`, approve or deny `plux login`'s code: never on arrival, only after the person
  confirms they started it; the server requires a second factor in the session.
- `auth.schema.ts`: Zod form schemas. `auth.messages.ts`: strings. `webauthn.ts`: passkey assertion and registration.
