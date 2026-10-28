<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Apps

The organisation home lists your apps as cards. **Grid** reflows from one column on a narrow screen
to four on a wide one; **List** shows the same facts as rows. The choice, the search and every
filter are part of the address, so you can bookmark or share a view.

## Read a card

Each card shows a picture, the name, the app's key, the description and the owning team. The
picture is a live screenshot of one of the app's screens once a plugin canvas has been opened, else
the app's uploaded image, else the first letter of the name on a colour that stays with the app.
Below are one badge per environment with its live release (**production release 3**, or **staging
not live**), the date of the last publish, the **Active devices** of the last seven days, the number
of **Plugins**, and a health badge: **Healthy**, **Degraded** or **Unhealthy** with the crash-free
share, and whether errors are rising or falling against the week before. **No devices** means no
device was active, so there is no rate. The meters at the foot show the app's **Limits usage**
against the limit in force; a badge appears from 80 percent (**Near limit**) and at the limit
(**At limit**).

## Find apps

- **Search apps** matches the name, key, description and tags.
- **Team** and **Tag** keep the apps of one team or carrying one tag.
- **Active**, **Archived** and **All** choose which apps are listed; archived apps carry an
  **Archived** badge and no statistics.
- **My apps** keeps the apps you hold a grant on, directly or through a team.
- **Sort by** orders by **Name**, **Last activity** (newest publish first) or **Health** (lowest
  crash-free share first).
- **Pin {app}** moves an app to the top of every order; the pins are saved with your preferences.

## Card actions

The **Actions for {app}** menu offers **Open**, **Settings**, **Duplicate** (a new name and key; the
copy opens at once), **Export .plux archive**, and **Archive** or **Unarchive**. An action your
permissions do not allow is shown dimmed: duplicating and archiving need the permission to manage
apps, and exporting needs read access to the app's plugins.

## App settings

**Settings** in the app navigation opens the app's settings. **Name** and **Description** can be
changed by people who manage the app. **Limits usage** lists every limit of the app with what it
uses, the limit in force and a meter. **Host fonts** links to the typography page, where the font
files of the host app are listed. People who may open the organisation's administration also find a
link to it, where the app's access and limits are set.
