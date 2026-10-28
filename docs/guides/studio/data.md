<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Data

The **Data** area of an app holds the plugin's **data sources** (REST and GraphQL) and the
**collections** of its on-device database. The concepts are in the [data guide](../data.md) and
the [database guide](../database.md); this guide covers the screens.

## Review and import data sources

1. Open **Data** and choose the plugin and the environment. **API definitions** lists the plugin's
   data sources with their kind and type.
2. **Import an API definition** reads an OpenAPI document or a GraphQL schema (with its operations)
   and shows the sources it can add. **Apply** writes them to the plugin's draft; imported sources
   are not saved until you apply. A construct with no Plux type is reported and left out.
3. Open a source for its **Operations**, **Schemas** and **Base URL**. The base URL is an app
   variable; the page shows the **URL** each environment sets, and tells you when none does. Edit
   the variables in the app document.
4. Choose whether the source goes **Through the Plux gateway** (the default) or **Straight to the
   origin**, as the [gateway guide](../gateway.md) describes.

The command-line equivalents are `plux import openapi` and `plux import graphql`
([CLI reference](../../reference/cli.md)).

## Edit mocks

**Edit the mock** opens the mock of an operation as JSON. The canvas shows the mock while no real
data is available, and the **Data state** switch (*Success*, *Loading*, *Empty*, *Error*) previews
the others. **Use this mock** keeps your edit; invalid JSON is refused.

## Design collections

**Collections** lists the on-device database collections of the plugin. Open one to edit its
**Key**, **Primary key**, **Indexes** (one per line, fields separated by commas) and **Fields**,
and to see which fields are marked **sensitive**. **Migration plans** list the steps
from each earlier version; **Preview the migration** shows what a migration from the last release
does to its data before you publish.

Problems in a draft appear under **Problems**; the validator is the same one the server runs. A
draft that is read-only is held by another editor ([locks](canvas.md#undo-locks-and-conflicts)).
