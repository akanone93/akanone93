// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { type DescMethodUnary, fromBinary, toBinary } from "@bufbuild/protobuf";
import type { Page } from "@playwright/test";
import {
  AnalyticsService,
  ApprovalService,
  AppService,
  AssetService,
  AuditService,
  BreakGlassService,
  ComplianceService,
  ComponentService,
  ControlService,
  DeviceService,
  DevSessionService,
  DocumentService,
  ExperimentService,
  FederationService,
  FlagService,
  FunctionAdminService,
  GovernanceService,
  IdentityService,
  InstallationService,
  LanguageServerService,
  LicenseService,
  ManifestService,
  NotificationService,
  OrgService,
  PluginService,
  PreferencesService,
  PublishService,
  ReleaseRequestService,
  ReleaseService,
  ReplayService,
  RolloutService,
  SecurityAdminService,
  TemplateService,
  TestRunService,
} from "@plux/api";
import { createUpstream, type UpstreamData } from "../apps/web/src/test-upstream.ts";

const services = [
  AnalyticsService,
  ApprovalService,
  AppService,
  AssetService,
  AuditService,
  BreakGlassService,
  ComplianceService,
  ComponentService,
  ControlService,
  DeviceService,
  DevSessionService,
  DocumentService,
  ExperimentService,
  FederationService,
  FlagService,
  FunctionAdminService,
  GovernanceService,
  IdentityService,
  InstallationService,
  LanguageServerService,
  LicenseService,
  ManifestService,
  NotificationService,
  OrgService,
  PluginService,
  PreferencesService,
  PublishService,
  ReleaseRequestService,
  ReleaseService,
  ReplayService,
  RolloutService,
  SecurityAdminService,
  TemplateService,
  TestRunService,
];

/**
 * The BFF's /api/* answered by the in-memory upstream the unit tests use: real Connect messages, no live server.
 * Every service the upstream implements is routed; a method it lacks answers `unimplemented`.
 */
export async function mockUpstream(p: Page, data: UpstreamData): Promise<void> {
  const transport = createUpstream(data);
  await p.route("**/api/plux.v1.*/*", async (route) => {
    const [, , typeName, name] = new URL(route.request().url()).pathname.split("/");
    const desc = services.find((s) => s.typeName === typeName)?.methods.find((m) => m.name === name);
    if (desc === undefined || desc.methodKind !== "unary") {
      return route.fulfill({
        status: 404,
        contentType: "application/json",
        body: JSON.stringify({ code: "unimplemented", message: "unmocked" }),
      });
    }
    try {
      const input = fromBinary(desc.input, route.request().postDataBuffer() ?? new Uint8Array());
      const res = await transport.unary(desc as DescMethodUnary, undefined, undefined, undefined, input as never);
      return route.fulfill({
        status: 200,
        contentType: "application/proto",
        body: Buffer.from(toBinary(desc.output, res.message as never)),
      });
    } catch (e) {
      const err = e as { code: number; rawMessage: string };
      return route.fulfill({
        status: 400,
        contentType: "application/json",
        body: JSON.stringify({ code: String(err.code), message: err.rawMessage }),
      });
    }
  });
}
