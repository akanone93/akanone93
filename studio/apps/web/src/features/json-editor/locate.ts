// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/** A span of the editor's text, as character offsets: `end` is exclusive. */
export interface Span {
  readonly start: number;
  readonly end: number;
}

const WHITESPACE = new Set([" ", "\t", "\n", "\r"]);

/** The segments of an RFC 6901 pointer, unescaped; undefined when it is not a pointer. */
export function pointerSegments(pointer: string): string[] | undefined {
  if (pointer === "") {
    return [];
  }
  if (!pointer.startsWith("/")) {
    return undefined;
  }
  return pointer
    .slice(1)
    .split("/")
    .map((s) => s.replaceAll("~1", "/").replaceAll("~0", "~"));
}

class Scanner {
  i = 0;
  constructor(readonly text: string) {}

  skip(): void {
    while (this.i < this.text.length && WHITESPACE.has(this.text.charAt(this.i))) {
      this.i += 1;
    }
  }

  /** Reads a string literal at the cursor and returns its decoded value, or undefined when malformed. */
  string(): string | undefined {
    const from = this.i;
    if (this.text.charAt(from) !== '"') {
      return undefined;
    }
    this.i += 1;
    while (this.i < this.text.length) {
      const c = this.text.charAt(this.i);
      if (c === "\\") {
        this.i += 2;
      } else if (c === '"') {
        this.i += 1;
        try {
          const value: unknown = JSON.parse(this.text.slice(from, this.i));
          return typeof value === "string" ? value : undefined;
        } catch {
          return undefined;
        }
      } else {
        this.i += 1;
      }
    }
    return undefined;
  }

  /** Skips one value of any kind; false when the text is malformed there. */
  value(): boolean {
    this.skip();
    const c = this.text.charAt(this.i);
    if (c === '"') {
      return this.string() !== undefined;
    }
    if (c === "{" || c === "[") {
      return this.container(c);
    }
    const from = this.i;
    while (this.i < this.text.length && !",}] \t\n\r".includes(this.text.charAt(this.i))) {
      this.i += 1;
    }
    return this.i > from;
  }

  /** Reads the `"key":` of an object member and moves to its value; undefined when malformed. */
  memberKey(): string | undefined {
    const key = this.string();
    if (key === undefined) {
      return undefined;
    }
    this.skip();
    if (this.text.charAt(this.i) !== ":") {
      return undefined;
    }
    this.i += 1;
    this.skip();
    return key;
  }

  /** After a member: "more" past a comma, "end" past the closing bracket `close`, "bad" otherwise. */
  after(close: string): "more" | "end" | "bad" {
    this.skip();
    const c = this.text.charAt(this.i);
    this.i += 1;
    return c === close ? "end" : c === "," ? "more" : "bad";
  }

  container(open: string): boolean {
    const close = open === "{" ? "}" : "]";
    this.i += 1;
    this.skip();
    if (this.text.charAt(this.i) === close) {
      this.i += 1;
      return true;
    }
    for (;;) {
      this.skip();
      if (open === "{" && this.memberKey() === undefined) {
        return false;
      }
      if (!this.value()) {
        return false;
      }
      const next = this.after(close);
      if (next !== "more") {
        return next === "end";
      }
    }
  }
}

/** Descends one level: moves the cursor to the member `segment` of the object or array at the cursor. */
function enter(s: Scanner, segment: string): boolean {
  s.skip();
  const open = s.text.charAt(s.i);
  if (open !== "{" && open !== "[") {
    return false;
  }
  const close = open === "{" ? "}" : "]";
  s.i += 1;
  for (let index = 0; ; index += 1) {
    s.skip();
    if (s.text.charAt(s.i) === close) {
      return false;
    }
    const name = open === "{" ? s.memberKey() : String(index);
    if (name === undefined) {
      return false;
    }
    if (name === segment) {
      return true;
    }
    if (!s.value() || s.after(close) !== "more") {
      return false;
    }
  }
}

/**
 * Where the value a JSON Pointer names sits in `text`, or the nearest ancestor that exists; undefined when
 * the text is not JSON up to there. The validator reports pointers, the editor needs ranges.
 */
export function locate(text: string, pointer: string): Span | undefined {
  const segments = pointerSegments(pointer);
  if (segments === undefined) {
    return undefined;
  }
  const s = new Scanner(text);
  s.skip();
  let at = s.i;
  for (const segment of segments) {
    const saved = s.i;
    if (!enter(s, segment)) {
      s.i = saved;
      break;
    }
    at = s.i;
  }
  s.i = at;
  const start = at;
  if (!s.value()) {
    return undefined;
  }
  return { start, end: s.i };
}

/** The 1-based line and column of a character offset. */
export function lineColumn(text: string, offset: number): { line: number; column: number } {
  let line = 1;
  let last = -1;
  for (let i = text.indexOf("\n"); i !== -1 && i < offset; i = text.indexOf("\n", i + 1)) {
    line += 1;
    last = i;
  }
  return { line, column: offset - last };
}
