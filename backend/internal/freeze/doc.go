// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Package freeze stores an organisation's change-freeze windows and refuses
// production changes made inside one (GOV-025, ADR-0076).
//
// The windows are data the core owns: calendar entries with a time zone, a
// wall-clock start, a duration and an optional recurrence, per organisation
// and environment. What a window means, including recurrence and daylight-
// saving time, is decided by a governance.FreezeCalendar, which the
// enterprise edition supplies; the community calendar accepts no window and
// holds nothing back. Both writing a window and being held back by one need
// the licence's FeatureChangeFreezes: a lapsed licence keeps the windows
// readable and in force until it expires, as GOV-030 asks, and a community
// installation neither stores nor enforces any.
//
// The check is made where a production change is made. The approval engine
// asks it for every production artifact it is given, which covers promotion,
// function deployment and the other guarded changes; the kill switch and the
// rollback ask it themselves. Break-glass is the one way through: its callers
// do not ask (GOV-024).
//
// Invariants:
//   - Only production changes are ever held; the caller says whether a
//     change is one.
//   - A calendar that cannot decide fails the change; a freeze is never
//     silently lifted.
//   - Every window written or removed is audited.
//
// Specification: GOV-025, GOV-030, LIM-001; ADR-0076.
package freeze
