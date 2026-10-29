// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Package replay stores structural session replays (ANL-050, ADR-0101): the
// screens a device opened, the layout of their nodes, which node was
// touched and which non-sensitive state entries changed. It never trusts the
// device: consent, the security profile, the replay.* limits and the
// redaction rules of SCH-012 are checked again on every upload, and what a
// replay may not hold is dropped before it is stored.
package replay
