// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { availableLocales } from "@plux/i18n";
import { Field, Select } from "@plux/ui";
import { type ReactElement, useState } from "react";
import { CustomFrameDialog } from "./custom-frame-dialog.tsx";
import { type DeviceFrame, decodeCustom, frameCatalogue } from "./frames.ts";
import { previewMessages } from "./preview.messages.ts";
import { type MockState, mockStates, type PreviewState, textScales, withMock } from "./switches.ts";

const CUSTOM = "custom";

/** The preview switches of the canvas (STU-058) and the device frame select (STU-057). */
export function PreviewSwitcher({
  state,
  dataSources,
  onChange,
  includePseudo,
  part = "all",
}: {
  readonly state: PreviewState;
  /** Data sources of the plugin whose mock state can be set; one select sets them all when several. */
  readonly dataSources: readonly string[];
  readonly onChange: (next: PreviewState) => void;
  readonly includePseudo: boolean;
  /** `main` renders the frame, theme, locale and text scale (the action bar); `extra` the rest; `all` both. */
  readonly part?: "all" | "main" | "extra";
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
      className="flex flex-wrap items-end gap-3"
    >
      {part !== "extra" && (
        <>
          <Field label={m.frame}>
            <Select
              value={state.deviceFrame}
              options={frames}
              onValueChange={(v) => (v === CUSTOM ? setDialog(true) : onChange({ ...state, deviceFrame: v }))}
            />
          </Field>
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
          <Field label={m.mock}>
            <Select
              value={mock}
              options={mockStates.map((s) => ({ value: s, label: mockLabel[s] }))}
              onValueChange={(v) =>
                onChange(dataSources.reduce((acc, src) => withMock(acc, src, v as MockState), state))
              }
            />
          </Field>
        </>
      )}
      <CustomFrameDialog open={dialog} onClose={() => setDialog(false)} onUse={useFrame} />
    </div>
  );
}
