// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button, Dialog, Field, Icon, Input, Select } from "@plux/ui";
import { type ReactElement, useState } from "react";
import { designMessages } from "./design.messages.ts";
import { baseRows, brandNames, contrastOf, groupRows, type Mode, putToken, validName } from "./design-model.ts";
import { modeLabel } from "./parts.tsx";
import { Frame, useDesign } from "./scope.tsx";

/** The colour tokens that carry a value in `mode`, with that value. */
function coloursIn(ctx: ReturnType<typeof useDesign>, mode: Mode): [string, string][] {
  const tokens = ctx.value?.["tokens"];
  return baseRows(ctx.rows)
    .filter((r) => r.type === "color")
    .flatMap((r): [string, unknown][] => [
      [r.path.join("."), mode === "light" ? r.light : mode === "dark" ? r.dark : contrastOf(tokens, r.path)],
    ])
    .filter((e): e is [string, string] => typeof e[1] === "string");
}

function ModeItem({ mode }: { readonly mode: Mode }): ReactElement {
  const m = designMessages(useLingui().i18n);
  const ctx = useDesign();
  const label = modeLabel(m, mode);
  const colours = coloursIn(ctx, mode);
  return (
    <li className="flex flex-col gap-1">
      <span className="text-body">{m.modeCount(label, colours.length)}</span>
      <ul aria-label={m.swatches(label)} className="flex flex-wrap gap-2">
        {colours.map(([name, value]) => (
          <li key={name} className="flex items-center gap-1">
            <span
              aria-hidden="true"
              className="inline-block size-4 rounded-sm border border-hairline"
              style={{ backgroundColor: value }}
            />
            <span className="font-mono text-caption">{name}</span>
          </li>
        ))}
      </ul>
    </li>
  );
}

/** D57 (create): a brand overlay replaces one base token for one brand. */
function CreateBrandDialog(props: { readonly onClose: () => void }): ReactElement {
  const m = designMessages(useLingui().i18n);
  const ctx = useDesign();
  const base = baseRows(ctx.rows);
  const [name, setName] = useState("");
  const [token, setToken] = useState(base[0]?.path.join(".") ?? "");
  const [value, setValue] = useState("");
  const chosen = base.find((r) => r.path.join(".") === token);
  const ready = ctx.canEdit && validName(name) && !name.includes(".") && chosen !== undefined && value !== "";
  return (
    <Dialog open onOpenChange={(o) => !o && props.onClose()} title={m.createBrandTitle} labels={{ close: m.close }}>
      <div className="flex flex-col gap-3">
        <Field label={m.brandName}>
          <Input value={name} onChange={(e) => setName(e.target.value)} />
        </Field>
        <Select
          label={m.brandToken}
          options={base.map((r) => ({ value: r.path.join("."), label: r.path.join(".") }))}
          value={token}
          onValueChange={setToken}
        />
        <Field label={m.brandValue}>
          <Input value={value} onChange={(e) => setValue(e.target.value)} />
        </Field>
        <Button
          disabled={!ready}
          onClick={() => {
            ctx.write(`brand:${name}`, (t) => putToken(t, ["brand", name, ...token.split(".")], value, chosen?.type));
            props.onClose();
          }}
        >
          {m.brandCreate}
        </Button>
      </div>
    </Dialog>
  );
}

function Brands(): ReactElement {
  const m = designMessages(useLingui().i18n);
  const ctx = useDesign();
  const [creating, setCreating] = useState(false);
  const names = brandNames(ctx.rows);
  return (
    <section aria-label={m.brandsHeading} className="flex flex-col gap-2">
      <h2 className="text-label font-medium">{m.brandsHeading}</h2>
      {names.length === 0 ? (
        <p className="text-body text-secondary">{m.noBrands}</p>
      ) : (
        <ul>
          {names.map((b) => (
            <li key={b} className="text-body">
              {m.brandOverrides(b, groupRows(ctx.rows, "brand").filter((r) => r.path[0] === b).length)}
            </li>
          ))}
        </ul>
      )}
      <div>
        <Button variant="secondary" disabled={!ctx.canEdit} onClick={() => setCreating(true)}>
          <Icon name="plus" />
          {m.createBrand}
        </Button>
      </div>
      {creating && <CreateBrandDialog onClose={() => setCreating(false)} />}
    </section>
  );
}

/** A16: light, dark and high-contrast modes and the brand overlays (STU-112). */
export function ThemesPage(): ReactElement {
  const m = designMessages(useLingui().i18n);
  return (
    <Frame id="design-themes" title={m.themesTitle} caption={m.themesCaption}>
      <section aria-label={m.modesHeading} className="flex flex-col gap-2">
        <h2 className="text-label font-medium">{m.modesHeading}</h2>
        <ul className="flex flex-col gap-3">
          {(["light", "dark", "contrast"] as const).map((mode) => (
            <ModeItem key={mode} mode={mode} />
          ))}
        </ul>
      </section>
      <Brands />
    </Frame>
  );
}
