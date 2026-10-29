// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/** Hands bytes to the browser as a file download; the object URL is revoked as soon as the click has been made. */
export function download(name: string, bytes: Uint8Array<ArrayBuffer>, type: string): void {
  const url = URL.createObjectURL(new Blob([bytes], { type }));
  const link = document.createElement("a");
  link.href = url;
  link.download = name;
  link.click();
  URL.revokeObjectURL(url);
}
