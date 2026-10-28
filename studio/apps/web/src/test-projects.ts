// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { Code, ConnectError, type ServiceImpl } from "@connectrpc/connect";
import type { AppProjectService } from "@plux/api";
import { hexOf } from "./features/new-app/blank-project.ts";

/** The shell settings of an app, as the fixture holds them (GEN-004). */
export interface ShellFixture {
  name: string;
  package: string;
  applicationId: string;
  bundleId: string;
  appVersion: string;
  splashColor: string;
  router: string;
  deviceApis: string[];
  push: boolean;
  deepLinkHosts: string[];
  deepLinkSchemes: string[];
  packages: string[];
  entryRoute: string;
}

/**
 * Project import and export, the sample apps, the shell settings and onboarding (STU-024, STU-015, GEN-004): what
 * the RPCs return, and a record of what they received.
 */
export interface ProjectsData {
  /**
   * Every request received, as `import:<organizationId>:<key>:<name>:<entries>`, `sample:<organizationId>:<id>:<key>:<name>`,
   * `generate:<appId>:<environment>:<channel>`, `job:<id>`, `download:<id>` and `shell:<appId>:<json>`.
   */
  calls: string[];
  /** The archives ImportApp received. */
  archives?: Uint8Array[];
  /** The sample apps the server embeds. */
  samples?: { id: string; key: string; name: string; description: string; size: number }[];
  /** A request of this kind is refused with this message (and, for an import, the registered code in its text). */
  refuse?: { import?: string; sample?: string; generate?: string; shell?: string };
  /** The statuses a project job reports one after the other; the last repeats. Default: `succeeded`. */
  jobStatuses?: string[];
  /** The failure a `failed` job reports. */
  jobError?: { code: string; message: string };
  /** The shell settings now stored (`current`) and those the project was last generated from (`generated`). */
  shell?: { current: ShellFixture; generated?: ShellFixture };
  /** The onboarding steps; absent means every step is done, which hides the checklist. */
  onboarding?: { key: string; done: boolean }[];
}

const blankShell: ShellFixture = {
  name: "Shop",
  package: "shop",
  applicationId: "com.example.shop",
  bundleId: "com.example.shop",
  appVersion: "1.0.0+1",
  splashColor: hexOf(0xffffff),
  router: "",
  deviceApis: [],
  push: false,
  deepLinkHosts: [],
  deepLinkSchemes: [],
  packages: [],
  entryRoute: "home",
};

const reasons: Readonly<Record<keyof ShellFixture, string>> = {
  name: "The name is the launcher label and the store listing, both compiled into the app.",
  package: "The Dart package name is the project's identity; renaming it needs a regenerated project.",
  applicationId: "The Android application ID identifies the app in Google Play and cannot change in place.",
  bundleId: "The iOS bundle ID identifies the app in the App Store and cannot change in place.",
  appVersion: "The version is shown by the stores and is set in pubspec.yaml.",
  splashColor: "The launch screen is native and is drawn before Plux starts.",
  router: "The router adapter is a compiled dependency of the shell.",
  deviceApis: "Device APIs add Android permissions and iOS usage descriptions to the native manifests.",
  push: "Push adds the notification permission and the iOS entitlement to the native project.",
  deepLinkHosts: "Deep-link hosts are declared in the native manifests and entitlements.",
  deepLinkSchemes: "Deep-link schemes are declared in the native manifests.",
  packages: "Optional packages are dependencies of the shell and need a build.",
  entryRoute: "The entry route is compiled into the shell's options.",
};

function text(value: ShellFixture[keyof ShellFixture]): string {
  return Array.isArray(value) ? [...value].sort().join(",") : String(value);
}

/** What differs between two settings, in the server's field order, with the reason each needs a store build. */
export function shellChanges(generated: ShellFixture, current: ShellFixture) {
  return (Object.keys(reasons) as (keyof ShellFixture)[])
    .filter((field) => text(generated[field]) !== text(current[field]))
    .map((field) => ({
      field,
      oldValue: text(generated[field]),
      newValue: text(current[field]),
      reason: reasons[field],
    }));
}

/** The handlers of AppProjectService over `data`; `addApp` makes an imported app appear in the app list. */
export function projectsHandlers(
  data: ProjectsData,
  addApp: (app: { id: string; key: string; name: string }) => void,
): Partial<ServiceImpl<typeof AppProjectService>> {
  const refuse = (kind: keyof NonNullable<ProjectsData["refuse"]>) => {
    const message = data.refuse?.[kind];
    if (message !== undefined) {
      throw new ConnectError(message, Code.InvalidArgument);
    }
  };
  let polls = 0;
  const current = () => data.shell?.current ?? blankShell;
  return {
    importApp: (req) => {
      data.calls.push(`import:${req.organizationId}:${req.key}:${req.name}:${req.archive.length}`);
      data.archives?.push(req.archive);
      refuse("import");
      const key = req.key === "" ? "imported" : req.key;
      const app = { id: `app-${key}`, key, name: req.name === "" ? key : req.name };
      addApp(app);
      return { app: { ...app, organizationId: req.organizationId }, documents: 8, errorDiagnostics: 0 };
    },
    listSampleApps: () => ({
      samples: (data.samples ?? []).map((s) => ({ ...s, size: BigInt(s.size), sha256: "00" })),
    }),
    importSampleApp: (req) => {
      data.calls.push(`sample:${req.organizationId}:${req.sampleId}:${req.key}:${req.name}`);
      refuse("sample");
      const sample = data.samples?.find((s) => s.id === req.sampleId);
      const key = req.key === "" ? (sample?.key ?? req.sampleId) : req.key;
      const app = { id: `app-${key}`, key, name: req.name === "" ? (sample?.name ?? key) : req.name };
      addApp(app);
      return { app: { ...app, organizationId: req.organizationId }, documents: 12 };
    },
    getShellStatus: () => {
      const generated = data.shell?.generated;
      const changes = generated === undefined ? [] : shellChanges(generated, current());
      return {
        current: current(),
        ...(generated !== undefined && { generated }),
        updateRequired: changes.length > 0,
        changes,
      };
    },
    setShellSettings: (req) => {
      data.calls.push(`shell:${req.appId}:${JSON.stringify(req.settings)}`);
      refuse("shell");
      const settings = req.settings;
      if (settings !== undefined) {
        data.shell = {
          ...data.shell,
          current: {
            name: settings.name,
            package: settings.package,
            applicationId: settings.applicationId,
            bundleId: settings.bundleId,
            appVersion: settings.appVersion,
            splashColor: settings.splashColor,
            router: settings.router,
            deviceApis: [...settings.deviceApis],
            push: settings.push,
            deepLinkHosts: [...settings.deepLinkHosts],
            deepLinkSchemes: [...settings.deepLinkSchemes],
            packages: [...settings.packages],
            entryRoute: settings.entryRoute,
          },
        };
      }
      return { settings: current() };
    },
    generateProject: (req) => {
      data.calls.push(`generate:${req.appId}:${req.environmentKey}:${req.channelKey}`);
      refuse("generate");
      polls = 0;
      return { job: { id: "job-1", appId: req.appId, format: "zip", status: "queued" } };
    },
    getProjectJob: (req) => {
      data.calls.push(`job:${req.jobId}`);
      const statuses = data.jobStatuses ?? ["succeeded"];
      const status = statuses[Math.min(polls, statuses.length - 1)] ?? "succeeded";
      polls += 1;
      return {
        job: {
          id: req.jobId,
          appId: req.appId,
          format: "zip",
          status,
          size: status === "succeeded" ? 2048n : 0n,
          errorCode: status === "failed" ? (data.jobError?.code ?? "PLX-9999") : "",
          errorMessage: status === "failed" ? (data.jobError?.message ?? "generation failed") : "",
        },
      };
    },
    downloadProject: (req) => {
      data.calls.push(`download:${req.jobId}`);
      return { filename: "shop-project.zip", archive: new Uint8Array([80, 75, 3, 4]), sha256: "00" };
    },
  };
}
