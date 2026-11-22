// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The app the navigation is about (#202, #208). One rule decides it everywhere, so the app's areas cannot appear on one
 * page and vanish on the next:
 *
 * 1. the app in the page's address (`/o/:org/apps/:app/…`, the canvases included);
 * 2. otherwise the app an organisation-wide page is filtered on (`?app=`, the approvals pages);
 * 3. otherwise the app last opened in this organisation, remembered in this browser;
 * 4. otherwise the organisation's first app.
 *
 * Steps 2 to 4 only ever choose an app of `readable`, the apps the server listed for this person, so a remembered or
 * edited value can name nothing else. Choosing an app here grants nothing: the route guards and the server still check
 * every page and call.
 */
export function chooseCurrentApp({
  inPath,
  inSearch,
  remembered,
  readable,
}: {
  readonly inPath: string | undefined;
  readonly inSearch: string | undefined;
  readonly remembered: string | undefined;
  readonly readable: readonly string[];
}): string | undefined {
  if (inPath !== undefined && inPath !== "") {
    return inPath;
  }
  for (const candidate of [inSearch, remembered]) {
    if (candidate !== undefined && readable.includes(candidate)) {
      return candidate;
    }
  }
  return readable[0];
}

/** The key of the remembered app of one organisation; per organisation, so switching keeps each one's choice. */
function storageKey(organisationId: string): string {
  return `plux.studio.app.${organisationId}`;
}

/** The longest value read back: an app id is a UUID; anything longer was not written here. */
const MAX_REMEMBERED = 64;

/**
 * The app last opened in `organisationId` in this browser, or undefined. Storage can be missing or refuse (private
 * windows, blocked site data); the navigation then falls back to the organisation's first app.
 */
export function rememberedApp(organisationId: string): string | undefined {
  try {
    const value = globalThis.localStorage?.getItem(storageKey(organisationId)) ?? undefined;
    return value !== undefined && value !== "" && value.length <= MAX_REMEMBERED ? value : undefined;
  } catch {
    return undefined;
  }
}

/** Remembers `appId` as the app last opened in `organisationId`; a refusal of the storage is not an error. */
export function rememberApp(organisationId: string, appId: string): void {
  if (appId === "" || appId.length > MAX_REMEMBERED) {
    return;
  }
  try {
    globalThis.localStorage?.setItem(storageKey(organisationId), appId);
  } catch {
    // Remembering is a convenience; without storage the first app is chosen.
  }
}
