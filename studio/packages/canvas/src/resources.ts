// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Ownership of CanvasKit objects. CanvasKit objects live in the WebAssembly
 * heap and are never collected, so the renderer registers every long-lived
 * object here and deletes them all on dispose; per-frame objects are counted
 * so tests can prove nothing leaks.
 */

/** A CanvasKit (Embind) object. */
export interface Deletable {
  delete(): void;
  isDeleted(): boolean;
}

/** The set of CanvasKit objects a renderer owns. */
export class Resources {
  private readonly owned = new Set<Deletable>();
  /** Per-frame objects created and deleted; equal when nothing leaked. */
  created = 0;
  deleted = 0;

  /** Registers a long-lived object and returns it. */
  own<T extends Deletable>(object: T): T {
    this.owned.add(object);
    return object;
  }

  /** Deletes an owned object. */
  free(object: Deletable): void {
    if (this.owned.delete(object)) {
      object.delete();
    }
  }

  /** Deletes a per-frame object at once, counting it. */
  discard(object: Deletable): void {
    this.created++;
    object.delete();
    this.deleted++;
  }

  /** The number of live owned objects. */
  get live(): number {
    return this.owned.size;
  }

  /** The live owned objects. */
  list(): Deletable[] {
    return [...this.owned];
  }

  /** Deletes every owned object. */
  dispose(): void {
    for (const object of this.owned) {
      object.delete();
    }
    this.owned.clear();
  }
}
