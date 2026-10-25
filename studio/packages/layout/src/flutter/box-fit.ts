// SPDX-FileCopyrightText: 2014 The Flutter Authors
// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: BSD-3-Clause AND AGPL-3.0-only

// Ported from Flutter 3.47.5 (framework 6a19cca56475dbfba1478ee68d7bd0c2ef891da1): packages/flutter/lib/src/painting/box_fit.dart
// Copyright 2014 The Flutter Authors. All rights reserved. Use of this source code is governed by a BSD-style license (BSD-3-Clause).

/** How a box is inscribed into another box (`BoxFit`) and the sizes that result (`applyBoxFit`). */

import { Size } from "./geometry.ts";

export enum BoxFit {
  fill = "fill",
  contain = "contain",
  cover = "cover",
  fitWidth = "fitWidth",
  fitHeight = "fitHeight",
  none = "none",
  scaleDown = "scaleDown",
}

/** The region of the input that is shown (`source`) and the region of the output it fills (`destination`). */
export interface FittedSizes {
  readonly source: Size;
  readonly destination: Size;
}

/** Applies `fit` to inscribe `inputSize` into `outputSize` (painting/box_fit.dart `applyBoxFit`). */
export function applyBoxFit(fit: BoxFit, inputSize: Size, outputSize: Size): FittedSizes {
  if (inputSize.height <= 0 || inputSize.width <= 0 || outputSize.height <= 0 || outputSize.width <= 0) {
    return { source: Size.zero, destination: Size.zero };
  }
  let source: Size;
  let destination: Size;
  const outputWider = outputSize.width / outputSize.height > inputSize.width / inputSize.height;
  switch (fit) {
    case BoxFit.fill:
      source = inputSize;
      destination = outputSize;
      break;
    case BoxFit.contain:
      source = inputSize;
      destination = outputWider
        ? new Size((source.width * outputSize.height) / source.height, outputSize.height)
        : new Size(outputSize.width, (source.height * outputSize.width) / source.width);
      break;
    case BoxFit.cover:
      source = outputWider
        ? new Size(inputSize.width, (inputSize.width * outputSize.height) / outputSize.width)
        : new Size((inputSize.height * outputSize.width) / outputSize.height, inputSize.height);
      destination = outputSize;
      break;
    case BoxFit.fitWidth:
      if (outputWider) {
        // Like "cover".
        source = new Size(inputSize.width, (inputSize.width * outputSize.height) / outputSize.width);
        destination = outputSize;
      } else {
        // Like "contain".
        source = inputSize;
        destination = new Size(outputSize.width, (source.height * outputSize.width) / source.width);
      }
      break;
    case BoxFit.fitHeight:
      if (outputWider) {
        // Like "contain".
        source = inputSize;
        destination = new Size((source.width * outputSize.height) / source.height, outputSize.height);
      } else {
        // Like "cover".
        source = new Size((inputSize.height * outputSize.width) / outputSize.height, inputSize.height);
        destination = outputSize;
      }
      break;
    case BoxFit.none:
      source = new Size(Math.min(inputSize.width, outputSize.width), Math.min(inputSize.height, outputSize.height));
      destination = source;
      break;
    case BoxFit.scaleDown: {
      source = inputSize;
      destination = inputSize;
      const aspectRatio = inputSize.width / inputSize.height;
      if (destination.height > outputSize.height) {
        destination = new Size(outputSize.height * aspectRatio, outputSize.height);
      }
      if (destination.width > outputSize.width) {
        destination = new Size(outputSize.width, outputSize.width / aspectRatio);
      }
      break;
    }
  }
  return { source, destination };
}
