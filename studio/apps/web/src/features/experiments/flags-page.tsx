// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { FlagType } from "@plux/api";
import { Button, Field, Input, Select } from "@plux/ui";
import { type ReactElement, useState } from "react";
import { Frame } from "../releases/frame.tsx";
import { Table } from "../security/parts.tsx";
import { useEnvironment, usePermissions, useScope } from "../security/use-security.ts";
import { type ExperimentsMessages, experimentsMessages } from "./experiments.messages.ts";
import { useExperimentActions, useFlags } from "./use-experiments.ts";

function typeNames(m: ExperimentsMessages): Readonly<Record<number, string>> {
  return {
    [FlagType.BOOL]: m.typeBool,
    [FlagType.NUMBER]: m.typeNumber,
    [FlagType.STRING]: m.typeString,
    [FlagType.JSON]: m.typeJson,
  };
}

const validJson = (raw: string) => {
  try {
    JSON.parse(raw);
    return true;
  } catch {
    return false;
  }
};

interface Form {
  readonly key: string;
  readonly type: FlagType;
  readonly description: string;
  readonly defaultJson: string;
  readonly existing: boolean;
}

const blank: Form = { key: "", type: FlagType.BOOL, description: "", defaultJson: "false", existing: false };

/** A9 and D49: the flags of one environment, and the form that creates or edits one. */
export function FlagsPage(): ReactElement {
  const m = experimentsMessages(useLingui().i18n);
  const { app, env } = useScope();
  const environment = useEnvironment(app, env);
  const flags = useFlags(app, environment.environment?.id);
  const actions = useExperimentActions(app, environment.environment?.id ?? "");
  const allowed = usePermissions().has("flags.manage");
  const [form, setForm] = useState<Form>(blank);
  const [failed, setFailed] = useState(false);
  const names = typeNames(m);
  const valid = form.key !== "" && validJson(form.defaultJson);
  const save = () => {
    setFailed(false);
    const spec = { type: form.type, description: form.description, defaultJson: form.defaultJson };
    const call = form.existing ? actions.updateFlag(form.key, spec) : actions.createFlag(form.key, spec);
    call.then(
      () => setForm(blank),
      () => setFailed(true),
    );
  };
  return (
    <Frame
      id="flags-title"
      title={m.flagsTitle}
      caption={m.flagsCaption}
      loading={environment.loading || flags.isPending}
      failed={environment.failed || flags.isError}
    >
      {(flags.data?.flags ?? []).length === 0 ? (
        <p className="text-body text-secondary">{m.noFlags}</p>
      ) : (
        <Table
          caption={m.flagsTitle}
          heads={[m.flagKey, m.colType, m.colDefault, ""]}
          rows={(flags.data?.flags ?? []).map((f) => ({
            key: f.id,
            cells: [
              f.key,
              names[f.type] ?? "",
              f.defaultJson,
              <span key="a" className="flex gap-2">
                <Button
                  variant="secondary"
                  disabled={!allowed}
                  aria-label={m.editNamed(f.key)}
                  onClick={() =>
                    setForm({
                      key: f.key,
                      type: f.type,
                      description: f.description,
                      defaultJson: f.defaultJson,
                      existing: true,
                    })
                  }
                >
                  {m.editFlag}
                </Button>
                <Button
                  variant="secondary"
                  disabled={!allowed}
                  aria-label={m.deleteNamed(f.key)}
                  onClick={() => void actions.deleteFlag(f.key).catch(() => setFailed(true))}
                >
                  {m.deleteFlag}
                </Button>
              </span>,
            ],
          }))}
        />
      )}
      <Field label={m.flagKey}>
        <Input disabled={form.existing} value={form.key} onChange={(e) => setForm({ ...form, key: e.target.value })} />
      </Field>
      <Field label={m.flagType}>
        <Select
          options={[FlagType.BOOL, FlagType.NUMBER, FlagType.STRING, FlagType.JSON].map((t) => ({
            value: String(t),
            label: names[t] ?? "",
          }))}
          value={String(form.type)}
          onValueChange={(v) => setForm({ ...form, type: Number(v) as FlagType })}
        />
      </Field>
      <Field label={m.flagDescription}>
        <Input value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} />
      </Field>
      <Field label={m.flagDefault}>
        <Input value={form.defaultJson} onChange={(e) => setForm({ ...form, defaultJson: e.target.value })} />
      </Field>
      {!validJson(form.defaultJson) && (
        <p role="alert" className="text-body text-danger">
          {m.badJson}
        </p>
      )}
      {failed && (
        <p role="alert" className="text-body text-danger">
          {m.actionFailed}
        </p>
      )}
      <Button disabled={!allowed || !valid} onClick={save}>
        {m.saveFlag}
      </Button>
    </Frame>
  );
}
