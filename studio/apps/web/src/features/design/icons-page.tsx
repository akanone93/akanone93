// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button, Field, Icon, Input } from "@plux/ui";
import { type ReactElement, useState } from "react";
import { designMessages } from "./design.messages.ts";
import { groupRows, putToken, validName } from "./design-model.ts";
import { RemoveToken, show } from "./parts.tsx";
import { Frame, useDesign } from "./scope.tsx";

function AddIcon(): ReactElement {
  const m = designMessages(useLingui().i18n);
  const ctx = useDesign();
  const [set, setSet] = useState("");
  const [name, setName] = useState("");
  const [source, setSource] = useState("");
  const ready = ctx.canEdit && validName(set) && validName(name) && source !== "";
  return (
    <div className="flex flex-wrap items-end gap-3">
      <Field label={m.iconSet}>
        <Input value={set} disabled={!ctx.canEdit} onChange={(e) => setSet(e.target.value)} />
      </Field>
      <Field label={m.iconName}>
        <Input value={name} disabled={!ctx.canEdit} onChange={(e) => setName(e.target.value)} />
      </Field>
      <Field label={m.iconSource}>
        <Input value={source} disabled={!ctx.canEdit} onChange={(e) => setSource(e.target.value)} />
      </Field>
      <Button
        variant="secondary"
        disabled={!ready}
        onClick={() => {
          ctx.write(`icon:${set}.${name}`, (t) =>
            putToken(t, ["icons", ...set.split("."), ...name.split(".")], source),
          );
          setName("");
          setSource("");
        }}
      >
        <Icon name="plus" />
        {m.addIcon}
      </Button>
    </div>
  );
}

function Sets(): ReactElement {
  const m = designMessages(useLingui().i18n);
  const ctx = useDesign();
  const icons = groupRows(ctx.rows, "icons");
  const sets = [...new Set(icons.map((r) => r.path[0] ?? ""))];
  return (
    <>
      {sets.length === 0 && <p className="text-body text-secondary">{m.noIcons}</p>}
      {sets.map((set) => (
        <section key={set} aria-label={set} className="flex flex-col gap-1">
          <h2 className="font-mono text-label font-medium">{set}</h2>
          <ul>
            {icons
              .filter((r) => r.path[0] === set)
              .map((r) => (
                <li key={r.path.join(".")} className="flex items-center gap-2 text-body">
                  <span className="font-mono">{r.path.slice(1).join(".")}</span>
                  <span className="text-secondary">{show(r.light)}</span>
                  <RemoveToken path={["icons", ...r.path]} />
                </li>
              ))}
          </ul>
        </section>
      ))}
      <AddIcon />
    </>
  );
}

/** A18: the icon sets of the app and the icons each provides (STU-112). */
export function IconsPage(): ReactElement {
  const m = designMessages(useLingui().i18n);
  return (
    <Frame id="design-icons" title={m.iconsTitle} caption={m.iconsCaption}>
      <Sets />
    </Frame>
  );
}
