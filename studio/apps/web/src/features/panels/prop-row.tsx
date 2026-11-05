// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { Icon, type IconName } from "@plux/ui";
import { ToggleGroup, ToggleGroupItem } from "@plux/ui/components/toggle-group";
import type { ReactElement, ReactNode } from "react";

/** A property's name as the inspector shows it: `textAlign` reads "Text align", `onPressed` "On pressed". */
export function propLabel(name: string): string {
  const words = name
    .replace(/([a-z0-9])([A-Z])/g, "$1 $2")
    .replace(/([A-Z])([A-Z][a-z])/g, "$1 $2")
    .toLowerCase();
  return words.charAt(0).toUpperCase() + words.slice(1);
}

/**
 * One property of the inspector, in one row (STU-062): its name at the start — a dot when it is set, a size-class mark
 * when the size class overrides it, a reset under the pointer — and its editor at the end. A composite editor
 * (decoration, text style, list, expression) spans the row under its name instead.
 */
export function PropRow(props: {
  readonly name: string;
  /** A ready, translated label; when absent the property's name is shown as words ("textAlign" reads "Text align"). */
  readonly label?: string;
  readonly set: boolean;
  /** The size class that overrides the value here ("M", "E"), or undefined. */
  readonly override?: string | undefined;
  readonly wide?: boolean;
  readonly resetLabel: string;
  readonly onReset?: (() => void) | undefined;
  readonly extra?: ReactNode;
  readonly children: ReactNode;
}): ReactElement {
  return (
    <div className="prop-row" data-wide={props.wide || undefined} data-set={props.set || undefined}>
      <div className="prop-name" title={props.name}>
        <span className="prop-dot" aria-hidden="true" />
        <span className="prop-name-text">{props.label ?? propLabel(props.name)}</span>
        {props.override !== undefined && (
          <span className="prop-override" aria-hidden="true">
            {props.override}
          </span>
        )}
        {props.onReset !== undefined && props.set && (
          <button type="button" className="prop-reset" aria-label={props.resetLabel} onClick={props.onReset}>
            <Icon name="arrow-counter-clockwise" size="pill" />
          </button>
        )}
      </div>
      <div className="prop-control">
        {props.children}
        {props.extra}
      </div>
    </div>
  );
}

/** The nine alignments of a box, in reading order, as Flutter names them. */
const ALIGNMENTS: readonly string[] = [
  "topLeft",
  "topCenter",
  "topRight",
  "centerLeft",
  "center",
  "centerRight",
  "bottomLeft",
  "bottomCenter",
  "bottomRight",
];

/**
 * An alignment as a 3×3 grid of points, as design tools pick it: the point of the value is filled; other named
 * alignments (start, end…) stay in the list beside it.
 */
export function AlignmentPicker(props: {
  readonly label: string;
  readonly value: string | undefined;
  readonly names: readonly string[];
  readonly readOnly: boolean;
  readonly onChange: (value: string | undefined) => void;
  readonly more: ReactNode;
}): ReactElement {
  const grid = ALIGNMENTS.filter((a) => props.names.includes(a));
  return (
    <div className="prop-align">
      {grid.length === 9 && (
        // biome-ignore lint/a11y/useSemanticElements: a grid of radio buttons, styled as points
        <div role="radiogroup" aria-label={props.label} className="prop-align-grid">
          {grid.map((a) => (
            // biome-ignore lint/a11y/useSemanticElements: a radio option drawn as a point of the grid
            <button
              key={a}
              type="button"
              role="radio"
              aria-checked={props.value === a}
              aria-label={a}
              title={a}
              disabled={props.readOnly}
              className="prop-align-point"
              onClick={() => props.onChange(props.value === a ? undefined : a)}
            />
          ))}
        </div>
      )}
      <div className="prop-align-more">{props.more}</div>
    </div>
  );
}

/** Whether a short choice reads best as a segmented control: few options with short names. */
export function segmentable(names: readonly string[]): boolean {
  return names.length >= 2 && names.length <= 4 && names.every((n) => n.length <= 9);
}

/** A choice among a few named values as one segmented control; the chosen one again clears it. */
export function SegmentedChoice(props: {
  readonly label: string;
  readonly names: readonly string[];
  readonly value: string | undefined;
  readonly readOnly: boolean;
  readonly onChange: (value: string | undefined) => void;
}): ReactElement {
  return (
    <ToggleGroup
      aria-label={props.label}
      value={props.value === undefined ? [] : [props.value]}
      disabled={props.readOnly}
      onValueChange={(v) => props.onChange(v[0] === undefined ? undefined : String(v[0]))}
      className="prop-segments"
      size="sm"
    >
      {props.names.map((n) => (
        <ToggleGroupItem key={n} value={n} aria-label={n} className="prop-segment">
          {n}
        </ToggleGroupItem>
      ))}
    </ToggleGroup>
  );
}

/** A section of properties: a quiet heading with its icon, name and how many are set out of all. */
export function PropGroupHead(props: {
  readonly icon: IconName;
  readonly label: string;
  readonly set: number;
  readonly count: number;
}): ReactElement {
  return (
    <>
      <Icon name={props.icon} size="pill" />
      <span className="flex-1 text-start">{props.label}</span>
      <span className="prop-section-count">{props.set > 0 ? `${props.set}/${props.count}` : props.count}</span>
      <Icon name="caret-down" size="pill" className="prop-section-caret" />
    </>
  );
}
