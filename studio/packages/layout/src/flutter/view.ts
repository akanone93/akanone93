// SPDX-FileCopyrightText: 2014 The Flutter Authors
// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: BSD-3-Clause AND AGPL-3.0-only

// Ported from Flutter 3.47.5 (framework 6a19cca56475dbfba1478ee68d7bd0c2ef891da1): packages/flutter/lib/src/rendering/view.dart
// Copyright 2014 The Flutter Authors. All rights reserved. Use of this source code is governed by a BSD-style license (BSD-3-Clause).

/** The root of a render tree: a frame of fixed logical size. */

import { BoxConstraints } from "./constraints.ts";
import type { Size } from "./geometry.ts";
import { type RenderBox, RenderBoxWithChild } from "./render-box.ts";

/**
 * The root render object (`RenderView`, layout only). It takes the
 * frame's logical size and lays its child out under tight constraints of
 * that size, as Flutter does for the app's root widget. Unlike Flutter's,
 * it is a [RenderBox] so the frame itself has a size and a rect.
 */
export class RenderView extends RenderBoxWithChild {
  private _configurationSize: Size;

  constructor(options: { size: Size; child?: RenderBox | null }) {
    super();
    this._configurationSize = options.size;
    this.child = options.child ?? null;
  }

  /** The frame's logical size (physical size / device pixel ratio). */
  get configurationSize(): Size {
    return this._configurationSize;
  }

  set configurationSize(value: Size) {
    if (this._configurationSize.equals(value)) {
      return;
    }
    this._configurationSize = value;
    this.markNeedsLayout();
  }

  /** Schedules the first layout. Call once after attaching to an owner. */
  prepareInitialFrame(): void {
    this.scheduleInitialLayout();
  }

  protected override computeDryLayout(_constraints: BoxConstraints): Size {
    return this._configurationSize;
  }

  protected override performLayout(): void {
    this.size = this._configurationSize;
    this.child?.layout(BoxConstraints.tight(this._configurationSize));
  }
}
