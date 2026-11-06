// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";
import { useLingui } from "@lingui/react";
import { enums, type PropDescriptor, valueTypes, widget } from "@plux/schema";
import { Button, Icon, type IconName, Input, Menu, Select, TypeTag, tooltipDelayMs } from "@plux/ui";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@plux/ui/components/collapsible";
import { InputGroup, InputGroupAddon, InputGroupInput } from "@plux/ui/components/input-group";
import { Switch } from "@plux/ui/components/switch";
import { TooltipProvider } from "@plux/ui/components/tooltip";
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
import { DockTabs, type Pane, useStoredFlag } from "./dock-tabs.tsx";
import { IconField } from "./icon-field.tsx";
import { ImageSourceField } from "./image-field.tsx";
import { pageSettingsMessages } from "./page-settings.messages.ts";
import { PageSettingsTab } from "./page-settings.tsx";
import { panelMessages, panelsMessages } from "./panels.messages.ts";
import { AlignmentPicker, PropGroupHead, PropRow, propLabel, SegmentedChoice, segmentable } from "./prop-row.tsx";
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
import "./inspector.css";

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
    <div className="prop-pxl">
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
      {errors[0] !== undefined ? (
        <p className="prop-pxl-error" role="alert">
          {errors[0]}
        </p>
      ) : (
        description !== undefined && <p className="prop-pxl-note">{description}</p>
      )}
    </div>
  );
}

type TextMode = "literal" | "t" | "expr";

const modeIcons: Record<TextMode, IconName> = { literal: "text-t", t: "translate", expr: "code" };

/** The button at the end of a text field that turns it into a literal, a translation key or a PXL expression. */
function ModeMenu(p: {
  readonly label: string;
  readonly shown: TextMode;
  readonly modes: readonly { readonly value: TextMode; readonly label: string }[];
  readonly disabled: boolean;
  readonly onMode: (mode: TextMode) => void;
}) {
  const current = p.modes.find((x) => x.value === p.shown)?.label ?? "";
  return (
    <Menu
      trigger={
        <button
          type="button"
          className="prop-mode"
          data-mode={p.shown}
          aria-label={p.label}
          title={current}
          disabled={p.disabled}
        >
          <Icon name={modeIcons[p.shown]} size="pill" />
        </button>
      }
      entries={p.modes.map((x) => ({
        kind: "item" as const,
        id: x.value,
        label: x.label,
        onSelect: () => p.onMode(x.value),
      }))}
    />
  );
}

/**
 * A binding-aware text editor in one row: the literal (or the translation key) with a button at its end that switches
 * between a literal, a translation key and a PXL expression; an expression opens its editor under the row.
 */
function TextEditor(p: EditorProps) {
  const b = bindingOf(p.value);
  const mode: TextMode = b?.kind === "t" ? "t" : b?.kind === "expr" ? "expr" : "literal";
  const [forced, setForced] = useState<TextMode | null>(null);
  const shown = forced ?? mode;
  const modes: { value: TextMode; label: string }[] = [
    { value: "literal", label: p.m.literal },
    ...(p.sources.keys.length > 0 ? [{ value: "t" as const, label: p.m.useKey }] : []),
    { value: "expr", label: p.m.pxl },
  ];
  return (
    <div className="prop-text" data-mode={shown}>
      <div className="prop-inline">
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
        <ModeMenu
          label={p.m.modeOf(p.name, modes.find((x) => x.value === shown)?.label ?? "")}
          shown={shown}
          modes={modes}
          disabled={p.readOnly}
          onMode={(next) => setForced(next === mode ? null : next)}
        />
      </div>
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
    <div className="prop-number">
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
      {p.unit !== "" && <span className="prop-unit">{p.unit}</span>}
    </div>
  );
}

function BoolEditor(p: EditorProps) {
  return (
    <span className="prop-switch">
      <Switch
        size="sm"
        aria-label={p.name}
        checked={p.value === true}
        disabled={p.readOnly}
        onCheckedChange={(on) => {
          p.onChange(on);
          p.onEnd();
        }}
      />
      {p.mixed && <span className="text-caption text-tertiary">{p.m.mixed}</span>}
    </span>
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
    <div className="prop-insets">
      {insetSides.map((side) => (
        <span key={side} className="prop-inset" title={labels[side]}>
          <span className="prop-inset-side" aria-hidden="true">
            {labels[side].charAt(0)}
          </span>
          <Input
            type="number"
            aria-label={`${p.name} ${labels[side]}`}
            value={typeof o[side] === "number" ? o[side] : ""}
            placeholder={p.mixed ? p.m.mixed : "0"}
            readOnly={p.readOnly}
            onChange={(e) => change(side, e.target.value)}
            onBlur={p.onEnd}
          />
        </span>
      ))}
    </div>
  );
}

/** Alignment constants and enum values are both a pick from named options: a segmented control when they are few. */
function ChoiceEditor(p: EditorProps & { names: readonly string[]; label: string }) {
  if (segmentable(p.names) && !p.mixed) {
    return (
      <SegmentedChoice
        label={p.label}
        names={p.names}
        value={typeof p.value === "string" ? p.value : undefined}
        readOnly={p.readOnly}
        onChange={(v) => {
          p.onChange(v);
          p.onEnd();
        }}
      />
    );
  }
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
        variant="ghost"
        size="sm"
        className="prop-add"
        disabled={p.readOnly}
        onClick={() => {
          set([...items, emptyItem(item)]);
          p.onEnd();
        }}
      >
        <Icon name="plus" size="pill" />
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
    <div className="prop-grid prop-grid-nested">
      {fields.map((f) => (
        <PropRow
          key={f.name}
          name={f.name}
          set={o[f.name] !== undefined}
          wide={wideType(f.type)}
          resetLabel={p.m.clearProp(f.name)}
          onReset={p.readOnly ? undefined : () => change(f.name, undefined)}
        >
          <ValueEditor
            {...p}
            type={f.type}
            name={`${p.name}.${f.name}`}
            value={o[f.name]}
            mixed={false}
            unit=""
            onChange={(nv) => change(f.name, nv)}
          />
        </PropRow>
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
    case "alignment": {
      const names = constantsOf("Alignment").map((c) => c.name);
      const label = `${p.name}: ${p.m.alignment}`;
      return (
        <AlignmentPicker
          label={label}
          names={names}
          value={typeof p.value === "string" ? p.value : undefined}
          readOnly={p.readOnly}
          onChange={(v) => {
            p.onChange(v);
            p.onEnd();
          }}
          more={<ChoiceEditor {...p} label={label} names={names} />}
        />
      );
    }
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
  /** A request to show a pane (the canvas's toolbar opens Actions, PC-19): a new `n` shows it again. */
  readonly tabRequest?: { readonly tab: string; readonly n: number } | undefined;
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

/** One property of the selection in the inspector: its row, its editor, its reset and its "extract to a key". */
function PropertyField(p: {
  readonly prop: PropDescriptor;
  readonly sv: ReturnType<typeof sharedValue>;
  readonly set: boolean;
  readonly bp: Breakpoint;
  readonly canEdit: boolean;
  /** Whether a translatable literal may be moved to a translation key here (one widget, editable, catalogues open). */
  readonly extractable: boolean;
  readonly m: Messages;
  readonly extractLabel: string;
  readonly sources: Sources;
  readonly onApply: (value: unknown) => void;
  readonly onEnd: () => void;
  readonly onExtract: (text: string) => void;
}) {
  const { prop, sv, m } = p;
  const value = sv.mixed ? undefined : sv.value;
  const overridden = !sv.mixed && sv.overridden;
  const literal = typeof value === "string" && value !== "" ? value : undefined;
  return (
    <PropRow
      name={prop.name}
      set={p.set}
      override={overridden ? overrideMark[p.bp] : undefined}
      wide={wideType(prop.type)}
      resetLabel={overridden ? m.resetOf(prop.name) : m.clearProp(prop.name)}
      onReset={
        p.canEdit
          ? () => {
              p.onApply(undefined);
              p.onEnd();
            }
          : undefined
      }
      extra={
        p.extractable && isTranslatable(prop) && literal !== undefined ? (
          <button
            type="button"
            className="prop-mode"
            aria-label={p.extractLabel}
            title={p.extractLabel}
            onClick={() => p.onExtract(literal)}
          >
            <Icon name="globe" size="pill" />
          </button>
        ) : undefined
      }
    >
      <ValueEditor
        type={prop.type}
        name={prop.name}
        value={value}
        mixed={sv.mixed}
        unit={unitOf(prop)}
        readOnly={!p.canEdit}
        sources={p.sources}
        m={m}
        onChange={p.onApply}
        onEnd={p.onEnd}
      />
    </PropRow>
  );
}

/** The mark of a size class whose override a property shows. */
const overrideMark: Record<Breakpoint, string | undefined> = { compact: undefined, medium: "M", expanded: "E" };

/** A property whose editor takes the inspector's width under its name: decorations, text styles, objects, lists. */
function wideType(type: string): boolean {
  const kind = editorKind(type, enumNames);
  const bare = type.replace(/\?$/, "");
  return kind === "list" || (kind === "object" && bare !== "IconData" && bare !== "ImageSource");
}

/** The size class an edit applies to, as a segmented control (WCAG radio group; arrows move, in reading order). */
function SizeClasses(props: {
  readonly bp: Breakpoint;
  readonly labels: Record<Breakpoint, string>;
  readonly label: string;
  readonly onChange: (bp: Breakpoint) => void;
}) {
  const icons: Record<Breakpoint, IconName> = { compact: "device-mobile", medium: "columns", expanded: "monitor" };
  return (
    // biome-ignore lint/a11y/useSemanticElements: a labelled radio group of size classes, not a form's fieldset
    <div
      role="radiogroup"
      aria-label={props.label}
      className="size-classes"
      onKeyDown={(e) => {
        const step = { ArrowRight: 1, ArrowDown: 1, ArrowLeft: -1, ArrowUp: -1 }[e.key];
        if (step === undefined) {
          return;
        }
        e.preventDefault();
        const horizontal = e.key === "ArrowLeft" || e.key === "ArrowRight";
        const rtl = horizontal && getComputedStyle(e.currentTarget).direction === "rtl";
        const at = (breakpoints.indexOf(props.bp) + (rtl ? -step : step) + breakpoints.length) % breakpoints.length;
        props.onChange(breakpoints[at] ?? props.bp);
        e.currentTarget.querySelectorAll<HTMLElement>("[role=radio]")[at]?.focus();
      }}
    >
      {breakpoints.map((b) => (
        // biome-ignore lint/a11y/useSemanticElements: a segmented control's option, styled as a pill
        <button
          key={b}
          type="button"
          role="radio"
          aria-checked={props.bp === b}
          aria-label={props.labels[b]}
          title={`${props.label}: ${props.labels[b]}`}
          tabIndex={props.bp === b ? 0 : -1}
          className="size-class"
          onClick={() => props.onChange(b)}
        >
          <Icon name={icons[b]} size="pill" />
          <span className="size-class-name">{props.labels[b]}</span>
        </button>
      ))}
    </div>
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
  const [filter, setFilter] = useState("");
  const [setOnly, setSetOnly] = useStoredFlag("plux.inspector.setOnly", false);

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
  const q = filter.trim().toLowerCase();
  const rows = props.flatMap((prop) => {
    const sv = sharedValue(selected, prop.name, bp);
    const set = !sv.mixed && sv.value !== undefined;
    const shown =
      (q === "" || prop.name.toLowerCase().includes(q) || propLabel(prop.name).toLowerCase().includes(q)) &&
      (!setOnly || set || sv.mixed);
    return shown ? [{ prop, sv, set }] : [];
  });
  const sections = propGroups(rows.map((r) => r.prop)).map((g) => ({
    ...g,
    rows: g.props.flatMap((pr) => rows.find((r) => r.prop === pr) ?? []),
  }));
  return (
    <div className="prop-pane">
      <div className="prop-toolbar">
        <InputGroup className="prop-filter">
          <InputGroupAddon>
            <Icon name="magnifying-glass" />
          </InputGroupAddon>
          <InputGroupInput
            aria-label={m.filterProps}
            placeholder={m.filterProps}
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
          />
        </InputGroup>
        <div className="prop-toolbar-row">
          <SizeClasses bp={bp} labels={labels} label={m.breakpoint} onChange={setBp} />
          <button type="button" className="prop-set-only" aria-pressed={setOnly} onClick={() => setSetOnly(!setOnly)}>
            <span className="prop-dot" aria-hidden="true" />
            {m.setOnly}
          </button>
        </div>
      </div>
      <LockedNote text={lockedNote} />
      {!canEdit && lockedNote === null && <p className="prop-note">{m.readOnly}</p>}
      {ids.length > 1 && <p className="prop-note">{m.multi(ids.length)}</p>}
      {ids.length === 1 && q === "" && !setOnly && (
        <InstanceOverrides node={selected[0]} doc={p.screenOf.get(first)} edit={p.edit} canEdit={canEdit} />
      )}
      {sections.length === 0 && <p className="prop-note">{m.noPropMatch}</p>}
      {sections.map((group) => (
        <Collapsible key={group.id} defaultOpen className="prop-section">
          <CollapsibleTrigger className="prop-section-head">
            <PropGroupHead
              icon={groupIcons[group.id]}
              label={groupLabels[group.id]}
              set={group.rows.filter((r) => r.set).length}
              count={group.rows.length}
            />
          </CollapsibleTrigger>
          <CollapsibleContent className="prop-grid">
            {group.rows.map(({ prop, sv, set }) => (
              <PropertyField
                key={prop.name}
                prop={prop}
                sv={sv}
                set={set}
                bp={bp}
                canEdit={canEdit}
                extractable={l10n !== null && canEdit && ids.length === 1}
                m={m}
                extractLabel={lm.extract}
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
                onApply={(v) => apply(prop.name, v)}
                onEnd={() => {
                  burst.current += 1;
                }}
                onExtract={(text) => setExtracting({ name: prop.name, text })}
              />
            ))}
          </CollapsibleContent>
        </Collapsible>
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
    <div className="prop-pane">
      <LockedNote text={lockedNote} />
      {!canEdit && lockedNote === null && <p className="prop-note">{m.readOnly}</p>}
      <div className="a11y-role">
        <span className="a11y-role-name">{descriptor.accessibility.role}</span>
        <span className="text-xs text-muted-foreground">
          {m.role} · {descriptor.accessibility.interactive ? m.interactive : m.notInteractive}
        </span>
      </div>
      {descriptor.accessibility.minTarget === true && <p className="prop-note">{m.minTarget}</p>}
      <div className="prop-grid">
        {(["label", "hint"] as const).map((f) => (
          <PropRow
            key={f}
            name={f}
            label={f === "label" ? m.semLabel : m.semHint}
            set={typeof sem[f] === "string"}
            resetLabel=""
          >
            <Input
              aria-label={f === "label" ? m.semLabel : m.semHint}
              value={typeof sem[f] === "string" ? sem[f] : ""}
              readOnly={!canEdit}
              onChange={(e) => set(f, e.target.value === "" ? undefined : e.target.value)}
              onBlur={() => {
                burst.current += 1;
              }}
            />
          </PropRow>
        ))}
        {flags.map(([f, label]) => (
          <PropRow key={f} name={f} label={label} set={sem[f] === true} resetLabel="">
            <span className="prop-switch">
              <Switch
                size="sm"
                aria-label={label}
                checked={sem[f] === true}
                disabled={!canEdit}
                onCheckedChange={(on) => {
                  set(f, on ? true : undefined);
                  burst.current += 1;
                }}
              />
            </span>
          </PropRow>
        ))}
      </div>
      <h3 className="pane-head">{m.a11yProblems}</h3>
      {found.length === 0 ? (
        <p className="prop-note">{m.a11yNone}</p>
      ) : (
        <ul className="flex flex-col gap-1.5">
          {found.map((x) => (
            <li key={`${x.code}${x.path}`} className="a11y-problem">
              <TypeTag>{x.code}</TypeTag>
              <span>{x.message}</span>
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
    rows: readonly { name: string; type?: string; description?: string; required?: boolean; deprecated?: unknown }[],
  ) =>
    rows.length === 0 ? null : (
      <section className="flex flex-col gap-1">
        <h3 className="pane-head">{title}</h3>
        <ul className="doc-list">
          {rows.map((r) => (
            <li key={r.name} className="doc-item">
              <div className="doc-line">
                <code className="doc-name">{r.name}</code>
                {"type" in r && typeof r.type === "string" && <span className="doc-type">{r.type}</span>}
                {r.required === true && <span className="doc-flag">{m.docsRequired}</span>}
                {r.deprecated !== undefined && <span className="doc-flag">{m.docsDeprecated}</span>}
              </div>
              {r.description !== undefined && <p className="doc-what">{r.description}</p>}
            </li>
          ))}
        </ul>
      </section>
    );
  return (
    <div className="flex flex-col gap-3">
      <div className="doc-intro">
        <h2 className="doc-title">{d.type}</h2>
        <p className="text-sm">{d.description}</p>
      </div>
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

type Tool = ReturnType<typeof editorTools>[number];

/** The full-screen editors, grouped: data (JSON, state, data sources, database), logic, content and look. */
const editorGroups: readonly { readonly id: "data" | "logic" | "content"; readonly tools: readonly Tool["id"][] }[] = [
  { id: "content", tools: ["json", "l10n", "theme"] },
  { id: "data", tools: ["state", "data", "db"] },
  { id: "logic", tools: ["graph", "rust"] },
];

/** The Editors pane: every full-screen editor as a row with its mark, name and what it edits, in three groups. */
function EditorsPane({ tools, m }: { readonly tools: readonly Tool[]; readonly m: Messages }) {
  const heads = { data: m.editorGroupData, logic: m.editorGroupLogic, content: m.editorGroupContent };
  return (
    <div className="editors-pane">
      <p className="prop-note">{m.editorsIntro}</p>
      {editorGroups.map((g) => {
        const items = g.tools.flatMap((id) => tools.find((t) => t.id === id && t.open !== undefined) ?? []);
        return items.length === 0 ? null : (
          <section key={g.id} aria-label={heads[g.id]} className="editors-group">
            <h3 className="editors-head">{heads[g.id]}</h3>
            {items.map((t) => (
              <button key={t.id} type="button" className="editor-launch" aria-label={t.label} onClick={t.open}>
                <span className="editor-launch-mark" aria-hidden="true">
                  <Icon name={t.icon} />
                </span>
                <span className="editor-launch-text">
                  <span className="editor-launch-name">{m.editorShort(t.id)}</span>
                  <span className="editor-launch-what">{t.label}</span>
                </span>
                <Icon name="arrow-up-right" size="pill" className="editor-launch-go" />
              </button>
            ))}
          </section>
        );
      })}
    </div>
  );
}

/** The inspector's header: what is selected (its mark, type or count, id) and the screen it is on. */
function InspectorHead(props: {
  readonly ids: readonly string[];
  readonly index: NodeIndex;
  readonly type: string | undefined;
  readonly screenTitle: string | undefined;
  readonly m: Messages;
}) {
  const { ids, m } = props;
  const first = ids[0];
  const title = first === undefined ? m.overview : ids.length > 1 ? m.multiTitle(ids.length) : (props.type ?? first);
  const sub =
    first === undefined
      ? undefined
      : ids.length === 1
        ? first
        : [...new Set(ids.map((i) => props.index.type.get(i) ?? i))].join(", ");
  return (
    <div className="inspector-id">
      <div className="flex min-w-0 flex-1 flex-col">
        <span className="dock-head-kicker">{first === undefined ? m.dock : (props.screenTitle ?? m.dock)}</span>
        <strong className="dock-head-title">{title}</strong>
        {sub !== undefined && (
          <span className={ids.length === 1 ? "inspector-sub truncate font-mono" : "inspector-sub truncate"}>
            {sub}
          </span>
        )}
      </div>
      {ids.length > 1 && <span className="inspector-count">{ids.length}</span>}
    </div>
  );
}

/** With nothing selected, the Page pane shows the first screen's page. */
function firstRoot(screens: DockProps["screens"]): readonly string[] {
  const root = screens[0]?.root;
  return isObj(root) && typeof root["id"] === "string" ? [root["id"]] : [];
}

/**
 * The inspector (STU-062): a dock along the canvas's end whose rail lists its panes — Properties first and open, then
 * Editors (the full-screen editors, STU-063), Actions, JSON, Code, Localisation, Accessibility, Performance and Docs —
 * beside a header naming the selection, as the left dock does on its side. The open pane takes the dock's whole height.
 */
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
  const panes: readonly Pane[] = [
    {
      value: "properties",
      label: m.tabProperties,
      icon: "sliders",
      panel: body((i) => <PropertiesTab {...props} ids={i} m={m} />),
    },
    {
      value: "page",
      label: pageSettingsMessages(i18n).tab,
      icon: "file",
      panel: (
        <PageSettingsTab
          ids={ids.length > 0 ? ids : firstRoot(props.screens)}
          screenOf={props.screenOf}
          pathOf={props.pathOf}
          documents={props.documents}
          appDocuments={props.appDocuments}
          onOpenState={props.onOpenState}
          onOpenData={props.onOpenOverlay === undefined ? undefined : () => props.onOpenOverlay?.("data")}
          onOpenGraph={props.onOpenGraph}
        />
      ),
    },
    {
      value: "editors",
      label: m.editorsTab,
      icon: "stack",
      meta: String(tools.filter((t) => t.open !== undefined).length),
      panel: <EditorsPane tools={tools} m={m} />,
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
  const [tab, setTab] = useState("properties");
  const request = props.tabRequest;
  useEffect(() => {
    if (request !== undefined) {
      setTab(request.tab);
      setFolded(false);
    }
  }, [request, setFolded]);
  const first = ids[0];
  const type = first === undefined ? undefined : props.index.type.get(first);
  const descriptor = type === undefined ? undefined : widget(type);
  const screenTitle = (() => {
    const screen = first === undefined ? undefined : props.screenOf.get(first);
    return props.screens.find((x) => x.id === screen)?.title;
  })();
  const glyph: IconName =
    first === undefined ? "sliders" : descriptor === undefined ? "cube" : typeIcon(descriptor.type);
  return (
    <aside
      aria-label={m.dock}
      className="plugin-dock plugin-dock-end plugin-inspector canvas-panel canvas-side glass"
      data-folded={folded ? "" : undefined}
    >
      <TooltipProvider delay={tooltipDelayMs}>
        <DockTabs
          side="end"
          panes={panes}
          defaultValue="properties"
          value={tab}
          onValueChange={setTab}
          mark={glyph}
          folded={folded}
          onFold={setFolded}
          foldLabel={m.foldInspector}
          unfoldLabel={m.openInspector}
          head={<InspectorHead ids={ids} index={props.index} type={type} screenTitle={screenTitle} m={m} />}
        />
      </TooltipProvider>
    </aside>
  );
}
