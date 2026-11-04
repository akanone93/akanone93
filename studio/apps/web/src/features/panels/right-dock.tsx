// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";
import { useLingui } from "@lingui/react";
import { enums, type PropDescriptor, valueTypes, widget } from "@plux/schema";
import { Button, Field, Icon, type IconName, Input, Select, TypeTag, tooltipDelayMs } from "@plux/ui";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@plux/ui/components/collapsible";
import { Tabs as ShadcnTabs, TabsContent, TabsList, TabsTrigger } from "@plux/ui/components/tabs";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@plux/ui/components/tooltip";
import type { Problem } from "@plux/validator";
import { lazy, Suspense, useEffect, useMemo, useRef, useState } from "react";
import { actionGraphMessages } from "../action-graph/action-graph.messages.ts";
import { designerMessages } from "../designers/designers.messages.ts";
import { useCanEdit, useEditor } from "../editor/editor-provider.tsx";
import type { JsonValue } from "../editor/json-patch.ts";
import { jsonEditorMessages } from "../json-editor/json-editor.messages.ts";
import { InstanceOverrides } from "../library/canvas/instance-overrides.tsx";
import { ExtractKeyDialog, localisationMessages, useL10nSessions } from "../localisation/index.ts";
import { linkMessages } from "../plugin-canvas/links.messages.ts";
import { routeOptions } from "../plugin-canvas/links.ts";
import { type Node, readPlugin } from "../plugin-canvas/plugin-model.ts";
import { uuidv7 } from "../plugin-canvas/screens.ts";
import type { EditFn } from "../plugin-canvas/select/edits.ts";
import type { NodeIndex } from "../plugin-canvas/select/tree.ts";
import type { PxlLanguage } from "../problems/use-validation.ts";
import { rustEditorMessages } from "../rust-editor/rust-editor.messages.ts";
import type { TestsClient } from "../rust-editor/test-runner.ts";
import { stateMessages } from "../state-designer/index.ts";
import { ActionsTab } from "./actions-tab.tsx";
import { typeIcon } from "./catalogue.ts";
import { DockExpand, useStoredFlag } from "./dock-tabs.tsx";
import { IconField } from "./icon-field.tsx";
import { ImageSourceField } from "./image-field.tsx";
import { panelMessages, panelsMessages } from "./panels.messages.ts";
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
import { lockedIds } from "./studio-meta.ts";
import { ColorField, DecorationEditor, TextStyleEditor } from "./style-editors.tsx";
import { CodeTab, JsonTab, LocalisationTab, PerformanceTab } from "./tab-panels.tsx";
import { tabMessages } from "./tabs.messages.ts";

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
          set([...items, emptyItem(item)]);
          p.onEnd();
        }}
      >
        {p.m.addItem}
      </Button>
    </div>
  );
}

/** A new list item of `type`: an empty object for a value type, zero for a number, else an empty string. */
function emptyItem(type: string): unknown {
  const kind = editorKind(type, enumNames);
  return kind === "object" ? {} : kind === "number" || kind === "int" ? 0 : kind === "bool" ? false : "";
}

/** A value type with an editor of its own (decorations, text styles); every other one edits its fields. */
function StyleObjectEditor(p: EditorProps & { bare: string }) {
  const own = { name: p.name, value: p.value, readOnly: p.readOnly, onChange: p.onChange, onEnd: p.onEnd };
  if (p.bare === "BoxDecoration") {
    return <DecorationEditor {...own} />;
  }
  if (p.bare === "IconData") {
    return <IconField {...own} />;
  }
  if (p.bare === "ImageSource") {
    return <ImageSourceField {...own} />;
  }
  return p.bare === "TextStyle" ? <TextStyleEditor {...own} /> : <ObjectEditor {...p} />;
}

/** A colour property: the swatch field with the theme's roles and tokens (STU-062, THM-001). */
function StyleColor(p: EditorProps) {
  const b = bindingOf(p.value);
  return b?.kind === "expr" ? (
    <ColorEditor {...p} />
  ) : (
    <ColorField
      name={p.name}
      value={p.value}
      mixed={p.mixed}
      readOnly={p.readOnly}
      onChange={p.onChange}
      onEnd={p.onEnd}
    />
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
      return <StyleColor {...p} />;
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
      return <StyleObjectEditor {...p} bare={bare} />;
    default:
      return <TextEditor {...p} />;
  }
}

interface DockProps {
  readonly screens: readonly { readonly id: string; readonly root: unknown; readonly title?: string }[];
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
  /** Opens the localisation table (STU-063) and the state designer (STU-063); each button shows only when given. */
  readonly onOpenLocalisation?: (() => void) | undefined;
  readonly onOpenState?: (() => void) | undefined;
  /** The app's schema version, for the documents that key extraction creates (I18N-007). */
  readonly schemaVersion?: JsonValue | undefined;
  /** Opens one of the designers (data sources, DB schema, theme) as a full-screen overlay (STU-063). */
  readonly onOpenOverlay?: ((id: "data" | "db" | "theme") => void) | undefined;
  /** Opens the full-screen action graph editor (STU-065); the button shows only when given. */
  readonly onOpenGraph?: (() => void) | undefined;
  readonly onOpenRust?: (() => void) | undefined;
  /** Opens the Rust editor on the named function (Code tab, FN-033). */
  readonly onOpenFunction?: ((name: string) => void) | undefined;
  /** Runs a function's stored tests from the Code tab (FN-034). */
  readonly tests?: TestsClient | undefined;
}

/** The registry marks string props whose text the user sees (`translatable`, I18N-007). */
const isTranslatable = (prop: PropDescriptor): boolean => prop.translatable === true;

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

/** The sections the inspector sorts a widget's properties into, by the property's name. */
type PropGroup = "layout" | "appearance" | "content" | "behaviour" | "other";

const groupOrder: readonly PropGroup[] = ["content", "layout", "appearance", "behaviour", "other"];

const groupIcons: Record<PropGroup, IconName> = {
  layout: "grid-four",
  appearance: "palette",
  content: "text-t",
  behaviour: "hand-pointing",
  other: "sliders",
};

const groupPatterns: readonly (readonly [PropGroup, RegExp])[] = [
  [
    "layout",
    /^(width|height|size|padding|margin|align|alignment|mainAxis|crossAxis|spacing|runSpacing|gap|flex|fit|aspectRatio|constraints|position|top|bottom|left|right|start|end|direction|axis|clip|expand|scroll|physics|itemExtent|shrinkWrap|reverse|verticalDirection|textDirection)/i,
  ],
  [
    "appearance",
    /(color|colour|background|decoration|border|radius|elevation|shadow|style|theme|opacity|gradient|font|weight|outline|shape|tint|overlay|splash|highlight|filter|blend)/i,
  ],
  [
    "content",
    /^(data|text|label|title|subtitle|child|children|icon|image|src|url|value|placeholder|hint|items|leading|trailing|content|semanticsLabel|tooltip|message|actions|body|header|footer)/i,
  ],
  [
    "behaviour",
    /^(on[A-Z]|enabled|disabled|readOnly|autofocus|keyboard|obscure|max|min|divisions|validator|autovalidate|selected|checked|tristate|duration|curve|delay|repeat|loop|interactive|controller)/i,
  ],
];

/** The group of a property: the first whose pattern its name matches, else "other". */
function propGroupOf(name: string): PropGroup {
  return groupPatterns.find(([, pattern]) => pattern.test(name))?.[0] ?? "other";
}

/** The properties in their sections, in the inspector's order; empty sections are left out. */
function propGroups(props: readonly PropDescriptor[]): { id: PropGroup; props: PropDescriptor[] }[] {
  return groupOrder
    .map((id) => ({ id, props: props.filter((p) => propGroupOf(p.name) === id) }))
    .filter((g) => g.props.length > 0);
}

/** A collapsible section of the inspector's properties: its icon, name and count, open at first. */
function PropSection(props: {
  readonly label: string;
  readonly icon: IconName;
  readonly count: number;
  readonly children: React.ReactNode;
}) {
  return (
    <Collapsible defaultOpen className="prop-section">
      <CollapsibleTrigger className="prop-section-head">
        <Icon name={props.icon} size="pill" />
        <span className="flex-1 text-start">{props.label}</span>
        <span className="prop-section-count">{props.count}</span>
        <Icon name="caret-down" size="pill" className="prop-section-caret" />
      </CollapsibleTrigger>
      <CollapsibleContent className="prop-section-body">{props.children}</CollapsibleContent>
    </Collapsible>
  );
}

/** Whether the selection may be edited: the editor can edit and no selected widget is locked in Layers (STU-061). */
function useSelectionEditable(p: DockProps, ids: readonly string[]): { canEdit: boolean; lockedNote: string | null } {
  const canEdit = useCanEdit();
  const locked = useMemo(() => lockedIds(p.screens.map((s) => s.root as Node | undefined)), [p.screens]);
  const note = panelsMessages(useLingui().i18n).lockedNote;
  const isLocked = ids.some((id) => locked.has(id));
  return { canEdit: canEdit && !isLocked, lockedNote: isLocked ? note : null };
}

/** The note a tab shows over a locked selection. */
function LockedNote({ text }: { readonly text: string | null }) {
  return text === null ? null : (
    <p className="locked-note">
      <Icon name="lock" size="pill" />
      {text}
    </p>
  );
}

function PropertiesTab(p: DockProps & { ids: readonly string[]; m: Messages }) {
  const { m, ids } = p;
  const { canEdit, lockedNote } = useSelectionEditable(p, ids);
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
  const l10n = useL10nSessions();
  const lm = localisationMessages(useLingui().i18n);
  const [extracting, setExtracting] = useState<{ readonly name: string; readonly text: string } | null>(null);

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
  const groupLabels: Record<PropGroup, string> = {
    layout: m.groupLayout,
    appearance: m.groupAppearance,
    content: m.groupContent,
    behaviour: m.groupBehaviour,
    other: m.groupOther,
  };
  return (
    <div className="flex flex-col gap-3">
      <LockedNote text={lockedNote} />
      {!canEdit && lockedNote === null && <p className="text-caption text-muted-foreground">{m.readOnly}</p>}
      {ids.length > 1 && <p className="text-caption text-muted-foreground">{m.multi(ids.length)}</p>}
      {/* biome-ignore lint/a11y/useSemanticElements: a labelled radio group of size classes, not a form's fieldset */}
      <div
        role="radiogroup"
        aria-label={m.breakpoint}
        className="size-classes"
        onKeyDown={(e) => {
          const step = { ArrowRight: 1, ArrowDown: 1, ArrowLeft: -1, ArrowUp: -1 }[e.key];
          if (step === undefined) {
            return;
          }
          e.preventDefault();
          const rtl =
            getComputedStyle(e.currentTarget).direction === "rtl" &&
            e.key.startsWith("Arrow") &&
            !e.key.endsWith("Up") &&
            !e.key.endsWith("Down");
          const at = (breakpoints.indexOf(bp) + (rtl ? -step : step) + breakpoints.length) % breakpoints.length;
          setBp(breakpoints[at] ?? bp);
          e.currentTarget.querySelectorAll<HTMLElement>("[role=radio]")[at]?.focus();
        }}
      >
        <span className="size-classes-label" aria-hidden="true">
          {m.breakpoint}
        </span>
        {breakpoints.map((b) => (
          // biome-ignore lint/a11y/useSemanticElements: a segmented control's option, styled as a pill
          <button
            key={b}
            type="button"
            role="radio"
            aria-checked={bp === b}
            tabIndex={bp === b ? 0 : -1}
            className="size-class"
            onClick={() => setBp(b)}
          >
            {labels[b]}
          </button>
        ))}
      </div>
      {ids.length === 1 && (
        <InstanceOverrides node={selected[0]} doc={p.screenOf.get(first)} edit={p.edit} canEdit={canEdit} />
      )}
      {propGroups(props).map((group) => (
        <PropSection
          key={group.id}
          label={groupLabels[group.id]}
          icon={groupIcons[group.id]}
          count={group.props.length}
        >
          {group.props.map((prop: PropDescriptor) => {
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
                {l10n !== null &&
                  canEdit &&
                  ids.length === 1 &&
                  isTranslatable(prop) &&
                  !mixed &&
                  typeof sv.value === "string" &&
                  sv.value !== "" && (
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => setExtracting({ name: prop.name, text: sv.value as string })}
                    >
                      <Icon name="globe" />
                      {lm.extract}
                    </Button>
                  )}
                {!mixed && sv.overridden && (
                  <div className="flex items-center gap-2 text-caption text-muted-foreground">
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
        </PropSection>
      ))}
      {l10n !== null && extracting !== null && (
        <ExtractKeyDialog
          open
          onClose={() => setExtracting(null)}
          sessions={l10n}
          text={extracting.text}
          prefix={p.index.type.get(first) ?? ""}
          schemaVersion={p.schemaVersion ?? 1}
          newId={uuidv7}
          replaceProp={(reference, step) => {
            const doc = p.screenOf.get(first);
            if (doc !== undefined) {
              p.edit(doc, setPropRecipe(first, bp, extracting.name, reference), { step });
            }
          }}
        />
      )}
    </div>
  );
}

function AccessibilityTab(p: DockProps & { ids: readonly string[]; m: Messages }) {
  const { m, ids } = p;
  const { canEdit, lockedNote } = useSelectionEditable(p, ids);
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
    return <p className="text-body text-muted-foreground">{m.nothing}</p>;
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
      <LockedNote text={lockedNote} />
      {!canEdit && lockedNote === null && <p className="text-caption text-muted-foreground">{m.readOnly}</p>}
      <p className="text-body">
        {m.role}: <TypeTag>{descriptor.accessibility.role}</TypeTag>{" "}
        {descriptor.accessibility.interactive ? m.interactive : m.notInteractive}
      </p>
      {descriptor.accessibility.minTarget === true && (
        <p className="text-caption text-muted-foreground">{m.minTarget}</p>
      )}
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
        <p className="text-body text-muted-foreground">{m.a11yNone}</p>
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
    return <p className="text-body text-muted-foreground">{m.nothing}</p>;
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
    return <p className="text-body text-muted-foreground">{m.nothing}</p>;
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
              {r.required === true && <span className="text-caption text-muted-foreground"> {m.docsRequired}</span>}
              {r.deprecated !== undefined && (
                <span className="text-caption text-muted-foreground"> {m.docsDeprecated}</span>
              )}
              {r.description !== undefined && <p className="text-caption text-muted-foreground">{r.description}</p>}
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
/**
 * The inspector while nothing is selected: the plugin at a glance — how many screens, widgets, state entries and data
 * sources it has and what validation found — over its screens with the entry marked, and how to start editing.
 */
function PluginOverview(props: DockProps & { readonly m: ReturnType<typeof panelMessages> }) {
  const { m } = props;
  const model = useMemo(() => readPlugin(props.documents ?? []), [props.documents]);
  const plugin = useMemo(() => {
    const doc = (props.documents ?? []).find((d) => d.kind === "plugin");
    try {
      return doc === undefined
        ? undefined
        : (JSON.parse(new TextDecoder().decode(doc.content)) as Record<string, unknown>);
    } catch {
      return undefined;
    }
  }, [props.documents]);
  const count = (v: unknown) => (Array.isArray(v) ? v.length : 0);
  const errors = props.problems.filter((x) => x.severity === "error").length;
  const warnings = props.problems.filter((x) => x.severity === "warning").length;
  const figures: readonly {
    key: Parameters<typeof m.overviewFigure>[0];
    value: number;
    icon: IconName;
    tone?: string;
  }[] = [
    { key: "screens", value: props.screens.length, icon: "device-mobile" },
    // The widgets inside the screens, as the left dock counts them: a screen's root is the screen, not a widget.
    { key: "widgets", value: Math.max(0, props.index.parent.size - props.screens.length), icon: "cube" },
    { key: "state", value: count(plugin?.["state"]), icon: "variable" },
    { key: "data", value: count(plugin?.["dataSources"]), icon: "database" },
    { key: "errors", value: errors, icon: "x-circle", ...(errors > 0 ? { tone: "error" } : {}) },
    { key: "warnings", value: warnings, icon: "warning", ...(warnings > 0 ? { tone: "warning" } : {}) },
  ];
  return (
    <section aria-label={m.overview} className="flex flex-col gap-4">
      <p className="inspector-empty">{m.nothing}</p>
      <dl className="overview-figures">
        {figures.map((f) => (
          <div key={f.key} className="overview-figure" data-tone={f.tone}>
            <dt>
              <Icon name={f.icon} size="pill" />
              {m.overviewFigure(f.key)}
            </dt>
            <dd>{f.value}</dd>
          </div>
        ))}
      </dl>
      {props.screens.length > 0 && (
        <div className="flex flex-col gap-1.5">
          <h3 className="inspector-label">{m.overviewScreens}</h3>
          <ul className="flex flex-col gap-1">
            {props.screens.map((x) => (
              <li key={x.id} className="overview-screen">
                <Icon name="device-mobile" size="pill" />
                <span className="min-w-0 flex-1 truncate">{x.title}</span>
                {x.id === model.entryPage && <span className="overview-entry">{m.entry}</span>}
              </li>
            ))}
          </ul>
        </div>
      )}
    </section>
  );
}

/** How many nodes the screen of the selection holds; with nothing selected, the largest screen's count. */
function usePageNodeCount(
  nodes: ReadonlyMap<string, unknown>,
  ids: readonly string[],
  screenOf: ReadonlyMap<string, string>,
): number {
  return useMemo(() => {
    const perScreen = new Map<string | undefined, number>();
    for (const id of nodes.keys()) {
      const screen = screenOf.get(id);
      perScreen.set(screen, (perScreen.get(screen) ?? 0) + 1);
    }
    const own = ids[0] === undefined ? undefined : perScreen.get(screenOf.get(ids[0]));
    return own ?? Math.max(0, ...perScreen.values());
  }, [nodes, ids, screenOf]);
}

/** The inspector folded to a strip: the button that opens it, the selection's mark and how many are selected. */
function FoldedInspector({
  m,
  glyph,
  count,
  onOpen,
}: {
  readonly m: ReturnType<typeof panelMessages>;
  readonly glyph: IconName;
  readonly count: number;
  readonly onOpen: () => void;
}) {
  return (
    <aside
      aria-label={m.dock}
      className="plugin-dock plugin-dock-end plugin-inspector canvas-panel canvas-side glass"
      data-folded=""
    >
      <DockExpand side="end" label={m.openInspector} onExpand={onOpen} />
      <div className="inspector-strip">
        <span className="inspector-glyph" aria-hidden="true">
          <Icon name={glyph} />
        </span>
        {count > 0 && <span className="inspector-count">{count}</span>}
      </div>
    </aside>
  );
}

/** The full-screen editors the inspector opens, with their labels and marks; a missing opener disables one. */
function editorTools(props: DockProps, i18n: I18n) {
  const jsonLabel = jsonEditorMessages(i18n).open;
  const dm = designerMessages(i18n);
  const designerLabels = { data: dm.dataOpen, db: dm.dbOpen, theme: dm.themeOpen };
  const openOverlay = (id: "data" | "db" | "theme") =>
    props.onOpenOverlay === undefined ? undefined : () => props.onOpenOverlay?.(id);
  const graphLabel = actionGraphMessages(i18n).open;
  const tools: readonly {
    id: Parameters<ReturnType<typeof panelMessages>["editorShort"]>[0];
    label: string;
    icon: IconName;
    open: (() => void) | undefined;
  }[] = [
    { id: "json", label: jsonLabel, icon: "braces", open: props.onOpenJson },
    { id: "l10n", label: localisationMessages(i18n).open, icon: "translate", open: props.onOpenLocalisation },
    { id: "state", label: stateMessages(i18n).open, icon: "variable", open: props.onOpenState },
    { id: "data", label: designerLabels.data, icon: "database", open: openOverlay("data") },
    { id: "db", label: designerLabels.db, icon: "table", open: openOverlay("db") },
    { id: "theme", label: designerLabels.theme, icon: "palette", open: openOverlay("theme") },
    { id: "graph", label: graphLabel, icon: "flow-arrow", open: props.onOpenGraph },
    { id: "rust", label: rustEditorMessages(i18n).open, icon: "lightning", open: props.onOpenRust },
  ];
  return tools;
}

export function RightDock(props: DockProps) {
  const { i18n } = useLingui();
  const m = panelMessages(i18n);
  const ids = useEditor((st) => st.selection.ids);
  const tm = tabMessages(i18n);
  const nodes = useLiveNodes(props);
  const pageNodes = usePageNodeCount(nodes, ids, props.screenOf);
  const body = (panel: (ids: readonly string[]) => React.ReactNode) =>
    ids.length === 0 ? <PluginOverview {...props} m={m} /> : panel(ids);
  const tools = editorTools(props, i18n);
  const tabs: readonly { value: string; label: string; icon: IconName; panel: React.ReactNode }[] = [
    {
      value: "properties",
      label: m.tabProperties,
      icon: "sliders",
      panel: body((i) => <PropertiesTab {...props} ids={i} m={m} />),
    },
    {
      value: "actions",
      label: linkMessages(i18n).tab,
      icon: "hand-pointing",
      panel: <ActionsPanel {...props} ids={ids} />,
    },
    {
      value: "json",
      label: tm.tabJson,
      icon: "braces",
      panel: body((i) => <JsonTab ids={i} nodes={nodes} onOpen={props.onOpenJson} />),
    },
    {
      value: "code",
      label: tm.tabCode,
      icon: "code",
      panel: (
        <CodeTab
          ids={ids}
          nodes={nodes}
          documents={props.documents}
          onOpenFunction={props.onOpenFunction}
          tests={props.tests}
        />
      ),
    },
    {
      value: "localisation",
      label: tm.tabLocalisation,
      icon: "translate",
      panel: body((i) => <LocalisationTab ids={i} nodes={nodes} onOpen={props.onOpenLocalisation} />),
    },
    {
      value: "accessibility",
      label: m.tabAccessibility,
      icon: "accessibility",
      panel: body((i) => <AccessibilityTab {...props} ids={i} m={m} />),
    },
    {
      value: "performance",
      label: tm.tabPerformance,
      icon: "gauge",
      panel: <PerformanceTab ids={ids} nodes={nodes} pageNodes={pageNodes} />,
    },
    {
      value: "docs",
      label: m.tabDocs,
      icon: "book",
      panel: body((i) => <DocsTab ids={i} index={props.index} m={m} />),
    },
  ];
  const [folded, setFolded] = useStoredFlag("plux.dock.end.folded", false);
  const [toolsOpen, setToolsOpen] = useStoredFlag("plux.inspector.editors", true);
  const [tabsOpen, setTabsOpen] = useStoredFlag("plux.inspector.details", true);
  const first = ids[0];
  const type = first === undefined ? undefined : props.index.type.get(first);
  const descriptor = type === undefined ? undefined : widget(type);
  const screenTitle = (() => {
    const screen = first === undefined ? undefined : props.screenOf.get(first);
    return props.screens.find((x) => x.id === screen)?.title;
  })();
  const glyph: IconName =
    first === undefined ? "sliders" : descriptor === undefined ? "cube" : typeIcon(descriptor.type);
  if (folded) {
    return <FoldedInspector m={m} glyph={glyph} count={ids.length} onOpen={() => setFolded(false)} />;
  }
  return (
    <aside aria-label={m.dock} className="plugin-dock plugin-dock-end plugin-inspector canvas-panel canvas-side glass">
      <div className="inspector-head">
        <span className="inspector-glyph" aria-hidden="true">
          <Icon name={glyph} />
        </span>
        <div className="flex min-w-0 flex-1 flex-col">
          <strong className="truncate text-sm">
            {first === undefined ? m.dock : ids.length > 1 ? m.multiTitle(ids.length) : (type ?? first)}
          </strong>
          {ids.length === 1 ? (
            <span className="inspector-sub truncate font-mono">{first}</span>
          ) : (
            <span className="inspector-sub truncate">
              {first === undefined ? m.overview : [...new Set(ids.map((i) => props.index.type.get(i) ?? i))].join(", ")}
            </span>
          )}
        </div>
        {ids.length > 1 ? (
          <span className="inspector-count">{ids.length}</span>
        ) : (
          screenTitle !== undefined && <span className="inspector-screen">{screenTitle}</span>
        )}
        <button
          type="button"
          className="inspector-fold"
          aria-label={m.foldInspector}
          title={m.foldInspector}
          onClick={() => setFolded(true)}
        >
          <Icon name="panel-end-close" />
        </button>
      </div>
      <TooltipProvider delay={tooltipDelayMs}>
        <Collapsible open={toolsOpen} onOpenChange={setToolsOpen} className="inspector-tools">
          <CollapsibleTrigger className="inspector-section-head">
            <Icon name="caret-down" size="pill" className="inspector-section-caret" />
            <span className="flex-1 text-start">{m.editors}</span>
            <span className="inspector-section-count">{tools.filter((t) => t.open !== undefined).length}</span>
          </CollapsibleTrigger>
          <CollapsibleContent>
            <section aria-label={m.editors} className="inspector-tool-grid">
              {tools
                .filter((t) => t.open !== undefined)
                .map((t) => (
                  <Tooltip key={t.id}>
                    <TooltipTrigger
                      render={<button type="button" className="inspector-tool" aria-label={t.label} onClick={t.open} />}
                    >
                      <Icon name={t.icon} />
                      <span className="inspector-tool-name" aria-hidden="true">
                        {m.editorShort(t.id)}
                      </span>
                    </TooltipTrigger>
                    <TooltipContent>{t.label}</TooltipContent>
                  </Tooltip>
                ))}
            </section>
          </CollapsibleContent>
        </Collapsible>
      </TooltipProvider>
      <Collapsible open={tabsOpen} onOpenChange={setTabsOpen} className="inspector-details">
        <CollapsibleTrigger className="inspector-section-head">
          <Icon name="caret-down" size="pill" className="inspector-section-caret" />
          <span className="flex-1 text-start">{m.detailsSection}</span>
        </CollapsibleTrigger>
        <CollapsibleContent className="inspector-details-body">
          <ShadcnTabs defaultValue="properties" className="inspector-tabs-root">
            <TabsList className="inspector-tabs">
              {tabs.map((t) => (
                <TabsTrigger key={t.value} value={t.value} className="inspector-tab">
                  <Icon name={t.icon} size="pill" />
                  <span className="truncate">{t.label}</span>
                </TabsTrigger>
              ))}
            </TabsList>
            {tabs.map((t) => (
              <TabsContent key={t.value} value={t.value} className="inspector-body">
                {t.panel}
              </TabsContent>
            ))}
          </ShadcnTabs>
        </CollapsibleContent>
      </Collapsible>
    </aside>
  );
}
