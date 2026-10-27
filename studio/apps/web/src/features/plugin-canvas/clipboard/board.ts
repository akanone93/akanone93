// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { CLIPBOARD_MIME } from "./envelope.ts";

/** The custom type the async Clipboard API needs for a web custom format (F7). */
export const CLIPBOARD_WEB_TYPE = `web ${CLIPBOARD_MIME}`;
const TEXT_TYPE = "text/plain";

/** The part of the async Clipboard API the clipboard uses; the item methods are absent in older browsers. */
export interface Board {
  readonly writeText: (text: string) => Promise<void>;
  readonly readText: () => Promise<string>;
  readonly write?: (items: ClipboardItem[]) => Promise<void>;
  readonly read?: () => Promise<ClipboardItems>;
}

/**
 * Writes the envelope as a ClipboardItem under the custom type plus the text/plain JSON fallback. Where
 * ClipboardItem is unavailable, or the browser refuses the custom type, it writes plain text.
 */
export async function writeEnvelope(board: Board, json: string): Promise<void> {
  if (board.write !== undefined && typeof ClipboardItem !== "undefined") {
    try {
      await board.write([
        new ClipboardItem({
          [CLIPBOARD_WEB_TYPE]: new Blob([json], { type: CLIPBOARD_WEB_TYPE }),
          [TEXT_TYPE]: new Blob([json], { type: TEXT_TYPE }),
        }),
      ]);
      return;
    } catch {
      // The browser refused the custom type: the text fallback below still serves.
    }
  }
  await board.writeText(json);
}

async function itemText(items: ClipboardItems, type: string): Promise<string | undefined> {
  for (const item of items) {
    if (item.types.includes(type)) {
      return (await item.getType(type)).text();
    }
  }
  return undefined;
}

/** Reads the clipboard text, preferring the custom type and falling back to text/plain, then to readText. */
export async function readEnvelopeText(board: Board): Promise<string> {
  if (board.read !== undefined) {
    try {
      const items = await board.read();
      const text = (await itemText(items, CLIPBOARD_WEB_TYPE)) ?? (await itemText(items, TEXT_TYPE));
      if (text !== undefined) {
        return text;
      }
    } catch {
      // Reading items was refused: readText below still serves.
    }
  }
  return board.readText();
}
