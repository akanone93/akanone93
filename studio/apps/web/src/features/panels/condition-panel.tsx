// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Icon } from "@plux/ui";
import { Input } from "@plux/ui/components/input";
import { createContext, type ReactElement, useContext } from "react";
import { type ConditionMessages, conditionMessages } from "./condition-panel.messages.ts";

/**
 * The conditions panel of the inspector (PC-25, SCH-023): a widget's visibility condition, an `If`'s condition and a
 * `Match`'s value, what each is at design time, and the branch the canvas draws — following the data, or picked here
 * to design a branch the data does not show.
 */

type Obj = Record<string, unknown>;
const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);

/** The branch picked for each widget on the canvas (PC-25); null outside a canvas. */
export interface BranchPreview {
  readonly pickOf: (id: string) => string | undefined;
  readonly pick: (id: string, pick: string | undefined) => void;
}
export const BranchPreviewContext = createContext<BranchPreview | null>(null);

const exprText = (v: unknown): string =>
  typeof v === "boolean" ? String(v) : isObj(v) && typeof v["$expr"] === "string" ? v["$expr"] : "";

function Picks(p: {
  readonly id: string;
  readonly options: readonly { readonly value: string; readonly label: string }[];
  readonly m: ConditionMessages;
}) {
  const preview = useContext(BranchPreviewContext);
  if (preview === null) {
    return null;
  }
  const current = preview.pickOf(p.id) ?? "auto";
  return (
    <div className="state-chips" role="radiogroup" aria-label={p.m.draw}>
      {p.options.map((o) => (
        // biome-ignore lint/a11y/useSemanticElements: a chip of a segmented choice
        <button
          key={o.value}
          type="button"
          role="radio"
          aria-checked={current === o.value}
          className="state-chip"
          onClick={() => preview.pick(p.id, o.value === "auto" ? undefined : o.value)}
        >
          {o.label}
        </button>
      ))}
    </div>
  );
}

function Worked({ result, m }: { readonly result: boolean | undefined; readonly m: ConditionMessages }) {
  return (
    <p className="cond-worked" data-result={String(result)}>
      <Icon name={result === undefined ? "question" : result ? "eye" : "eye-slash"} size="pill" />
      {m.worked(String(result))}
    </p>
  );
}

/** The options of a `Match`: each case, then `otherwise`. */
function matchOptions(node: Obj, m: ConditionMessages) {
  const props = isObj(node["props"]) ? node["props"] : {};
  const cases = Array.isArray(props["cases"]) ? props["cases"].map(String) : [];
  return [
    { value: "auto", label: m.pick("auto") },
    ...cases.map((c, i) => ({ value: `case:${i}`, label: c })),
    { value: "otherwise", label: m.pick("otherwise") },
  ];
}

export function ConditionPanel(p: {
  readonly id: string;
  readonly node: Obj;
  readonly canEdit: boolean;
  /** A condition's design-time value; undefined when only a device knows it. */
  readonly decide: (expr: string) => boolean | undefined;
  readonly setVisible: (value: unknown) => void;
}): ReactElement {
  const m = conditionMessages(useLingui().i18n);
  const type = p.node["type"];
  const props = isObj(p.node["props"]) ? p.node["props"] : {};
  const visible = exprText(p.node["visible"]);
  const decided = (text: string) => (text === "true" ? true : text === "false" ? false : p.decide(text));
  return (
    <section className="lay-card cond-panel" aria-label={m.title}>
      <h3 className="lay-title">
        <Icon name="flow-arrow" size="pill" />
        {m.title}
      </h3>
      {type === "If" && (
        <div className="cond-block">
          <span className="lay-label">{m.condition}</span>
          <code className="cond-expr">{exprText(props["condition"]) || "—"}</code>
          <Worked result={decided(exprText(props["condition"]))} m={m} />
          <Picks id={p.id} m={m} options={["auto", "then", "else"].map((v) => ({ value: v, label: m.pick(v) }))} />
        </div>
      )}
      {type === "Match" && (
        <div className="cond-block">
          <span className="lay-label">{m.value}</span>
          <code className="cond-expr">{exprText(props["value"]) || String(props["value"] ?? "—")}</code>
          <Picks id={p.id} m={m} options={matchOptions(p.node, m)} />
        </div>
      )}
      <div className="cond-block">
        <span className="lay-label">{m.visibleWhen}</span>
        <Input
          key={visible}
          aria-label={m.visibleWhen}
          className="cond-input"
          placeholder={m.visiblePlaceholder}
          defaultValue={visible}
          readOnly={!p.canEdit}
          onBlur={(e) => {
            const text = e.target.value.trim();
            if (text !== visible) {
              p.setVisible(
                text === "" ? undefined : text === "true" || text === "false" ? text === "true" : { $expr: text },
              );
            }
          }}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.currentTarget.blur();
            }
          }}
        />
        {visible !== "" && (
          <>
            <Worked result={decided(visible)} m={m} />
            <Picks id={p.id} m={m} options={["auto", "shown", "hidden"].map((v) => ({ value: v, label: m.pick(v) }))} />
          </>
        )}
      </div>
    </section>
  );
}
