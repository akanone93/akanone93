<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Plux Test

**Plux Test** lists the declarative test scenarios of an app and the runs that `plux test` and CI
report. Scenarios run headlessly on the real runtime; how they are written and run is in the
[testing guide](../testing.md).

## Write a scenario

1. Open **Plux Test** and choose **New scenario** on the **Scenarios** tab.
2. Give it a **File** name (lower-case letters, digits and hyphens), a **Name** and the **Page
   route** it tests.
3. **Add step**: **Tap** or **Enter text** on a widget by its **Test ID**, **Wait for** a widget, or
   **Scroll**. Every step needs a valid test ID, and entered text needs the text.
4. **Add expectation**: **Is visible**, **Is not visible**, **Text equals** or **Navigated to**. A
   scenario needs at least one complete expectation.
5. **Save scenario**. You can read scenarios without permission to change them; **Remove** deletes
   one.

**Create test from device session** starts a scenario from a device session.

## Read the runs

**Runs** lists each reported run with its label, source, result, time and duration. Open one for
the results per scenario and step, with the message of a failed step; **Passed, failed and skipped
scenarios** gives the totals. A run appears when it is reported:

```bash
plux test --report --app <app-id> --label "pull request 123"
```

`--report` records the run with your credential and needs the permission `plugin.edit`; see
[the CLI reference](../../reference/cli.md). Golden image comparisons are separate
([golden tests](../golden-tests.md)).
