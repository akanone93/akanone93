<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Analytics and Replay

**Analytics** shows how an app is used and where it fails, from the events devices send. What
devices record, and what they never do, is in the [analytics guide](../analytics.md); the
[personal data](../../operations/personal-data.md) page lists where each field lives. Pick the
environment and a date range (**From**, **To**) first.

## Overview and usage

**Overview** shows sessions, screen views, errors, **Firing alerts** and screen views per day.
**Usage** breaks screen views down by plugin, then by page: choose a plugin, then a page, to see
that page's views per day.

## Funnels

A funnel counts the sessions that reach each step, doing the steps in order. **Create funnel** with a
name and the steps, one event name per line. The funnel page shows sessions per step and the share
of the first step. **Delete** removes it.

## Errors

**Errors** groups errors and crashes by fingerprint, newest first, with the code, route, affected
devices, last seen and a trend; **Fatal** marks crashes. Open a group for the occurrences. A
**Symbolicated path** takes you to the **Node** and **Document path** in the plugin, and **Open
node** opens it on the canvas, so you go from a crash to the widget that caused it.

## Performance

**Performance** shows render-time percentiles per day, in milliseconds.

## Replay a session

**Replays** lists recorded sessions with the start, screen, duration and event count. A replay is
structural: it shows the screens, the interactions and the state as the session went, not pixels, and
values that were not recorded show as **Hidden**. Open one, press **Play**, move on the **Timeline**, and read
**State so far** next to the **Replay canvas**. A replay that reached a limit is marked **Cut at a
limit**. Nothing is listed when the app did not record replays in that environment.

Delays in the pipeline: [Telemetry backlog](../../runbooks/telemetry-backlog.md).
