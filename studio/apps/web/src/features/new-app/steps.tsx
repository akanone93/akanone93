// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { AppProjectService } from "@plux/api";
import { limitDefault } from "@plux/schema";
import { Field, Input, Select, Textarea } from "@plux/ui";
import { useQuery } from "@tanstack/react-query";
import type { ReactElement } from "react";
import { dialogMessages } from "../../components/dialogs/dialogs.messages.ts";
import { keyFromName } from "../../lib/keys.ts";
import { useApp } from "../../lib/use-app.tsx";
import {
  type EnvironmentChoice,
  environmentChoices,
  hexOf,
  localeChoices,
  securityProfiles,
  themePresets,
} from "./blank-project.ts";
import { Choice } from "./choice.tsx";
import { type ArchiveIssue, type Draft, inspectArchive, maxLocales, type StepProblem } from "./draft.ts";
import { newAppMessages } from "./new-app.messages.ts";

export interface StepProps {
  readonly draft: Draft;
  readonly update: (patch: Partial<Draft>) => void;
  /** True once Next was pressed on this step, so its problems are shown. */
  readonly attempted: boolean;
  readonly problem: StepProblem | undefined;
}

const mebibyte = 1024 * 1024;

/** A size such as `8 MB` in the reader's language. */
function sizeText(locale: string, bytes: number): string {
  const kilo = bytes < mebibyte;
  return new Intl.NumberFormat(locale, {
    style: "unit",
    unit: kilo ? "kilobyte" : "megabyte",
    unitDisplay: "narrow",
    maximumFractionDigits: 1,
  }).format(bytes / (kilo ? 1024 : mebibyte));
}

function issueText(m: ReturnType<typeof newAppMessages>, locale: string, issue: ArchiveIssue): string {
  switch (issue.kind) {
    case "extension":
      return m.archiveExtension;
    case "unreadable":
      return m.archiveUnreadable;
    case "bytes":
      return m.archiveTooBig(sizeText(locale, issue.size), sizeText(locale, issue.max));
    case "entries":
      return m.archiveTooManyFiles(issue.count, issue.max);
    case "expanded":
      return m.archiveTooLarge(sizeText(locale, issue.size), sizeText(locale, issue.max));
  }
}

function TemplateList({ draft, update, attempted, problem }: StepProps): ReactElement {
  const m = newAppMessages(useLingui().i18n);
  const { session } = useApp();
  const samples = useQuery({
    queryKey: ["sample-apps"],
    queryFn: () => createClient(AppProjectService, session.transport).listSampleApps({}),
  });
  const list = samples.data?.samples ?? [];
  return (
    <fieldset className="flex flex-col gap-2">
      <legend className="text-label font-medium text-primary">{m.templateLegend}</legend>
      {samples.isPending && (
        <p role="status" className="text-body text-secondary">
          {m.templatesLoading}
        </p>
      )}
      {samples.isError && (
        <p role="alert" className="text-body text-danger">
          {m.templatesFailed}
        </p>
      )}
      {samples.isSuccess && list.length === 0 && <p className="text-body text-secondary">{m.templatesEmpty}</p>}
      {list.map((sample) => (
        <Choice
          key={sample.id}
          type="radio"
          name="template"
          checked={draft.sampleId === sample.id}
          label={sample.name}
          hint={sample.description}
          onChange={() =>
            update({
              sampleId: sample.id,
              ...(!draft.keyEdited && { name: sample.name, key: sample.key }),
            })
          }
        />
      ))}
      {attempted && problem === "template" && (
        <p role="alert" className="text-caption text-danger">
          {m.templateRequired}
        </p>
      )}
    </fieldset>
  );
}

function ArchivePicker({ draft, update, attempted, problem }: StepProps): ReactElement {
  const { i18n } = useLingui();
  const m = newAppMessages(i18n);
  const chosen = draft.archive;
  const error =
    chosen?.issue !== undefined
      ? issueText(m, i18n.locale, chosen.issue)
      : attempted && problem === "archive"
        ? m.archiveRequired
        : undefined;
  return (
    <Field
      label={m.archiveLabel}
      description={
        chosen !== undefined && chosen.issue === undefined
          ? m.archiveChosen(chosen.name, sizeText(i18n.locale, chosen.bytes.length))
          : m.archiveHint(sizeText(i18n.locale, limitDefault("archive.bytes")))
      }
      error={error}
    >
      <Input
        type="file"
        accept=".plux"
        onChange={async (e) => {
          const file = e.target.files?.[0];
          if (file === undefined) {
            update({ archive: undefined });
            return;
          }
          const bytes = new Uint8Array(await file.arrayBuffer());
          const issue = inspectArchive(file.name, bytes);
          update({ archive: { name: file.name, bytes, ...(issue !== undefined && { issue }) } });
        }}
      />
    </Field>
  );
}

/** Step 1 (O3, `step=source`): blank, from a template, from a `.plux` archive, or (arriving with P12) from a prompt. */
export function SourceStep(props: StepProps): ReactElement {
  const m = newAppMessages(useLingui().i18n);
  const { draft, update } = props;
  return (
    <div className="flex flex-col gap-3">
      <fieldset className="flex flex-col gap-2">
        <legend className="text-label font-medium text-primary">{m.sourceLegend}</legend>
        <Choice
          type="radio"
          name="source"
          checked={draft.source === "blank"}
          label={m.sourceBlank}
          hint={m.sourceBlankHint}
          onChange={() => update({ source: "blank" })}
        />
        <Choice
          type="radio"
          name="source"
          checked={draft.source === "template"}
          label={m.sourceTemplate}
          hint={m.sourceTemplateHint}
          onChange={() => update({ source: "template" })}
        />
        <Choice
          type="radio"
          name="source"
          checked={draft.source === "import"}
          label={m.sourceImport}
          hint={m.sourceImportHint}
          onChange={() => update({ source: "import" })}
        />
        <Choice
          type="radio"
          name="source"
          checked={false}
          disabled
          label={m.sourceAi}
          hint={m.sourceAiHint}
          onChange={() => undefined}
        />
      </fieldset>
      {draft.source === "template" && <TemplateList {...props} />}
      {draft.source === "import" && <ArchivePicker {...props} />}
    </div>
  );
}

/** Step 2 (`step=details`): the name, the key and, for a blank app, a description. */
export function DetailsStep({ draft, update, attempted, problem }: StepProps): ReactElement {
  const d = dialogMessages(useLingui().i18n);
  const m = newAppMessages(useLingui().i18n);
  return (
    <div className="flex flex-col gap-3">
      {draft.source === "import" && <p className="text-body text-secondary">{m.importKeepHint}</p>}
      <Field label={d.createName} error={attempted && problem === "name" ? d.createNameInvalid : undefined}>
        <Input
          value={draft.name}
          autoComplete="off"
          onChange={(e) =>
            update({ name: e.target.value, ...(!draft.keyEdited && { key: keyFromName(e.target.value) }) })
          }
        />
      </Field>
      <Field
        label={d.createKey}
        description={d.createKeyHint}
        error={attempted && problem === "key" ? d.createKeyInvalid : undefined}
      >
        <Input
          value={draft.key}
          autoComplete="off"
          spellCheck={false}
          onChange={(e) => update({ key: e.target.value, keyEdited: true })}
        />
      </Field>
      {draft.source === "blank" && (
        <Field label={m.description} description={m.descriptionHint}>
          <Textarea value={draft.description} rows={2} onChange={(e) => update({ description: e.target.value })} />
        </Field>
      )}
    </div>
  );
}

function languageName(locale: string, code: string): string {
  try {
    return new Intl.DisplayNames([locale], { type: "language" }).of(code) ?? code;
  } catch {
    return code;
  }
}

/** Step 3 (`step=locales`): the supported languages and the default one (I18N-001). */
export function LocalesStep({ draft, update, attempted, problem }: StepProps): ReactElement {
  const { i18n } = useLingui();
  const m = newAppMessages(i18n);
  const name = (code: string) => m.language(languageName(i18n.locale, code), code);
  const toggle = (code: string, on: boolean) => {
    const locales = on ? [...draft.locales, code] : draft.locales.filter((l) => l !== code);
    update({
      locales,
      defaultLocale: locales.includes(draft.defaultLocale) ? draft.defaultLocale : (locales[0] ?? ""),
    });
  };
  return (
    <div className="flex flex-col gap-3">
      <p className="text-body text-secondary">{m.localesHint(maxLocales)}</p>
      <Field label={m.localesDefault}>
        <Select
          label={m.localesDefault}
          options={draft.locales.map((code) => ({ value: code, label: name(code) }))}
          value={draft.defaultLocale}
          onValueChange={(defaultLocale) => update({ defaultLocale })}
        />
      </Field>
      <fieldset className="flex flex-col gap-2">
        <legend className="text-label font-medium text-primary">{m.localesLegend}</legend>
        <div className="grid grid-cols-2 gap-2">
          {localeChoices.map((code) => (
            <Choice
              key={code}
              type="checkbox"
              checked={draft.locales.includes(code)}
              label={name(code)}
              onChange={(on) => toggle(code, on)}
            />
          ))}
        </div>
        {attempted && problem === "locales" && (
          <p role="alert" className="text-caption text-danger">
            {m.localesRequired}
          </p>
        )}
        {attempted && problem === "tooManyLocales" && (
          <p role="alert" className="text-caption text-danger">
            {m.localesTooMany(maxLocales)}
          </p>
        )}
      </fieldset>
    </div>
  );
}

/** Step 4 (`step=theme`): a ready-made theme or a custom primary colour (SCH-020). */
export function ThemeStep({ draft, update, attempted, problem }: StepProps): ReactElement {
  const m = newAppMessages(useLingui().i18n);
  const names = { violet: m.themeViolet, blue: m.themeBlue, green: m.themeGreen, slate: m.themeSlate } as const;
  const example = hexOf(0x5b3df5);
  return (
    <fieldset className="flex flex-col gap-2">
      <legend className="text-label font-medium text-primary">{m.themeLegend}</legend>
      <div className="grid grid-cols-2 gap-2">
        {themePresets.map((preset) => (
          <Choice
            key={preset.id}
            type="radio"
            name="theme"
            checked={draft.themeId === preset.id}
            label={names[preset.id]}
            hint={m.themeSwatch(names[preset.id], preset.primary)}
            leading={
              <span
                aria-hidden="true"
                className="mt-1 inline-block size-4 shrink-0 rounded-full border border-hairline"
                style={{ backgroundColor: preset.primary }}
              />
            }
            onChange={() => update({ themeId: preset.id })}
          />
        ))}
        <Choice
          type="radio"
          name="theme"
          checked={draft.themeId === "custom"}
          label={m.themeCustom}
          onChange={() => update({ themeId: "custom" })}
        />
      </div>
      {draft.themeId === "custom" && (
        <Field
          label={m.themeCustomLabel}
          description={m.themeCustomHint(example)}
          error={attempted && problem === "colour" ? m.themeInvalid(example) : undefined}
        >
          <Input
            value={draft.customPrimary}
            autoComplete="off"
            spellCheck={false}
            onChange={(e) => update({ customPrimary: e.target.value })}
          />
        </Field>
      )}
    </fieldset>
  );
}

/** Step 5 (`step=security`): the security profile (§15.12, SEC-180). */
export function SecurityStep({ draft, update }: StepProps): ReactElement {
  const m = newAppMessages(useLingui().i18n);
  const text = {
    standard: [m.securityStandard, m.securityStandardHint],
    strict: [m.securityStrict, m.securityStrictHint],
    maximum: [m.securityMaximum, m.securityMaximumHint],
  } as const;
  return (
    <fieldset className="flex flex-col gap-2">
      <legend className="text-label font-medium text-primary">{m.securityLegend}</legend>
      {securityProfiles.map((profile) => (
        <Choice
          key={profile}
          type="radio"
          name="profile"
          checked={draft.profile === profile}
          label={text[profile][0]}
          hint={text[profile][1]}
          onChange={() => update({ profile })}
        />
      ))}
      <p className="text-caption text-secondary">{m.securityLater}</p>
    </fieldset>
  );
}

/** Step 6 (`step=environments`): the environments; production is always kept (GOV-010). */
export function EnvironmentsStep({ draft, update }: StepProps): ReactElement {
  const m = newAppMessages(useLingui().i18n);
  const names: Readonly<Record<EnvironmentChoice, string>> = {
    development: m.environmentDevelopment,
    staging: m.environmentStaging,
    production: m.environmentProduction,
  };
  return (
    <fieldset className="flex flex-col gap-2">
      <legend className="text-label font-medium text-primary">{m.environmentsLegend}</legend>
      <p className="text-body text-secondary">{m.environmentsHint}</p>
      {environmentChoices.map((env) => (
        <Choice
          key={env}
          type="checkbox"
          checked={env === "production" || draft.environments.includes(env)}
          disabled={env === "production"}
          label={names[env]}
          onChange={(on) =>
            update({
              environments: on ? [...draft.environments, env] : draft.environments.filter((e) => e !== env),
            })
          }
        />
      ))}
    </fieldset>
  );
}
