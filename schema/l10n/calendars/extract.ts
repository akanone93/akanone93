// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

// Extracts the Umm al-Qura month-length table and the Persian leap-year
// corrections from a pinned ICU release into umalqura.json and persian.json
// (ADR-0067, I18N-003). Run once when the pinned release changes:
//
//   bun schema/l10n/calendars/extract.ts
//
// The sources are fetched, their SHA-256 checked against the pins below, and
// the tables cross-checked against ICU's own year-start estimates.
import { createHash } from "node:crypto";
import { writeFileSync } from "node:fs";

const release = "release-78.1";
const base = `https://raw.githubusercontent.com/unicode-org/icu/${release}/icu4c/source/i18n/`;
const pins: Record<string, string> = {
  "islamcal.cpp": "a665b4eed397fc890786a27d27e80c754f71620101d79bc6a2b1bfa7d00bb6cb",
  "persncal.cpp": "31e7e93bb7dc1e4436c58ffd44fd1ed2686ceb79228b8d989f6573ec15ed5f45",
};

async function fetchPinned(name: string): Promise<{ url: string; text: string; sha256: string }> {
  const url = base + name;
  const res = await fetch(url);
  if (!res.ok) throw new Error(`${url}: HTTP ${res.status}`);
  const bytes = new Uint8Array(await res.arrayBuffer());
  const sha256 = createHash("sha256").update(bytes).digest("hex");
  if (sha256 !== pins[name]) throw new Error(`${name}: sha256 ${sha256}, pinned ${pins[name]}`);
  return { url, text: new TextDecoder().decode(bytes), sha256 };
}

function constInt(text: string, name: string): number {
  const m = new RegExp(`${name}\\s*=\\s*(\\d+)`).exec(text);
  if (!m) throw new Error(`constant ${name} not found`);
  return Number(m[1]);
}

function arrayBody(source: string, decl: string): string {
  const text = source.replace(/\/\/.*$/gm, "");
  const start = text.indexOf(decl);
  if (start < 0) throw new Error(`${decl} not found`);
  const open = text.indexOf("{", start);
  const close = text.indexOf("};", open);
  return text.slice(open + 1, close);
}

const isl = await fetchPinned("islamcal.cpp");
const yearStart = constInt(isl.text, "UMALQURA_YEAR_START");
const yearEnd = constInt(isl.text, "UMALQURA_YEAR_END");
const masks = [...arrayBody(isl.text, "UMALQURA_MONTHLENGTH[]").matchAll(/0x[0-9A-Fa-f]{4}/g)].map((m) => Number(m[0]));
if (masks.length !== yearEnd - yearStart + 1) throw new Error(`${masks.length} masks`);
const fix = arrayBody(isl.text, "umAlQuraYrStartEstimateFix[]")
  .replace(/\/\/.*$/gm, "")
  .split(",")
  .map((s) => s.trim())
  .filter((s) => s !== "")
  .map(Number);
if (fix.length < masks.length) throw new Error(`${fix.length} year-start fixes`);

// ICU's year start (days from the civil epoch) for 1300 AH, then the check
// that the month lengths reproduce every later estimate.
const start = (y: number) => Math.trunc(354.3672 * (y - yearStart) + 460322.05 + 0.5) + fix[y - yearStart];
let day = start(yearStart);
for (let y = yearStart; y <= yearEnd; y++) {
  if (day !== start(y)) throw new Error(`year ${y}: months give day ${day}, ICU estimate ${start(y)}`);
  for (let m = 0; m < 12; m++) day += 29 + ((masks[y - yearStart] >> (11 - m)) & 1);
}

writeFileSync(
  new URL("umalqura.json", import.meta.url),
  JSON.stringify(
    {
      source: { url: isl.url, release, file: "islamcal.cpp", sha256: isl.sha256, license: "Unicode-3.0" },
      note: "Bit (11-m) of monthMasks[y-yearStart] is set when month m (0-based) of year y has 30 days, else 29. epochDay1300 is the day of 1 Muharram 1300, in days after the civil epoch (JDN 1948440).",
      yearStart,
      yearEnd,
      civilEpochJdn: 1948440,
      epochDay1300: start(yearStart),
      monthMasks: masks,
    },
    null,
    1,
  ) + "\n",
);

const per = await fetchPinned("persncal.cpp");
const years = [...arrayBody(per.text, "nonLeapYears[]").matchAll(/\d+/g)].map((m) => Number(m[0]));
writeFileSync(
  new URL("persian.json", import.meta.url),
  JSON.stringify(
    {
      source: { url: per.url, release, file: "persncal.cpp", sha256: per.sha256, license: "Unicode-3.0" },
      note: "The 33-year arithmetic is in code; these are the years ICU 78 corrects to non-leap (the following year is then leap).",
      epochJdn: constInt(per.text, "PERSIAN_EPOCH"),
      nonLeapYears: years,
    },
    null,
    1,
  ) + "\n",
);
