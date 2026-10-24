<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: AGPL-3.0-only
-->

# auth feature

Implements: STU-013, STU-012 (P1 sign-in, P4 invitation).

Exports: `SignInPage`, `InvitePage`.

- `sign-in-page.tsx`: password, TOTP/passkey step, provider buttons.
- `invite-page.tsx`: accept an invitation.
- `auth.schema.ts`: Zod form schemas. `auth.messages.ts`: strings. `webauthn.ts`: passkey assertion.
