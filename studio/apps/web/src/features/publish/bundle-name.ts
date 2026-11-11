// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/** What the publish steps call a bundle: a plugin by its key, the app bundle (key "") by its name (STU-100). */
export function bundleName(pluginKey: string, appBundle: string): string {
  return pluginKey === "" ? appBundle : pluginKey;
}
