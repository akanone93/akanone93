// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { ConnectError } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import type { ShellSettings } from "@plux/api";
import { Button, Field, Input, Select } from "@plux/ui";
import { type FormEvent, type ReactElement, useState } from "react";
import { Choice } from "../new-app/choice.tsx";
import { shellSettingsMessages } from "./shell-settings.messages.ts";
import { useSaveShell } from "./use-shell.ts";

/** The device APIs the generator knows, in the order the form lists them (permissions.go of the generator). */
const deviceApis = [
  "camera",
  "photos",
  "files",
  "location",
  "contacts",
  "biometrics",
  "notifications",
  "clipboard",
  "share",
  "haptics",
] as const;

const routers = ["none", "go_router", "auto_route"] as const;

interface Values {
  readonly name: string;
  readonly package: string;
  readonly applicationId: string;
  readonly bundleId: string;
  readonly appVersion: string;
  readonly splashColor: string;
  readonly router: string;
  readonly deviceApis: readonly string[];
  readonly push: boolean;
  readonly deepLinkHosts: string;
  readonly deepLinkSchemes: string;
  readonly packages: string;
  readonly entryRoute: string;
}

function valuesOf(s: ShellSettings): Values {
  return {
    name: s.name,
    package: s.package,
    applicationId: s.applicationId,
    bundleId: s.bundleId,
    appVersion: s.appVersion,
    splashColor: s.splashColor,
    router: s.router === "" ? "none" : s.router,
    deviceApis: s.deviceApis,
    push: s.push,
    deepLinkHosts: s.deepLinkHosts.join(", "),
    deepLinkSchemes: s.deepLinkSchemes.join(", "),
    packages: s.packages.join(", "),
    entryRoute: s.entryRoute,
  };
}

function listOf(text: string): string[] {
  return text
    .split(",")
    .map((part) => part.trim())
    .filter((part) => part !== "");
}

type TextKey = "name" | "package" | "applicationId" | "bundleId" | "appVersion" | "splashColor" | "entryRoute";
type ListKey = "deepLinkHosts" | "deepLinkSchemes" | "packages";

/** The form of the native shell settings (GEN-004): everything about the app that needs a store build. */
export function ShellForm({
  appId,
  current,
  canManage,
}: {
  readonly appId: string;
  readonly current: ShellSettings;
  readonly canManage: boolean;
}): ReactElement {
  const m = shellSettingsMessages(useLingui().i18n);
  const save = useSaveShell(appId);
  const [values, setValues] = useState<Values>(() => valuesOf(current));
  const [working, setWorking] = useState(false);
  const [outcome, setOutcome] = useState<{ readonly ok: boolean; readonly text: string } | undefined>(undefined);
  const set = (patch: Partial<Values>) => setValues((v) => ({ ...v, ...patch }));
  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (working || !canManage) {
      return;
    }
    setWorking(true);
    setOutcome(undefined);
    try {
      await save({
        ...values,
        router: values.router === "none" ? "" : values.router,
        deviceApis: [...values.deviceApis],
        deepLinkHosts: listOf(values.deepLinkHosts),
        deepLinkSchemes: listOf(values.deepLinkSchemes),
        packages: listOf(values.packages),
      });
      setOutcome({ ok: true, text: m.saved });
    } catch (error) {
      const raw = error instanceof ConnectError ? error.rawMessage : "";
      setOutcome({ ok: false, text: raw === "" ? m.saveFailed : raw });
    } finally {
      setWorking(false);
    }
  };
  const text = (field: TextKey, label: string) => (
    <Field label={label}>
      <Input
        value={values[field]}
        disabled={!canManage}
        autoComplete="off"
        spellCheck={false}
        onChange={(e) => set({ [field]: e.target.value })}
      />
    </Field>
  );
  const list = (field: ListKey, label: string) => (
    <Field label={label} description={m.listHint}>
      <Input
        value={values[field]}
        disabled={!canManage}
        autoComplete="off"
        spellCheck={false}
        onChange={(e) => set({ [field]: e.target.value })}
      />
    </Field>
  );
  const apiNames: Readonly<Record<(typeof deviceApis)[number], string>> = {
    camera: m.deviceCamera,
    photos: m.devicePhotos,
    files: m.deviceFiles,
    location: m.deviceLocation,
    contacts: m.deviceContacts,
    biometrics: m.deviceBiometrics,
    notifications: m.deviceNotifications,
    clipboard: m.deviceClipboard,
    share: m.deviceShare,
    haptics: m.deviceHaptics,
  };
  return (
    <form
      noValidate
      onSubmit={(e) => void submit(e)}
      aria-labelledby="shell-form-title"
      className="flex flex-col gap-3"
    >
      <h2 id="shell-form-title" className="text-section">
        {m.formTitle}
      </h2>
      {!canManage && <p className="text-body text-secondary">{m.needsManage}</p>}
      <div className="grid grid-cols-2 gap-3">
        {text("name", m.fieldName)}
        {text("package", m.fieldPackage)}
        {text("applicationId", m.fieldApplicationId)}
        {text("bundleId", m.fieldBundleId)}
        {text("appVersion", m.fieldAppVersion)}
        {text("splashColor", m.fieldSplashColor)}
        {text("entryRoute", m.fieldEntryRoute)}
        <Field label={m.fieldRouter}>
          <Select
            label={m.fieldRouter}
            disabled={!canManage}
            options={routers.map((r) => ({ value: r, label: r === "none" ? m.routerNone : r }))}
            value={values.router}
            onValueChange={(router) => set({ router })}
          />
        </Field>
        {list("deepLinkHosts", m.fieldDeepLinkHosts)}
        {list("deepLinkSchemes", m.fieldDeepLinkSchemes)}
        {list("packages", m.fieldPackages)}
      </div>
      <fieldset className="flex flex-col gap-2">
        <legend className="text-label font-medium text-primary">{m.fieldDeviceApis}</legend>
        <div className="grid grid-cols-2 gap-2">
          {deviceApis.map((api) => (
            <Choice
              key={api}
              type="checkbox"
              checked={values.deviceApis.includes(api)}
              disabled={!canManage}
              label={apiNames[api]}
              onChange={(on) =>
                set({ deviceApis: on ? [...values.deviceApis, api] : values.deviceApis.filter((a) => a !== api) })
              }
            />
          ))}
        </div>
      </fieldset>
      <Choice
        type="checkbox"
        checked={values.push}
        disabled={!canManage}
        label={m.fieldPush}
        onChange={(push) => set({ push })}
      />
      {outcome !== undefined && (
        <p
          role={outcome.ok ? "status" : "alert"}
          className={outcome.ok ? "text-body text-secondary" : "text-body text-danger"}
        >
          {outcome.text}
        </p>
      )}
      <div>
        <Button type="submit" disabled={!canManage} loading={working} loadingLabel={m.saving}>
          {m.save}
        </Button>
      </div>
    </form>
  );
}
