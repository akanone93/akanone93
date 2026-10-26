// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { AppService, PluginService, type PluginSummary } from "@plux/api";
import { Button, Dialog, Field, Input, StatusPill, Textarea } from "@plux/ui";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate, useParams } from "@tanstack/react-router";
import { type ComponentType, type ReactNode, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useApp } from "../../lib/use-app.tsx";
import { EditorProvider, useEditSession } from "../editor/editor-provider.tsx";
import { uuidv7 } from "../plugin-canvas/screens.ts";
import { usePluginDocuments } from "../plugin-canvas/use-plugin-documents.ts";
import { appCanvasMessages } from "./app-canvas.messages.ts";
import {
  APP_CANVAS_PATH,
  type ArrowLayerProps,
  BOX,
  type BoxLink,
  canvasDocument,
  canvasRecipe,
  type Frame,
  fitViewport,
  type Guide,
  marquee,
  type PlacedBox,
  type Point,
  placeBoxes,
  pluginKeyFrom,
  readSavedCanvas,
  type SavedCanvas,
  select,
  snap,
  type Viewport,
} from "./app-canvas.model.ts";
import { PluginSafety, type SafetyMode } from "./plugin-safety.tsx";
import { TrashDialog } from "./trash-dialog.tsx";

/** The routing step supplies the arrow layer here (S3.2); until then nothing is drawn between boxes. */
export type ArrowLayer = ComponentType<ArrowLayerProps>;

type Dialogue = { kind: "new" | "settings" | "duplicate" | "entry" | "frame" } | null;

const decoder = new TextDecoder();

function parseJson(bytes: Uint8Array): unknown {
  try {
    return JSON.parse(decoder.decode(bytes));
  } catch {
    return undefined;
  }
}

export interface AppCanvasPageProps {
  readonly arrows?: ArrowLayer | undefined;
}

export function AppCanvasPage(props: AppCanvasPageProps): ReactNode {
  const params = useParams({ strict: false }) as { app?: string };
  const appId = params.app ?? "";
  const docs = usePluginDocuments(appId, "");
  return (
    <EditorProvider appId={appId} pluginId="" documents={docs.data}>
      <AppCanvasView appId={appId} documents={docs.data} arrows={props.arrows} />
    </EditorProvider>
  );
}

interface ViewProps {
  readonly appId: string;
  readonly documents: ReturnType<typeof usePluginDocuments>["data"];
  readonly arrows?: ArrowLayer | undefined;
}

function AppCanvasView({ appId, documents, arrows: Arrows }: ViewProps): ReactNode {
  const { i18n } = useLingui();
  const m = appCanvasMessages(i18n);
  const { session } = useApp();
  const navigate = useNavigate();
  const params = useParams({ strict: false }) as { org?: string };
  const queryClient = useQueryClient();
  const edit = useEditSession();
  const plugins = useMemo(() => createClient(PluginService, session.transport), [session]);
  const apps = useMemo(() => createClient(AppService, session.transport), [session]);

  // One bounded call per app (plan §4.3 #14); refreshed when the window regains focus, because the
  // events stream carries no plugin-level events yet.
  const summaries = useQuery({
    queryKey: ["plugin-summaries", appId],
    queryFn: async () => (await plugins.listPluginSummaries({ appId })).summaries,
    refetchOnWindowFocus: true,
  });
  const app = useQuery({ queryKey: ["app", appId], queryFn: async () => (await apps.getApp({ id: appId })).app });
  const list = summaries.data ?? [];
  const entryKey = app.data?.defaultPluginKey ?? "";

  const saved = useMemo(() => {
    const doc = documents?.find((d) => d.path === APP_CANVAS_PATH);
    return readSavedCanvas(doc === undefined ? undefined : parseJson(doc.content));
  }, [documents]);
  const links = useMemo<BoxLink[]>(() => {
    const idOf = new Map(list.map((s) => [s.plugin?.key ?? "", s.plugin?.id ?? ""]));
    return list.flatMap((s) =>
      s.linksOut.flatMap((to) => {
        const target = idOf.get(to);
        return target === undefined || s.plugin === undefined ? [] : [{ from: s.plugin.id, to: target }];
      }),
    );
  }, [list]);
  const [positions, setPositions] = useState<ReadonlyMap<string, Point>>(new Map());
  const [frames, setFrames] = useState<readonly Frame[]>([]);
  useEffect(() => {
    setPositions(saved.positions);
    setFrames(saved.frames);
  }, [saved]);
  const boxes = useMemo(
    () =>
      placeBoxes(
        list.flatMap((s) => (s.plugin === undefined ? [] : [{ id: s.plugin.id, key: s.plugin.key }])),
        positions,
        links,
      ),
    [list, positions, links],
  );
  const summaryOf = useMemo(() => new Map(list.map((s) => [s.plugin?.id ?? "", s])), [list]);

  const [selected, setSelected] = useState<ReadonlySet<string>>(new Set());
  const [hover, setHover] = useState<string | undefined>();
  const [guides, setGuides] = useState<readonly Guide[]>([]);
  const [view, setView] = useState<Viewport>({ x: 0, y: 0, z: 1 });
  const [message, setMessage] = useState<string | null>(null);
  const [dialogue, setDialogue] = useState<Dialogue>(null);
  const [safety, setSafety] = useState<SafetyMode | null>(null);
  const [trashOpen, setTrashOpen] = useState(false);
  const stage = useRef<HTMLDivElement>(null);

  const persist = useCallback(
    (next: SavedCanvas) => {
      const exists = documents?.some((d) => d.path === APP_CANVAS_PATH) === true;
      if (exists) {
        edit.edit(APP_CANVAS_PATH, canvasRecipe(next), { group: "canvas" });
      } else {
        void edit.create(APP_CANVAS_PATH, canvasDocument(uuidv7(), next)).then((r) => {
          if (!r.ok) {
            setMessage(m.failed);
          }
        });
      }
    },
    [documents, edit, m.failed],
  );
  const commit = useCallback(
    (nextPositions: ReadonlyMap<string, Point>, nextFrames: readonly Frame[]) => {
      setPositions(nextPositions);
      setFrames(nextFrames);
      persist({ positions: nextPositions, frames: nextFrames });
    },
    [persist],
  );

  const fit = useCallback(() => {
    const el = stage.current;
    setView(fitViewport(boxes, { width: el?.clientWidth ?? 0, height: el?.clientHeight ?? 0 }));
  }, [boxes]);
  const fitted = useRef(false);
  useEffect(() => {
    if (!fitted.current && boxes.length > 0) {
      fitted.current = true;
      fit();
    }
  }, [boxes, fit]);

  const arrange = useCallback(() => {
    const laid = placeBoxes(
      boxes.map((b) => ({ id: b.id, key: b.key })),
      new Map(),
      links,
    );
    commit(new Map(laid.map((b) => [b.id, { x: b.x, y: b.y }])), frames);
  }, [boxes, links, frames, commit]);

  // Dragging: every selected box moves with the pointer; the dragged one snaps to the others.
  const drag = useRef<{ id: string; start: Point; origin: ReadonlyMap<string, Point>; moved: boolean } | null>(null);
  const onBoxDown = (e: React.PointerEvent, box: PlacedBox) => {
    e.stopPropagation();
    const additive = e.shiftKey || e.metaKey || e.ctrlKey;
    const next = selected.has(box.id) && !additive ? selected : select(selected, box.id, additive);
    setSelected(next);
    drag.current = {
      id: box.id,
      start: { x: e.clientX, y: e.clientY },
      origin: new Map(boxes.filter((b) => next.has(b.id)).map((b) => [b.id, { x: b.x, y: b.y }])),
      moved: false,
    };
    (e.currentTarget as Element).setPointerCapture?.(e.pointerId);
  };
  const onMove = (e: React.PointerEvent) => {
    const d = drag.current;
    if (d === null) {
      return;
    }
    const dx = (e.clientX - d.start.x) / view.z;
    const dy = (e.clientY - d.start.y) / view.z;
    if (!d.moved && Math.abs(dx) + Math.abs(dy) < 3) {
      return;
    }
    d.moved = true;
    const lead = d.origin.get(d.id);
    if (lead === undefined) {
      return;
    }
    const others = boxes.filter((b) => !d.origin.has(b.id));
    const s = snap({ x: lead.x + dx, y: lead.y + dy }, others);
    setGuides(s.guides);
    const ox = s.point.x - lead.x;
    const oy = s.point.y - lead.y;
    setPositions((prev) => {
      const next = new Map(prev.size === 0 ? boxes.map((b) => [b.id, { x: b.x, y: b.y }] as const) : prev);
      for (const b of boxes) {
        next.set(b.id, next.get(b.id) ?? { x: b.x, y: b.y });
      }
      for (const [id, o] of d.origin) {
        next.set(id, { x: o.x + ox, y: o.y + oy });
      }
      return next;
    });
  };
  const onUp = () => {
    const d = drag.current;
    drag.current = null;
    setGuides([]);
    if (d?.moved === true) {
      const all = new Map(boxes.map((b) => [b.id, { x: b.x, y: b.y }]));
      commit(all, frames);
    }
  };

  // Marquee on the background; panning is the wheel / the x, y, z of the viewport (zoom to fit).
  const [rubber, setRubber] = useState<{ from: Point; to: Point } | null>(null);
  const world = (e: React.PointerEvent): Point => {
    const r = stage.current?.getBoundingClientRect();
    return { x: view.x + (e.clientX - (r?.left ?? 0)) / view.z, y: view.y + (e.clientY - (r?.top ?? 0)) / view.z };
  };
  const rect = rubber && {
    x: Math.min(rubber.from.x, rubber.to.x),
    y: Math.min(rubber.from.y, rubber.to.y),
    width: Math.abs(rubber.to.x - rubber.from.x),
    height: Math.abs(rubber.to.y - rubber.from.y),
  };

  const refresh = () => queryClient.invalidateQueries({ queryKey: ["plugin-summaries", appId] });
  const mutation = useMutation({
    mutationFn: async (run: () => Promise<unknown>) => run(),
    onSuccess: async () => {
      setMessage(null);
      setDialogue(null);
      await Promise.all([refresh(), queryClient.invalidateQueries({ queryKey: ["app", appId] })]);
    },
    onError: () => setMessage(m.failed),
  });

  const one = selected.size === 1 ? summaryOf.get([...selected][0] ?? "") : undefined;
  const open = (b: PlacedBox) =>
    void navigate({
      to: "/o/$org/apps/$app/plugins/$plugin",
      params: { org: params.org ?? "", app: appId, plugin: b.id },
    });
  const detail = summaryOf.get(hover ?? [...selected][0] ?? "");

  return (
    <section aria-label={m.title} className="relative flex h-full w-full flex-col bg-canvas">
      <div role="toolbar" aria-label={m.toolbar} className="flex items-center gap-2 p-2">
        <Button onClick={() => setDialogue({ kind: "new" })}>{m.newPlugin}</Button>
        <Button disabled={selected.size === 0} onClick={() => setDialogue({ kind: "frame" })}>
          {m.newFrame}
        </Button>
        <Button onClick={arrange}>{m.autoLayout}</Button>
        <Button onClick={fit}>{m.zoomToFit}</Button>
        <Button disabled={one === undefined} onClick={() => setDialogue({ kind: "settings" })}>
          {m.settings}
        </Button>
        <Button disabled={one === undefined} onClick={() => setDialogue({ kind: "duplicate" })}>
          {m.duplicate}
        </Button>
        <Button
          disabled={one?.plugin === undefined}
          onClick={() => {
            const p = one?.plugin;
            if (p?.archived === true) {
              mutation.mutate(() => plugins.archivePlugin({ id: p.id, archived: false }));
            } else if (p !== undefined) {
              setSafety("archive");
            }
          }}
        >
          {one?.plugin?.archived === true ? m.restore : m.archive}
        </Button>
        <Button disabled={one?.plugin === undefined} onClick={() => setSafety("delete")}>
          {m.deletePlugin}
        </Button>
        <Button onClick={() => setTrashOpen(true)}>{m.trash}</Button>
        <Button disabled={one === undefined} onClick={() => setDialogue({ kind: "entry" })}>
          {m.setEntry}
        </Button>
        <output aria-live="polite" className="text-data text-secondary">
          {message ?? m.selected(selected.size)}
        </output>
      </div>
      <div
        ref={stage}
        className="relative flex-1 overflow-hidden"
        onPointerDown={(e) => {
          setSelected(new Set());
          setRubber({ from: world(e), to: world(e) });
        }}
        onPointerMove={(e) => {
          onMove(e);
          if (rubber !== null) {
            const to = world(e);
            setRubber({ from: rubber.from, to });
          }
        }}
        onPointerUp={() => {
          onUp();
          if (rect) {
            setSelected(marquee(boxes, rect));
          }
          setRubber(null);
        }}
      >
        {list.length === 0 && summaries.isSuccess ? <p className="p-4 text-secondary">{m.empty}</p> : null}
        <div
          className="absolute start-0 top-0 origin-top-left"
          style={{ transform: `scale(${view.z}) translate(${-view.x}px, ${-view.y}px)` }}
        >
          {frames.map((f) => (
            <fieldset
              key={f.id}
              aria-label={m.frame(f.label)}
              className="absolute rounded-md border border-dashed"
              style={{ left: f.x, top: f.y, width: f.width, height: f.height }}
            >
              <span className="text-tag text-secondary">{f.label}</span>
            </fieldset>
          ))}
          {Arrows === undefined ? null : <Arrows boxes={boxes} links={links} boxSize={BOX} />}
          {guides.map((g) => (
            <div
              key={`${g.axis}${g.at}`}
              data-guide={g.axis}
              className="absolute bg-primary"
              style={
                g.axis === "x"
                  ? { left: g.at, top: -10000, width: 1, height: 20000 }
                  : { top: g.at, left: -10000, height: 1, width: 20000 }
              }
            />
          ))}
          {boxes.map((b) => (
            <PluginBoxCard
              key={b.id}
              box={b}
              summary={summaryOf.get(b.id)}
              selected={selected.has(b.id)}
              entry={b.key === entryKey}
              messages={m}
              onPointerDown={(e) => onBoxDown(e, b)}
              onOpen={() => open(b)}
              onHover={(on) => setHover(on ? b.id : undefined)}
            />
          ))}
          {rect ? (
            <div
              className="absolute border"
              style={{ left: rect.x, top: rect.y, width: rect.width, height: rect.height }}
            />
          ) : null}
        </div>
        {detail?.plugin === undefined ? null : <DetailCard summary={detail} messages={m} />}
      </div>
      {safety !== null && one?.plugin !== undefined && (
        <PluginSafety
          appId={appId}
          plugin={one.plugin}
          mode={safety}
          onClose={() => setSafety(null)}
          onDone={() => {
            setSafety(null);
            setMessage(null);
            setSelected(new Set());
            void refresh();
          }}
          onFailed={(text) => {
            setSafety(null);
            setMessage(text);
          }}
        />
      )}
      {trashOpen && <TrashDialog appId={appId} onClose={() => setTrashOpen(false)} onRestored={() => void refresh()} />}
      <PluginDialogs
        dialogue={dialogue}
        messages={m}
        summary={one}
        taken={new Set(list.map((s) => s.plugin?.key ?? ""))}
        selectedBoxes={boxes.filter((b) => selected.has(b.id))}
        close={() => setDialogue(null)}
        onCreate={(name, key) => mutation.mutate(() => plugins.createPlugin({ appId, key, name }))}
        onSettings={(id, name, description, tags) =>
          mutation.mutate(() => plugins.updatePlugin({ id, name, metadata: { description, setTags: true, tags } }))
        }
        onDuplicate={(id, name, key) =>
          mutation.mutate(() => plugins.duplicatePlugin({ id, name, key, session: uuidv7() }))
        }
        onEntry={(key) =>
          mutation.mutate(() => apps.updateApp({ id: appId, name: app.data?.name ?? "", defaultPluginKey: key }))
        }
        onFrame={(label) => {
          const sel = boxes.filter((b) => selected.has(b.id));
          const minX = Math.min(...sel.map((b) => b.x)) - 24;
          const minY = Math.min(...sel.map((b) => b.y)) - 40;
          const maxX = Math.max(...sel.map((b) => b.x + BOX.width)) + 24;
          const maxY = Math.max(...sel.map((b) => b.y + BOX.height)) + 24;
          commit(new Map(boxes.map((b) => [b.id, { x: b.x, y: b.y }])), [
            ...frames,
            { id: uuidv7(), label, x: minX, y: minY, width: maxX - minX, height: maxY - minY },
          ]);
          setDialogue(null);
        }}
      />
    </section>
  );
}

interface CardProps {
  readonly box: PlacedBox;
  readonly summary: PluginSummary | undefined;
  readonly selected: boolean;
  readonly entry: boolean;
  readonly messages: ReturnType<typeof appCanvasMessages>;
  readonly onPointerDown: (e: React.PointerEvent) => void;
  readonly onOpen: () => void;
  readonly onHover: (on: boolean) => void;
}

/** One plugin box with its live badges: lock, unpublished changes, validation, kill switch. */
export function PluginBoxCard(props: CardProps): ReactNode {
  const { box, summary: s, messages: m } = props;
  const name = s?.plugin?.name ?? box.key;
  return (
    <button
      type="button"
      aria-label={m.plugin(name, box.key)}
      aria-pressed={props.selected}
      data-plugin={box.key}
      className="absolute flex flex-col items-start gap-1 rounded-md border p-2 text-start"
      style={{
        left: box.x,
        top: box.y,
        width: BOX.width,
        height: BOX.height,
        opacity: s?.plugin?.archived === true ? 0.6 : 1,
      }}
      onPointerDown={props.onPointerDown}
      onDoubleClick={props.onOpen}
      onKeyDown={(e) => {
        if (e.key === "Enter") {
          props.onOpen();
        }
      }}
      onPointerEnter={() => props.onHover(true)}
      onPointerLeave={() => props.onHover(false)}
      onFocus={() => props.onHover(true)}
      onBlur={() => props.onHover(false)}
    >
      <span className="text-title">{name}</span>
      <span className="text-data text-secondary">{box.key}</span>
      <span className="flex flex-wrap gap-1">
        {props.entry ? <StatusPill status="neutral" label={m.entryBadge} /> : null}
        {s?.plugin?.archived === true ? <StatusPill status="neutral" label={m.archivedBadge} /> : null}
        {s !== undefined && s.lockHolder !== "" ? (
          <StatusPill status="warning" label={m.lockedBy(s.lockHolder)} />
        ) : null}
        {s?.unpublishedChanges === true ? <StatusPill status="warning" label={m.unpublished} /> : null}
        {s !== undefined && s.validationErrors > 0n ? (
          <StatusPill status="danger" label={m.validationErrors(Number(s.validationErrors))} />
        ) : s !== undefined ? (
          <StatusPill status="success" label={m.valid} />
        ) : null}
        {s?.killed === true ? <StatusPill status="danger" label={m.killed} /> : null}
      </span>
    </button>
  );
}

/** The hover/focus detail card of a plugin (E5, STU-031). */
function DetailCard(props: { summary: PluginSummary; messages: ReturnType<typeof appCanvasMessages> }): ReactNode {
  const { summary: s, messages: m } = props;
  return (
    <aside aria-label={s.plugin?.name} className="absolute end-4 top-4 flex w-64 flex-col gap-1 rounded-md border p-3">
      <span className="text-section">{s.plugin?.name}</span>
      <span className="text-data">{m.screens(Number(s.screens))}</span>
      <span className="text-data">{m.versions(Number(s.versions))}</span>
      <span className="text-data">{m.bundleSize(Math.ceil(Number(s.bundleSize) / 1024))}</span>
      <span className="text-data">{m.linksOut(s.linksOut.length)}</span>
      <span className="text-data">{m.linksIn(s.linksIn.length)}</span>
      <span className="text-data">{m.functions(s.functionsUsed.length)}</span>
      {s.plugin?.description ? <span>{s.plugin.description}</span> : null}
    </aside>
  );
}

interface DialogsProps {
  readonly dialogue: Dialogue;
  readonly messages: ReturnType<typeof appCanvasMessages>;
  readonly summary: PluginSummary | undefined;
  readonly taken: ReadonlySet<string>;
  readonly selectedBoxes: readonly PlacedBox[];
  readonly close: () => void;
  readonly onCreate: (name: string, key: string) => void;
  readonly onSettings: (id: string, name: string, description: string, tags: string[]) => void;
  readonly onDuplicate: (id: string, name: string, key: string) => void;
  readonly onEntry: (key: string) => void;
  readonly onFrame: (label: string) => void;
}

/** The name a dialog opens with: the plugin's own for settings, a numbered copy for a duplicate. */
function initialName(kind: NonNullable<Dialogue>["kind"], current: string): string {
  if (kind === "settings") {
    return current;
  }
  return kind === "duplicate" ? `${current} 2` : "";
}

interface Fields {
  readonly name: string;
  readonly description: string;
  readonly tags: string[];
}

const splitTags = (text: string): string[] =>
  text
    .split(",")
    .map((t) => t.trim())
    .filter((t) => t !== "");

function dialogTitle(kind: NonNullable<Dialogue>["kind"], m: ReturnType<typeof appCanvasMessages>): string {
  const titles = {
    new: m.newPlugin,
    settings: m.settings,
    duplicate: m.duplicate,
    entry: m.setEntry,
    frame: m.newFrame,
  };
  return titles[kind];
}

/** Runs the dialog's action; an empty name does nothing, as the entry dialog has no name. */
function submitDialogue(
  kind: NonNullable<Dialogue>["kind"],
  f: Fields,
  p: PluginSummary["plugin"],
  on: DialogsProps,
): void {
  if (kind === "entry") {
    if (p !== undefined) {
      on.onEntry(p.key);
    }
  } else if (f.name === "") {
    return;
  } else if (kind === "new") {
    on.onCreate(f.name, pluginKeyFrom(f.name, on.taken));
  } else if (kind === "frame") {
    on.onFrame(f.name);
  } else if (p !== undefined && kind === "settings") {
    on.onSettings(p.id, f.name, f.description, f.tags);
  } else if (p !== undefined) {
    on.onDuplicate(p.id, f.name, pluginKeyFrom(f.name, on.taken));
  }
}

/** D16 new plugin, D17 settings, D18 duplicate, D25 entry plugin, and the frame label (D20). */
export function PluginDialogs(props: DialogsProps): ReactNode {
  const { dialogue, messages: m, summary: s } = props;
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [tags, setTags] = useState("");
  useEffect(() => {
    if (dialogue === null) {
      return;
    }
    const p = s?.plugin;
    setName(initialName(dialogue.kind, p?.name ?? ""));
    setDescription(p?.description ?? "");
    setTags((p?.tags ?? []).join(", "));
  }, [dialogue, s]);
  if (dialogue === null) {
    return null;
  }
  const submit = () =>
    submitDialogue(dialogue.kind, { name: name.trim(), description, tags: splitTags(tags) }, s?.plugin, props);
  const title = dialogTitle(dialogue.kind, m);
  return (
    <Dialog
      open
      onOpenChange={(o) => {
        if (!o) {
          props.close();
        }
      }}
      title={title}
      size={dialogue.kind === "settings" ? "md" : "sm"}
      labels={{ close: m.close }}
    >
      {dialogue.kind === "entry" ? (
        <p>{s?.plugin?.name}</p>
      ) : (
        <Field label={dialogue.kind === "frame" ? m.frameLabel : m.name}>
          <Input value={name} onChange={(e) => setName(e.target.value)} />
        </Field>
      )}
      {dialogue.kind === "settings" ? (
        <>
          <Field label={m.description}>
            <Textarea value={description} onChange={(e) => setDescription(e.target.value)} />
          </Field>
          <Field label={m.tags}>
            <Input value={tags} onChange={(e) => setTags(e.target.value)} />
          </Field>
        </>
      ) : null}
      <div className="flex justify-end gap-2">
        <Button onClick={props.close}>{m.cancel}</Button>
        <Button onClick={submit}>{m.save}</Button>
      </div>
    </Dialog>
  );
}
