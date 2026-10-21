<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

## Your task: P9 E5k — honour a kill switch's targeting rule on the device (`REL-060`, `SYN-060`)

`schema/control.fbs` `KillSwitch.rule` is an optional `TargetingRule` ("absent means every device of the channel"), but `packages/plux_flutter/lib/src/control/control_reader.dart` (around line 172–200) ignores it, so a targeted switch disables the thing on every device. The rule type is already read for flags and experiments (`lib/src/control/control_experiments_reader.dart`, `lib/src/experiments/targeting.dart`, facts from the device context).
Do only: read `KillSwitch.rule` with that reader and apply a switch only on devices it matches; an unreadable rule applies the switch to everyone (fail safe — say so in the doc comment). Keep the change inside `lib/src/control/` and the switch's application point (`live_control.dart`).
Tests `[REL-060]`: a targeted switch applies only on matching devices; absent rule applies everywhere; unreadable rule applies everywhere. Build documents with the generated builder like `test/control/control_experiments_test.dart`.
Done when: `cd packages/plux_flutter && flutter test test/control && dart analyze lib test` pass.
