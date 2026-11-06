// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { availableLocales } from "@plux/i18n";
import { Field, Select } from "@plux/ui";
import { type ReactElement, useState } from "react";
import { CustomFrameDialog } from "./custom-frame-dialog.tsx";
import type { FrameView } from "./frame-view.ts";
import { type DeviceFrame, decodeCustom, frameCatalogue } from "./frames.ts";
import { previewMessages } from "./preview.messages.ts";
import { type MockState, mockStates, type PreviewState, textScales, withMock } from "./switches.ts";

const CUSTOM = "custom";

/** The frame's orientation and whether its status bar, cutout and home indicator are drawn (STU-057, PC-11). */
function FrameViewSwitches({
  view,
  onView,
}: {
  readonly view: FrameView;
  readonly onView: (next: FrameView) => void;
}): ReactElement {
  const m = previewMessages(useLingui().i18n);
  return (
    <>
      <Field label={m.orientation}>
        <Select
          value={view.orientation}
          options={[
            { value: "portrait", label: m.portrait },
            { value: "landscape", label: m.landscape },
          ]}
          onValueChange={(v) => onView({ ...view, orientation: v === "landscape" ? "landscape" : "portrait" })}
        />
      </Field>
      <Field label={m.chrome}>
        <Select
          value={view.chrome ? "on" : "off"}
          options={[
            { value: "on", label: m.chromeShown },
            { value: "off", label: m.chromeHidden },
          ]}
          onValueChange={(v) => onView({ ...view, chrome: v === "on" })}
        />
      </Field>
    </>
  );
}

/** The brand select's value for the theme without an overlay. */
const NO_BRAND = "-";

/** The preview switches of the canvas (STU-058) and the device frame select (STU-057). */
export function PreviewSwitcher({
  state,
  dataSources,
  sourceNames = {},
  onChange,
  includePseudo,
  brands = [],
  part = "all",
  view,
  onView,
}: {
  readonly state: PreviewState;
  /** Data sources of the plugin whose mock state can be set; one select sets them all when several. */
  readonly dataSources: readonly string[];
  /** Each source's name; with several sources each gets its own state select too (PC-23). */
  readonly sourceNames?: Readonly<Record<string, string>>;
  readonly onChange: (next: PreviewState) => void;
  readonly includePseudo: boolean;
  /** The theme's brand overlays the screens can be previewed in (THM-003); the select shows only when there are some. */
  readonly brands?: readonly string[];
  /** `main` renders the frame, theme, locale and text scale (the action bar); `extra` the rest; `all` both. */
  readonly part?: "all" | "main" | "extra";
  /** The frame's orientation and chrome (PC-11); their switches show with the frame when given. */
  readonly view?: FrameView;
  readonly onView?: (next: FrameView) => void;
}): ReactElement {
  const m = previewMessages(useLingui().i18n);
  const [dialog, setDialog] = useState(false);
  const custom = decodeCustom(state.deviceFrame);
  const frames = [
    ...frameCatalogue.map((f) => ({ value: f.id, label: m.frameNames[f.id] ?? f.id })),
    ...(custom === undefined ? [] : [{ value: custom.id, label: `${custom.width}×${custom.height}` }]),
    { value: CUSTOM, label: m.customFrame },
  ];
  const useFrame = (f: DeviceFrame) => {
    setDialog(false);
    onChange({ ...state, deviceFrame: f.id });
  };
  const mock = dataSources.length > 0 ? (state.mockStates[dataSources[0] as string] ?? "success") : "success";
  const mockLabel: Record<MockState, string> = {
    success: m.mockSuccess,
    loading: m.mockLoading,
    empty: m.mockEmpty,
    error: m.mockError,
  };
  return (
    <div
      {...(part === "main" ? {} : { role: "toolbar", "aria-label": m.switcher })}
      className="grid grid-cols-2 items-end gap-3"
    >
      {part !== "extra" && (
        <>
          {/* The frame names are long ("iPhone 15 Pro Max · 430 × 932"): the frame takes the whole row. */}
          <div className="col-span-2">
            <Field label={m.frame}>
              <Select
                value={state.deviceFrame}
                options={frames}
                onValueChange={(v) => (v === CUSTOM ? setDialog(true) : onChange({ ...state, deviceFrame: v }))}
              />
            </Field>
          </div>
          {view !== undefined && onView !== undefined && <FrameViewSwitches view={view} onView={onView} />}
          <Field label={m.theme}>
            <Select
              value={state.theme}
              options={[
                { value: "light", label: m.themeLight },
                { value: "dark", label: m.themeDark },
              ]}
              onValueChange={(v) => onChange({ ...state, theme: v === "dark" ? "dark" : "light" })}
            />
          </Field>
          <Field label={m.locale}>
            <Select
              value={state.locale}
              options={availableLocales(includePseudo).map((l) => ({ value: l.tag, label: l.nativeName }))}
              onValueChange={(v) => onChange({ ...state, locale: v })}
            />
          </Field>
          <Field label={m.textScale}>
            <Select
              value={String(state.textScale)}
              options={textScales.map((s) => ({ value: String(s), label: `${Math.round(s * 100)}%` }))}
              onValueChange={(v) => onChange({ ...state, textScale: Number(v) })}
            />
          </Field>
        </>
      )}
      {part !== "main" && (
        <>
          <Field label={m.platform}>
            <Select
              value={state.platform === "" ? "frame" : state.platform}
              options={[
                { value: "frame", label: m.platformFrame },
                { value: "material", label: m.material },
                { value: "cupertino", label: m.cupertino },
              ]}
              onValueChange={(v) => onChange({ ...state, platform: v === "material" || v === "cupertino" ? v : "" })}
            />
          </Field>
          {brands.length > 0 && (
            <Field label={m.brand}>
              <Select
                value={state.brandOverlay === "" ? NO_BRAND : state.brandOverlay}
                options={[{ value: NO_BRAND, label: m.brandNone }, ...brands.map((b) => ({ value: b, label: b }))]}
                onValueChange={(v) => onChange({ ...state, brandOverlay: v === NO_BRAND ? "" : v })}
              />
            </Field>
          )}
          <Field label={m.mock}>
            <Select
              value={mock}
              options={mockStates.map((s) => ({ value: s, label: mockLabel[s] }))}
              onValueChange={(v) =>
                onChange(dataSources.reduce((acc, src) => withMock(acc, src, v as MockState), state))
              }
            />
          </Field>
          {dataSources.length > 1 &&
            dataSources.map((src) => (
              <Field key={src} label={m.mockOf(sourceNames[src] ?? src)}>
                <Select
                  value={state.mockStates[src] ?? "success"}
                  options={mockStates.map((s) => ({ value: s, label: mockLabel[s] }))}
                  onValueChange={(v) => onChange(withMock(state, src, v as MockState))}
                />
              </Field>
            ))}
        </>
      )}
      <CustomFrameDialog open={dialog} onClose={() => setDialog(false)} onUse={useFrame} />
    </div>
  );
}
