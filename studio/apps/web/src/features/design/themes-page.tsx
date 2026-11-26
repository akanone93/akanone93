// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { AlertDialog, Button, Dialog, Field, Icon, Input, Select } from "@plux/ui";
import { type ReactElement, useState } from "react";
import { ColourField } from "../../components/colour-field.tsx";
import { PageEmpty } from "../../components/page.tsx";
import { designMessages } from "./design.messages.ts";
import {
  BRANDS_GROUP,
  baseRows,
  brandNames,
  dropToken,
  groupRows,
  putToken,
  setMode,
  typedValue,
  validName,
} from "./design-model.ts";
import { ModePreview } from "./mode-preview.tsx";
import { RemoveToken, show } from "./parts.tsx";
import { Frame, useDesign } from "./scope.tsx";

/** The theme's modes, each previewed on its own surface (STU-112). */
function Modes(): ReactElement {
  const m = designMessages(useLingui().i18n);
  const ctx = useDesign();
  const raw = ctx.value?.["tokens"];
  const tokens = raw !== null && typeof raw === "object" && !Array.isArray(raw) ? (raw as Record<string, unknown>) : {};
  return (
    <section aria-label={m.modesHeading} className="flex flex-col gap-3">
      <h2 className="text-label font-medium">{m.modesHeading}</h2>
      <div className="mode-previews">
        {(["light", "dark", "contrast"] as const).map((mode) => (
          <ModePreview key={mode} tokens={tokens} mode={mode} />
        ))}
      </div>
    </section>
  );
}

/**
 * D57: a brand overlay replaces base tokens for one brand. Without `brand` it creates an overlay with its first
 * override; with one, it adds an override to that overlay (or replaces the one it has for the token).
 */
function BrandDialog(props: { readonly brand?: string; readonly onClose: () => void }): ReactElement {
  const m = designMessages(useLingui().i18n);
  const ctx = useDesign();
  const base = baseRows(ctx.rows);
  const [name, setName] = useState(props.brand ?? "");
  const [token, setToken] = useState(base[0]?.path.join(".") ?? "");
  const [value, setValue] = useState("");
  const [dark, setDark] = useState("");
  const chosen = base.find((r) => r.path.join(".") === token);
  const ready = ctx.canEdit && validName(name) && !name.includes(".") && chosen !== undefined && value !== "";
  const field = (label: string, v: string, set: (v: string) => void) =>
    chosen?.type === "color" ? (
      <ColourField value={v} pickLabel={m.pickColour(label)} onChange={set} />
    ) : (
      <Input value={v} onChange={(e) => set(e.target.value)} />
    );
  return (
    <Dialog
      open
      onOpenChange={(o) => !o && props.onClose()}
      title={props.brand === undefined ? m.createBrandTitle : m.brandAddTitle(props.brand)}
      labels={{ close: m.close }}
    >
      <div className="flex flex-col gap-3">
        {props.brand === undefined && (
          <Field label={m.brandName}>
            <Input value={name} onChange={(e) => setName(e.target.value)} />
          </Field>
        )}
        <Field label={m.brandToken}>
          <Select
            label={m.brandToken}
            options={base.map((r) => ({ value: r.path.join("."), label: r.path.join(".") }))}
            value={token}
            onValueChange={setToken}
          />
        </Field>
        <Field label={m.brandValue}>{field(m.brandValue, value, setValue)}</Field>
        <Field label={m.brandDarkValue} description={m.brandDarkHint}>
          {field(m.brandDarkValue, dark, setDark)}
        </Field>
        <Button
          disabled={!ready}
          onClick={() => {
            const path = [...BRANDS_GROUP, name, ...token.split(".")];
            ctx.write(`brand:${name}:${token}`, (t) => {
              putToken(t, path, typedValue(chosen?.type, value), chosen?.type);
              setMode(t, path, "dark", dark.trim() === "" ? undefined : typedValue(chosen?.type, dark));
            });
            props.onClose();
          }}
        >
          {props.brand === undefined ? m.brandCreate : m.brandAdd}
        </Button>
      </div>
    </Dialog>
  );
}

/** A colour value as CSS draws it: a hex colour, else nothing (an alias or an expression is named, not drawn). */
const swatchColour = (v: unknown): string | undefined =>
  typeof v === "string" && /^#(?:[0-9a-f]{3,4}|[0-9a-f]{6}|[0-9a-f]{8})$/i.test(v.trim()) ? v.trim() : undefined;

/**
 * One brand overlay as a compact card (build log row 58): its colours side by side, each a swatch of its light value
 * over its dark one with its name and value, then its other tokens one per line; each removable, more can be added, or
 * the overlay removed.
 */
function BrandCard({ brand }: { readonly brand: string }): ReactElement {
  const m = designMessages(useLingui().i18n);
  const ctx = useDesign();
  const [adding, setAdding] = useState(false);
  const [removing, setRemoving] = useState(false);
  const rows = groupRows(ctx.rows, BRANDS_GROUP)
    .filter((r) => r.path[0] === brand)
    .map((r) => ({ ...r, path: r.path.slice(1) }));
  const colours = rows.filter((r) => r.type === "color");
  const others = rows.filter((r) => r.type !== "color");
  const remove = (path: readonly string[]) => <RemoveToken path={[...BRANDS_GROUP, brand, ...path]} />;
  return (
    <section aria-label={brand} className="design-card brand-card">
      <div className="flex flex-wrap items-center gap-2">
        <h3 className="me-auto text-label font-medium">{m.brandOverrides(brand, rows.length)}</h3>
        <Button variant="secondary" size="sm" disabled={!ctx.canEdit} onClick={() => setAdding(true)}>
          <Icon name="plus" />
          {m.brandAdd}
        </Button>
        <Button
          variant="ghost"
          size="sm"
          aria-label={m.brandRemove(brand)}
          disabled={!ctx.canEdit}
          onClick={() => setRemoving(true)}
        >
          <Icon name="trash" />
        </Button>
      </div>
      {colours.length > 0 && (
        <ul className="brand-swatches">
          {colours.map((r) => {
            const name = r.path.join(".");
            const light = show(r.light);
            const dark = r.dark === undefined ? light : show(r.dark);
            return (
              <li key={name} className="brand-swatch">
                <span role="img" aria-label={m.brandColour(name, light, dark)} className="brand-swatch-colours">
                  <span style={{ backgroundColor: swatchColour(r.light) }} />
                  <span style={{ backgroundColor: swatchColour(r.dark ?? r.light) }} />
                </span>
                <span className="brand-swatch-name" title={name}>
                  {name}
                </span>
                <span className="brand-swatch-value" title={dark === light ? light : `${light} · ${dark}`}>
                  <span>{light}</span>
                  {dark !== light && <span> · {dark}</span>}
                </span>
                <span className="brand-swatch-remove">{remove(r.path)}</span>
              </li>
            );
          })}
        </ul>
      )}
      {others.length > 0 && (
        <ul className="brand-others">
          {others.map((r) => {
            const name = r.path.join(".");
            return (
              <li key={name}>
                <span className="truncate font-mono text-caption" title={name}>
                  {name}
                </span>
                <span className="token-value" title={show(r.light)}>
                  {show(r.light)}
                </span>
                {r.dark !== undefined && show(r.dark) !== show(r.light) && (
                  <span className="token-value" title={show(r.dark)}>
                    {show(r.dark)}
                  </span>
                )}
                {remove(r.path)}
              </li>
            );
          })}
        </ul>
      )}
      {adding && <BrandDialog brand={brand} onClose={() => setAdding(false)} />}
      <AlertDialog
        open={removing}
        onOpenChange={setRemoving}
        title={m.brandRemoveTitle(brand)}
        description={m.brandRemoveText}
        labels={{ cancel: m.cancel, confirm: m.brandRemoveConfirm }}
        onConfirm={() => ctx.write(`brand:${brand}:remove`, (t) => dropToken(t, [...BRANDS_GROUP, brand]))}
      />
    </section>
  );
}

function Brands(): ReactElement {
  const m = designMessages(useLingui().i18n);
  const ctx = useDesign();
  const [creating, setCreating] = useState(false);
  const names = brandNames(ctx.rows);
  return (
    <section aria-label={m.brandsHeading} className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center gap-2">
        <h2 className="me-auto text-label font-medium">{m.brandsHeading}</h2>
        <Button variant="secondary" disabled={!ctx.canEdit} onClick={() => setCreating(true)}>
          <Icon name="plus" />
          {m.createBrand}
        </Button>
      </div>
      {names.length === 0 ? (
        <PageEmpty icon="palette" text={m.noBrands} />
      ) : (
        <div className="brand-grid">
          {names.map((b) => (
            <BrandCard key={b} brand={b} />
          ))}
        </div>
      )}
      {creating && <BrandDialog onClose={() => setCreating(false)} />}
    </section>
  );
}

/** A16: light, dark and high-contrast modes and the brand overlays (STU-112). */
export function ThemesPage(): ReactElement {
  const m = designMessages(useLingui().i18n);
  return (
    <Frame id="design-themes" title={m.themesTitle} caption={m.themesCaption}>
      <Modes />
      <Brands />
    </Frame>
  );
}
