// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { create } from "@bufbuild/protobuf";
import { FieldMaskSchema } from "@bufbuild/protobuf/wkt";
import { useMutation, useQuery } from "@connectrpc/connect-query";
import { useLingui } from "@lingui/react";
import { PreferencesSchema, PreferencesService } from "@plux/api";
import { availableLocales } from "@plux/i18n";
import { Field, Select } from "@plux/ui";
import { useState } from "react";
import { asDensity, asTheme, type DensityPreference, type ThemePreference } from "../../lib/preferences.ts";
import { useApp } from "../../lib/use-app.tsx";
import { profileMessages } from "./profile.messages.ts";
import { useAppliedPreferences } from "./use-applied-preferences.ts";

/** U2: theme, language and density, stored through PreferencesService with an update mask and applied at once (STU-014). */
export function ProfilePage() {
  const { i18n } = useLingui();
  const m = profileMessages(i18n);
  const { studio } = useApp();
  const prefs = useQuery(PreferencesService.method.getPreferences, {});
  const update = useMutation(PreferencesService.method.updatePreferences);
  const [status, setStatus] = useState<"idle" | "saved" | "failed">("idle");
  useAppliedPreferences(prefs.data?.preferences);

  const store = async (field: "theme" | "language" | "density", value: string) => {
    setStatus("idle");
    try {
      await update.mutateAsync({
        preferences: create(PreferencesSchema, { [field]: value }),
        updateMask: create(FieldMaskSchema, { paths: [field] }),
      });
      await prefs.refetch();
      if (field === "language") {
        await studio.activate(value);
      }
      setStatus("saved");
    } catch {
      setStatus("failed");
    }
  };

  const theme: ThemePreference = asTheme(prefs.data?.preferences?.theme ?? "");
  const density: DensityPreference = asDensity(prefs.data?.preferences?.density ?? "");
  const language = prefs.data?.preferences?.language || studio.locale();
  return (
    <section aria-labelledby="profile-title" className="flex max-w-prose flex-col gap-6">
      <header>
        <h1 id="profile-title" className="text-screen">
          {m.title}
        </h1>
        <p className="text-body text-secondary">{m.caption}</p>
      </header>
      <Field label={m.theme}>
        <Select
          value={theme}
          options={[
            { value: "system", label: m.themeSystem },
            { value: "light", label: m.themeLight },
            { value: "dark", label: m.themeDark },
            { value: "contrast", label: m.themeContrast },
          ]}
          onValueChange={(v) => void store("theme", v)}
        />
      </Field>
      <Field label={m.language}>
        <Select
          value={language}
          options={availableLocales(import.meta.env.DEV).map((l) => ({ value: l.tag, label: l.nativeName }))}
          onValueChange={(v) => void store("language", v)}
        />
      </Field>
      <Field label={m.density}>
        <Select
          value={density}
          options={[
            { value: "comfortable", label: m.densityComfortable },
            { value: "compact", label: m.densityCompact },
          ]}
          onValueChange={(v) => void store("density", v)}
        />
      </Field>
      {status === "saved" && <p role="status">{m.saved}</p>}
      {status === "failed" && (
        <p role="alert" className="text-danger">
          {m.saveFailed}
        </p>
      )}
    </section>
  );
}
