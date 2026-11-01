// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { create } from "@bufbuild/protobuf";
import { FieldMaskSchema } from "@bufbuild/protobuf/wkt";
import { useMutation, useQuery } from "@connectrpc/connect-query";
import { useLingui } from "@lingui/react";
import { PreferencesSchema, PreferencesService } from "@plux/api";
import { availableLocales } from "@plux/i18n";
import { Icon, Select } from "@plux/ui";
import { Avatar, AvatarFallback } from "@plux/ui/components/avatar";
import { Badge } from "@plux/ui/components/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@plux/ui/components/card";
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldLabel,
  FieldLegend,
  FieldSet,
  FieldTitle,
} from "@plux/ui/components/field";
import { RadioGroup, RadioGroupItem } from "@plux/ui/components/radio-group";
import { Separator } from "@plux/ui/components/separator";
import { type ReactElement, useId, useState } from "react";
import { asDensity, asTheme, type DensityPreference, type ThemePreference } from "../../lib/preferences.ts";
import { useApp } from "../../lib/use-app.tsx";
import { useCurrentUser } from "../../lib/use-current-user.ts";
import { MuteSettings } from "../notifications/mute-settings.tsx";
import { initialsOf } from "../shell/app-sidebar.tsx";
import { profileMessages } from "./profile.messages.ts";
import { ProfileLayout } from "./profile-layout.tsx";
import { useAppliedPreferences } from "./use-applied-preferences.ts";
import "./profile.css";

type Messages = ReturnType<typeof profileMessages>;

/** The person's name and email address, read-only: they come from sign-in. */
function AccountCard({ m }: { readonly m: Messages }): ReactElement {
  const user = useCurrentUser().data;
  const name = user?.user?.displayName || user?.user?.email || "";
  const organisations = new Set(user?.memberships.map((x) => x.organizationId)).size;
  return (
    <Card>
      <CardHeader>
        <CardTitle>{m.account}</CardTitle>
        <CardDescription>{m.accountCaption}</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-wrap items-center gap-4">
        <Avatar className="size-14 rounded-xl">
          <AvatarFallback className="rounded-xl text-lg">{initialsOf(name)}</AvatarFallback>
        </Avatar>
        <div className="flex min-w-0 flex-1 flex-col gap-1">
          <p className="truncate font-medium">{name}</p>
          <p className="truncate text-sm text-muted-foreground">{user?.user?.email}</p>
          {user !== undefined && <p className="text-sm text-muted-foreground">{m.memberOf(organisations)}</p>}
        </div>
        {user?.installationAdmin === true && (
          <Badge variant="secondary">
            <Icon name="server" data-icon="inline-start" />
            {m.installationAdmin}
          </Badge>
        )}
      </CardContent>
    </Card>
  );
}

const themes: readonly {
  readonly value: ThemePreference;
  readonly key: "themeLight" | "themeDark" | "themeSystem" | "themeContrast";
}[] = [
  { value: "light", key: "themeLight" },
  { value: "dark", key: "themeDark" },
  { value: "system", key: "themeSystem" },
  { value: "contrast", key: "themeContrast" },
];

/** A small picture of a page in one theme, drawn with the theme's own colours (profile.css). */
function ThemePreview({ theme }: { readonly theme: ThemePreference }): ReactElement {
  return (
    <span className="theme-preview" data-preview={theme} aria-hidden="true">
      <span className="theme-preview-bar" />
      <span className="theme-preview-line" />
      <span className="theme-preview-line theme-preview-short" />
    </span>
  );
}

/** U2: the account, theme, density, language and notifications, stored through PreferencesService (STU-014). */
export function ProfilePage() {
  const { i18n } = useLingui();
  const m = profileMessages(i18n);
  const { studio } = useApp();
  const prefs = useQuery(PreferencesService.method.getPreferences, {});
  const update = useMutation(PreferencesService.method.updatePreferences);
  const [status, setStatus] = useState<"idle" | "saved" | "failed">("idle");
  const themeId = useId();
  const densityId = useId();
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
    <ProfileLayout
      id="profile-title"
      title={m.title}
      caption={m.caption}
      actions={
        <>
          {status === "saved" && (
            <p role="status" className="flex items-center gap-1 text-sm text-muted-foreground">
              <Icon name="check" />
              {m.saved}
            </p>
          )}
          {status === "failed" && (
            <p role="alert" className="text-sm text-destructive">
              {m.saveFailed}
            </p>
          )}
        </>
      }
    >
      <AccountCard m={m} />
      <Card>
        <CardHeader>
          <CardTitle>{m.appearance}</CardTitle>
          <CardDescription>{m.appearanceCaption}</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-6">
          <FieldSet>
            <FieldLegend id={themeId} variant="label">
              {m.theme}
            </FieldLegend>
            <FieldDescription>{m.themeCaption}</FieldDescription>
            <RadioGroup
              aria-labelledby={themeId}
              value={theme}
              onValueChange={(v) => void store("theme", String(v))}
              className="grid grid-cols-2 gap-3 @2xl/main:grid-cols-4"
            >
              {themes.map((t) => (
                <FieldLabel key={t.value} className="theme-option">
                  <ThemePreview theme={t.value} />
                  <span className="flex items-center gap-2 px-3 pb-3">
                    <RadioGroupItem value={t.value} />
                    <span className="text-sm font-medium">{m[t.key]}</span>
                  </span>
                </FieldLabel>
              ))}
            </RadioGroup>
          </FieldSet>
          <Separator />
          <FieldSet>
            <FieldLegend id={densityId} variant="label">
              {m.density}
            </FieldLegend>
            <RadioGroup
              aria-labelledby={densityId}
              value={density}
              onValueChange={(v) => void store("density", String(v))}
              className="grid gap-3 sm:grid-cols-2"
            >
              {(["comfortable", "compact"] as const).map((d) => (
                <FieldLabel key={d}>
                  <Field orientation="horizontal">
                    <RadioGroupItem value={d} />
                    <FieldContent>
                      <FieldTitle>{d === "compact" ? m.densityCompact : m.densityComfortable}</FieldTitle>
                      {d === "compact" && <FieldDescription>{m.densityCaption}</FieldDescription>}
                    </FieldContent>
                  </Field>
                </FieldLabel>
              ))}
            </RadioGroup>
          </FieldSet>
          <Separator />
          <Field orientation="responsive">
            <FieldContent>
              <FieldTitle>{m.language}</FieldTitle>
            </FieldContent>
            <div className="w-full sm:w-56">
              <Select
                label={m.language}
                value={language}
                options={availableLocales(import.meta.env.DEV).map((l) => ({ value: l.tag, label: l.nativeName }))}
                onValueChange={(v) => void store("language", v)}
              />
            </div>
          </Field>
        </CardContent>
      </Card>
      <MuteSettings />
    </ProfileLayout>
  );
}
