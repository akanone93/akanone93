// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * An SVG asset as the canvas draws it (build log rows 25 and 39). A device draws an SVG from the `vector_graphics`
 * form the server compiles (CMP-031); CanvasKit draws no SVG, so Studio has the browser draw the stored file to a
 * PNG. The browser draws an SVG it loads as an image with no script and nothing fetched, so an uploaded file can do
 * nothing but draw.
 */

/** A size in logical pixels. */
export interface Size {
  readonly width: number;
  readonly height: number;
}

const SVG_TYPE = "image/svg+xml";

/** The root `<svg …>` tag of a document. */
const ROOT = /<svg\b[^>]*>/i;

/** A length in pixels as an attribute gives it: a number, or a number of `px`; anything else is no length. */
function pixels(value: string | undefined): number | null {
  const m = value === undefined ? null : /^\s*(\d+(?:\.\d+)?)\s*(px)?\s*$/.exec(value);
  const n = m === null ? Number.NaN : Number(m[1]);
  return Number.isFinite(n) && n > 0 ? n : null;
}

/** The value of one attribute of a tag. */
function attribute(tag: string, name: string): string | undefined {
  const m = new RegExp(`\\s${name}\\s*=\\s*(?:"([^"]*)"|'([^']*)')`, "i").exec(tag);
  return m === null ? undefined : (m[1] ?? m[2]);
}

/** A `viewBox`'s width and height. */
function viewBoxSize(value: string | undefined): Size | null {
  const parts =
    value
      ?.trim()
      .split(/[\s,]+/)
      .map(Number) ?? [];
  const [, , width = 0, height = 0] = parts;
  return parts.length === 4 && width > 0 && height > 0 ? { width, height } : null;
}

/**
 * The size an SVG declares: its `width` and `height` in pixels, either one taken from the `viewBox`'s proportion when
 * the other is missing, else the `viewBox`'s; null when it declares neither.
 */
export function svgSize(text: string): Size | null {
  const tag = ROOT.exec(text)?.[0];
  if (tag === undefined) {
    return null;
  }
  const box = viewBoxSize(attribute(tag, "viewBox"));
  const width = pixels(attribute(tag, "width"));
  const height = pixels(attribute(tag, "height"));
  if (width !== null && height !== null) {
    return { width, height };
  }
  if (box === null) {
    return null;
  }
  if (width !== null) {
    return { width, height: (width * box.height) / box.width };
  }
  if (height !== null) {
    return { width: (height * box.width) / box.height, height };
  }
  return box;
}

/** The longest side a raster takes, in pixels. */
const MIN_SIDE = 256;
const MAX_SIDE = 1024;

/**
 * The pixel size the canvas draws an SVG at: four times its declared size, so it stays sharp when the canvas is
 * zoomed in, with the longer side between 256 and 1024 pixels; a square of 256 for an SVG that declares no size.
 */
export function rasterSize(size: Size | null): Size {
  if (size === null) {
    return { width: MIN_SIDE, height: MIN_SIDE };
  }
  const longer = Math.max(size.width, size.height);
  const scale = Math.min(MAX_SIDE, Math.max(MIN_SIDE, longer * 4)) / longer;
  return { width: Math.max(1, Math.round(size.width * scale)), height: Math.max(1, Math.round(size.height * scale)) };
}

/**
 * The SVG with its root sized to `to` pixels and a `viewBox`, so its drawing scales to that size: an SVG with only a
 * `width` and `height` is given the `viewBox` of its declared size. Null when it has no root element.
 */
export function sizedSvg(text: string, declared: Size | null, to: Size): string | null {
  const tag = ROOT.exec(text)?.[0];
  if (tag === undefined) {
    return null;
  }
  let sized = tag.replace(/\s(width|height)\s*=\s*(?:"[^"]*"|'[^']*')/gi, "");
  if (attribute(tag, "viewBox") === undefined && declared !== null) {
    sized = sized.replace(/^<svg\b/i, `<svg viewBox="0 0 ${declared.width} ${declared.height}"`);
  }
  sized = sized.replace(/^<svg\b/i, `<svg width="${to.width}" height="${to.height}"`);
  return text.replace(tag, () => sized);
}

/**
 * The PNG bytes of an SVG drawn by the browser at its `rasterSize`; null when it is not an SVG the browser can draw.
 * It runs on the main thread, as only a document draws an SVG, once per asset and canvas.
 */
export async function rasteriseSvg(bytes: Uint8Array): Promise<Uint8Array | null> {
  const text = new TextDecoder().decode(bytes);
  const declared = svgSize(text);
  const to = rasterSize(declared);
  const sized = sizedSvg(text, declared, to);
  if (sized === null) {
    return null;
  }
  const url = URL.createObjectURL(new Blob([sized], { type: SVG_TYPE }));
  try {
    const image = new Image(to.width, to.height);
    image.src = url;
    await image.decode();
    const canvas = new OffscreenCanvas(to.width, to.height);
    canvas.getContext("2d")?.drawImage(image, 0, 0, to.width, to.height);
    const png = await canvas.convertToBlob({ type: "image/png" });
    return new Uint8Array(await png.arrayBuffer());
    // refusal-ok: the browser drawing a file, no server call; an SVG it cannot draw stays the canvas's placeholder.
  } catch {
    return null;
  } finally {
    URL.revokeObjectURL(url);
  }
}
