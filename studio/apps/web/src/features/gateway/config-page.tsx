// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { create } from "@bufbuild/protobuf";
import { useLingui } from "@lingui/react";
import { ConfigTier, ConfigType, type ConfigValue, ConfigValueSchema } from "@plux/api";
import { Dialog, Field, Input, Select } from "@plux/ui";
import { Checkbox } from "@plux/ui/components/checkbox";
import { type ReactElement, useId, useState } from "react";
import { DataTable } from "../../components/page.tsx";
import { type GatewayMessages, gatewayMessages } from "./gateway.messages.ts";
import { ChangeButton, GatewayFrame, RemoveDialog, useGatewayScope } from "./parts.tsx";
import { useConfigValues, useGatewayChanges } from "./use-gateway.ts";

/** A key as the server accepts it: a letter, then letters, digits and underscores (DAT-090). */
const KEY = /^[A-Za-z][A-Za-z0-9_]*$/;

/** A value as the dialog edits it; `version` is the stored version (0 for a new key), checked on save. */
export interface ValueDraft {
  readonly key: string;
  readonly type: ConfigType;
  readonly tier: ConfigTier;
  readonly sensitive: boolean;
  readonly strict: boolean;
  readonly value: string;
  readonly version: bigint;
  readonly isNew: boolean;
}

const newDraft: ValueDraft = {
  key: "",
  type: ConfigType.STRING,
  tier: ConfigTier.SERVER_BOUND,
  sensitive: false,
  strict: false,
  value: "",
  version: 0n,
  isNew: true,
};

/** The draft of a stored value; a sensitive value's content is never returned, so it starts empty. */
function draftOf(v: ConfigValue): ValueDraft {
  return {
    key: v.key,
    type: v.type,
    tier: v.tier,
    sensitive: v.sensitive,
    strict: v.strict,
    value: v.sensitive ? "" : v.value,
    version: v.version,
    isNew: false,
  };
}

/**
 * The value a draft describes, with only the combinations the server allows: a sensitive value is server-bound,
 * a strict one live. Undefined while the key is not valid or the value is empty.
 */
export function configValueOf(d: ValueDraft): ConfigValue | undefined {
  if (!KEY.test(d.key) || d.value === "") {
    return undefined;
  }
  const live = d.tier === ConfigTier.LIVE;
  return create(ConfigValueSchema, {
    key: d.key,
    type: d.type,
    tier: d.tier,
    sensitive: !live && d.sensitive,
    strict: live && d.strict,
    value: d.value,
  });
}

/** An environment's server-held configuration values (DAT-090), each editable and removable (build log row 199). */
export function ConfigValuesPage(): ReactElement {
  const m = gatewayMessages(useLingui().i18n);
  const { app, environment, can } = useGatewayScope();
  const environmentId = environment.environment?.id;
  const list = useConfigValues(app, environmentId);
  const changes = useGatewayChanges();
  const [draft, setDraft] = useState<ValueDraft | undefined>(undefined);
  const [removing, setRemoving] = useState<string | undefined>(undefined);
  const [saveError, setSaveError] = useState<unknown>(undefined);
  const types = typeNames(m);
  const run = (work: Promise<unknown>) => {
    setSaveError(undefined);
    work.catch((e: unknown) => setSaveError(e));
  };
  return (
    <GatewayFrame
      id="gateway-config-title"
      title={m.configTitle}
      caption={m.configCaption}
      loading={environment.loading || list.isPending}
      failed={environment.failed || list.isError}
      error={list.error}
      saveError={saveError}
    >
      <div>
        <ChangeButton can={can} label={m.addValue} onClick={() => setDraft(newDraft)} />
      </div>
      <DataTable
        caption={m.configTitle}
        heads={[m.colKey, m.colType, m.colTier, m.colValue, m.colVersion, m.colActions]}
        actions
        empty={m.noValues}
        rows={(list.data?.values ?? []).map((v) => ({
          key: v.key,
          cells: [
            <span key="k" className="font-mono">
              {v.key}
            </span>,
            types.get(v.type) ?? "",
            v.tier === ConfigTier.LIVE ? m.tierLive : m.tierServer,
            v.sensitive ? (
              <span key="v" className="text-muted-foreground">
                {m.sealed}
              </span>
            ) : (
              <span key="v" className="font-mono" dir="ltr">
                {v.value}
              </span>
            ),
            String(v.version),
            <span key="a" className="inline-flex gap-1">
              <ChangeButton can={can} size="sm" variant="ghost" label={m.edit} onClick={() => setDraft(draftOf(v))} />
              <ChangeButton can={can} size="sm" variant="ghost" label={m.remove} onClick={() => setRemoving(v.key)} />
            </span>,
          ],
        }))}
      />
      {draft !== undefined && environmentId !== undefined && (
        <ValueDialog
          draft={draft}
          m={m}
          onClose={() => setDraft(undefined)}
          onSave={(value, version) => {
            setDraft(undefined);
            run(changes.saveValue(app, environmentId, value, version));
          }}
        />
      )}
      <RemoveDialog
        name={removing}
        m={m}
        onClose={() => setRemoving(undefined)}
        onConfirm={() => {
          const key = removing;
          setRemoving(undefined);
          if (key !== undefined && environmentId !== undefined) {
            run(changes.deleteValue(app, environmentId, key));
          }
        }}
      />
    </GatewayFrame>
  );
}

/** The names of the value types. */
function typeNames(m: GatewayMessages): ReadonlyMap<ConfigType, string> {
  return new Map([
    [ConfigType.STRING, m.typeString],
    [ConfigType.INTEGER, m.typeInteger],
    [ConfigType.DECIMAL, m.typeDecimal],
    [ConfigType.BOOLEAN, m.typeBoolean],
  ]);
}

/** The dialog that adds or edits a value. */
function ValueDialog(props: {
  readonly draft: ValueDraft;
  readonly m: GatewayMessages;
  readonly onClose: () => void;
  readonly onSave: (value: ConfigValue, version: bigint) => void;
}): ReactElement {
  const { m } = props;
  const [d, setD] = useState(props.draft);
  const [tried, setTried] = useState(false);
  const flagId = useId();
  const set = (change: Partial<ValueDraft>) => setD({ ...d, ...change });
  const live = d.tier === ConfigTier.LIVE;
  const save = () => {
    const value = configValueOf(d);
    if (value === undefined) {
      setTried(true);
      return;
    }
    props.onSave(value, d.version);
  };
  const keyError = tried && !KEY.test(d.key) ? { error: m.required } : {};
  const valueError = tried && d.value === "" ? { error: m.required } : {};
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) {
          props.onClose();
        }
      }}
      size="sm"
      title={m.valueDialog}
      labels={{ close: m.close }}
    >
      <Field label={m.fieldKey} {...keyError}>
        <Input dir="ltr" value={d.key} disabled={!d.isNew} onChange={(e) => set({ key: e.target.value })} />
      </Field>
      <Field label={m.fieldType}>
        <Select
          label={m.fieldType}
          options={[...typeNames(m)].map(([value, label]) => ({ value: String(value), label }))}
          value={String(d.type)}
          onValueChange={(v) => set({ type: Number(v) as ConfigType })}
        />
      </Field>
      <Field label={m.fieldTier}>
        <Select
          label={m.fieldTier}
          options={[
            { value: String(ConfigTier.SERVER_BOUND), label: m.tierServer },
            { value: String(ConfigTier.LIVE), label: m.tierLive },
          ]}
          value={String(d.tier)}
          onValueChange={(v) => set({ tier: Number(v) as ConfigTier })}
        />
      </Field>
      <div className="flex items-center gap-2 text-body">
        <Checkbox
          id={flagId}
          checked={live ? d.strict : d.sensitive}
          onCheckedChange={(v) => set(live ? { strict: v === true } : { sensitive: v === true })}
        />
        <label htmlFor={flagId}>{live ? m.fieldStrict : m.fieldSensitive}</label>
      </div>
      <Field label={m.fieldValue} {...valueError}>
        <Input
          dir="ltr"
          type={d.sensitive && !live ? "password" : "text"}
          autoComplete="off"
          placeholder={d.sensitive && !d.isNew ? m.newSealedValue : undefined}
          value={d.value}
          onChange={(e) => set({ value: e.target.value })}
        />
      </Field>
      <div className="flex justify-end gap-2">
        <ChangeButton can label={m.cancel} variant="secondary" onClick={props.onClose} />
        <ChangeButton can label={m.save} onClick={save} />
      </div>
    </Dialog>
  );
}
