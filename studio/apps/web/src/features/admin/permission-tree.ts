// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/** One permission of the catalogue as the role builder shows it. */
export interface TreeAction {
  readonly name: string;
  readonly action: string;
  readonly description: string;
  readonly risk: string;
  readonly stepUp: boolean;
}

export interface TreeFeature {
  /** Empty for the actions that sit directly under their area. */
  readonly feature: string;
  readonly actions: readonly TreeAction[];
}

export interface TreeArea {
  readonly area: string;
  readonly features: readonly TreeFeature[];
}

interface Entry {
  readonly name: string;
  readonly description: string;
  readonly risk: string;
  readonly stepUp: boolean;
}

/**
 * Groups the permission catalogue as area, feature and action (F14): the first dotted segment is the area, the last is
 * the action and anything between is the feature. The order is alphabetical so the tree never depends on server order.
 */
export function buildTree(entries: readonly Entry[]): TreeArea[] {
  const areas = new Map<string, Map<string, TreeAction[]>>();
  for (const e of entries) {
    const parts = e.name.split(".");
    const area = parts[0] ?? e.name;
    const action = parts.length === 1 ? e.name : (parts[parts.length - 1] ?? e.name);
    const feature = parts.slice(1, -1).join(".");
    const features = areas.get(area) ?? new Map<string, TreeAction[]>();
    const actions = features.get(feature) ?? [];
    actions.push({ name: e.name, action, description: e.description, risk: e.risk, stepUp: e.stepUp });
    features.set(feature, actions);
    areas.set(area, features);
  }
  return [...areas.entries()]
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([area, features]) => ({
      area,
      features: [...features.entries()]
        .sort(([a], [b]) => a.localeCompare(b))
        .map(([feature, actions]) => ({
          feature,
          actions: [...actions].sort((a, b) => a.name.localeCompare(b.name)),
        })),
    }));
}

/** The set with every name of `names` added or removed. */
export function toggleAll(selected: ReadonlySet<string>, names: readonly string[], on: boolean): Set<string> {
  const next = new Set(selected);
  for (const n of names) {
    if (on) {
      next.add(n);
    } else {
      next.delete(n);
    }
  }
  return next;
}
