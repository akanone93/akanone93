// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { enums, type PropDescriptor, valueTypes, widget } from "@plux/schema";
import { Button, Field, FloatingPanel, Icon, Input, Select, Tabs, TypeTag } from "@plux/ui";
import type { Problem } from "@plux/validator";
import { lazy, Suspense, useEffect, useMemo, useRef, useState } from "react";
import { useCanEdit, useEditor } from "../editor/editor-provider.tsx";
import { jsonEditorMessages } from "../json-editor/json-editor.messages.ts";
import { linkMessages } from "../plugin-canvas/links.messages.ts";
import { routeOptions } from "../plugin-canvas/links.ts";
import { readPlugin } from "../plugin-canvas/plugin-model.ts";
import type { EditFn } from "../plugin-canvas/select/edits.ts";
import type { NodeIndex } from "../plugin-canvas/select/tree.ts";
import type { PxlLanguage } from "../problems/use-validation.ts";
import { ActionsTab } from "./actions-tab.tsx";
import { panelMessages } from "./panels.messages.ts";
import {
  type Breakpoint,
  bindingOf,
  breakpoints,
  editorKind,
  insetSides,
  isAccessibility,
  nodesById,
  type Obj,
  pickerHex,
  pointerOf,
  problemsAt,
  projectSources,
  setPropRecipe,
  setSemanticsRecipe,
  sharedProps,
  sharedValue,
  unitOf,
} from "./properties-model.ts";
import type { PxlEditorSource } from "./pxl-editor.tsx";
import { type PxlScopeCache, pxlSource } from "./pxl-source.ts";

/** Milliseconds a PXL field waits after the last keystroke before the validator worker checks it (F9). */
export const PXL_DEBOUNCE_MS = 300;

/** The CodeMirror editor is its own chunk: it loads when the first PXL field opens, not with the shell (STU-122). */
const PxlEditor = lazy(() => import("./pxl-editor.tsx"));

const enumNames: ReadonlySet<string> = new Set(enums.map((e) => e.name));
const constantsOf = (type: string) => valueTypes.find((v) => v.name === type)?.constants ?? [];

type Messages = ReturnType<typeof panelMessages>;
type Check = (expr: string) => Promise<readonly string[]>;

export interface Sources {
  readonly tokens: readonly string[];
  readonly keys: readonly { readonly id: string; readonly key: string }[];
  readonly check: Check | undefined;
  /** The language service for the field's expression; undefined without the validator module. */
  readonly pxl?: PxlEditorSource | undefined;
}

export interface EditorProps {
  readonly type: string;
  readonly name: string;
  readonly value: unknown;
  readonly mixed: boolean;
  readonly unit: string;
  readonly readOnly: boolean;
  readonly sources: Sources;
  readonly m: Messages;
  readonly onChange: (value: unknown) => void;
  /** Ends a burst of changes (blur): the next change starts a new undo step. */
  readonly onEnd: () => void;
}

const isObj = (v: unknown): v is Record<string, unknown> => typeof v === "object" && v !== null && !Array.isArray(v);
const numberOrUndefined = (s: string): number | undefined =>
  s.trim() === "" || Number.isNaN(Number(s)) ? undefined : Number(s);

function PxlField(p: EditorProps & { expr: string }) {
  const [errors, setErrors] = useState<readonly string[]>([]);
  const [busy, setBusy] = useState(false);
  const [type, setType] = useState("");
  const check = p.sources.check;
  useEffect(() => {
    if (check === undefined || p.expr === "") {
      setErrors([]);
      return undefined;
    }
    let live = true;
    setBusy(true);
    const timer = setTimeout(() => {
      check(p.expr)
        .then((e) => live && setErrors(e))
        .catch(() => live && setErrors([]))
        .finally(() => live && setBusy(false));
    }, PXL_DEBOUNCE_MS);
    return () => {
      live = false;
      clearTimeout(timer);
    };
  }, [check, p.expr]);
  const label = `${p.name}: ${p.m.pxl}`;
  const change = (text: string) => p.onChange(text === "" ? undefined : { $expr: text });
  const plain = (
    <Input
      value={p.expr}
      readOnly={p.readOnly}
      aria-label={label}
      onChange={(e) => change(e.target.value)}
      onBlur={p.onEnd}
    />
  );
  const description = busy ? p.m.pxlChecking : type === "" || p.expr === "" ? undefined : p.m.pxlResultType(type);
  return (
    <Field label={p.m.pxl} error={errors[0]} description={description}>
      {p.sources.pxl === undefined ? (
        plain
      ) : (
        <Suspense fallback={plain}>
          <PxlEditor
            value={p.expr}
            label={label}
            readOnly={p.readOnly}
            source={p.sources.pxl}
            onChange={change}
            onBlur={p.onEnd}
            onType={setType}
          />
        </Suspense>
      )}
    </Field>
  );
}

/** A binding-aware text editor: literal, translation key or PXL expression. */
function TextEditor(p: EditorProps) {
  const b = bindingOf(p.value);
  const mode = b?.kind === "t" ? "t" : b?.kind === "expr" ? "expr" : "literal";
  const [forced, setForced] = useState<"t" | "expr" | null>(null);
  const shown = forced ?? mode;
  const options = [
    { value: "literal", label: p.m.literal },
    ...(p.sources.keys.length > 0 ? [{ value: "t", label: p.m.useKey }] : []),
    { value: "expr", label: p.m.pxl },
  ];
  return (
    <div className="flex flex-col gap-1">
      <Select
        label={`${p.name}: ${p.m.literal}`}
        options={options}
        value={shown}
        disabled={p.readOnly}
        onValueChange={(v) => setForced(v === "literal" ? null : v === "t" ? "t" : "expr")}
      />
      {shown === "literal" && (
        <Input
          value={typeof p.value === "string" ? p.value : ""}
          placeholder={p.mixed ? p.m.mixed : undefined}
          aria-label={p.name}
          readOnly={p.readOnly}
          onChange={(e) => p.onChange(e.target.value === "" ? undefined : e.target.value)}
          onBlur={p.onEnd}
        />
      )}
      {shown === "t" && (
        <Select
          label={`${p.name}: ${p.m.useKey}`}
          options={p.sources.keys.map((k) => ({ value: k.id, label: k.key }))}
          value={b?.kind === "t" ? b.text : ""}
          disabled={p.readOnly}
          onValueChange={(v) => p.onChange({ $t: v })}
        />
      )}
      {shown === "expr" && <PxlField {...p} expr={b?.kind === "expr" ? b.text : ""} />}
    </div>
  );
}

function ColorEditor(p: EditorProps) {
  const b = bindingOf(p.value);
  const hex = pickerHex(p.value);
  return (
    <div className="flex items-center gap-2">
      <input
        type="color"
        aria-label={p.name}
        value={hex}
        disabled={p.readOnly}
        onChange={(e) => p.onChange(e.target.value)}
        onBlur={p.onEnd}
      />
      {p.sources.tokens.length > 0 && (
        <Select
          label={`${p.name}: ${p.m.useToken}`}
          options={p.sources.tokens.map((t) => ({ value: t, label: t }))}
          value={b?.kind === "token" ? b.text : ""}
          disabled={p.readOnly}
          onValueChange={(v) => p.onChange({ $token: v })}
        />
      )}
      {p.mixed && <span className="text-caption text-tertiary">{p.m.mixed}</span>}
    </div>
  );
}

function NumberEditor(p: EditorProps & { integer: boolean }) {
  return (
    <div className="flex items-center gap-2">
      <Input
        type="number"
        step={p.integer ? 1 : "any"}
        aria-label={p.name}
        value={typeof p.value === "number" ? p.value : ""}
        placeholder={p.mixed ? p.m.mixed : undefined}
        readOnly={p.readOnly}
        onChange={(e) => p.onChange(numberOrUndefined(e.target.value))}
        onBlur={p.onEnd}
      />
      {p.unit !== "" && <span className="text-caption text-tertiary">{p.unit}</span>}
    </div>
  );
}

function BoolEditor(p: EditorProps) {
  return (
    <label className="flex items-center gap-2 text-body">
      <input
        type="checkbox"
        aria-label={p.name}
        checked={p.value === true}
        ref={(el) => {
          if (el) {
            el.indeterminate = p.mixed;
          }
        }}
        disabled={p.readOnly}
        onChange={(e) => {
          p.onChange(e.target.checked);
          p.onEnd();
        }}
      />
      {p.mixed && <span className="text-caption text-tertiary">{p.m.mixed}</span>}
    </label>
  );
}

function InsetsEditor(p: EditorProps) {
  const o = isObj(p.value) ? p.value : {};
  const labels = { left: p.m.left, top: p.m.top, right: p.m.right, bottom: p.m.bottom };
  const change = (side: (typeof insetSides)[number], text: string) => {
    const n = numberOrUndefined(text);
    const next = { ...o };
    if (n === undefined) {
      delete next[side];
    } else {
      next[side] = n;
    }
    p.onChange(Object.keys(next).length === 0 ? undefined : next);
  };
  return (
    <div className="grid grid-cols-2 gap-2">
      {insetSides.map((side) => (
        <Input
          key={side}
          type="number"
          aria-label={`${p.name} ${labels[side]}`}
          value={typeof o[side] === "number" ? o[side] : ""}
          placeholder={p.mixed ? p.m.mixed : labels[side]}
          readOnly={p.readOnly}
          onChange={(e) => change(side, e.target.value)}
          onBlur={p.onEnd}
        />
      ))}
    </div>
  );
}

/** Alignment constants and enum values are both a pick from named options. */
function ChoiceEditor(p: EditorProps & { names: readonly string[]; label: string }) {
  return (
    <Select
      label={p.label}
      options={p.names.map((n) => ({ value: n, label: n }))}
      value={typeof p.value === "string" ? p.value : ""}
      placeholder={p.mixed ? p.m.mixed : p.m.unset}
      disabled={p.readOnly}
      onValueChange={(v) => {
        p.onChange(v);
        p.onEnd();
      }}
    />
  );
}

function ListEditor(p: EditorProps) {
  const item = p.type.replace(/\?$/, "").slice("list<".length, -1);
  const items = Array.isArray(p.value) ? p.value : [];
  const set = p.onChange;
  return (
    <div className="flex flex-col gap-2">
      {items.map((v, i) => (
        // biome-ignore lint/suspicious/noArrayIndexKey: list items have no identity of their own
        <div key={i} className="flex items-start gap-2">
          <ValueEditor
            {...p}
            type={item}
            name={`${p.name} ${i + 1}`}
            value={v}
            mixed={false}
            onChange={(nv) => set(items.map((x, j) => (j === i ? nv : x)))}
          />
          <Button
            variant="ghost"
            size="sm"
            disabled={p.readOnly}
            aria-label={p.m.removeItem(i + 1)}
            onClick={() => {
              set(items.filter((_, j) => j !== i));
              p.onEnd();
            }}
          >
            <Icon name="x" />
          </Button>
        </div>
      ))}
      <Button
        variant="secondary"
        size="sm"
        disabled={p.readOnly}
        onClick={() => {
          set([...items, ""]);
          p.onEnd();
        }}
      >
        {p.m.addItem}
      </Button>
    </div>
  );
}

function ObjectEditor(p: EditorProps) {
  const fields = valueTypes.find((v) => v.name === p.type.replace(/\?$/, ""))?.fields ?? [];
  const o = isObj(p.value) ? p.value : {};
  const change = (field: string, nv: unknown) => {
    const next = { ...o };
    if (nv === undefined) {
      delete next[field];
    } else {
      next[field] = nv;
    }
    p.onChange(Object.keys(next).length === 0 ? undefined : next);
  };
  return (
    <div className="flex flex-col gap-2 ps-3">
      {fields.map((f) => (
        <Field key={f.name} label={f.name}>
          <ValueEditor
            {...p}
            type={f.type}
            name={`${p.name}.${f.name}`}
            value={o[f.name]}
            mixed={false}
            unit=""
            onChange={(nv) => change(f.name, nv)}
          />
        </Field>
      ))}
    </div>
  );
}

/** Recursive value editor: one control per type; nested objects and lists edit their fields and items. */
export function ValueEditor(p: EditorProps) {
  const kind = editorKind(p.type, enumNames);
  const bare = p.type.replace(/\?$/, "");
  switch (kind) {
    case "number":
    case "int":
      return <NumberEditor {...p} integer={kind === "int"} />;
    case "bool":
      return <BoolEditor {...p} />;
    case "color":
      return <ColorEditor {...p} />;
    case "insets":
      return <InsetsEditor {...p} />;
    case "alignment":
      return (
        <ChoiceEditor
          {...p}
          label={`${p.name}: ${p.m.alignment}`}
          names={constantsOf("Alignment").map((c) => c.name)}
        />
      );
    case "enum":
      return (
        <ChoiceEditor
          {...p}
          label={p.name}
          names={(enums.find((e) => e.name === bare)?.values ?? []).map((v) => v.name)}
        />
      );
    case "list":
      return <ListEditor {...p} />;
    case "object":
      return <ObjectEditor {...p} />;
    default:
      return <TextEditor {...p} />;
  }
}

interface DockProps {
  readonly screens: readonly { readonly id: string; readonly root: unknown }[];
  readonly index: NodeIndex;
  readonly screenOf: ReadonlyMap<string, string>;
  readonly edit: EditFn;
  /** Page document path by screen ID, to validate PXL fields in the worker. */
  readonly pathOf: ReadonlyMap<string, string>;
  readonly problems: readonly Problem[];
  readonly documents:
    | readonly { readonly path: string; readonly kind: string; readonly content: Uint8Array }[]
    | undefined;
  /** Every document of the app, to offer other plugins' screens as link targets (F6). */
  readonly appDocuments?: readonly { readonly kind: string; readonly content: Uint8Array }[] | undefined;
  /** Validates the page with an edit applied; undefined while the worker is unavailable. */
  readonly validatePage?: ((file: string, data: string) => Promise<readonly Problem[]>) | undefined;
  /** The PXL language service of the loaded project (PXL-007); the inline PXL fields use it. */
  readonly language?: PxlLanguage | undefined;
  /** Opens the full-screen JSON editor (STU-064); the button shows only when given. */
  readonly onOpenJson?: (() => void) | undefined;
}

/** The selection's nodes from the edited documents (the session's), falling back to the server's screens. */
function useLiveNodes(p: DockProps): Map<string, Obj> {
  const docs = useEditor((st) => st.docs);
  return useMemo(
    () =>
      nodesById(
        p.screens.map((s) => {
          const file = p.pathOf.get(s.id);
          const content = file === undefined ? undefined : docs[file]?.content;
          return isObj(content) && "root" in content ? (content as Obj).root : s.root;
        }),
      ),
    [p.screens, p.pathOf, docs],
  );
}

/** A debounced-field check: the page with the expression applied, validated in the worker; errors at the prop. */
function pageCheck(
  validate: DockProps["validatePage"],
  file: string | undefined,
  content: unknown,
  id: string,
  bp: Breakpoint,
  name: string,
): Check | undefined {
  if (validate === undefined || file === undefined || content === undefined) {
    return undefined;
  }
  return async (expr) => {
    const page = structuredClone(content) as { root?: unknown };
    setPropRecipe(id, bp, name, { $expr: expr })(page.root);
    const ptr = pointerOf(page, id);
    const where = bp === "compact" ? "props" : `responsive/${bp}`;
    const found = await validate(file, JSON.stringify(page));
    return problemsAt(found, file, ptr === null ? null : `${ptr}/${where}/${name}`)
      .filter((x) => x.severity === "error")
      .map((x) => x.message);
  };
}

function PropertiesTab(p: DockProps & { ids: readonly string[]; m: Messages }) {
  const { m, ids } = p;
  const canEdit = useCanEdit();
  const [bp, setBp] = useState<Breakpoint>("compact");
  const burst = useRef(0);
  const scopes = useRef<PxlScopeCache>(new Map());
  const nodes = useLiveNodes(p);
  const selected = ids.flatMap((id) => {
    const n = nodes.get(id);
    return n === undefined ? [] : [n];
  });
  const types = ids.map((id) => p.index.type.get(id) ?? "");
  const props = sharedProps(types);
  const docsState = useEditor((st) => st.docs);
  const { tokens, keys } = useMemo(() => projectSources(p.documents ?? []), [p.documents]);
  const first = ids[0] ?? "";
  const screen = p.screenOf.get(first);
  const file = screen === undefined ? undefined : p.pathOf.get(screen);

  const apply = (name: string, value: unknown) => {
    for (const id of ids) {
      const doc = p.screenOf.get(id);
      if (doc !== undefined) {
        p.edit(doc, setPropRecipe(id, bp, name, value), {
          group: `prop:${ids.join(",")}:${bp}:${name}:${burst.current}`,
        });
      }
    }
  };
  const makeCheck = (name: string): Check | undefined =>
    pageCheck(p.validatePage, file, file === undefined ? undefined : docsState[file]?.content, first, bp, name);

  const labels: Record<Breakpoint, string> = { compact: m.compact, medium: m.medium, expanded: m.expanded };
  return (
    <div className="flex flex-col gap-3">
      {!canEdit && <p className="text-caption text-secondary">{m.readOnly}</p>}
      {ids.length > 1 && <p className="text-caption text-secondary">{m.multi(ids.length)}</p>}
      <Select
        label={m.breakpoint}
        options={breakpoints.map((b) => ({ value: b, label: labels[b] }))}
        value={bp}
        onValueChange={(v) => setBp(v as Breakpoint)}
      />
      {props.map((prop: PropDescriptor) => {
        const sv = sharedValue(selected, prop.name, bp);
        const mixed = sv.mixed;
        return (
          <Field key={prop.name} label={prop.name}>
            <ValueEditor
              type={prop.type}
              name={prop.name}
              value={mixed ? undefined : sv.value}
              mixed={mixed}
              unit={unitOf(prop)}
              readOnly={!canEdit}
              sources={{
                tokens,
                keys,
                check: makeCheck(prop.name),
                pxl: pxlSource(
                  p.language,
                  file,
                  file === undefined ? undefined : docsState[file]?.content,
                  first,
                  bp,
                  prop.name,
                  scopes.current,
                ),
              }}
              m={m}
              onChange={(v) => apply(prop.name, v)}
              onEnd={() => {
                burst.current += 1;
              }}
            />
            {!mixed && sv.overridden && (
              <div className="flex items-center gap-2 text-caption text-secondary">
                <span>{m.overridden}</span>
                <Button
                  variant="ghost"
                  size="sm"
                  disabled={!canEdit}
                  aria-label={m.resetOf(prop.name)}
                  onClick={() => {
                    apply(prop.name, undefined);
                    burst.current += 1;
                  }}
                >
                  <Icon name="arrow-counter-clockwise" />
                  {m.reset}
                </Button>
              </div>
            )}
          </Field>
        );
      })}
    </div>
  );
}

function AccessibilityTab(p: DockProps & { ids: readonly string[]; m: Messages }) {
  const { m, ids } = p;
  const canEdit = useCanEdit();
  const burst = useRef(0);
  const nodes = useLiveNodes(p);
  const id = ids[0];
  const node = id === undefined ? undefined : nodes.get(id);
  const type = id === undefined ? undefined : p.index.type.get(id);
  const descriptor = type === undefined ? undefined : widget(type);
  const semantics = node?.semantics;
  const sem = isObj(semantics) ? semantics : {};
  const doc = id === undefined ? undefined : p.screenOf.get(id);
  const file = doc === undefined ? undefined : p.pathOf.get(doc);
  const pointer = useEditor((st) => {
    const s = file === undefined ? undefined : st.docs[file];
    return s === undefined || id === undefined ? null : pointerOf(s.content, id);
  });
  if (id === undefined || ids.length !== 1 || doc === undefined || descriptor === undefined) {
    return <p className="text-body text-secondary">{m.nothing}</p>;
  }
  const set = (field: string, value: unknown) =>
    p.edit(doc, setSemanticsRecipe(id, field, value), { group: `sem:${id}:${field}:${burst.current}` });
  const found = file === undefined ? [] : problemsAt(p.problems, file, pointer).filter(isAccessibility);
  const flags: [string, string][] = [
    ["header", m.semHeader],
    ["button", m.semButton],
    ["liveRegion", m.semLive],
    ["excludeSemantics", m.semExclude],
  ];
  return (
    <div className="flex flex-col gap-3">
      {!canEdit && <p className="text-caption text-secondary">{m.readOnly}</p>}
      <p className="text-body">
        {m.role}: <TypeTag>{descriptor.accessibility.role}</TypeTag>{" "}
        {descriptor.accessibility.interactive ? m.interactive : m.notInteractive}
      </p>
      {descriptor.accessibility.minTarget === true && <p className="text-caption text-secondary">{m.minTarget}</p>}
      {(["label", "hint"] as const).map((f) => (
        <Field key={f} label={f === "label" ? m.semLabel : m.semHint}>
          <Input
            value={typeof sem[f] === "string" ? sem[f] : ""}
            readOnly={!canEdit}
            onChange={(e) => set(f, e.target.value)}
            onBlur={() => {
              burst.current += 1;
            }}
          />
        </Field>
      ))}
      {flags.map(([f, label]) => (
        <label key={f} className="flex items-center gap-2 text-body">
          <input
            type="checkbox"
            checked={sem[f] === true}
            disabled={!canEdit}
            onChange={(e) => {
              set(f, e.target.checked ? true : undefined);
              burst.current += 1;
            }}
          />
          {label}
        </label>
      ))}
      <h3 className="text-label font-medium">{m.a11yProblems}</h3>
      {found.length === 0 ? (
        <p className="text-body text-secondary">{m.a11yNone}</p>
      ) : (
        <ul className="flex flex-col gap-1">
          {found.map((x) => (
            <li key={`${x.code}${x.path}`} className="text-body">
              <TypeTag>{x.code}</TypeTag> {x.message}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

/** The Actions tab (F6) for the selected widget: its events, the app's screens, and the edit pipeline. */
function ActionsPanel(p: DockProps & { ids: readonly string[] }) {
  const canEdit = useCanEdit();
  const m = linkMessages(useLingui().i18n);
  const nodes = useLiveNodes(p);
  const id = p.ids.length === 1 ? p.ids[0] : undefined;
  const node = id === undefined ? undefined : nodes.get(id);
  const routes = useMemo(() => {
    const own = readPlugin(p.documents ?? []).screens;
    return routeOptions(own, readPlugin(p.appDocuments ?? []).screens);
  }, [p.documents, p.appDocuments]);
  const doc = id === undefined ? undefined : p.screenOf.get(id);
  if (id === undefined || node === undefined || doc === undefined) {
    return <p className="text-body text-secondary">{m.nothing}</p>;
  }
  return (
    <ActionsTab
      key={id}
      nodeId={id}
      node={node}
      routes={routes}
      canEdit={canEdit}
      apply={(recipe) => p.edit(doc, recipe)}
    />
  );
}

function DocsTab({ ids, index, m }: { ids: readonly string[]; index: NodeIndex; m: Messages }) {
  const type = ids[0] === undefined ? undefined : index.type.get(ids[0]);
  const d = type === undefined ? undefined : widget(type);
  if (d === undefined) {
    return <p className="text-body text-secondary">{m.nothing}</p>;
  }
  const section = (
    title: string,
    rows: readonly { name: string; description?: string; required?: boolean; deprecated?: unknown }[],
  ) =>
    rows.length === 0 ? null : (
      <section className="flex flex-col gap-1">
        <h3 className="text-label font-medium">{title}</h3>
        <ul className="flex flex-col gap-1">
          {rows.map((r) => (
            <li key={r.name} className="text-body">
              <TypeTag>{r.name}</TypeTag>
              {r.required === true && <span className="text-caption text-secondary"> {m.docsRequired}</span>}
              {r.deprecated !== undefined && <span className="text-caption text-secondary"> {m.docsDeprecated}</span>}
              {r.description !== undefined && <p className="text-caption text-secondary">{r.description}</p>}
            </li>
          ))}
        </ul>
      </section>
    );
  return (
    <div className="flex flex-col gap-3">
      <h2 className="text-section">{d.type}</h2>
      <p className="text-body">{d.description}</p>
      {section(m.docsProps, d.props)}
      {section(m.docsEvents, d.events)}
      {section(m.docsSlots, d.slots)}
    </div>
  );
}

/** The right dock of the plugin canvas (S2.6): Properties, Accessibility and Docs of the selection. */
export function RightDock(props: DockProps) {
  const { i18n } = useLingui();
  const m = panelMessages(i18n);
  const ids = useEditor((st) => st.selection.ids);
  const jsonLabel = jsonEditorMessages(i18n).open;
  const body = (panel: (ids: readonly string[]) => React.ReactNode) =>
    ids.length === 0 ? <p className="text-body text-secondary">{m.nothing}</p> : panel(ids);
  return (
    <FloatingPanel label={m.dock} className="absolute end-4 top-16 bottom-16 w-80 overflow-y-auto">
      {props.onOpenJson !== undefined && (
        <Button variant="secondary" size="sm" onClick={props.onOpenJson}>
          <Icon name="pencil-simple" />
          {jsonLabel}
        </Button>
      )}
      <Tabs
        defaultValue="properties"
        tabs={[
          {
            value: "properties",
            label: m.tabProperties,
            panel: body((i) => <PropertiesTab {...props} ids={i} m={m} />),
          },
          { value: "actions", label: linkMessages(i18n).tab, panel: <ActionsPanel {...props} ids={ids} /> },
          {
            value: "accessibility",
            label: m.tabAccessibility,
            panel: body((i) => <AccessibilityTab {...props} ids={i} m={m} />),
          },
          { value: "docs", label: m.tabDocs, panel: body((i) => <DocsTab ids={i} index={props.index} m={m} />) },
        ]}
      />
    </FloatingPanel>
  );
}
