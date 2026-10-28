<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Experiments and Flags

An experiment varies part of an app for a share of devices and measures which variant wins. A
feature flag is a typed value with a default and values per segment. The model, the statistics
and the command line are in [Rollouts and experiments](../rollouts-and-experiments.md).

## Create an experiment

1. On the plugin canvas, select the widgets to vary and choose **Create experiment from
   selection**, or open **Experiments** and choose **New experiment**.
2. Walk the steps **Target**, **Variants**, **Allocation**, **Targeting**, **Metrics** and
   **Schedule**:
   - *Target*: the experiment key and its subject: a **Page**, a **Component subtree**, a **Prop
     value**, a **Flow** or a **Plugin version**.
   - *Variants*: exactly one **Control** and any number of others. *Allocation*: shares in percent
     that add up to 100, and a **Holdout in percent**.
   - *Targeting*: **Audience segments** and **Attribute conditions**.
   - *Metrics*: the primary metric event and its kind (**Share of subjects** or **Mean of a
     property**), the direction of **Improvement**, **Guardrail events**, and the **Subjects per
     variant before a result**.
3. **Create experiment**. Studio marks the fields to fix; a draft over the segment, condition or
   metric limits is refused ([limits](../../reference/limits.md)).

## Run it and read the result

An experiment is *Draft*, *Running*, *Stopped* or *Concluded*. **Start experiment** and **Stop
experiment** change its state. **Results** shows, per variant, the subjects, share or mean, the
difference from control, the interval, the p-value and a verdict: *Improved*, *Worsened*,
*Inconclusive* or *Guardrail tripped*. If the observed split differs from the plan, Studio shows a
**sample ratio mismatch** warning: do not trust that result until you find the cause.

**Ship this variant** creates a publish job with your **Release label**, so the winner goes through
the same checks and approvals as any release ([publishing](publishing.md)).

## Feature flags

**Feature flags** lists the flags of the environment. A flag has a key, a **Type** (*Boolean*,
*Number*, *String* or *JSON*), a **Default value** and **Values per segment**: the first segment a
device belongs to decides, and devices in none get the default. **Save flag**, **Edit** or
**Delete**.
